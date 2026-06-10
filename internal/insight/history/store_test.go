package history

import (
	"context"
	"fmt"
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

// seedTier inserts a continuous, monotonically-climbing cumulative series into one
// resolution tier: `n` rows spaced `step` seconds apart ending at `endTS`, each
// adding `perStep` bytes down (up = half). Lets tests build realistic multi-tier
// fixtures (raw/1m/1h/1d) without waiting on the rollup. `perStep`/`step` must stay
// well under the rate cap so genuine data isn't clamped.
func seedTier(t *testing.T, s *Store, res, hash string, endTS, step, n, perStep int64) {
	t.Helper()
	for i := int64(0); i < n; i++ {
		ts := endTS - (n-1-i)*step
		v := i * perStep
		if _, err := s.db.Exec(`INSERT INTO samples(res,hash,ts,down,up) VALUES(?,?,?,?,?)`, res, hash, ts, v, v/2); err != nil {
			t.Fatal(err)
		}
	}
}

// globalAt reads the reconstructed/live global ("") cumulative row for a tier at a ts.
func globalAt(t *testing.T, s *Store, res string, ts int64) (int64, int64) {
	t.Helper()
	var d, u int64
	if err := s.db.QueryRow(`SELECT down, up FROM samples WHERE res=? AND hash='' AND ts=?`, res, ts).Scan(&d, &u); err != nil {
		t.Fatalf("no global %s row @%d: %v", res, ts, err)
	}
	return d, u
}

// Each range must select the resolution tier whose bucket suits it, and the grid
// step must come from the RANGE (so the grid stays bounded), regardless of fallback.
// With every tier fully populated, every range fills its window.
func TestStepMatchesRangeAndFills(t *testing.T) {
	s, err := New(t.TempDir() + "/tiers.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base + 400*86400) // far enough that 1y of 1d data fits
	s.now = func() int64 { return now }

	seedTier(t, s, "raw", "", now, 1, 900, 1<<10)    // 15m of raw
	seedTier(t, s, "1m", "", now, 60, 1440, 1<<16)   // 24h of 1m
	seedTier(t, s, "1h", "", now, 3600, 168, 1<<24)  // 7d of 1h
	seedTier(t, s, "1d", "", now, 86400, 366, 1<<30) // ~1y of 1d

	cases := []struct {
		secs, wantStep int64
	}{
		{900, 1}, {3600, 60}, {21600, 60}, {86400, 60}, {604800, 3600}, {31536000, 86400},
	}
	for _, c := range cases {
		ser, err := s.Query(context.Background(), c.secs, "")
		if err != nil {
			t.Fatal(err)
		}
		if ser.Step != c.wantStep {
			t.Errorf("range %ds: step=%d, want %d", c.secs, ser.Step, c.wantStep)
		}
		if slots := (ser.End - ser.Start) / ser.Step; slots > 2000 {
			t.Errorf("range %ds: %d grid slots — unbounded (step not from range?)", c.secs, slots)
		}
		nz := 0
		for _, p := range ser.Points {
			if p.Down > 0 {
				nz++
			}
		}
		if nz < len(ser.Points)/2 {
			t.Errorf("range %ds: only %d/%d slots nonzero — should fill from full data", c.secs, nz, len(ser.Points))
		}
	}
}

// When the range's tier is empty but a finer tier has data (right after a restart or
// a history trim, before coarse tiers roll up), the query falls back to the finer
// tier's SAMPLES but still grids at the RANGE's step — so the grid stays bounded
// (~hundreds of points, never 600k/31M) and the available data still renders.
func TestFallbackToFinerTierStaysBounded(t *testing.T) {
	s, err := New(t.TempDir() + "/fallback.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base + 7*86400)
	s.now = func() int64 { return now }

	// Only the raw tier holds data (15 min) — coarse tiers empty.
	seedTier(t, s, "raw", "", now, 1, 900, 1<<20)

	for _, c := range []struct {
		name     string
		secs     int64
		wantStep int64
	}{{"7d", 604800, 3600}, {"1y", 31536000, 86400}} {
		ser, err := s.Query(context.Background(), c.secs, "")
		if err != nil {
			t.Fatal(err)
		}
		if ser.Step != c.wantStep {
			t.Fatalf("%s: step=%d, want %d (range step, not the raw fallback's 1)", c.name, ser.Step, c.wantStep)
		}
		if slots := (ser.End - ser.Start) / ser.Step; slots > 1000 {
			t.Fatalf("%s: %d grid slots — fallback must not grid at the fine tier's step", c.name, slots)
		}
		nz := 0
		for _, p := range ser.Points {
			if p.Down > 0 {
				nz++
			}
		}
		if nz == 0 {
			t.Fatalf("%s: the available raw data must still render (bucketed), got all zero", c.name)
		}
	}

	// A finer tier with several hours of 1m data serves a 7d request too (gridded at 1h).
	seedTier(t, s, "1m", "", now, 60, 360, 1<<20) // 6h of 1m
	ser, _ := s.Query(context.Background(), 604800, "")
	if ser.Step != 3600 {
		t.Fatalf("7d via 1m fallback: step=%d, want 3600", ser.Step)
	}
	nz := 0
	for _, p := range ser.Points {
		if p.Down > 0 {
			nz++
		}
	}
	if nz < 5 {
		t.Fatalf("7d should show the ~6h of 1m data; only %d slots nonzero", nz)
	}
}

// RebuildGlobalFromTorrents reconstructs "" per tier as the carry-forward Σ of the
// per-torrent series: one "" row per distinct ts = sum of each torrent's last value
// ≤ that ts. Exercises a staggered appearance (raw) and two torrents sharing a
// bucket ts (1h).
func TestRebuildGlobalFromTorrents(t *testing.T) {
	s, err := New(t.TempDir() + "/rebuild.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	const MB = 1 << 20
	s.now = func() int64 { return base + 10_000 }

	// raw: AAA at base..base+4 (0,10,20,30,40 MB); BBB appears later at base+2..base+4 (0,5,10 MB).
	seedTier(t, s, "raw", "AAA", base+4, 1, 5, 10*MB)
	seedTier(t, s, "raw", "BBB", base+4, 1, 3, 5*MB)
	// 1h: AAA & BBB share bucket ts base-7200/base-3600/base.
	seedTier(t, s, "1h", "AAA", base, 3600, 3, 100*MB)
	seedTier(t, s, "1h", "BBB", base, 3600, 3, 50*MB)

	n, err := s.RebuildGlobalFromTorrents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 8 { // 5 distinct raw ts + 3 distinct 1h ts
		t.Fatalf("wrote %d global rows, want 8", n)
	}

	// raw carry-forward sum (BBB carries forward / is absent before base+2)
	for _, w := range []struct{ ts, down int64 }{
		{base, 0}, {base + 1, 10 * MB}, {base + 2, 20 * MB}, {base + 3, 35 * MB}, {base + 4, 50 * MB},
	} {
		if d, u := globalAt(t, s, "raw", w.ts); d != w.down || u != w.down/2 {
			t.Errorf("raw ''@%d = %d/%d, want %d/%d", w.ts, d, u, w.down, w.down/2)
		}
	}
	// 1h shared-ts sum
	if d, _ := globalAt(t, s, "1h", base); d != 300*MB {
		t.Errorf("1h ''@base = %d, want %d", d, 300*MB)
	}
	if d, _ := globalAt(t, s, "1h", base-3600); d != 150*MB {
		t.Errorf("1h ''@base-3600 = %d, want %d", d, 150*MB)
	}
	// exactly one "" row per (res,ts)
	var dups int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM (SELECT res,ts FROM samples WHERE hash='' GROUP BY res,ts HAVING COUNT(*)>1)`).Scan(&dups); err != nil {
		t.Fatal(err)
	}
	if dups != 0 {
		t.Fatalf("%d duplicate (res,ts) global rows", dups)
	}
}

// Rebuild is idempotent (running it twice yields identical "" rows) and never touches
// per-torrent rows.
func TestRebuildIdempotent(t *testing.T) {
	s, err := New(t.TempDir() + "/idem.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	s.now = func() int64 { return base + 100 }
	seedTier(t, s, "raw", "AAA", base+4, 1, 5, 1<<20)
	seedTier(t, s, "raw", "BBB", base+4, 1, 5, 2<<20)
	aaaBefore := rawCount(t, s, "AAA")

	snap := func() string {
		rows, err := s.db.Query(`SELECT res,ts,down,up FROM samples WHERE hash='' ORDER BY res,ts`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var res string
			var ts, d, u int64
			rows.Scan(&res, &ts, &d, &u)
			out = append(out, fmt.Sprintf("%s,%d,%d,%d", res, ts, d, u))
		}
		return fmt.Sprint(out)
	}
	if _, err := s.RebuildGlobalFromTorrents(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := snap()
	if _, err := s.RebuildGlobalFromTorrents(context.Background()); err != nil {
		t.Fatal(err)
	}
	if second := snap(); second != first {
		t.Fatalf("rebuild not idempotent:\n first=%s\nsecond=%s", first, second)
	}
	if rawCount(t, s, "AAA") != aaaBefore {
		t.Fatal("rebuild altered per-torrent rows")
	}
}

// Rebuild DELETES stale global rows (e.g. left from a different counter) rather than
// merging — proving the full-rebuild, not gap-fill, behaviour.
func TestRebuildDeletesStaleGlobalRows(t *testing.T) {
	s, err := New(t.TempDir() + "/stale.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	s.now = func() int64 { return base + 100 }
	seedTier(t, s, "raw", "AAA", base+4, 1, 5, 1<<20)
	// a bogus "" row at a ts the rebuild won't produce
	if _, err := s.db.Exec(`INSERT INTO samples(res,hash,ts,down,up) VALUES('raw','',?,?,?)`, base+999, 123456789, 123456789); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RebuildGlobalFromTorrents(context.Background()); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM samples WHERE res='raw' AND hash='' AND down=123456789`).Scan(&n)
	if n != 0 {
		t.Fatalf("stale global row survived rebuild (%d rows) — must DELETE not merge", n)
	}
}

// A per-torrent reset (re-hash drops the counter) shows as the running sum dipping;
// at query time that negative delta clamps to 0 (no spike).
func TestRebuildHandlesReset(t *testing.T) {
	s, err := New(t.TempDir() + "/reset.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	const MB = 1 << 20
	s.now = func() int64 { return base + 30 }
	// AAA cumulative 0,10,20 then RESET to 5, then 6 MB.
	for i, v := range []int64{0, 10, 20, 5, 6} {
		if _, err := s.db.Exec(`INSERT INTO samples(res,hash,ts,down,up) VALUES('raw','AAA',?,?,?)`, base+int64(i), v*MB, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.RebuildGlobalFromTorrents(context.Background()); err != nil {
		t.Fatal(err)
	}
	// "" cumulative mirrors the running sum incl. the dip
	if d, _ := globalAt(t, s, "raw", base+2); d != 20*MB {
		t.Errorf("'' @base+2 = %d, want %d", d, 20*MB)
	}
	if d, _ := globalAt(t, s, "raw", base+3); d != 5*MB {
		t.Errorf("'' @base+3 (reset) = %d, want %d", d, 5*MB)
	}
	// query derives no negative rate across the reset
	ser, _ := s.Query(context.Background(), 30, "")
	for _, p := range ser.Points {
		if p.Down < 0 {
			t.Fatalf("negative rate after reconstructed reset: %+v", p)
		}
	}
}

// Rebuild with no per-torrent rows clears all "" rows and reports 0 written.
func TestRebuildEmptySet(t *testing.T) {
	s, err := New(t.TempDir() + "/empty.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	s.now = func() int64 { return base }
	s.db.Exec(`INSERT INTO samples(res,hash,ts,down,up) VALUES('raw','',?,?,?)`, base, 9, 9)
	n, err := s.RebuildGlobalFromTorrents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("wrote %d rows, want 0 (no per-torrent data)", n)
	}
	if rawCount(t, s, "") != 0 {
		t.Fatal("stale global rows must be cleared even when nothing to rebuild")
	}
}

// Live Sample() writes the global "" as the SUM of the tick's per-torrent counters,
// NOT g.DownTotal/UpTotal; an added (large) torrent's jump clamps at query time.
func TestSampleGlobalIsSumOfTorrents(t *testing.T) {
	s, err := New(t.TempDir() + "/sum.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	const MB = 1 << 20
	s.now = func() int64 { return base + 30 }

	s.Sample([]model.Torrent{
		{Hash: "A", Completed: 3 * MB, UpTotal: 1 * MB},
		{Hash: "B", Completed: 7 * MB, UpTotal: 2 * MB},
	}, model.Globals{DownTotal: 999, UpTotal: 999}, base)
	if d, u := globalAt(t, s, "raw", base); d != 10*MB || u != 3*MB {
		t.Fatalf("'' = %d/%d, want sum 10/3 MiB (NOT g.DownTotal=999)", d, u)
	}

	// next tick: A grows a little, and a large torrent C appears → sum jumps ~20 GiB.
	s.Sample([]model.Torrent{
		{Hash: "A", Completed: 4 * MB, UpTotal: 1 * MB},
		{Hash: "B", Completed: 7 * MB, UpTotal: 2 * MB},
		{Hash: "C", Completed: 20 << 30, UpTotal: 0},
	}, model.Globals{}, base+1)
	ser, _ := s.Query(context.Background(), 30, "")
	for _, p := range ser.Points {
		if p.Down > 1<<30 { // the 20 GiB add must clamp, not render as tens of GB/s
			t.Fatalf("add-torrent jump not clamped: %d B/s @%d", p.Down, p.TS)
		}
	}
}

// End-to-end: rebuild the global from per-torrent 1h data, then a 7d Query fills.
func TestRebuildThenLongRangeFills(t *testing.T) {
	s, err := New(t.TempDir() + "/e2e.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base + 8*86400)
	s.now = func() int64 { return now }
	// three torrents, each 7d of 1h data; global "" left empty.
	seedTier(t, s, "1h", "AAA", now, 3600, 168, 5<<20)
	seedTier(t, s, "1h", "BBB", now, 3600, 168, 3<<20)
	seedTier(t, s, "1h", "CCC", now, 3600, 168, 1<<20)

	if _, err := s.RebuildGlobalFromTorrents(context.Background()); err != nil {
		t.Fatal(err)
	}
	ser, err := s.Query(context.Background(), 604800, "")
	if err != nil {
		t.Fatal(err)
	}
	if ser.Step != 3600 {
		t.Fatalf("7d step=%d, want 3600", ser.Step)
	}
	nz := 0
	for _, p := range ser.Points {
		if p.Down > 0 {
			nz++
		}
	}
	if nz < len(ser.Points)/2 {
		t.Fatalf("rebuilt global should fill the 7d window; only %d/%d nonzero", nz, len(ser.Points))
	}
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

	// Global "" series = Σ per-torrent. One torrent climbing +1 MiB/s down,
	// +512 KiB/s up for 11 ticks drives it.
	for i := 0; i <= 10; i++ {
		s.Sample([]model.Torrent{{Hash: "AAA", Completed: int64(i) << 20, UpTotal: int64(i) * (512 << 10)}}, model.Globals{}, base+int64(i))
	}
	if got := rawCount(t, s, ""); got != 11 {
		t.Fatalf("raw global rows = %d, want 11", got)
	}

	// Dedup: an unchanged counter must NOT add a row.
	s.Sample([]model.Torrent{{Hash: "AAA", Completed: 10 << 20, UpTotal: 10 * (512 << 10)}}, model.Globals{}, base+11)
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
	s.Sample([]model.Torrent{{Hash: "AAA", Completed: 0, UpTotal: 0}}, model.Globals{}, base+12)
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
// series via one torrent: active for 2s, idle ~2min, then active.
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
	s.Sample([]model.Torrent{{Hash: "X", Completed: 0}}, model.Globals{}, base+0)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 1 * MB}}, model.Globals{}, base+1)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 2 * MB}}, model.Globals{}, base+2)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 3 * MB}}, model.Globals{}, base+120)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 4 * MB}}, model.Globals{}, base+121)

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
	s.Sample([]model.Torrent{{Hash: "AAA", Completed: 0}}, model.Globals{}, now-300)
	s.Sample([]model.Torrent{{Hash: "AAA", Completed: 10 << 20}}, model.Globals{}, now-1)

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
	s.Sample([]model.Torrent{{Hash: "X", Completed: 100 * MB}}, model.Globals{}, base+10)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 101 * MB}}, model.Globals{}, base+11)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 102 * MB}}, model.Globals{}, base+12)

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

