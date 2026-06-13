// Package api wires the HTTP surface: the SSE stream, one-shot JSON endpoints,
// and the embedded SPA.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/config"
	"github.com/mlamp/rtorrent-webui/internal/insight/history"
	"github.com/mlamp/rtorrent-webui/internal/insight/search"
	"github.com/mlamp/rtorrent-webui/internal/model"
	"github.com/mlamp/rtorrent-webui/internal/rpc"
	"github.com/mlamp/rtorrent-webui/internal/scgi"
	"github.com/mlamp/rtorrent-webui/internal/sse"
	"github.com/mlamp/rtorrent-webui/web"
)

// Version is the webui build version, injected at link time
// (-ldflags "-X github.com/mlamp/rtorrent-webui/internal/api.Version=x.y.z").
var Version = "dev"

type Server struct {
	hub    *sse.Hub
	rpc    *rpc.Client
	detail DetailRPC // detail-tab data source (defaults to rpc; swapped in -mock mode)
	// source, when set, feeds /api/torrents and /api/stats instead of dialing
	// rtorrent per request — same shape as poll.Source / rpc.Client.Poll. Normal
	// mode wires the shared poll source (first-seen overlay included); -mock mode
	// wires the synthetic one.
	source func(context.Context) ([]model.Torrent, model.Globals, error)
	// mock short-circuits the /healthz rtorrent probe: in -mock mode there is no
	// daemon whose reachability could meaningfully be reported.
	mock    bool
	view    string
	name    string // optional instance label surfaced to the SPA via /api/version
	geo     GeoLookup
	dirs    []string
	history *history.Store
	search  *search.Registry

	rpcPassthrough bool
	rpcAllow       config.MethodSet
	rpcDeny        config.MethodSet

	maxUpload int64 // .torrent upload cap in bytes (max_upload_mb)

	mux *http.ServeMux
}

func New(hub *sse.Hub, r *rpc.Client, view string) *Server {
	if view == "" {
		view = "main"
	}
	s := &Server{hub: hub, rpc: r, detail: r, view: view, maxUpload: maxUploadBytes, mux: http.NewServeMux()}
	s.routes()
	return s
}

// SetDetailRPC overrides the source for the detail tabs (files/peers/trackers/
// pieces). Used by -mock mode so the detail view works without a live rtorrent.
func (s *Server) SetDetailRPC(d DetailRPC) { s.detail = d }

// SetSource overrides the /api/torrents and /api/stats data source. The wiring
// always sets it (normal mode: the shared poll source, so one-shot GETs serve
// the same first-seen-overlaid data as the SSE stream; -mock mode: the
// synthetic source). Without it those endpoints dial the live rtorrent client.
func (s *Server) SetSource(src func(context.Context) ([]model.Torrent, model.Globals, error)) {
	s.source = src
}

// SetMockMode marks this server as running without a daemon: /healthz reports
// always-ok instead of probing the (absent) rtorrent socket.
func (s *Server) SetMockMode(on bool) { s.mock = on }

// poll returns the current torrents+globals from the wired source when one is
// set, otherwise from the live rtorrent client.
func (s *Server) poll(ctx context.Context) ([]model.Torrent, model.Globals, error) {
	if s.source != nil {
		return s.source(ctx)
	}
	return s.rpc.Poll(ctx, s.view)
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /api/version", s.handleVersion)
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/torrents", s.handleTorrents)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.actionRoutes()
	s.detailRoutes()
	s.insightRoutes()
	s.mux.HandleFunc("POST /api/rpc", s.handleRPCPassthrough)
	s.mux.Handle("/", web.SPAHandler())
}

// --- helpers ---

func writeOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": data})
}

// writeOKStatus is writeOK with an explicit status code. Headers must be set
// BEFORE WriteHeader freezes them — a late Content-Type would be ignored and
// the JSON body sniffed as text/plain.
func writeOKStatus(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": data})
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": map[string]string{"code": code, "message": msg},
	})
}

// writeRPCErr maps an rtorrent transport/RPC error to an HTTP response. A dial
// failure (daemon down/restarting/crash-looping) becomes 503 "rtorrent_unreachable"
// so the UI can show a transient "reconnecting" state instead of a hard error; any
// other RPC error stays 502 "rpc_error".
func writeRPCErr(w http.ResponseWriter, err error) {
	if errors.Is(err, scgi.ErrUnreachable) {
		writeErr(w, http.StatusServiceUnavailable, "rtorrent_unreachable",
			"rtorrent is not reachable (restarting?) — retrying")
		return
	}
	writeErr(w, http.StatusBadGateway, "rpc_error", err.Error())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if s.mock { // -mock mode: no daemon to probe, always healthy
		writeOK(w, map[string]bool{"ok": true})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if _, err := s.rpc.APIVersion(ctx); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "rtorrent_unreachable", err.Error())
		return
	}
	writeOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	cv, _ := s.rpc.ClientVersion(ctx)
	av, _ := s.rpc.APIVersion(ctx)
	writeOK(w, map[string]any{"webui": Version, "rtorrent": cv, "api": av})
}

// handleConfig serves static UI config (the instance name). Deliberately separate
// from /api/version — that one dials rtorrent and can be slow or fail when the
// daemon is down, but the branding name must always load promptly regardless.
func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, map[string]any{"name": s.name})
}

func (s *Server) handleTorrents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	torrents, globals, err := s.poll(ctx)
	if err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, map[string]any{"globals": globals, "torrents": torrents})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	_, globals, err := s.poll(ctx)
	if err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, globals)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "no_flush", "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sub := s.hub.Subscribe()
	defer s.hub.Unsubscribe(sub)

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.Closed():
			return
		case msg := <-sub.Ch():
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, msg.Data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
