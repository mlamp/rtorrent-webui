package api

import (
	"io"
	"net/http"
	"strings"
)

// maxRPCProxyBytes bounds a forwarded request body. XML-RPC calls are tiny; even
// a large system.multicall stays well under this. rtorrent's own SCGI body limit
// is ~16 MiB, so we cap there and let an oversize request fail clean.
const maxRPCProxyBytes = 16 << 20

// EnableRPCProxy mounts a raw XML-RPC/JSON-RPC byte-pipe at path (default
// "/RPC2"), forwarding request bodies to rtorrent verbatim — the same thing
// nginx's `scgi_pass` does. It lets *arr clients (Sonarr/Radarr/Lidarr/…) and
// any XML-RPC tool point straight at the webui instead of a separate nginx shim.
// Returns the resolved path (for logging).
//
// SECURITY: this is UNFILTERED, full control of rtorrent (including execute.*).
// Unlike /api/rpc there is no per-method denylist — treat it as root-equivalent.
// It inherits the webui's auth (see [api.BasicAuth] wiring in main): with
// auth.mode="basic" callers must present the same credentials, e.g.
// http://user:pass@host/RPC2 — exactly how nginx+ruTorrent share an htpasswd;
// with auth.mode="none" it is open and MUST stay on an internal network.
//
// The configured path is normalized to an exact, rooted mount point: a trailing
// slash is stripped (else ServeMux treats it as a subtree and 307-redirects the
// canonical path), and empty / non-rooted / reserved values fall back to the
// default. Reserved = /healthz (auth-exempt for GET — mounting the proxy there
// would be an auth bypass) and the /api/ surface (would shadow API routes). When
// the resolved path differs from the request, the caller can log the override.
func (s *Server) EnableRPCProxy(path string) string {
	path = strings.TrimRight(path, "/")
	if path == "" || !strings.HasPrefix(path, "/") || reservedProxyPath(path) {
		path = "/RPC2"
	}
	s.mux.HandleFunc(path, s.handleRPCProxy)
	return path
}

// reservedProxyPath reports whether path collides with a route the unfiltered
// proxy must never occupy: the auth-exempt health check, or the JSON API surface.
func reservedProxyPath(path string) bool {
	return path == "/healthz" || strings.HasPrefix(path, "/api/")
}

func (s *Server) handleRPCProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRPCProxyBytes))
	if err != nil {
		http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	// Forward the wire dialect verbatim. rtorrent routes on CONTENT_TYPE:
	// text/xml -> XML-RPC, application/json -> JSON-RPC. Default to XML-RPC,
	// which is what *arr clients send.
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/xml"
	}
	ctx, cancel := reqCtx(r)
	defer cancel()
	out, err := s.rpc.Forward(ctx, ct, body)
	if err != nil {
		http.Error(w, "rtorrent unreachable: "+err.Error(), http.StatusBadGateway)
		return
	}
	// rtorrent answers in the dialect it was addressed in; mirror it onto the
	// response (the SCGI layer strips rtorrent's own CGI response headers).
	respCT := "text/xml; charset=UTF-8"
	if strings.Contains(strings.ToLower(ct), "json") {
		respCT = "application/json"
	}
	w.Header().Set("Content-Type", respCT)
	_, _ = w.Write(out)
}