// A cumulative-counter RE-BASELINE mid-series (rtorrent restart, a webui counter-
// source change between versions, or a counter wrap) must NOT render as an absurd
// rate spike. This reproduces the 2026.06.8→.9 incident: the global counter's source
// changed, so the 1h/1d tiers held old-baseline values and the first new sample
// jumped +168 GB in one step — which rendered as 40+ GB/s. Such a delta implies a
// rate no real link can sustain, so (like a negative reset) it must clamp to 0.
func TestNoSpikeOnCounterRebaseline(t *testing.T) {
	s, err := New(t.TempDir() + "/rebaseline.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	const GB = 1 << 30
	const MB = 1 << 20
	now := int64(base + 20)
	s.now = func() int64 { return now }

	// Old baseline counter climbing at a real ~1 MiB/s, then a restart/version change
	// RE-BASELINES it by +168 GB in one step, then real ~1 MiB/s resumes.
	s.Sample([]model.Torrent{{Hash: "X", Completed: 10 * GB}}, model.Globals{}, base+5)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 10*GB + MB}}, model.Globals{}, base+6)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 178 * GB}}, model.Globals{}, base+10)    // +168 GB step (re-baseline)
	s.Sample([]model.Torrent{{Hash: "X", Completed: 178*GB + MB}}, model.Globals{}, base+11) // real ~1 MiB/s again

	ser, err := s.Query(context.Background(), 20, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ser.Points {
		if p.Down > 64*MB { // real rate is ~1 MiB/s; the 168 GB step would be tens of GB/s
			t.Fatalf("counter re-baseline rendered as a spike: %d B/s at %d", p.Down, p.TS)
		}
	}
	var sawReal bool
	for _, p := range ser.Points {
		if p.Down >= MB*8/10 && p.Down <= 2*MB {
			sawReal = true
		}
	}
	if !sawReal {
		t.Fatal("the real ~1 MiB/s rate must survive the clamp")
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
