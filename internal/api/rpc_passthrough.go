package api

import (
	"encoding/json"
	"net/http"

	"github.com/mlamp/rtorrent-webui/internal/config"
)

// EnablePassthrough turns on POST /api/rpc with optional allow/deny method
// lists. Entries ending in '*' match the whole method-name prefix (family).
func (s *Server) EnablePassthrough(allow, deny []string) {
	s.rpcPassthrough = true
	s.rpcAllow = config.NewMethodSet(allow)
	s.rpcDeny = config.NewMethodSet(deny)
}

func (s *Server) handleRPCPassthrough(w http.ResponseWriter, r *http.Request) {
	if !s.rpcPassthrough {
		writeErr(w, http.StatusForbidden, "disabled", "rpc passthrough is disabled")
		return
	}
	var req struct {
		Method string `json:"method"`
		Params []any  `json:"params"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Method == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "expected {method, params}")
		return
	}
	if s.rpcDeny.Matches(req.Method) {
		writeErr(w, http.StatusForbidden, "denied", "method is denied: "+req.Method)
		return
	}
	if !s.rpcAllow.Empty() && !s.rpcAllow.Matches(req.Method) {
		writeErr(w, http.StatusForbidden, "not_allowed", "method not in allowlist: "+req.Method)
		return
	}
	ctx, cancel := reqCtx(r)
	defer cancel()
	res, err := s.rpc.Call(ctx, req.Method, req.Params...)
	if err != nil {
		writeRPCErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": json.RawMessage(res)})
}
