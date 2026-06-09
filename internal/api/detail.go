package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/mlamp/rtorrent-webui/internal/model"
)

// GeoLookup resolves an IP to an ISO-3166 alpha-2 country code ("" if unknown).
// Wired in M5; nil until then.
type GeoLookup interface {
	Country(ip string) string
}

// DetailRPC is the subset of rpc.Client the detail endpoints use, behind an
// interface so -mock mode can serve detail data without a live rtorrent.
type DetailRPC interface {
	Files(ctx context.Context, hash string) ([]model.File, error)
	Peers(ctx context.Context, hash string) ([]model.Peer, error)
	Trackers(ctx context.Context, hash string) ([]model.Tracker, error)
	Pieces(ctx context.Context, hash string) (model.Pieces, error)
	SetFilePriority(ctx context.Context, hash string, index, prio int) error
	SetTrackerEnabled(ctx context.Context, hash string, index int, enabled bool) error
}

// SetGeo installs a GeoIP lookup used to annotate the peer list.
func (s *Server) SetGeo(g GeoLookup) { s.geo = g }

func (s *Server) detailRoutes() {
	s.mux.HandleFunc("GET /api/torrents/{hash}/files", s.handleFiles)
	s.mux.HandleFunc("GET /api/torrents/{hash}/peers", s.handlePeers)
	s.mux.HandleFunc("GET /api/torrents/{hash}/trackers", s.handleTrackers)
	s.mux.HandleFunc("GET /api/torrents/{hash}/pieces", s.handlePieces)
	s.mux.HandleFunc("PUT /api/torrents/{hash}/files/{index}/priority", s.handleFilePriority)
	s.mux.HandleFunc("PUT /api/torrents/{hash}/trackers/{index}/enabled", s.handleTrackerEnabled)
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	files, err := s.detail.Files(ctx, r.PathValue("hash"))
	if err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, files)
}

func (s *Server) handlePieces(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	pieces, err := s.detail.Pieces(ctx, r.PathValue("hash"))
	if err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, pieces)
}

func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	peers, err := s.detail.Peers(ctx, r.PathValue("hash"))
	if err != nil {
		writeRPCErr(w, err)
		return
	}
	if s.geo != nil {
		for i := range peers {
			peers[i].Country = s.geo.Country(peers[i].Address)
		}
	}
	writeOK(w, peers)
}

func (s *Server) handleTrackers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	trackers, err := s.detail.Trackers(ctx, r.PathValue("hash"))
	if err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, trackers)
}

func (s *Server) handleFilePriority(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_index", "index must be an integer")
		return
	}
	var body struct {
		Priority int `json:"priority"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if err := s.detail.SetFilePriority(ctx, r.PathValue("hash"), index, body.Priority); err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleTrackerEnabled(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_index", "index must be an integer")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if err := s.detail.SetTrackerEnabled(ctx, r.PathValue("hash"), index, body.Enabled); err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, map[string]bool{"ok": true})
}
