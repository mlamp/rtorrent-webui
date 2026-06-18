// Command rtorrent-webui serves the embedded Svelte SPA and a JSON/SSE API
// backed by JSON-RPC over rtorrent's SCGI socket.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/api"
	"github.com/mlamp/rtorrent-webui/internal/config"
	"github.com/mlamp/rtorrent-webui/internal/insight/geoip"
	"github.com/mlamp/rtorrent-webui/internal/insight/search"
	"github.com/mlamp/rtorrent-webui/internal/insight/system"
	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/poll"
	"github.com/mlamp/rtorrent-webui/internal/rpc"
	"github.com/mlamp/rtorrent-webui/internal/scgi"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)

	configPath := flag.String("config", "", "path to TOML config")
	// Flag overrides (applied over the config / defaults when set).
	addr := flag.String("addr", "", "override server.listen")
	rtAddr := flag.String("rtorrent", "", "override rtorrent.socket (host:port or /path.sock)")
	interval := flag.Duration("interval", 0, "override rtorrent.poll_interval (live cadence)")
	idleInterval := flag.Duration("idle-interval", 0, "override rtorrent.idle_poll_interval (background history cadence when no client is watching)")
	view := flag.String("view", "", "override rtorrent.view")
	geoipDB := flag.String("geoip-db", "", "override insight.geoip_db")
	historyDB := flag.String("history-db", "", "override insight.history_db")
	diskDirs := flag.String("disk-dirs", "", "override downloads.dirs (comma-separated)")
	mock := flag.Int("mock", 0, "serve N synthetic torrents instead of rtorrent (load testing)")
	rebuildHistory := flag.Bool("rebuild-history", false, "reconstruct the global '' history series from the per-torrent series, then exit (one-time data hatch)")
	flag.Parse()

	cfg := config.Default()
	if *configPath != "" {
		c, err := config.Load(*configPath)
		if err != nil {
			logger.Fatal(err)
		}
		cfg = c
		logger.Printf("loaded config %s", *configPath)
	}
	// apply overrides
	if *addr != "" {
		cfg.Server.Listen = *addr
	}
	if *rtAddr != "" {
		cfg.Rtorrent.Socket = *rtAddr
	}
	if *interval > 0 {
		cfg.Rtorrent.PollInterval = config.Duration(*interval)
	}
	if *idleInterval > 0 {
		cfg.Rtorrent.IdleInterval = config.Duration(*idleInterval)
	}
	if *view != "" {
		cfg.Rtorrent.View = *view
	}
	if *geoipDB != "" {
		cfg.Insight.GeoIPDB = *geoipDB
	}
	if *historyDB != "" {
		cfg.Insight.HistoryDB = *historyDB
	}
	if *diskDirs != "" {
		cfg.Downloads.Dirs = strings.Split(*diskDirs, ",")
	}

	scgiClient := scgi.New(cfg.Rtorrent.Socket, cfg.Rtorrent.MaxInflight, 3*time.Second, cfg.Rtorrent.RPCTimeout.D())
	rpcClient := rpc.New(scgiClient)

	// One-time data hatch: rebuild the global series from the per-torrent rows
	// and exit, before the poll loop / server start.
	if *rebuildHistory {
		n, err := runRebuildHistory(context.Background(), cfg.Insight.HistoryDB)
		if err != nil {
			logger.Fatalf("rebuild-history: %v", err)
		}
		logger.Printf("rebuild-history: wrote %d global rows from per-torrent series; exiting", n)
		os.Exit(0)
	}

	// nil when history is disabled. The live poll source overlays each torrent's
	// stable "added" time from the history store's first_seen record — rtorrent
	// has no reliable added-at of its own.
	histStore := openHistory(cfg.Insight.HistoryDB, *mock, logger)

	var src poll.Source
	if *mock > 0 {
		src = poll.MockSource(*mock)
		logger.Printf("MOCK mode: %d synthetic torrents", *mock)
	} else {
		src = overlayFirstSeen(func(ctx context.Context) ([]model.Torrent, model.Globals, error) {
			return rpcClient.Poll(ctx, cfg.Rtorrent.View)
		}, histStore)
	}

	hub := sse.NewHub()
	poller := poll.New(src, hub, cfg.Rtorrent.PollInterval.D(), cfg.Rtorrent.IdleInterval.D(), logger)
	// Live cadence while a browser is watching; the loop keeps running at the idle
	// cadence otherwise so history is still recorded with no tab open.
	hub.OnActivity(func() { poller.SetActive(true) }, func() { poller.SetActive(false) })

	srv := api.New(hub, rpcClient, cfg.Rtorrent.View)
	srv.SetName(cfg.Server.Name)
	srv.SetSearch(search.NewRegistry()) // seam only in v1
	srv.SetDirs(cfg.Downloads.Dirs)
	srv.SetDefaultDir(cfg.Downloads.DefaultDir)        // additional allowed deletion root
	srv.SetDeleteWithData(cfg.Features.DeleteWithData) // off unless the operator opts in
	// Directory browser: on for a unix-socket (same-host) daemon, or when forced
	// via downloads.browse for a shared-mount split. The endpoint still requires a
	// resolvable root, so this is a safe no-op when none are configured.
	srv.SetBrowse(scgiClient.Network() == "unix" || cfg.Downloads.Browse)
	srv.SetMaxUploadBytes(int64(cfg.Rtorrent.MaxUploadMB) << 20)
	if *mock > 0 {
		srv.SetDetailRPC(poll.NewMockDetail()) // detail tabs work without a live rtorrent
	}

	if cfg.Insight.GeoIPDB != "" {
		if g, err := geoip.New(cfg.Insight.GeoIPDB); err == nil {
			srv.SetGeo(g)
			logger.Printf("geoip: %s", cfg.Insight.GeoIPDB)
		} else {
			logger.Printf("geoip disabled: %v", err)
		}
	}
	if histStore != nil {
		srv.SetHistory(histStore)
		// One combined sink per tick: cumulative transfer counters + system gauges.
		sysColl := system.New()
		poller.SetSink(func(torrents []model.Torrent, g model.Globals, ts int64) {
			histStore.Sample(torrents, g, ts)
			histStore.SampleGauges(sysColl.Collect(torrents, g), ts)
		})
	}
	if cfg.Features.RPCPassthrough {
		srv.EnablePassthrough(cfg.Features.RPCAllowlist, cfg.Features.RPCDenylist)
		logger.Printf("rpc passthrough ENABLED (allow=%d deny=%d)", len(cfg.Features.RPCAllowlist), len(cfg.Features.RPCDenylist))
	}
	if cfg.Features.RPCProxy {
		path := srv.EnableRPCProxy(cfg.Features.RPCProxyPath)
		if cfg.Features.RPCProxyPath != "" && path != cfg.Features.RPCProxyPath {
			logger.Printf("rpc proxy: configured path %q is invalid or reserved — mounting at %q instead", cfg.Features.RPCProxyPath, path)
		}
		authNote := "OPEN (auth.mode=none) — keep on an internal network"
		if cfg.Auth.Mode == "basic" {
			authNote = "behind basic auth — clients use http://user:pass@host" + path
		}
		logger.Printf("rpc proxy ENABLED at %s — raw XML/JSON-RPC byte-pipe to rtorrent, UNFILTERED full control; %s", path, authNote)
	}

	// Launch the perpetual poll loop now that the (optional) history sink is wired;
	// it runs at the idle cadence until a client connects.
	poller.Start()

	var handler http.Handler = apiHandler(srv, src, *mock)
	if cfg.Auth.Mode == "basic" {
		creds, err := cfg.Auth.Credentials()
		if err != nil {
			logger.Fatal(err)
		}
		if creds.Empty() {
			logger.Fatal("auth.mode=basic but no users/htpasswd configured")
		}
		handler = api.BasicAuth(cfg.Auth.Realm, creds.Verify, handler)
		logger.Printf("auth: basic")
	}

	logger.Printf("rtorrent-webui listening on %s (rtorrent %s, poll %s)", cfg.Server.Listen, cfg.Rtorrent.Socket, cfg.Rtorrent.PollInterval.D())
	if err := http.ListenAndServe(cfg.Server.Listen, handler); err != nil {
		logger.Fatal(err)
	}
}
