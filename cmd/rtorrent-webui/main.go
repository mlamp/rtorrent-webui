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
	"github.com/mlamp/rtorrent-webui/internal/insight/history"
	"github.com/mlamp/rtorrent-webui/internal/insight/search"
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

	rpcClient := rpc.New(scgi.New(cfg.Rtorrent.Socket, cfg.Rtorrent.MaxInflight, 3*time.Second, cfg.Rtorrent.RPCTimeout.D()))

	var src poll.Source
	if *mock > 0 {
		src = poll.MockSource(*mock)
		logger.Printf("MOCK mode: %d synthetic torrents", *mock)
	} else {
		src = func(ctx context.Context) ([]model.Torrent, model.Globals, error) {
			return rpcClient.Poll(ctx, cfg.Rtorrent.View)
		}
	}

	hub := sse.NewHub()
	poller := poll.New(src, hub, cfg.Rtorrent.PollInterval.D(), cfg.Rtorrent.IdleInterval.D(), logger)
	// Live cadence while a browser is watching; the loop keeps running at the idle
	// cadence otherwise so history is still recorded with no tab open.
	hub.OnActivity(func() { poller.SetActive(true) }, func() { poller.SetActive(false) })

	srv := api.New(hub, rpcClient, cfg.Rtorrent.View)
	srv.SetSearch(search.NewRegistry()) // seam only in v1
	srv.SetDirs(cfg.Downloads.Dirs)
	if *mock > 0 {
		srv.SetDetailRPC(poll.NewMockDetail()) // detail tabs work without a live rtorrent
		srv.SetSource(src)                     // /healthz + /api/torrents + /api/stats serve mock data too
	}

	if cfg.Insight.GeoIPDB != "" {
		if g, err := geoip.New(cfg.Insight.GeoIPDB); err == nil {
			srv.SetGeo(g)
			logger.Printf("geoip: %s", cfg.Insight.GeoIPDB)
		} else {
			logger.Printf("geoip disabled: %v", err)
		}
	}
	if cfg.Insight.HistoryDB != "" {
		if h, err := history.New(cfg.Insight.HistoryDB); err == nil {
			srv.SetHistory(h)
			poller.SetSink(h.Sample)
			logger.Printf("history: %s (tiers raw 15m / 1m 24h / 1h 7d / 1d 1y)", cfg.Insight.HistoryDB)
		} else {
			logger.Printf("history disabled: %v", err)
		}
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

	var handler http.Handler = srv.Handler()
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
