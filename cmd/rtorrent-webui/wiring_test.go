package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/api"
	"github.com/mlamp/rtorrent-webui/internal/insight/history"
	"github.com/mlamp/rtorrent-webui/internal/insight/system"
	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/poll"
	"github.com/mlamp/rtorrent-webui/internal/rpc"
	"github.com/mlamp/rtorrent-webui/internal/scgi"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// unreachableRPC returns a client whose socket never connects, with a short dial
// budget so tests fail fast instead of burning the production 3s retry window.
func unreachableRPC(t *testing.T) *rpc.Client {
	t.Helper()
	return rpc.New(scgi.New(filepath.Join(t.TempDir(), "absent.sock"), 1, 50*time.Millisecond, time.Second))
}

// --- -rebuild-history one-shot hatch ---

// The hatch is documented as "rebuild, then exit": when the store cannot even be
// opened, runRebuildHistory must surface an error so main exits non-zero instead
// of silently skipping the rebuild and starting the long-running server.
func TestRebuildHistoryErrorsWhenStoreCannotOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-subdir", "history.db") // parent dir absent -> unopenable
	if n, err := runRebuildHistory(context.Background(), path); err == nil {
		t.Fatalf("runRebuildHistory(%q) = (%d, nil), want error for an unopenable store", path, n)
	}
}

func TestRebuildHistoryErrorsWithoutDBPath(t *testing.T) {
	if n, err := runRebuildHistory(context.Background(), ""); err == nil {
		t.Fatalf("runRebuildHistory(\"\") = (%d, nil), want error when no history DB is configured", n)
	}
}

func TestRebuildHistoryRunsOnValidStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	h, err := history.New(path)
	if err != nil {
		t.Fatal(err)
	}
	// Seed one per-torrent sample so the rebuild has a series to sum.
	hash := "00000000000000000000000000000000000000AB"
	h.Sample([]model.Torrent{{Hash: hash, DownTotal: 10, UpTotal: 5}}, model.Globals{}, time.Now().Unix())
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	n, err := runRebuildHistory(context.Background(), path)
	if err != nil {
		t.Fatalf("runRebuildHistory on a valid store: %v", err)
	}
	if n == 0 {
		t.Fatal("runRebuildHistory wrote 0 global rows from a store with per-torrent samples")
	}
}

// --- history vs -mock mode ---

// -mock is a load-testing mode; its fabricated counters must never be committed
// into the configured persistent history DB (fake per-torrent rows plus a bogus
// global Σ series would pollute the real graphs for the tier retention window).
func TestMockModeNeverWritesConfiguredHistoryDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")
	logger := log.New(io.Discard, "", 0)

	h := openHistory(dbPath, 5, logger) // as main wires it for `-mock 5` + insight.history_db set
	if h != nil {
		// Replicate main's per-tick sink for one mock tick and show the damage.
		src := poll.MockSource(5)
		torrents, g, _ := src(context.Background())
		ts := time.Now().Unix()
		sysColl := system.New()
		h.Sample(torrents, g, ts)
		h.SampleGauges(sysColl.Collect(torrents, g), ts)
		seen := h.FirstSeen(context.Background())
		_ = h.Close()
		t.Fatalf("mock mode opened the configured history DB and persisted %d synthetic torrents", len(seen))
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("mock mode touched the configured history DB at %s (stat err: %v)", dbPath, err)
	}
}

func TestNormalModeOpensConfiguredHistoryDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "history.db")
	h := openHistory(dbPath, 0, log.New(io.Discard, "", 0))
	if h == nil {
		t.Fatal("openHistory returned nil for a writable path outside mock mode")
	}
	_ = h.Close()
}

// --- one poll source for SSE and the one-shot endpoints ---

// GET /api/torrents must serve the same first-seen-overlaid "added" the SSE
// stream gets: both surfaces read the single poll source.
func TestAPITorrentsServesOverlaidAdded(t *testing.T) {
	const hash = "00000000000000000000000000000000000000AA"
	const firstSeen = int64(1700000000) // stable first_seen recorded by history
	const metainfo = int64(1600000000)  // unreliable creation_date from rtorrent

	h, err := history.New(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	h.Sample([]model.Torrent{{Hash: hash, DownTotal: 1}}, model.Globals{}, firstSeen)

	base := poll.Source(func(context.Context) ([]model.Torrent, model.Globals, error) {
		return []model.Torrent{{Hash: hash, Added: metainfo}}, model.Globals{}, nil
	})
	src := overlayFirstSeen(base, h)

	hub := sse.NewHub()
	rpcClient := unreachableRPC(t)
	srv := api.New(hub, rpcClient, "main")
	srv.SetHistory(h)
	handler := apiHandler(srv, src, 0) // normal (non-mock) mode

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/api/torrents", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/torrents = %d (body %s), want 200 served from the poll source", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Torrents []model.Torrent `json:"torrents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data.Torrents) != 1 || resp.Data.Torrents[0].Added != firstSeen {
		t.Fatalf("GET /api/torrents added = %+v, want first_seen %d (the SSE stream's value), not metainfo %d",
			resp.Data.Torrents, firstSeen, metainfo)
	}
}

// Giving srv the poll source must NOT cost the /healthz rtorrent probe — only
// explicit mock mode (SetMockMode) may short-circuit it to always-ok.
func TestHealthzStillProbesRtorrentInNormalMode(t *testing.T) {
	hub := sse.NewHub()
	rpcClient := unreachableRPC(t)
	srv := api.New(hub, rpcClient, "main")
	src := poll.Source(func(context.Context) ([]model.Torrent, model.Globals, error) {
		return nil, model.Globals{}, nil
	})
	handler := apiHandler(srv, src, 0)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /healthz = %d with rtorrent unreachable, want 503 (probe must not be short-circuited)", rr.Code)
	}
}

// In mock mode there is no daemon to probe: /healthz stays always-ok and the
// endpoints serve the synthetic source.
func TestMockModeHealthzAndTorrents(t *testing.T) {
	hub := sse.NewHub()
	rpcClient := unreachableRPC(t)
	srv := api.New(hub, rpcClient, "main")
	handler := apiHandler(srv, poll.MockSource(3), 3)

	for _, path := range []string{"/healthz", "/api/torrents"} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s = %d in mock mode, want 200", path, rr.Code)
		}
	}
}
