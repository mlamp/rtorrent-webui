// Package history persists global + per-torrent transfer history in embedded
// SQLite (pure-Go modernc driver, so the binary stays static).
//
// Design notes:
//   - We store CUMULATIVE byte counters (downloaded/uploaded), not instantaneous
//     rates. Rates are derived at query time as Δbytes/Δt, so missed ticks self-
//     heal (the next sample's delta covers the gap) and de-duplication is trivial:
//     if a counter hasn't moved, we don't write a row. Idle/seeding-with-no-peers
//     torrents therefore cost nothing.
//   - Multi-resolution rollups keep storage bounded while retaining long history:
//     raw(15m) -> 1m(24h) -> 1h(7d) -> 1d(1y). A rollup keeps the LAST cumulative
//     value per bucket, so deriving Δ across buckets stays correct.
//   - Per-torrent history is kept across tiers; a `seen` table GCs rows for
//     torrents that have been removed from rtorrent.
package history

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

// Point is one derived (rate) sample: down/up are bytes/sec.
type Point struct {
	TS   int64 `json:"t"`
	Down int64 `json:"down"`
	Up   int64 `json:"up"`
}

// Series is a windowed rate result: a uniform time grid [Start, End] at Step-second
// resolution. The back end dictates Step (the chosen tier's bucket) and zero-fills
// idle slots, so every consumer draws a complete, time-correct series with no
// client-side gap logic.
type Series struct {
	Start  int64   `json:"start"`
	End    int64   `json:"end"`
	Step   int64   `json:"step"`
	Points []Point `json:"points"`
}

// GaugePoint is one stored gauge value at a timestamp (no derivation).
type GaugePoint struct {
	TS int64 `json:"t"`
	V  int64 `json:"v"`
}

type tier struct {
	res    string
	bucket int64 // seconds per bucket (raw = 1, i.e. unbucketed)
	retain int64 // seconds
}

// Resolution tiers (retention per Margus's spec).
var tiers = []tier{
	{"raw", 1, int64((15 * time.Minute).Seconds())},
	{"1m", 60, int64((24 * time.Hour).Seconds())},
	{"1h", 3600, int64((7 * 24 * time.Hour).Seconds())},
	{"1d", 86400, int64((365 * 24 * time.Hour).Seconds())},
}

// rollup chain: derive dst from src by keeping the last value per dst bucket.
var rollups = []struct {
	src, dst string
	bucket   int64
	window   int64 // re-roll this many seconds back each pass (idempotent)
}{
	{"raw", "1m", 60, 30 * 60},
	{"1m", "1h", 3600, 3 * 3600},
	{"1h", "1d", 86400, 50 * 3600},
}

// gaugeRollups uses the same tiers/windows as `rollups`, but the gauge rollup in
// maintain() AVERAGES per bucket (gauges are instantaneous, not cumulative).
var gaugeRollups = rollups

const seenGrace = 5 * 60 // purge a torrent's history this long after it disappears

type Store struct {
	db       *sql.DB
	last     map[string][2]int64 // series hash -> last written {down,up} (raw dedup)
	lastSeen int64               // unix secs of last `seen` refresh (throttled)
	now      func() int64
}

// schemaVersion is the version this build expects; New() applies every
// migration up to it. Bump it and append a migration when the schema changes.
const schemaVersion = 4

// migration mutates the schema from version-1 to its version, inside a tx.
type migration struct {
	version int
	name    string
	apply   func(*sql.Tx) error
}

