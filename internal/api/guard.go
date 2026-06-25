package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// SameOriginGuard protects the HTTP surface against two browser-driven attack
// classes and is applied to the whole handler (before auth):
//
//   - CSRF: any state-changing request (POST/PUT/PATCH/DELETE) that carries a
//     browser Origin or Referer must be same-origin with the request Host.
//     Browsers always send Origin on unsafe cross-origin requests, so an
//     attacker page cannot drive add-by-URL / start / stop / delete / the
//     root-equivalent /RPC2 even when the instance runs with auth.mode=none, and
//     even under basic auth (which a browser would otherwise replay
//     automatically). Non-browser clients (curl, *arr via /RPC2) send neither
//     header and pass through unaffected.
//
//   - DNS rebinding (opt-in): if allowedHosts is non-empty, the request Host must
//     be on the list. This is the only defense against a malicious page rebinding
//     its domain to a loopback/LAN address, since the rebound request is
//     genuinely same-origin. Empty = accept any Host (no behaviour change); set
//     it (or front the app with a reverse proxy that pins Host) when exposing the
//     port beyond loopback without auth.
func SameOriginGuard(allowedHosts []string, next http.Handler) http.Handler {
	allow := make(map[string]bool, len(allowedHosts))
	for _, h := range allowedHosts {
		if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
			allow[h] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(allow) > 0 {
			h := strings.ToLower(r.Host)
			if !allow[h] && !allow[hostname(h)] {
				http.Error(w, "host not allowed", http.StatusForbidden)
				return
			}
		}
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if o := r.Header.Get("Origin"); o != "" {
				if !originHostMatches(o, r.Host) {
					http.Error(w, "cross-origin request blocked", http.StatusForbidden)
					return
				}
			} else if ref := r.Header.Get("Referer"); ref != "" {
				if !originHostMatches(ref, r.Host) {
					http.Error(w, "cross-origin request blocked", http.StatusForbidden)
					return
				}
			}
			// neither header present → non-browser client → allow
		}
		next.ServeHTTP(w, r)
	})
}

// originHostMatches reports whether the host of an Origin/Referer URL matches the
// request Host (hostname comparison, port-insensitive; case-insensitive). A
// malformed value fails closed. Comparing hostnames (not host:port) avoids false
// positives behind a TLS-terminating proxy while still blocking the real CSRF
// threat — a different domain.
func originHostMatches(rawURL, reqHost string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Hostname(), hostname(reqHost))
}

// hostname strips an optional :port from a Host header value.
func hostname(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}
