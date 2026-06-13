package history

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

func torrent(hash string, down, up int64) model.Torrent {
	return model.Torrent{Hash: hash, DownTotal: down, UpTotal: up}
}

func seenCount(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM seen`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// GC must purge only relative to a KNOWN current torrent set. During a poll
// outage (rtorrent down, wedged, or a long idle_poll_interval) no Sample()
// refreshes last_seen, so wall-clock staleness says nothing about removal —
// one maintain() pass must not wipe every torrent's history and first_seen.
func TestMaintainGCSkipsDuringPollOutage(t *testing.T) {
	s, err := New(t.TempDir() + "/gc-outage.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base)
	s.now = func() int64 { return now }

	s.Sample([]model.Torrent{torrent("AAA", 1000, 500), torrent("BBB", 2000, 100)}, model.Globals{}, base)
	// Long-retention per-torrent history in the coarse tiers (the raw tier ages
	// out by retention within 15m — that part is normal).
	seedTier(t, s, "1h", "AAA", base, 3600, 5, 1<<20)
	seedTier(t, s, "1h", "BBB", base, 3600, 5, 1<<20)

	hourCount := func(hash string) int {
		t.Helper()
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM samples WHERE res='1h' AND hash=?`, hash).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	now = base + 3600 // 1h outage: no Sample() ran, maintain keeps firing
	s.maintain()

	if got := hourCount("AAA"); got != 5 {
		t.Errorf("outage GC wiped AAA history: 1h rows = %d, want 5", got)
	}
	if got := hourCount("BBB"); got != 5 {
		t.Errorf("outage GC wiped BBB history: 1h rows = %d, want 5", got)
	}
	if got := seenCount(t, s); got != 2 {
		t.Errorf("outage GC wiped seen/first_seen: rows = %d, want 2", got)
	}
}