// migrations are applied in order; each runs in its own transaction and bumps
// PRAGMA user_version on success. Never edit a released migration — append a new
// one. This is the upgrade path: any older DB walks forward to schemaVersion.
var migrations = []migration{
	{1, "cumulative-counter baseline", func(tx *sql.Tx) error {
		// Pre-0.2.2 DBs have a rate-based `samples` table (columns ts/hash/down/up,
		// no `res`). Those values are instantaneous rates, incompatible with the
		// cumulative-counter model, so there's nothing to carry over — drop it and
		// start clean. A DB already on the new schema (res present) is left intact.
		legacy, err := legacySamples(tx)
		if err != nil {
			return err
		}
		if legacy {
			if _, err := tx.Exec(`DROP TABLE samples`); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`
			CREATE TABLE IF NOT EXISTS samples (
				res  TEXT    NOT NULL,
				hash TEXT    NOT NULL,           -- '' = global totals
				ts   INTEGER NOT NULL,
				down INTEGER NOT NULL,           -- cumulative bytes downloaded
				up   INTEGER NOT NULL,           -- cumulative bytes uploaded
				PRIMARY KEY (res, hash, ts)
			) WITHOUT ROWID;
			CREATE TABLE IF NOT EXISTS seen (hash TEXT PRIMARY KEY, last_seen INTEGER NOT NULL);`)
		return err
	}},
	{2, "per-torrent first_seen", func(tx *sql.Tx) error {
		// Record the first time we observe each torrent so the UI can offer only
		// the time ranges that actually have data. Rollup tiers stamp buckets at
		// the bucket *start* (floor-to-minute/hour/day), so MIN(ts) across tiers
		// can't answer "how old is this series" — a minute-old torrent would look
		// up to a day old. A truthful first_seen avoids that.
		if _, err := tx.Exec(`ALTER TABLE seen ADD COLUMN first_seen INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		// Best-effort backfill for torrents already on record (coarse via the
		// bucket-floored MIN, but better than 0; new torrents get the exact ts).
		_, err := tx.Exec(`UPDATE seen SET first_seen =
			COALESCE((SELECT MIN(ts) FROM samples WHERE samples.hash = seen.hash), 0)`)
		return err
	}},
	{3, "gauge metrics table", func(tx *sql.Tx) error {
		// System/process GAUGE series (cpu%, load, mem%, peer count, session totals).
		// Unlike `samples` (cumulative counters → derived rates), these are stored as
		// the instantaneous value and rolled up by AVERAGE per bucket. All global, so
		// there's no hash column and no per-torrent GC.
		_, err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS metrics (
				res    TEXT    NOT NULL,           -- 'raw' | '1m' | '1h' | '1d'
				metric TEXT    NOT NULL,           -- 'cpu' | 'load1' | 'mem' | 'peers' | …
				ts     INTEGER NOT NULL,
				value  INTEGER NOT NULL,
				PRIMARY KEY (res, metric, ts)
			) WITHOUT ROWID;`)
		return err
	}},
	{4, "trim stale global rows from a reverted units change", func(tx *sql.Tx) error {
		// Released in 2026.06.8: a build briefly switched the global ('') series from
		// throttle.global_*.total to a payload running counter — a units change that was
		// incompatible with existing history and was reverted in the next build (the
		// global series is throttle totals again). This DELETE is a one-time trim of the
		// fine-resolution '' rows around that churn so the derived rate doesn't cross an
		// incompatible boundary; with throttle restored it's a harmless history trim that
		// re-accumulates. Per-torrent ('AAA'…) rows are untouched. (Migration logic is
		// frozen once released — do not edit.)
		_, err := tx.Exec(`DELETE FROM samples WHERE hash='' AND res IN ('raw','1m')`)
		return err
	}},
}

// init guards against developer drift: migrations must be contiguous and
// ascending from 1, and schemaVersion must equal the last one. A mismatch is a
// programming error (a migration that would silently never run), so fail loud.
func init() {
	prev := 0
	for _, m := range migrations {
		if m.version != prev+1 {
			panic(fmt.Sprintf("history: migrations must be contiguous ascending from 1; got v%d after v%d", m.version, prev))
		}
		prev = m.version
	}
	if prev != schemaVersion {
		panic(fmt.Sprintf("history: schemaVersion=%d but highest migration is v%d", schemaVersion, prev))
	}
}

// legacySamples reports whether `samples` is *positively* the pre-0.2.2 rate
// schema (columns ts/hash/down/up, no `res`). It returns false on any doubt —
// already-migrated, an empty/corrupt column list, or a foreign table — so we
// NEVER DROP a table we don't recognise. Dropping only the schema we created
// ourselves keeps data loss impossible on uncertainty.
func legacySamples(tx *sql.Tx) (bool, error) {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='samples'`).Scan(&n); err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil // fresh DB — no table yet
	}
	rows, err := tx.Query(`PRAGMA table_info(samples)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if cols["res"] {
		return false, nil // already the new schema — keep it
	}
	// Require the full old shape before deciding to drop.
	return cols["ts"] && cols["hash"] && cols["down"] && cols["up"], nil
}

