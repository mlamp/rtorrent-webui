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

	// Global: cumulative down +1 MiB/s, up +512 KiB/s for 11 ticks.
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

	// Derived rate should be ~1 MiB/s down, ~512 KiB/s up.
	pts, err := s.Query(context.Background(), 900, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) == 0 {
		t.Fatal("no derived points")
	}
	last := pts[len(pts)-1]
	if last.Down != 1<<20 || last.Up != 512<<10 {
		t.Fatalf("derived rate = %d/%d, want %d/%d", last.Down, last.Up, 1<<20, 512<<10)
	}

	// Counter reset must clamp to 0 (no negative spike).
	s.Sample(nil, model.Globals{DownTotal: 0, UpTotal: 0}, base+12)
	pts, _ = s.Query(context.Background(), 900, "")
	for _, p := range pts {
		if p.Down < 0 || p.Up < 0 {
			t.Fatalf("negative rate after reset: %+v", p)
		}
	}

	// Rollup: maintain() should populate the 1m tier.
	s.maintain()
	if tierCount(t, s, "1m") == 0 {
		t.Fatal("1m tier empty after rollup")
	}

	// Per-torrent + GC: add a torrent (ts >= 60s after last `seen` refresh so it's
	// recorded), then let it disappear and age past seenGrace (but not past the
	// raw age-prune window, so we isolate GC from prune).
	s.Sample([]model.Torrent{{Hash: "AAA", Completed: 5 << 20, UpTotal: 1 << 20}}, model.Globals{}, base+100)
	if rawCount(t, s, "AAA") == 0 {
		t.Fatal("per-torrent row not written")
	}
	s.now = func() int64 { return base + 500 } // past seenGrace(300s), within raw retain(900s)
	s.maintain()
	if got := rawCount(t, s, "AAA"); got != 0 {
		t.Fatalf("removed torrent AAA still has %d rows after GC", got)
	}
	if rawCount(t, s, "") == 0 {
		t.Fatal("global rows must survive GC")
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
