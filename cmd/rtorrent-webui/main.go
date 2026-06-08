// Command rtorrent-webui is a thin Go backend that serves the embedded Svelte
// SPA and (from M1 onward) proxies JSON-RPC over rtorrent's SCGI socket and
// fans out live state over SSE.
//
// M-setup: serves the themed shell + a couple of stub endpoints so the build
// and the Playwright screenshot loop work end-to-end.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/mlamp/rtorrent-webui/web"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	_ = flag.String("config", "", "path to TOML config (wired in later milestones)")
	flag.Parse()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"version":"dev","milestone":"M-setup"}}`))
	})
	mux.Handle("/", web.SPAHandler())

	log.Printf("rtorrent-webui (M-setup shell) listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