// migrate walks the DB from its current PRAGMA user_version up to schemaVersion,
// applying each pending migration transactionally. Returns the from/to versions.
func migrate(db *sql.DB) (from, to int, err error) {
	var v int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return 0, 0, err
	}
	from = v
	// Refuse a DB written by a newer build (downgrade): its schema is unknown to
	// us, so running against it would throw opaque SQL errors. Fail clearly so the
	// caller disables history instead of corrupting/misreading data.
	if v > schemaVersion {
		return v, v, fmt.Errorf("database schema v%d is newer than this build supports (v%d) — upgrade the binary", v, schemaVersion)
	}
	for _, m := range migrations {
		if m.version <= v {
			continue
		}
		tx, txErr := db.Begin()
		if txErr != nil {
			return from, v, txErr
		}
		if e := m.apply(tx); e != nil {
			_ = tx.Rollback()
			return from, v, fmt.Errorf("migration %d (%s): %w", m.version, m.name, e)
		}
		// user_version takes no bind params; m.version is a trusted int constant.
		if _, e := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, m.version)); e != nil {
			_ = tx.Rollback()
			return from, v, e
		}
		if e := tx.Commit(); e != nil {
			return from, v, e
		}
		v = m.version
	}
	return from, v, nil
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA synchronous=NORMAL`, `PRAGMA busy_timeout=5000`} {
		if _, err := db.Exec(p); err != nil {
			return nil, err
		}
	}
	from, to, err := migrate(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if from != to {
		log.Printf("history: schema migrated v%d -> v%d", from, to)
	}
	s := &Store{db: db, last: map[string][2]int64{}, now: func() int64 { return time.Now().Unix() }}
	go s.maintainLoop()
	return s, nil
}

// Sample records cumulative counters for the globals and every torrent whose
// counter moved since last time (dedup). Matches poll.Sink.
func (s *Store) Sample(torrents []model.Torrent, g model.Globals, ts int64) {
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	// Commit only on a clean run; roll back if we bail mid-way so a partial/empty
	// transaction is never committed.
	ok := false
	defer func() {
		if ok {
			_ = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}()

	ins, err := tx.Prepare(`INSERT OR REPLACE INTO samples(res,hash,ts,down,up) VALUES('raw',?,?,?,?)`)
	if err != nil {
		return
	}
	defer ins.Close()

	write := func(hash string, down, up int64) {
		if prev, ok := s.last[hash]; ok && prev[0] == down && prev[1] == up {
			return // unchanged -> skip
		}
		if _, err := ins.Exec(hash, ts, down, up); err == nil {
			s.last[hash] = [2]int64{down, up}
		}
	}

	// Global "" series = the session throttle totals (network bytes incl. protocol
	// overhead). This is a real cumulative counter, so dedup + rollup + derive all work
	// the same as the per-torrent series. (An earlier build briefly stored a payload
	// running counter here; that changed units mid-stream and was incompatible with
	// existing history, so it was reverted — the reactive payload "sum of torrents"
	// now lives in the live sidebar buffer instead.)
	write("", g.DownTotal, g.UpTotal)
	for _, t := range torrents {
		write(t.Hash, t.Completed, t.UpTotal)
	}

	// Record first sight *immediately* (unthrottled): first_seen must be the exact
	// first-sample ts, since FirstTS uses it to gate the UI's time-range buttons.
	// INSERT OR IGNORE is a cheap no-op once the row exists, so this costs a write
	// only for genuinely new torrents; it also seeds last_seen so GC works before
	// the first throttled refresh below.
	if firstStmt, err := tx.Prepare(`INSERT OR IGNORE INTO seen(hash,first_seen,last_seen) VALUES(?,?,?)`); err == nil {
		for _, t := range torrents {
			_, _ = firstStmt.Exec(t.Hash, ts, ts)
		}
		firstStmt.Close()
	}
	// Advance last_seen (for GC of removed torrents) at most once a minute. The CASE
	// also heals any first_seen left at 0 by the v2 backfill — adopting a current
	// bound beats the bucket-floored MIN(ts) FirstTS would otherwise fall back to.
	if ts-s.lastSeen >= 60 {
		s.lastSeen = ts
		if upd, err := tx.Prepare(`UPDATE seen SET last_seen=?,
			first_seen=CASE WHEN first_seen=0 THEN ? ELSE first_seen END WHERE hash=?`); err == nil {
			for _, t := range torrents {
				_, _ = upd.Exec(ts, ts, t.Hash)
			}
			upd.Close()
		}
	}
	ok = true
}

// SampleGauges records instantaneous gauge values (cpu/load/mem/peers/session
// totals) at ts into the raw tier. No dedup: gauges move essentially every tick,
// and the AVG rollup wants to see repeats (a steady peers=0 still matters). Raw
// retention is only 15m, so volume stays bounded. Empty map is a no-op.
func (s *Store) SampleGauges(m map[string]int64, ts int64) {
	if len(m) == 0 {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	ok := false
	defer func() {
		if ok {
			_ = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}()
	ins, err := tx.Prepare(`INSERT OR REPLACE INTO metrics(res,metric,ts,value) VALUES('raw',?,?,?)`)
	if err != nil {
		return
	}
	defer ins.Close()
	for k, v := range m {
		if _, err := ins.Exec(k, ts, v); err != nil {
			return
		}
	}
	ok = true
}

func (s *Store) maintainLoop() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		s.maintain()
	}
}

func (s *Store) maintain() {
	now := s.now()
	for _, r := range rollups {
		s.db.Exec(`
			INSERT OR REPLACE INTO samples(res,hash,ts,down,up)
			SELECT ?, hash, (ts/?)*?, down, up FROM (
				SELECT hash, ts, down, up,
				       ROW_NUMBER() OVER (PARTITION BY hash, ts/? ORDER BY ts DESC) AS rn
				FROM samples WHERE res=? AND ts >= ?
			) WHERE rn = 1`,
			r.dst, r.bucket, r.bucket, r.bucket, r.src, now-r.window)
	}
	for _, t := range tiers {
		s.db.Exec(`DELETE FROM samples WHERE res=? AND ts < ?`, t.res, now-t.retain)
	}
	// Gauge rollups: AVERAGE value per dst bucket (gauges are instantaneous, so
	// last-value-wins would throw away the bucket's shape). Stamp the bucket start,
	// matching the `samples` convention so the chart X-axis logic stays consistent.
	for _, r := range gaugeRollups {
		s.db.Exec(`
			INSERT OR REPLACE INTO metrics(res,metric,ts,value)
			SELECT ?, metric, (ts/?)*?, CAST(AVG(value) AS INTEGER)
			FROM metrics WHERE res=? AND ts >= ?
			GROUP BY metric, ts/?`,
			r.dst, r.bucket, r.bucket, r.src, now-r.window, r.bucket)
	}
	for _, t := range tiers {
		s.db.Exec(`DELETE FROM metrics WHERE res=? AND ts < ?`, t.res, now-t.retain)
	}
	// GC removed torrents (only once we actually know the current set).
	var seenCount int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM seen`).Scan(&seenCount)
	if seenCount > 0 {
		cut := now - seenGrace
		s.db.Exec(`DELETE FROM samples WHERE hash <> '' AND hash NOT IN (SELECT hash FROM seen WHERE last_seen >= ?)`, cut)
		s.db.Exec(`DELETE FROM seen WHERE last_seen < ?`, cut)
	}
}

