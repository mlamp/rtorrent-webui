package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/insight/disk"
	"github.com/mlamp/rtorrent-webui/internal/insight/history"
	"github.com/mlamp/rtorrent-webui/internal/insight/search"
)

// parseRange turns "15m"/"6h"/"7d"/"1w"/"3mo"/"1y" into seconds. time.ParseDuration
// has no day/week/month/year unit, so handle those, then fall back to it (default 1h).
func parseRange(s string) int64 {
	s = strings.TrimSpace(s)
	// two-char "mo" (months) before the single-char units
	if n := len(s); n >= 3 && s[n-2:] == "mo" {
		if v, err := strconv.ParseFloat(s[:n-2], 64); err == nil && v > 0 {
			return int64(v * 30 * 86400)
		}
	}
	if n := len(s); n >= 2 {
		mult := 0.0
		switch s[n-1] {
		case 'd':
			mult = 86400
		case 'w':
			mult = 7 * 86400
		case 'y':
			mult = 365 * 86400
		}
		if mult > 0 {
			if v, err := strconv.ParseFloat(s[:n-1], 64); err == nil && v > 0 {
				return int64(v * mult)
			}
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return int64(d.Seconds())
	}
	return 3600
}

func (s *Server) SetName(name string)          { s.name = name }
func (s *Server) SetDirs(dirs []string)        { s.dirs = dirs }
func (s *Server) SetHistory(h *history.Store)  { s.history = h }
func (s *Server) SetSearch(r *search.Registry) { s.search = r }

// metricKeys is the fixed gauge set the Insight dashboard charts.
var metricKeys = []string{"cpu", "load1", "load5", "load15", "mem", "peers", "sess_down", "sess_up"}

func (s *Server) insightRoutes() {
	s.mux.HandleFunc("GET /api/diskspace", s.handleDiskspace)
	s.mux.HandleFunc("GET /api/history", s.handleHistory)
	s.mux.HandleFunc("GET /api/metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
}

func (s *Server) handleDiskspace(w http.ResponseWriter, _ *http.Request) {
	writeOK(w, disk.Usage(s.dirs))
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeOK(w, map[string]any{"points": []any{}, "first": 0})
		return
	}
	rng := parseRange(r.URL.Query().Get("range"))
	hash := r.URL.Query().Get("hash")
	ctx, cancel := reqCtx(r)
	defer cancel()
	pts, err := s.history.Query(ctx, rng, hash)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "history_error", err.Error())
		return
	}
	if pts == nil {
		pts = []history.Point{}
	}
	// Earliest sample we hold for this series, so the client can offer only the
	// time ranges that have data (e.g. hide "1y" on a day-old torrent).
	first, _ := s.history.FirstTS(ctx, hash)
	writeOK(w, map[string]any{"points": pts, "first": first})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		out := map[string][]history.GaugePoint{}
		for _, k := range metricKeys {
			out[k] = []history.GaugePoint{}
		}
		writeOK(w, out)
		return
	}
	rng := parseRange(r.URL.Query().Get("range"))
	ctx, cancel := reqCtx(r)
	defer cancel()
	series, err := s.history.QueryGauges(ctx, rng, metricKeys)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "metrics_error", err.Error())
		return
	}
	// Ensure every key is present (empty slice if no data) for a stable client shape.
	for _, k := range metricKeys {
		if series[k] == nil {
			series[k] = []history.GaugePoint{}
		}
	}
	writeOK(w, series)
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
