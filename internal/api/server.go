// Package api wires the HTTP surface: the SSE stream, one-shot JSON endpoints,
// and the embedded SPA.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/rpc"
	"github.com/mlamp/rtorrent-webui/internal/sse"
	"github.com/mlamp/rtorrent-webui/web"
)

type Server struct {
	hub  *sse.Hub
	rpc  *rpc.Client
	view string
	geo  GeoLookup
	mux  *http.ServeMux
}

func New(hub *sse.Hub, r *rpc.Client, view string) *Server {
	if view == "" {
		view = "main"
	}
	s := &Server{hub: hub, rpc: r, view: view, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /api/version", s.handleVersion)
	s.mux.HandleFunc("GET /api/events", s.handleEvents)
	s.mux.HandleFunc("GET /api/torrents", s.handleTorrents)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.actionRoutes()
	s.detailRoutes()
	s.mux.Handle("/", web.SPAHandler())
}

// --- helpers ---

func writeOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
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
	writeOK(w, map[string]any{"webui": "dev", "rtorrent": cv, "api": av})
}

func (s *Server) handleTorrents(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	torrents, globals, err := s.rpc.Poll(ctx, s.view)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "rpc_error", err.Error())
		return
	}
	writeOK(w, map[string]any{"globals": globals, "torrents": torrents})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	_, globals, err := s.rpc.Poll(ctx, s.view)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "rpc_error", err.Error())
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
