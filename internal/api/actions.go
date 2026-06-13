package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/rpc"
)

const maxUploadBytes = 12 << 20 // default cap; ~12 MiB raw (SCGI body cap is 16 MiB before base64)

// SetMaxUploadBytes overrides the .torrent upload size cap (wired from the
// max_upload_mb config option; the default is maxUploadBytes).
func (s *Server) SetMaxUploadBytes(n int64) {
	if n > 0 {
		s.maxUpload = n
	}
}

func (s *Server) actionRoutes() {
	s.mux.HandleFunc("POST /api/torrents", s.handleAdd)
	s.mux.HandleFunc("POST /api/torrents/{hash}/start", s.actionHandler((*rpc.Client).Start))
	s.mux.HandleFunc("POST /api/torrents/{hash}/stop", s.actionHandler((*rpc.Client).Stop))
	s.mux.HandleFunc("POST /api/torrents/{hash}/pause", s.actionHandler((*rpc.Client).Pause))
	s.mux.HandleFunc("POST /api/torrents/{hash}/recheck", s.actionHandler((*rpc.Client).Recheck))
	s.mux.HandleFunc("POST /api/torrents/{hash}/announce", s.actionHandler((*rpc.Client).Announce))
	s.mux.HandleFunc("DELETE /api/torrents/{hash}", s.handleErase)
	s.mux.HandleFunc("PUT /api/torrents/{hash}/label", s.handleLabel)
	s.mux.HandleFunc("PUT /api/torrents/{hash}/priority", s.handlePriority)
	s.mux.HandleFunc("PUT /api/torrents/{hash}/directory", s.handleDirectory)
	s.mux.HandleFunc("PUT /api/throttle", s.handleThrottle)
}

func reqCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 10*time.Second)
}

// actionHandler adapts a (ctx, hash)->error rpc method to an HTTP handler.
func (s *Server) actionHandler(fn func(*rpc.Client, context.Context, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := reqCtx(r)
		defer cancel()
		if err := fn(s.rpc, ctx, r.PathValue("hash")); err != nil {
			writeRPCErr(w, err)
			return
		}
		writeOK(w, map[string]bool{"ok": true})
	}
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()

	var uri, label, dir string
	var start bool

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(s.maxUpload + 1<<20); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_form", err.Error())
			return
		}
		label, dir = r.FormValue("label"), r.FormValue("directory")
		start = r.FormValue("start") == "true"
		file, _, err := r.FormFile("torrent")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "no_file", "missing 'torrent' file")
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, s.maxUpload+1))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "read_error", err.Error())
			return
		}
		if int64(len(data)) > s.maxUpload {
			writeErr(w, http.StatusRequestEntityTooLarge, "too_large",
				fmt.Sprintf("torrent exceeds %d MiB", s.maxUpload>>20))
			return
		}
		uri = rpc.DataURI(data)
	} else {
		var body struct {
			Magnet    string `json:"magnet"`
			URL       string `json:"url"`
			Label     string `json:"label"`
			Directory string `json:"directory"`
			Start     bool   `json:"start"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_json", err.Error())
			return
		}
		label, dir, start = body.Label, body.Directory, body.Start
		switch {
		case body.Magnet != "":
			uri = body.Magnet
		case body.URL != "":
			uri = body.URL
		default:
			writeErr(w, http.StatusBadRequest, "no_source", "provide a torrent file, magnet, or url")
			return
		}
	}

	// label/dir ride to the daemon inside load.* command strings that rtorrent
	// re-parses with its command grammar. Quoting (below) makes ',' and ';'
	// literal, but a parsed argument starting with '$' is EXECUTED as a command
	// (substitution) regardless of quoting — reject those outright.
	if strings.HasPrefix(label, "$") {
		writeErr(w, http.StatusBadRequest, "invalid_label", "label may not start with '$'")
		return
	}
	if strings.HasPrefix(dir, "$") {
		writeErr(w, http.StatusBadRequest, "invalid_directory", "directory may not start with '$'")
		return
	}

	var cmds []string
	if label != "" {
		cmds = append(cmds, "d.custom1.set="+rpc.QuoteCommandValue(label))
	}
	if dir != "" {
		cmds = append(cmds, "d.directory.set="+rpc.QuoteCommandValue(dir))
	}
	if err := s.rpc.Load(ctx, start, uri, cmds...); err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOKStatus(w, http.StatusCreated, map[string]bool{"added": true})
}

func (s *Server) handleErase(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	// NOTE: data deletion (?data=true) is gated behind config; wired in M6.
	if err := s.rpc.Erase(ctx, r.PathValue("hash")); err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, map[string]bool{"erased": true})
}

func (s *Server) handleLabel(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	var body struct {
		Label string `json:"label"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if err := s.rpc.SetLabel(ctx, r.PathValue("hash"), body.Label); err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, map[string]bool{"ok": true})
}

func (s *Server) handlePriority(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	var body struct {
		Priority int `json:"priority"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if err := s.rpc.SetPriority(ctx, r.PathValue("hash"), body.Priority); err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleDirectory(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	var body struct {
		Directory string `json:"directory"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if err := s.rpc.SetDirectory(ctx, r.PathValue("hash"), body.Directory); err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, map[string]bool{"ok": true})
}

func (s *Server) handleThrottle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqCtx(r)
	defer cancel()
	var body struct {
		Down int64 `json:"down"`
		Up   int64 `json:"up"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json", err.Error())
		return
	}
	if err := s.rpc.SetGlobalThrottle(ctx, body.Down, body.Up); err != nil {
		writeRPCErr(w, err)
		return
	}
	writeOK(w, map[string]bool{"ok": true})
}
