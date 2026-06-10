package history

import (
	"context"
	"testing"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

func rawCount(t *testing.T, s *Store, hash string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM samples WHERE res='raw' AND hash=?`, hash).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func tierCount(t *testing.T, s *Store, res string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM samples WHERE res=?`, res).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// FirstTS must report the exact first-sight ts even when the seen-refresh would be
// throttled (process already running >60s) and a rollup has stamped bucket-floored
// rows into the coarse tiers. Regression for the "reports a time before the torrent
// existed" bug that defeated the adaptive range-button gating.
func TestFirstTSTruthfulDespiteThrottleAndRollup(t *testing.T) {
	s, err := New(t.TempDir() + "/fs.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const base = 1_700_000_000
	s.now = func() int64 { return base }
	s.lastSeen = base // simulate a seen-refresh moments ago, so base+5 is throttled

	// Torrent first observed 5s later (within the throttle window).
	s.Sample([]model.Torrent{{Hash: "BBB", Completed: 1 << 20, UpTotal: 1 << 10}}, model.Globals{}, base+5)
	// Rollup floors a 1d bucket to midnight (~base-80000s) — the trap MIN(all tiers) would hit.
	s.now = func() int64 { return base + 5 }
	s.maintain()

	got, err := s.FirstTS(context.Background(), "BBB")
	if err != nil {
		t.Fatal(err)
	}
	if got < base {
		t.Fatalf("FirstTS=%d reports BEFORE the first observation (%d) — bucket-floor leak", got, base+5)
	}
	if got != base+5 {
		t.Fatalf("FirstTS=%d, want exact first-sight %d", got, base+5)
	}
}

func TestCumulativeDerivationDedupRollupGC(t *testing.T) {
	s, err := New(t.TempDir() + "/h.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const base = 1_700_000_000
	s.now = func() int64 { return base + 13 }

	// Global "" series = throttle totals (cumulative): +1 MiB/s down, +512 KiB/s up
	// for 11 ticks.
	for i := 0; i <= 10; i++ {
		s.Sample(nil, model.Globals{DownTotal: int64(i) << 20, UpTotal: int64(i) * (512 << 10)}, base+int64(i))
	}
	if got := rawCount(t, s, ""); got != 11 {
		t.Fatalf("raw global rows = %d, want 11", got)
	}

	// Dedup: an unchanged counter must NOT add a row.
	s.Sample(nil, model.Globals{DownTotal: 10 << 20, UpTotal: 10 * (512 << 10)}, base+11)
	if got := rawCount(t, s, ""); got != 11 {
		t.Fatalf("after unchanged sample, raw rows = %d, want 11 (dedup)", got)
	}

	// Derived rate at an active grid slot should be exactly 1 MiB/s down, 512 KiB/s up.
	// A short range keeps the raw grid under the decimation target (exact, no averaging).
	ser, err := s.Query(context.Background(), 30, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ser.Points) == 0 {
		t.Fatal("no derived points")
	}
	var found bool
	for _, p := range ser.Points {
		if p.TS == base+5 {
			found = true
			if p.Down != 1<<20 || p.Up != 512<<10 {
				t.Fatalf("derived rate @%d = %d/%d, want %d/%d", p.TS, p.Down, p.Up, 1<<20, 512<<10)
			}
		}
	}
	if !found {
		t.Fatalf("no grid slot at base+5 (Start=%d End=%d Step=%d)", ser.Start, ser.End, ser.Step)
	}

	// Counter reset must clamp to 0 (no negative spike).
	s.Sample(nil, model.Globals{DownTotal: 0, UpTotal: 0}, base+12)
	ser, _ = s.Query(context.Background(), 30, "")
	for _, p := range ser.Points {
		if p.Down < 0 || p.Up < 0 {
			t.Fatalf("negative rate after reset: %+v", p)
		}
	}

	// Rollup: maintain() should populate the 1m tier.
	s.maintain()
	if tierCount(t, s, "1m") == 0 {
		t.Fatal("1m tier empty after rollup")
	}

	// Per-torrent + GC: add a fresh torrent, then let it disappear and age past
	// seenGrace (but within the raw age-prune window, so we isolate GC from prune).
	s.now = func() int64 { return base + 100 }
	s.Sample([]model.Torrent{{Hash: "BBB", Completed: 5 << 20, UpTotal: 1 << 20}}, model.Globals{}, base+100)
	if rawCount(t, s, "BBB") == 0 {
		t.Fatal("per-torrent row not written")
	}
	s.now = func() int64 { return base + 500 } // past seenGrace(300s), within raw retain(900s)
	s.maintain()
	if got := rawCount(t, s, "BBB"); got != 0 {
		t.Fatalf("removed torrent BBB still has %d rows after GC", got)
	}
	if rawCount(t, s, "") == 0 {
		t.Fatal("global rows must survive GC")
	}
}

// Idle gaps must read as 0 across the whole gap and the resume rate must land at its
// real time slot — the core of the windowed grid resample. Drives the global ""
// series via throttle totals: active for 2s, idle ~2min, then active.
func TestIdleGapZeroFill(t *testing.T) {
	s, err := New(t.TempDir() + "/gap.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const base = 1_700_000_000
	const MB = 1 << 20
	s.now = func() int64 { return base + 121 }

	// Active 0..2 (cumulative 0,1,2 MB), idle 3..119 (no samples), resume 120..121.
	s.Sample(nil, model.Globals{DownTotal: 0}, base+0)
	s.Sample(nil, model.Globals{DownTotal: 1 * MB}, base+1)
	s.Sample(nil, model.Globals{DownTotal: 2 * MB}, base+2)
	s.Sample(nil, model.Globals{DownTotal: 3 * MB}, base+120)
	s.Sample(nil, model.Globals{DownTotal: 4 * MB}, base+121)

	// range 130s → raw tier, step 1, grid [base-9, base+121] = 131 slots (< target, no decimation).
	ser, err := s.Query(context.Background(), 130, "")
	if err != nil {
		t.Fatal(err)
	}
	if ser.Step != 1 || ser.End != base+121 || ser.Start != base+121-130 {
		t.Fatalf("window wrong: Start=%d End=%d Step=%d", ser.Start, ser.End, ser.Step)
	}
	at := func(ts int64) (Point, bool) {
		for _, p := range ser.Points {
			if p.TS == ts {
				return p, true
			}
		}
		return Point{}, false
	}
	// active slots carry the real rate
	for _, ts := range []int64{base + 1, base + 2, base + 120, base + 121} {
		p, ok := at(ts)
		if !ok || p.Down != 1*MB {
			t.Fatalf("active slot @%d = %+v ok=%v, want Down=%d", ts, p, ok, 1*MB)
		}
	}
	// every slot inside the idle gap reads exactly 0 (this is the fix)
	for ts := int64(base + 3); ts <= base+119; ts++ {
		p, ok := at(ts)
		if !ok || p.Down != 0 || p.Up != 0 {
			t.Fatalf("idle slot @%d = %+v ok=%v, want 0/0", ts, p, ok)
		}
	}
}

// A fully-populated tier must fill the whole requested window — a 6h request with 6h
// of 1m data yields a rate at (nearly) every grid slot, it does NOT collapse to the
// most recent sliver. Regression guard for the time-based resample; the real-world
// "6h shows only recent data" symptom is data sparseness (a freshly-(re)started or
// migration-trimmed tier), not a query bug.
func TestRangeFillsWhenTierFullyPopulated(t *testing.T) {
	s, err := New(t.TempDir() + "/fill.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base + 6*3600)
	s.now = func() int64 { return now }

	// 6h of 1m "" rows, cumulative +1 MiB/min (so each 1m slot derives ~1MiB/60 B/s).
	for i := int64(0); i <= 6*60; i++ {
		if _, err := s.db.Exec(`INSERT INTO samples(res,hash,ts,down,up) VALUES('1m','',?,?,?)`, base+i*60, i<<20, (i<<20)/2); err != nil {
			t.Fatal(err)
		}
	}
	ser, err := s.Query(context.Background(), 6*3600, "")
	if err != nil {
		t.Fatal(err)
	}
	if ser.Step != 60 {
		t.Fatalf("6h should pick the 1m tier (step 60); got step %d", ser.Step)
	}
	nz := 0
	for _, p := range ser.Points {
		if p.Down > 0 {
			nz++
		}
	}
	if nz < len(ser.Points)*9/10 {
		t.Fatalf("6h of 1m data must fill the window; only %d/%d slots nonzero", nz, len(ser.Points))
	}
	want := int64(1<<20) / 60 // ~1 MiB per 60s bucket
	for _, p := range ser.Points {
		if p.Down > 0 && (p.Down < want*7/10 || p.Down > want*13/10) {
			t.Fatalf("derived rate %d B/s out of expected band (~%d B/s)", p.Down, want)
		}
	}
}

// A per-torrent query clamps the window start to the torrent's first_seen, so the
// chart never shows leading zeros for time before the torrent existed; the global ""
// series is never clamped.
func TestQueryClampsStartToFirstSeen(t *testing.T) {
	s, err := New(t.TempDir() + "/clamp.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base + 3600)
	s.now = func() int64 { return now }

	// AAA first observed only 5 min ago, with global "" throttle data alongside.
	s.Sample([]model.Torrent{{Hash: "AAA", Completed: 0}}, model.Globals{DownTotal: 0}, now-300)
	s.Sample([]model.Torrent{{Hash: "AAA", Completed: 10 << 20}}, model.Globals{DownTotal: 5 << 20}, now-1)

	per, err := s.Query(context.Background(), 3600, "AAA") // 1h range, only 5min of history
	if err != nil {
		t.Fatal(err)
	}
	if per.Start < now-360 {
		t.Fatalf("per-torrent Start=%d should clamp to first_seen (~%d), not now-range (%d)", per.Start, now-300, now-3600)
	}
	glob, err := s.Query(context.Background(), 3600, "")
	if err != nil {
		t.Fatal(err)
	}
	if glob.Start != now-3600 {
		t.Fatalf("global Start=%d must NOT clamp; want now-range %d", glob.Start, now-3600)
	}
}

// The FIRST stored sample carries a large cumulative total (rtorrent's session
// counter, or a torrent already part-done). The resample must NOT read that as a
// delta from zero — that produced a giant one-tick spike at series start that blew
// up the Y axis and crushed the real (flat/low) data into an invisible baseline.
// Times before the first sample have no data, so their rate is 0.
func TestNoSpuriousSpikeAtFirstSample(t *testing.T) {
	s, err := New(t.TempDir() + "/spike.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	const MB = 1 << 20
	now := int64(base + 60)
	s.now = func() int64 { return now }

	// First sample already at 100 MiB (e.g. a fresh history DB for a long-running
	// rtorrent), then a real ~1 MiB/s for a few ticks.
	s.Sample(nil, model.Globals{DownTotal: 100 * MB}, base+10)
	s.Sample(nil, model.Globals{DownTotal: 101 * MB}, base+11)
	s.Sample(nil, model.Globals{DownTotal: 102 * MB}, base+12)

	ser, err := s.Query(context.Background(), 60, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ser.Points {
		if p.Down > 2*MB { // real rate is ~1 MiB/s; anything near 100 MiB/s is the spurious spike
			t.Fatalf("spurious spike at series start: slot @%d Down=%d B/s (first cumulative misread as a delta from 0)", p.TS, p.Down)
		}
	}
	// the active ticks should still register their real ~1 MiB/s
	var sawReal bool
	for _, p := range ser.Points {
		if p.Down >= MB*8/10 {
			sawReal = true
		}
	}
	if !sawReal {
		t.Fatal("expected to see the real ~1 MiB/s rate after the first sample")
	}
}

// Gauges are stored as instantaneous values and rolled up by AVERAGE (not the
// last-value-wins of the cumulative `samples` tier), and queried back without any
// rate derivation.
func TestGaugeSampleRollupQuery(t *testing.T) {
	s, err := New(t.TempDir() + "/g.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const base = 1_700_000_000
	s.now = func() int64 { return base + 120 }

	// Two raw cpu samples in the same 1m bucket: 200 and 400 → AVG 300.
	s.SampleGauges(map[string]int64{"cpu": 200, "peers": 3}, base+0)
	s.SampleGauges(map[string]int64{"cpu": 400, "peers": 5}, base+30)

	var raw int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM metrics WHERE res='raw' AND metric='cpu'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != 2 {
		t.Fatalf("raw cpu rows = %d, want 2", raw)
	}

	// Rollup → the 1m bucket holds the AVERAGE (300), not last-value (400).
	s.maintain()
	var avg int64
	if err := s.db.QueryRow(`SELECT value FROM metrics WHERE res='1m' AND metric='cpu'`).Scan(&avg); err != nil {
		t.Fatal(err)
	}
	if avg != 300 {
		t.Fatalf("1m cpu rollup = %d, want 300 (AVG, not last-value)", avg)
	}

	// QueryGauges returns stored gauge magnitudes directly (no derivation).
	series, err := s.QueryGauges(context.Background(), 900, []string{"cpu", "peers"})
	if err != nil {
		t.Fatal(err)
	}
	if len(series["cpu"]) == 0 {
		t.Fatal("no cpu points")
	}
	for _, p := range series["cpu"] {
		if p.V != 200 && p.V != 400 {
			t.Fatalf("unexpected cpu gauge value %d (expected a stored magnitude)", p.V)
		}
	}
	if _, ok := series["peers"]; !ok {
		t.Fatal("requested 'peers' series missing from result")
	}
}

func TestFirstSeenMap(t *testing.T) {
	s, err := New(t.TempDir() + "/fseen.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const base = 1_700_000_000
	s.now = func() int64 { return base }
	s.Sample([]model.Torrent{{Hash: "AAA"}, {Hash: "BBB"}}, model.Globals{}, base+7)

	m := s.FirstSeen(context.Background())
	if m["AAA"] != base+7 || m["BBB"] != base+7 {
		t.Fatalf("FirstSeen = %+v, want AAA/BBB = %d", m, base+7)
	}
}