// tierOrder is finest -> coarsest; pickTier returns the coarsest tier whose
// resolution suits the range, and Query falls back toward finer tiers when a
// coarse tier has no data yet (e.g. right after start, before any rollup).
var tierOrder = []string{"raw", "1m", "1h", "1d"}

func pickTier(rangeSecs int64) (int, int) {
	switch {
	case rangeSecs <= 15*60:
		return 0, 300 // raw
	case rangeSecs <= 24*3600:
		return 1, 300 // 1m
	case rangeSecs <= 7*86400:
		return 2, 320 // 1h
	default:
		return 3, 366 // 1d
	}
}

// Query returns a windowed, zero-filled rate series for the range. The back end
// dictates the grid Step (the chosen tier's bucket) and resamples the cumulative
// counters onto it with carry-forward, so an idle slot reads exactly 0 and the
// series is time-uniform across [Start, End]. hash "" = the global payload series.
// Counter resets clamp to 0. The coarse→fine tier fallback keeps the chart
// populated right after startup (before a coarse tier has rolled up).
func (s *Store) Query(ctx context.Context, rangeSecs int64, hash string) (Series, error) {
	if rangeSecs <= 0 {
		rangeSecs = 3600
	}
	idx, target := pickTier(rangeSecs)
	now := s.now()
	start, end := now-rangeSecs, now
	// Never draw a grid that predates the torrent's first-seen sample — leading
	// zeros for time before we knew it would be misleading. The global "" series
	// has no first_seen and is left alone.
	if hash != "" {
		if fs, _ := s.FirstTS(ctx, hash); fs > 0 && fs > start {
			start = fs
		}
	}

	for i := idx; i >= 0; i-- {
		step := tiers[i].bucket
		samples, err := s.fetchSamples(ctx, tierOrder[i], hash, start, step)
		if err != nil {
			return Series{}, err
		}
		// Use this tier once it actually holds data, else fall to a finer one. The
		// finest tier (i==0) is the floor: resample whatever it has — possibly an
		// all-zero grid, the honest answer for a brand-new or idle series.
		if len(samples) >= 2 || i == 0 {
			pts := resampleGrid(samples, start, end, step)
			return Series{Start: start, End: end, Step: step, Points: decimate(pts, target)}, nil
		}
	}
	return Series{Start: start, End: end, Step: tiers[0].bucket}, nil // unreachable
}