// The purge cut must anchor to the last PERSISTED poll round, not wall clock:
// seen.last_seen lags the newest sample by up to ~60s (the refresh throttle),
// so during an outage starting at T there is a window (R+grace, T+grace] where
// a wall-clock cut sees every present torrent as stale and wipes everything.
func TestMaintainGCSafeNearOutageStart(t *testing.T) {
	s, err := New(t.TempDir() + "/gc-window.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base)
	s.now = func() int64 { return now }

	// 1s polls for a minute: the throttle pins AAA's last_seen at base while
	// samples keep landing until base+59 — then the daemon goes down.
	for ts := int64(base); ts < base+60; ts++ {
		now = ts
		s.Sample([]model.Torrent{torrent("AAA", 1000+ts-base, 500)}, model.Globals{}, ts)
	}

	now = base + 330 // inside (last_seen+grace, lastSample+grace]
	s.maintain()

	if got := rawCount(t, s, "AAA"); got == 0 {
		t.Error("present torrent AAA wiped by GC pass landing just after the outage began")
	}
	if got := seenCount(t, s); got != 1 {
		t.Errorf("seen rows = %d, want 1", got)
	}
}

// A write-failure era (ENOSPC analog: every seen UPDATE aborts) must not arm
// the GC: polls "succeed" in memory while nothing persists, and the first
// maintain() after recovery would otherwise see uniformly stale last_seen
// and wipe live torrents' history.
func TestMaintainGCSafeAfterWriteFailureEra(t *testing.T) {
	s, err := New(t.TempDir() + "/gc-enospc.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base)
	s.now = func() int64 { return now }

	s.Sample([]model.Torrent{torrent("AAA", 1000, 500)}, model.Globals{}, base)

	if _, err := s.db.Exec(`CREATE TRIGGER block_seen BEFORE UPDATE ON seen
		BEGIN SELECT RAISE(ABORT, 'enospc'); END`); err != nil {
		t.Fatal(err)
	}
	for _, dt := range []int64{100, 200, 300, 400} { // refresh rounds all fail silently
		now = base + dt
		s.Sample([]model.Torrent{torrent("AAA", 1000+dt, 500)}, model.Globals{}, now)
	}
	if _, err := s.db.Exec(`DROP TRIGGER block_seen`); err != nil {
		t.Fatal(err)
	}

	s.maintain() // first pass after "disk recovered"

	if got := rawCount(t, s, "AAA"); got == 0 {
		t.Error("live torrent AAA wiped after write-failure era")
	}
	if got := seenCount(t, s); got != 1 {
		t.Errorf("seen rows = %d, want 1", got)
	}
}

// GC must purge a torrent only on positive evidence it is stale — a stale `seen`
// row. A torrent that has samples but NO seen row (e.g. a transient seed-write
// failure left it un-seeded for a round) must not be wiped with no grace.
func TestMaintainGCKeepsSamplesWithoutSeenRow(t *testing.T) {
	s, err := New(t.TempDir() + "/gc-orphan.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base)
	s.now = func() int64 { return now }

	// A live, normally-tracked torrent so last_poll advances.
	s.Sample([]model.Torrent{torrent("AAA", 1000, 500)}, model.Globals{}, base)
	now = base + 90
	s.Sample([]model.Torrent{torrent("AAA", 2000, 900)}, model.Globals{}, now) // throttle fires -> last_poll

	// An orphan: raw samples but no seen row at all.
	if _, err := s.db.Exec(`INSERT INTO samples(res,hash,ts,down,up) VALUES('raw','ORPHAN',?,1,1)`, base); err != nil {
		t.Fatal(err)
	}

	now = base + 90 + seenGrace + 100
	s.Sample([]model.Torrent{torrent("AAA", 3000, 1200)}, model.Globals{}, now) // AAA still present
	s.maintain()

	if got := rawCount(t, s, "ORPHAN"); got == 0 {
		t.Error("orphan samples (no seen row) purged with no grace")
	}
}

// The GC anchor must not exceed the current clock: after a large backward clock
// step, a fresh last_poll left over from before the step would otherwise make
// every just-added torrent look ancient and purge it.
func TestMaintainGCSurvivesBackwardClockStep(t *testing.T) {
	s, err := New(t.TempDir() + "/gc-clock.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base)
	s.now = func() int64 { return now }

	s.Sample([]model.Torrent{torrent("AAA", 1000, 500)}, model.Globals{}, base)
	now = base + 90
	s.Sample([]model.Torrent{torrent("AAA", 2000, 900)}, model.Globals{}, now) // last_poll = base+90

	// Clock steps back ~1h; a torrent added now is seeded at the rewound time.
	now = base - 3600
	s.Sample([]model.Torrent{torrent("AAA", 2000, 900), torrent("BBB", 10, 10)}, model.Globals{}, now)
	s.maintain()

	if got := rawCount(t, s, "BBB"); got == 0 {
		t.Error("torrent added after a backward clock step was purged")
	}
}

// The flip side: with healthy polls, a torrent absent from the current set must
// still be purged once its grace expires.
func TestMaintainGCPurgesRemovedTorrentAfterGrace(t *testing.T) {
	s, err := New(t.TempDir() + "/gc-removed.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base)
	s.now = func() int64 { return now }

	s.Sample([]model.Torrent{torrent("AAA", 1000, 500), torrent("BBB", 2000, 100)}, model.Globals{}, base)
	now = base + seenGrace + 100
	s.Sample([]model.Torrent{torrent("AAA", 1100, 600)}, model.Globals{}, now) // BBB removed
	s.maintain()

	if got := rawCount(t, s, "BBB"); got != 0 {
		t.Errorf("removed torrent BBB still has %d raw rows, want 0", got)
	}
	if got := rawCount(t, s, "AAA"); got == 0 {
		t.Error("live torrent AAA was purged")
	}
	if got := seenCount(t, s); got != 1 {
		t.Errorf("seen rows = %d, want 1 (AAA only)", got)
	}
}

// A closed 1m gauge bucket must keep its true average even after the bucket's
// raw rows age past raw retention. The re-roll window may not exceed what raw
// retention can still answer for, or AVG degrades to the bucket's tail value.
func TestGaugeRollupSurvivesRawPruning(t *testing.T) {
	s, err := New(t.TempDir() + "/gauge-prune.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const b = 1_700_000_040 // 1m-bucket aligned (divisible by 60)
	now := int64(b)
	s.now = func() int64 { return now }

	// Shaped bucket: first half 100, second half 0 → true average 50.
	for i := int64(0); i < 60; i++ {
		v := int64(100)
		if i >= 30 {
			v = 0
		}
		s.SampleGauges(map[string]int64{"cpu": v}, b+i)
	}

	gauge1m := func() int64 {
		t.Helper()
		var v int64
		if err := s.db.QueryRow(`SELECT value FROM metrics WHERE res='1m' AND metric='cpu' AND ts=?`, int64(b)).Scan(&v); err != nil {
			t.Fatalf("no 1m cpu row @%d: %v", int64(b), err)
		}
		return v
	}

	now = b + 120
	s.maintain()
	if v := gauge1m(); v != 50 {
		t.Fatalf("fresh 1m rollup = %d, want 50", v)
	}

	// Age the bucket: first pass prunes the bucket's leading raw rows, the next
	// pass re-rolls. The stored average must not be rewritten from the tail.
	now = b + 930
	s.maintain()
	now = b + 960
	s.maintain()
	if v := gauge1m(); v != 50 {
		t.Errorf("1m average degraded to %d after raw pruning, want 50", v)
	}
}

// Production runs maintain() every 30s. As the re-roll cutoff (now-window)
// sweeps THROUGH a closed bucket, the AVG must not be recomputed from the
// bucket's surviving tail rows — the bucket either re-rolls complete or not
// at all. (The window size alone can't guarantee that; the cutoff must be
// bucket-aligned.)
func TestGaugeRollupContinuousMaintainKeepsAverage(t *testing.T) {
	s, err := New(t.TempDir() + "/gauge-sweep.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const b = 1_700_000_040 // 1m-bucket aligned
	now := int64(b)
	s.now = func() int64 { return now }

	for i := int64(0); i < 60; i++ {
		v := int64(100)
		if i >= 30 {
			v = 0
		}
		s.SampleGauges(map[string]int64{"cpu": v}, b+i)
	}

	for now = b + 60; now <= b+1200; now += 30 {
		s.maintain()
		var v int64
		if err := s.db.QueryRow(`SELECT value FROM metrics WHERE res='1m' AND metric='cpu' AND ts=?`, int64(b)).Scan(&v); err != nil {
			t.Fatalf("no 1m cpu row @%d (now=b+%d): %v", int64(b), now-b, err)
		}
		if v != 50 {
			t.Fatalf("1m average = %d at now=b+%d, want 50 throughout", v, now-b)
		}
	}
}

// FirstTS for the global "" series must report the earliest sample held in ANY
// tier — long-retention global history lives in 1m/1h/1d while raw holds 15m.
func TestFirstTSGlobalSeesCoarseTiers(t *testing.T) {
	s, err := New(t.TempDir() + "/firstts.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base + 30*86400)
	s.now = func() int64 { return now }

	// A month of 1d rows and a day of 1h rows; raw is empty (long since pruned).
	seedTier(t, s, "1d", "", now, 86400, 30, 1<<30)
	seedTier(t, s, "1h", "", now, 3600, 24, 1<<20)
	oldest := now - 29*86400

	got, err := s.FirstTS(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got != oldest {
		t.Errorf("FirstTS(\"\") = %d, want %d (oldest 1d row)", got, oldest)
	}
}

// Close() must stop the maintain loop goroutine.
func TestCloseStopsMaintainLoop(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 8; i++ {
		s, err := New(t.TempDir() + "/loop.db")
		if err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+2 { // tolerate runtime noise
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("goroutines: before=%d after=%d — maintainLoop leaked", before, runtime.NumGoroutine())
}

// New() must not leave a half-open handle behind when a startup PRAGMA fails.
// Observable contract: a read-only DB file fails loudly (the close itself is
// asserted by -race/leak hygiene, the error path must keep working).
func TestNewFailsOnReadOnlyDB(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod 0444 does not block writes")
	}
	path := t.TempDir() + "/ro.db"
	// A valid store first, so the file exists and is a real DB.
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Error("New() on a read-only DB succeeded, want error")
	}
}

// A torrent purged by GC and later re-added with unchanged counters must still
// get a baseline sample row — the dedup cache may not outlive the stored rows.
func TestReaddedTorrentGetsBaselineAfterGC(t *testing.T) {
	s, err := New(t.TempDir() + "/readd.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base)
	s.now = func() int64 { return now }

	s.Sample([]model.Torrent{torrent("AAA", 1000, 500)}, model.Globals{}, base)

	now = base + seenGrace + 100
	s.Sample(nil, model.Globals{}, now) // AAA removed from rtorrent
	s.maintain()
	if got := rawCount(t, s, "AAA"); got != 0 {
		t.Fatalf("setup: AAA not purged (raw rows = %d)", got)
	}

	now += 60
	s.Sample([]model.Torrent{torrent("AAA", 1000, 500)}, model.Globals{}, now) // re-added, idle
	if got := rawCount(t, s, "AAA"); got != 1 {
		t.Errorf("re-added torrent got %d raw rows, want 1 baseline row", got)
	}
}

// Series promises a uniform grid: Points[k].TS == Start + k*Step for every k,
// covering [Start, End] completely. No decimation may break the spacing.
func TestQueryGridIsUniformAtStep(t *testing.T) {
	s, err := New(t.TempDir() + "/grid.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	now := int64(base + 2*86400)
	s.now = func() int64 { return now }

	seedTier(t, s, "1m", "", now, 60, 1440, 1<<16) // 24h of 1m data

	ser, err := s.Query(context.Background(), 86400, "")
	if err != nil {
		t.Fatal(err)
	}
	wantLen := int((ser.End-ser.Start)/ser.Step) + 1
	if len(ser.Points) != wantLen {
		t.Fatalf("grid has %d points, want %d ((end-start)/step+1)", len(ser.Points), wantLen)
	}
	for k, p := range ser.Points {
		if want := ser.Start + int64(k)*ser.Step; p.TS != want {
			t.Fatalf("Points[%d].TS = %d, want %d — grid not uniform at Step=%d", k, p.TS, want, ser.Step)
		}
	}
}
