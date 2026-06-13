// Startup wiring extracted from main() so the flag-driven decisions are testable:
// the -rebuild-history one-shot hatch, the history-store open policy, the
// first-seen "added" overlay, and the API handler/source wiring.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/mlamp/rtorrent-webui/internal/api"
	"github.com/mlamp/rtorrent-webui/internal/insight/history"
	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/poll"
)

// runRebuildHistory is the -rebuild-history one-shot hatch: open the store,
// reconstruct the global "" series from the per-torrent rows, and report how
// many rows were written. Main exits right after. Every failure — no path
// configured, an unopenable store, a failed rebuild — is an error so the
// process exits non-zero instead of falling through to a long-running server
// with the requested rebuild silently skipped.
func runRebuildHistory(ctx context.Context, path string) (int, error) {
	if path == "" {
		return 0, errors.New("a history DB is required (set insight.history_db or -history-db)")
	}
	h, err := history.New(path)
	if err != nil {
		return 0, fmt.Errorf("open history DB %s: %w", path, err)
	}
	defer h.Close()
	return h.RebuildGlobalFromTorrents(ctx)
}

// openHistory opens the history store for this run, or returns nil when history
// is off: no path configured, the store cannot be opened, or -mock mode. Mock
// mode must never touch the configured store — every tick would otherwise commit
// the synthetic counters (fake per-torrent hashes plus a fabricated global Σ
// series) into the real data for the duration of the tier retention.
func openHistory(path string, mockTorrents int, logger *log.Logger) *history.Store {
	if path == "" {
		return nil
	}
	if mockTorrents > 0 {
		logger.Printf("history: OFF in mock mode — synthetic samples are never written to %s", path)
		return nil
	}
	h, err := history.New(path)
	if err != nil {
		logger.Printf("history disabled: %v", err)
		return nil
	}
	logger.Printf("history: %s (tiers raw 15m / 1m 24h / 1h 7d / 1d 1y; +system metrics)", path)
	return h
}

// overlayFirstSeen wraps base so each torrent's Added is replaced by the history
// store's stable first_seen record — rtorrent has no reliable added-at of its own.
func overlayFirstSeen(base poll.Source, h *history.Store) poll.Source {
	if h == nil {
		return base
	}
	return func(ctx context.Context) ([]model.Torrent, model.Globals, error) {
		torrents, g, err := base(ctx)
		if err != nil {
			return torrents, g, err
		}
		fs := h.FirstSeen(ctx)
		for i := range torrents {
			if v, ok := fs[torrents[i].Hash]; ok && v > 0 {
				torrents[i].Added = v // stable first-seen overrides metainfo creation_date
			}
		}
		return torrents, g, err
	}
}

// apiHandler wires srv's one-shot data source and returns the root HTTP handler
// (before the optional auth wrapper). srv always gets the poll source so
// /api/torrents and /api/stats serve the same (first-seen-overlaid) data as the
// SSE stream — not the raw rtorrent poll with its unreliable metainfo "added".
// Mock mode additionally short-circuits the /healthz daemon probe: there is no
// rtorrent whose reachability could be reported.
func apiHandler(srv *api.Server, src poll.Source, mockTorrents int) http.Handler {
	srv.SetSource(src)
	srv.SetMockMode(mockTorrents > 0)
	return srv.Handler()
}