// fetchSamples returns the stored cumulative samples for (res,hash) covering
// [start, end], plus the last sample at or before start-step so the first grid
// slot has a predecessor to derive a delta from. Ascending by ts.
func (s *Store) fetchSamples(ctx context.Context, res, hash string, start, step int64) ([]Point, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, down, up FROM samples
		WHERE res=? AND hash=? AND ts >= (SELECT COALESCE(MAX(ts), 0) FROM samples WHERE res=? AND hash=? AND ts <= ?)
		ORDER BY ts`, res, hash, res, hash, start-step)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Point
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.TS, &p.Down, &p.Up); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// resampleGrid turns cumulative samples into a uniform rate grid over [start, end]
// at `step` seconds: for each slot g, rate = max(0, C(g)-C(g-step))/step where C(g)
// is the last cumulative value at or before g (carry-forward). An idle slot (C
// unchanged) reads 0; a counter reset clamps to 0. samples must be ascending by ts.
// maxRateBytesPerSec caps a single grid slot's derived rate. A cumulative-counter
// delta implying a sustained rate above this is not torrent traffic — it's a counter
// RE-BASELINE: an rtorrent restart (session totals reset), a change in what the
// webui stores in the counter between versions, or a wrap. Like a negative delta
// (a reset), such a delta is clamped to 0 so a baseline step never renders as an
// absurd spike (the 2026.06.8→.9 incident showed 40+ GB/s). ~16 GiB/s is far above
// any real link — a saturated 100 GbE is ~12 GB/s — so real traffic is never clipped.
// This also hides any already-stored bad-baseline rows at render time, no migration
// needed.
const maxRateBytesPerSec = 16 << 30

func resampleGrid(samples []Point, start, end, step int64) []Point {
	if step <= 0 || end < start {
		return nil
	}
	// Monotonic cursor: cumAt(x) returns the last cumulative value with ts <= x,
	// carrying forward. Called with non-decreasing x (start-step, start, start+step…).
	// Before the first sample we carry the first value BACKWARD (not 0): we have no
	// data there, so the rate must read 0 — returning 0 would instead make the first
	// slot derive the whole cumulative total as one giant delta-from-zero spike.
	j := 0
	cumAt := func(x int64) (int64, int64) {
		for j < len(samples) && samples[j].TS <= x {
			j++
		}
		if j == 0 {
			if len(samples) == 0 {
				return 0, 0 // no data at all → flat zero
			}
			return samples[0].Down, samples[0].Up // carry first value back ⇒ rate 0 before it
		}
		return samples[j-1].Down, samples[j-1].Up
	}
	out := make([]Point, 0, (end-start)/step+1)
	prevDown, prevUp := cumAt(start - step)
	maxDelta := step * maxRateBytesPerSec // largest plausible delta for this grid step
	for g := start; g <= end; g += step {
		cd, cu := cumAt(g)
		dd, du := cd-prevDown, cu-prevUp
		// A negative delta is a counter reset; an implausibly large one is a
		// re-baseline. Both are non-traffic discontinuities → clamp to 0.
		if dd < 0 || dd > maxDelta {
			dd = 0
		}
		if du < 0 || du > maxDelta {
			du = 0
		}
		out = append(out, Point{TS: g, Down: dd / step, Up: du / step})
		prevDown, prevUp = cd, cu
	}
	return out
}

func decimate(pts []Point, target int) []Point {
	if target <= 0 || len(pts) <= target {
		return pts
	}
	out := make([]Point, 0, target)
	step := float64(len(pts)) / float64(target)
	for i := 0; i < target; i++ {
		lo := int(float64(i) * step)
		hi := int(float64(i+1) * step)
		if hi > len(pts) {
			hi = len(pts)
		}
		if lo >= hi {
			continue
		}
		var sd, su int64
		for j := lo; j < hi; j++ {
			sd += pts[j].Down
			su += pts[j].Up
		}
		n := int64(hi - lo)
		out = append(out, Point{TS: pts[hi-1].TS, Down: sd / n, Up: su / n})
	}
	return out
}

// FirstTS returns when we first observed a hash, so the UI can offer only the
// time ranges that actually have data (e.g. hide "1y" on a day-old torrent), or
// 0 if we hold nothing for it. hash "" = global.
//
// It prefers the truthful per-torrent first_seen: rollup tiers stamp buckets at
// the bucket start, so MIN(ts) across tiers would report a minute-old torrent as
// up to a day old. It falls back to the earliest sample for series we don't track
// in `seen` (the global "" series) or rows predating the first_seen backfill.
func (s *Store) FirstTS(ctx context.Context, hash string) (int64, error) {
	var fs sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT first_seen FROM seen WHERE hash=?`, hash).Scan(&fs); err == nil && fs.Valid && fs.Int64 > 0 {
		return fs.Int64, nil
	}
	// Fallback restricted to the raw tier: its timestamps are exact (rollup tiers
	// floor to the bucket start, so MIN across them could pre-date the first sample).
	var ts sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MIN(ts) FROM samples WHERE res='raw' AND hash=?`, hash).Scan(&ts); err != nil {
		return 0, err
	}
	if !ts.Valid {
		return 0, nil
	}
	return ts.Int64, nil
}

// QueryGauges returns each requested gauge series for the range, decimated to
// ~target points. Values are the stored gauges (no derivation). Like Query, it
// picks a resolution tier from the range and falls back to finer tiers when a
// coarse one hasn't rolled up yet. Every requested metric is present in the result
// (empty slice when it holds no data) so the client shape is stable.
func (s *Store) QueryGauges(ctx context.Context, rangeSecs int64, metrics []string) (map[string][]GaugePoint, error) {
	if rangeSecs <= 0 {
		rangeSecs = 3600
	}
	idx, target := pickTier(rangeSecs)
	from := s.now() - rangeSecs
	out := make(map[string][]GaugePoint, len(metrics))
	for _, m := range metrics {
		var best []GaugePoint
		for i := idx; i >= 0; i-- {
			pts, err := s.queryGaugeTier(ctx, tierOrder[i], m, from)
			if err != nil {
				return nil, err
			}
			if len(pts) >= 2 {
				best = pts
				break
			}
			if len(pts) > len(best) {
				best = pts // best-effort if every tier is sparse
			}
		}
		out[m] = decimateGauge(best, target)
	}
	return out, nil
}

func (s *Store) queryGaugeTier(ctx context.Context, res, metric string, from int64) ([]GaugePoint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, value FROM metrics
		WHERE res=? AND metric=? AND ts >= ?
		ORDER BY ts`, res, metric, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []GaugePoint
	for rows.Next() {
		var p GaugePoint
		if err := rows.Scan(&p.TS, &p.V); err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

func decimateGauge(pts []GaugePoint, target int) []GaugePoint {
	if target <= 0 || len(pts) <= target {
		return pts
	}
	out := make([]GaugePoint, 0, target)
	step := float64(len(pts)) / float64(target)
	for i := 0; i < target; i++ {
		lo := int(float64(i) * step)
		hi := int(float64(i+1) * step)
		if hi > len(pts) {
			hi = len(pts)
		}
		if lo >= hi {
			continue
		}
		var sum int64
		for j := lo; j < hi; j++ {
			sum += pts[j].V
		}
		n := int64(hi - lo)
		out = append(out, GaugePoint{TS: pts[hi-1].TS, V: sum / n})
	}
	return out
}

// FirstSeen returns hash -> first_seen epoch for every torrent on record, so the
// poll source can overlay a STABLE "added" time (rtorrent has none: d.load_date
// re-stamps on restart, d.creation_date is the metainfo date and often 0). Returns
// a non-nil (possibly empty) map; any error yields an empty map so the caller's
// overlay is always safe.
func (s *Store) FirstSeen(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	rows, err := s.db.QueryContext(ctx, `SELECT hash, first_seen FROM seen WHERE first_seen > 0`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var h string
		var fs int64
		if rows.Scan(&h, &fs) == nil && h != "" {
			out[h] = fs
		}
	}
	return out
}

func (s *Store) Close() error { return s.db.Close() }
