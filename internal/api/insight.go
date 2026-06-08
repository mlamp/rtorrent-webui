package api

import (
	"net/http"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/insight/disk"
	"github.com/mlamp/rtorrent-webui/internal/insight/history"
	"github.com/mlamp/rtorrent-webui/internal/insight/search"
)

func (s *Server) SetDirs(dirs []string)          { s.dirs = dirs }
func (s *Server) SetHistory(h *history.Store)     { s.history = h }
func (s *Server) SetSearch(r *search.Registry)    { s.search = r }

func (s *Server) insightRoutes() {
	s.mux.HandleFunc("GET /api/diskspace", s.handleDiskspace)
	s.mux.HandleFunc("GET /api/history", s.handleHistory)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
}

func (s *Server) handleDiskspace(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, disk.Usage(s.dirs))
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeOK(w, map[string]any{"points": []any{}})
		return
	}
	rng := int64(3600)
	if d, err := time.ParseDuration(r.URL.Query().Get("range")); err == nil && d > 0 {
		rng = int64(d.Seconds())
	}
	ctx, cancel := reqCtx(r)
	defer cancel()
	pts, err := s.history.Query(ctx, rng, r.URL.Query().Get("hash"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "history_error", err.Error())
		return
	}
	if pts == nil {
		pts = []history.Point{}
	}
	writeOK(w, map[string]any{"points": pts})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if s.search == nil || s.search.Empty() {
		writeErr(w, http.StatusNotImplemented, "not_implemented", "no search adapters configured")
		return
	}
	ctx, cancel := reqCtx(r)
	defer cancel()
	res, err := s.search.Search(ctx, r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "search_error", err.Error())
		return
	}
	writeOK(w, res)
}
