package history

import (
	"context"
	"database/sql"
	"testing"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// A fresh DB migrates to the current schema version with both tables present.
func TestMigrateFreshDB(t *testing.T) {
	s, err := New(t.TempDir() + "/fresh.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v := userVersion(t, s.db); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
	// new-schema column exists -> a Sample/Query round-trips
	s.now = func() int64 { return 1001 }
	s.Sample([]model.Torrent{{Hash: "X", DownTotal: 0}}, model.Globals{}, 1000)
	s.Sample([]model.Torrent{{Hash: "X", DownTotal: 1 << 20}}, model.Globals{}, 1001)
	ser, err := s.Query(context.Background(), 900, "")
	if err != nil {
		t.Fatalf("query on fresh db: %v", err)
	}
	if len(ser.Points) == 0 {
		t.Fatal("expected derived points on fresh db")
	}
}

// A pre-0.2.2 rate-based DB (samples without `res`) is detected, dropped and
// recreated — no "no such column: res" error, no manual wipe.
func TestMigrateLegacyRateDB(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	// Build the OLD schema by hand and stuff a rate row in it.
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`
		CREATE TABLE samples (ts INTEGER NOT NULL, hash TEXT NOT NULL, down INTEGER NOT NULL, up INTEGER NOT NULL);
		CREATE INDEX idx_samples ON samples(hash, ts);
		INSERT INTO samples(ts,hash,down,up) VALUES (1, '', 5000, 6000);`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	s, err := New(path)
	if err != nil {
		t.Fatalf("New() on legacy db must succeed (auto-migrate), got %v", err)
	}
	defer s.Close()
	if v := userVersion(t, s.db); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
	// legacy rate rows must be gone (incompatible), schema must be the new one
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM samples`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("legacy rows survived: %d", n)
	}
	// the previously-failing query path now works
	if _, err := s.Query(context.Background(), 900, ""); err != nil {
		t.Fatalf("query after legacy migrate: %v", err)
	}
}

// A DB already on the cumulative schema but written by a pre-versioning build
// (user_version still 0) must be adopted WITHOUT dropping its data.
func TestMigrateUnversionedNewSchemaPreservesData(t *testing.T) {
	path := t.TempDir() + "/unversioned.db"
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// exact current schema, but no user_version set, plus a real row
	if _, err := pre.Exec(`
		CREATE TABLE samples (res TEXT NOT NULL, hash TEXT NOT NULL, ts INTEGER NOT NULL,
			down INTEGER NOT NULL, up INTEGER NOT NULL, PRIMARY KEY (res,hash,ts)) WITHOUT ROWID;
		CREATE TABLE seen (hash TEXT PRIMARY KEY, last_seen INTEGER NOT NULL);
		INSERT INTO samples(res,hash,ts,down,up) VALUES ('raw','AAA',100, 12345, 678);`); err != nil {
		t.Fatal(err)
	}
	pre.Close()

	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v := userVersion(t, s.db); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
	// A per-torrent row must survive the 0→current migration walk (the v4 step only
	// clears the global "" series, never per-torrent data).
	var down int64
	if err := s.db.QueryRow(`SELECT down FROM samples WHERE res='raw' AND hash='AAA' AND ts=100`).Scan(&down); err != nil {
		t.Fatalf("pre-existing row must survive migration: %v", err)
	}
	if down != 12345 {
		t.Fatalf("row corrupted: down=%d want 12345", down)
	}
}

// A DB written by a newer build (user_version > schemaVersion) must be refused,
// not silently run against an unknown schema.
func TestMigrateRefusesNewerDB(t *testing.T) {
	path := t.TempDir() + "/future.db"
	future, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := future.Exec(`PRAGMA user_version = 999`); err != nil {
		t.Fatal(err)
	}
	future.Close()

	if _, err := New(path); err == nil {
		t.Fatal("New() must error on a newer-than-supported DB (downgrade), got nil")
	}
}

// A table named `samples` that is NOT the old rate schema (no res, no down/up)
// must be left untouched — never dropped on uncertainty.
func TestMigrateDoesNotDropForeignSamplesTable(t *testing.T) {
	path := t.TempDir() + "/foreign.db"
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// a foreign/empty-shaped table sharing the name; not our old schema
	if _, err := pre.Exec(`CREATE TABLE samples (foo TEXT); INSERT INTO samples(foo) VALUES ('keep');`); err != nil {
		t.Fatal(err)
	}
	var legacy bool
	tx, _ := pre.Begin()
	legacy, err = legacySamples(tx)
	tx.Rollback()
	pre.Close()
	if err != nil {
		t.Fatal(err)
	}
	if legacy {
		t.Fatal("foreign `samples` table must NOT be classified as legacy (would be dropped)")
	}
}

// An empty pre-0.2.2 table (old schema, no rows) is still detected as legacy.
func TestMigrateEmptyLegacyTable(t *testing.T) {
	path := t.TempDir() + "/emptylegacy.db"
	pre, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pre.Exec(`CREATE TABLE samples (ts INTEGER NOT NULL, hash TEXT NOT NULL, down INTEGER NOT NULL, up INTEGER NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	pre.Close()
	s, err := New(path)
	if err != nil {
		t.Fatalf("empty legacy table must migrate cleanly: %v", err)
	}
	defer s.Close()
	if v := userVersion(t, s.db); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
}

// The init() invariant must hold for the shipped migration set.
func TestMigrationsContiguousAndPinned(t *testing.T) {
	prev := 0
	for _, m := range migrations {
		if m.version != prev+1 {
			t.Fatalf("migrations not contiguous: v%d after v%d", m.version, prev)
		}
		prev = m.version
	}
	if prev != schemaVersion {
		t.Fatalf("schemaVersion=%d but highest migration is v%d", schemaVersion, prev)
	}
}

// Re-opening an already-migrated DB is a no-op (idempotent) and never errors.
func TestMigrateIdempotent(t *testing.T) {
	path := t.TempDir() + "/idem.db"
	s1, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()
	s2, err := New(path)
	if err != nil {
		t.Fatalf("second open must succeed: %v", err)
	}
	defer s2.Close()
	if v := userVersion(t, s2.db); v != schemaVersion {
		t.Fatalf("user_version drifted: %d", v)
	}
}

// Migration v4 (throttle→payload global series) must clear the stale global ""
// rows from the fine tiers (raw/1m) that we query at high resolution, keep the
// coarse 1h/1d "" rows (they age out within a week), and never touch per-torrent rows.
func TestMigrateV4ClearsStaleGlobalRows(t *testing.T) {
	path := t.TempDir() + "/v4.db"
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// A v3-shaped DB with throttle-style "" rows across every tier plus a per-torrent
	// row; user_version=3 so New() runs only the v4 migration.
	if _, err := db.Exec(`
		CREATE TABLE samples (res TEXT NOT NULL, hash TEXT NOT NULL, ts INTEGER NOT NULL,
			down INTEGER NOT NULL, up INTEGER NOT NULL, PRIMARY KEY (res,hash,ts)) WITHOUT ROWID;
		CREATE TABLE seen (hash TEXT PRIMARY KEY, first_seen INTEGER NOT NULL DEFAULT 0, last_seen INTEGER NOT NULL);
		CREATE TABLE metrics (res TEXT NOT NULL, metric TEXT NOT NULL, ts INTEGER NOT NULL, value INTEGER NOT NULL, PRIMARY KEY (res,metric,ts)) WITHOUT ROWID;
		INSERT INTO samples(res,hash,ts,down,up) VALUES
			('raw','',1,100,1),('1m','',60,200,2),('1h','',3600,300,3),('1d','',86400,400,4),
			('raw','AAA',1,10,1);
		PRAGMA user_version=3;`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v := userVersion(t, s.db); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}

	count := func(res, hash string) int {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM samples WHERE res=? AND hash=?`, res, hash).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("raw", "") != 0 || count("1m", "") != 0 {
		t.Fatalf("v4 must clear raw/1m global rows: raw=%d 1m=%d", count("raw", ""), count("1m", ""))
	}
	if count("1h", "") != 1 || count("1d", "") != 1 {
		t.Fatalf("v4 must keep 1h/1d global rows: 1h=%d 1d=%d", count("1h", ""), count("1d", ""))
	}
	if count("raw", "AAA") != 1 {
		t.Fatal("v4 must not touch per-torrent rows")
	}
}

// Migration v5 bumps the schema version and creates the meta key/value table.
func TestMigrateV5CreatesMetaTable(t *testing.T) {
	s, err := New(t.TempDir() + "/v5.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v := userVersion(t, s.db); v != schemaVersion {
		t.Fatalf("user_version = %d, want %d", v, schemaVersion)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("v5 must create the meta table")
	}
}

// The rebuild hatch stamps meta('global_rebuilt_at') with s.now().
func TestRebuildRecordsMetaTimestamp(t *testing.T) {
	s, err := New(t.TempDir() + "/meta.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const base = 1_700_000_000
	s.now = func() int64 { return base + 42 }
	s.Sample([]model.Torrent{{Hash: "X", DownTotal: 1 << 20}}, model.Globals{}, base)
	if _, err := s.RebuildGlobalFromTorrents(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got int64
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key='global_rebuilt_at'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != base+42 {
		t.Fatalf("global_rebuilt_at = %d, want %d", got, base+42)
	}
}
