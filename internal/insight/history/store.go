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

const seenGrace = 5 * 60 // purge a torrent's history this long after it disappears

type Store struct {
	db        *sql.DB
	last      map[string][2]int64 // series hash -> last written {down,up} (raw dedup)
	lastSeen  int64               // unix secs of last `seen` refresh (throttled)
	now       func() int64
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
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS samples (
			res  TEXT    NOT NULL,
			hash TEXT    NOT NULL,           -- '' = global totals
			ts   INTEGER NOT NULL,
			down INTEGER NOT NULL,           -- cumulative bytes downloaded
			up   INTEGER NOT NULL,           -- cumulative bytes uploaded
			PRIMARY KEY (res, hash, ts)
		) WITHOUT ROWID;
		CREATE TABLE IF NOT EXISTS seen (hash TEXT PRIMARY KEY, last_seen INTEGER NOT NULL);`); err != nil {
		return nil, err
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
	defer func() { _ = tx.Commit() }()

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

	write("", g.DownTotal, g.UpTotal)
	for _, t := range torrents {
		write(t.Hash, t.Completed, t.UpTotal)
	}

	// Refresh the `seen` set (for GC of removed torrents) at most once a minute.
	if ts-s.lastSeen >= 60 {
		s.lastSeen = ts
		seenStmt, err := tx.Prepare(`INSERT OR REPLACE INTO seen(hash,last_seen) VALUES(?,?)`)
		if err == nil {
			for _, t := range torrents {
				_, _ = seenStmt.Exec(t.Hash, ts)
			}
			seenStmt.Close()
		}
	}
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
	// GC removed torrents (only once we actually know the current set).
	var seenCount int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM seen`).Scan(&seenCount)
	if seenCount > 0 {
		cut := now - seenGrace
		s.db.Exec(`DELETE FROM samples WHERE hash <> '' AND hash NOT IN (SELECT hash FROM seen WHERE last_seen >= ?)`, cut)
		s.db.Exec(`DELETE FROM seen WHERE last_seen < ?`, cut)
	}
}

func pickTier(rangeSecs int64) (string, int) {
	switch {
	case rangeSecs <= 15*60:
		return "raw", 300
	case rangeSecs <= 24*3600:
		return "1m", 300
	case rangeSecs <= 7*86400:
		return "1h", 320
	default:
		return "1d", 366
	}
}

// Query returns a derived rate series (bytes/sec) for the range, decimated to
// ~target points. hash "" = global. Rates come from cumulative deltas, so gaps
// are averaged and counter resets are clamped to 0.
func (s *Store) Query(ctx context.Context, rangeSecs int64, hash string) ([]Point, error) {
	if rangeSecs <= 0 {
		rangeSecs = 3600
	}
	res, target := pickTier(rangeSecs)
	from := s.now() - rangeSecs

	// Fetch one sample before the window too, so the first in-range point has a
	// predecessor to derive a rate from.
	rows, err := s.db.QueryContext(ctx, `
		SELECT ts, down, up FROM samples
		WHERE res=? AND hash=? AND ts >= (SELECT COALESCE(MAX(ts), 0) FROM samples WHERE res=? AND hash=? AND ts < ?)
		ORDER BY ts`, res, hash, res, hash, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type raw struct{ ts, down, up int64 }
	var src []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.ts, &r.down, &r.up); err != nil {
			return nil, err
		}
		src = append(src, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pts := make([]Point, 0, len(src))
	for i := 1; i < len(src); i++ {
		dt := src[i].ts - src[i-1].ts
		if dt <= 0 {
			continue
		}
		dd := src[i].down - src[i-1].down
		du := src[i].up - src[i-1].up
		if dd < 0 {
			dd = 0 // counter reset (rtorrent restart / re-add)
		}
		if du < 0 {
			du = 0
		}
		if src[i].ts < from {
			continue
		}
		pts = append(pts, Point{TS: src[i].ts, Down: dd / dt, Up: du / dt})
	}
	return decimate(pts, target), nil
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

func (s *Store) Close() error { return s.db.Close() }
