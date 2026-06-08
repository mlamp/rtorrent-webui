// Package history persists global + per-torrent transfer-rate time series in an
// embedded SQLite database (pure-Go modernc driver, so the binary stays static).
package history

import (
	"context"
	"database/sql"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

// Point is one decimated time-series sample.
type Point struct {
	TS   int64 `json:"t"`
	Down int64 `json:"down"`
	Up   int64 `json:"up"`
}

type Store struct {
	db        *sql.DB
	retention time.Duration
}

// New opens (or creates) the history database.
func New(path string, retention time.Duration) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // serialize; avoids SQLITE_BUSY on the single writer
	for _, p := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := db.Exec(p); err != nil {
			return nil, err
		}
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS samples (
			ts   INTEGER NOT NULL,
			hash TEXT    NOT NULL,  -- "" = global totals
			down INTEGER NOT NULL,
			up   INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_samples ON samples(hash, ts);`); err != nil {
		return nil, err
	}
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	s := &Store{db: db, retention: retention}
	go s.pruneLoop()
	return s, nil
}

// Sample records the globals (always) and each active torrent's rates (to bound
// rows at scale, idle torrents are skipped). Matches poll.Sink.
func (s *Store) Sample(torrents []model.Torrent, g model.Globals, ts int64) {
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer func() { _ = tx.Commit() }()

	stmt, err := tx.Prepare(`INSERT INTO samples(ts,hash,down,up) VALUES(?,?,?,?)`)
	if err != nil {
		return
	}
	defer stmt.Close()

	_, _ = stmt.Exec(ts, "", g.DownRate, g.UpRate)
	for _, t := range torrents {
		if t.DownRate > 0 || t.UpRate > 0 {
			_, _ = stmt.Exec(ts, t.Hash, t.DownRate, t.UpRate)
		}
	}
}

// Query returns a series for the last rangeSecs, decimated to ~300 buckets.
// hash "" returns the global totals series.
func (s *Store) Query(ctx context.Context, rangeSecs int64, hash string) ([]Point, error) {
	if rangeSecs <= 0 {
		rangeSecs = 3600
	}
	bucket := rangeSecs / 300
	if bucket < 1 {
		bucket = 1
	}
	from := time.Now().Unix() - rangeSecs
	rows, err := s.db.QueryContext(ctx, `
		SELECT (ts/?)*? AS b, CAST(AVG(down) AS INTEGER), CAST(AVG(up) AS INTEGER)
		FROM samples WHERE hash=? AND ts>=?
		GROUP BY b ORDER BY b`, bucket, bucket, hash, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pts []Point
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.TS, &p.Down, &p.Up); err != nil {
			return nil, err
		}
		pts = append(pts, p)
	}
	return pts, rows.Err()
}

func (s *Store) pruneLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Unix() - int64(s.retention.Seconds())
		_, _ = s.db.Exec(`DELETE FROM samples WHERE ts < ?`, cutoff)
	}
}

func (s *Store) Close() error { return s.db.Close() }
