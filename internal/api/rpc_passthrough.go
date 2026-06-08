package api

import (
	"encoding/json"
	"net/http"
)

// EnablePassthrough turns on POST /api/rpc with optional allow/deny method lists.
func (s *Server) EnablePassthrough(allow, deny []string) {
	s.rpcPassthrough = true
	s.rpcAllow = toSet(allow)
	s.rpcDeny = toSet(deny)
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
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
	if s.rpcDeny[req.Method] {
		writeErr(w, http.StatusForbidden, "denied", "method is denied: "+req.Method)
		return
	}
	if len(s.rpcAllow) > 0 && !s.rpcAllow[req.Method] {
		writeErr(w, http.StatusForbidden, "not_allowed", "method not in allowlist: "+req.Method)
		return
	}
	ctx, cancel := reqCtx(r)
	defer cancel()
	res, err := s.rpc.Call(ctx, req.Method, req.Params...)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "rpc_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": json.RawMessage(res)})
}
