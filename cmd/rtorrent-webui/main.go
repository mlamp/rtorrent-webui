// Command rtorrent-webui serves the embedded Svelte SPA and a JSON/SSE API
// backed by JSON-RPC over rtorrent's SCGI socket.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/api"
	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/poll"
	"github.com/mlamp/rtorrent-webui/internal/rpc"
	"github.com/mlamp/rtorrent-webui/internal/scgi"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	rtAddr := flag.String("rtorrent", "127.0.0.1:5000", "rtorrent SCGI address (host:port or /path.sock)")
	interval := flag.Duration("interval", time.Second, "poll interval")
	view := flag.String("view", "main", "rtorrent view to poll")
	mock := flag.Int("mock", 0, "serve N synthetic torrents instead of rtorrent (load testing)")
	_ = flag.String("config", "", "path to TOML config (wired in later milestones)")
	flag.Parse()

	logger := log.New(os.Stderr, "", log.LstdFlags)

	rpcClient := rpc.New(scgi.New(*rtAddr, 8, 10*time.Second))

	var src poll.Source
	if *mock > 0 {
		src = poll.MockSource(*mock)
		logger.Printf("MOCK mode: %d synthetic torrents", *mock)
	} else {
		src = func(ctx context.Context) ([]model.Torrent, model.Globals, error) {
			return rpcClient.Poll(ctx, *view)
		}
	}

	hub := sse.NewHub()
	poller := poll.New(src, hub, *interval, logger)
	hub.OnActivity(poller.Start, poller.Stop)

	srv := api.New(hub, rpcClient, *view)

	logger.Printf("rtorrent-webui listening on %s (rtorrent %s, poll %s)", *addr, *rtAddr, *interval)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		logger.Fatal(err)
	}
}
