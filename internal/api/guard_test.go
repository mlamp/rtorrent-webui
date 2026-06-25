package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
}

func TestSameOriginGuardCSRF(t *testing.T) {
	g := SameOriginGuard(nil, okHandler())
	cases := []struct {
		name    string
		method  string
		host    string
		origin  string
		referer string
		want    int
	}{
		{"same-origin POST allowed", "POST", "tv.local:8080", "http://tv.local:8080", "", http.StatusNoContent},
		{"same-origin POST port-insensitive (TLS proxy)", "POST", "tv.local", "https://tv.local", "", http.StatusNoContent},
		{"cross-origin POST blocked", "POST", "tv.local:8080", "http://evil.example", "", http.StatusForbidden},
		{"cross-origin DELETE blocked", "DELETE", "tv.local:8080", "http://evil.example", "", http.StatusForbidden},
		{"no-Origin POST allowed (api/arr client)", "POST", "tv.local:8080", "", "", http.StatusNoContent},
		{"referer same-origin allowed", "PUT", "tv.local:8080", "", "http://tv.local:8080/app", http.StatusNoContent},
		{"referer cross-origin blocked", "PUT", "tv.local:8080", "", "http://evil.example/x", http.StatusForbidden},
		{"cross-origin GET allowed (not state-changing)", "GET", "tv.local:8080", "http://evil.example", "", http.StatusNoContent},
		{"malformed origin fails closed", "POST", "tv.local:8080", "://nonsense", "", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, "http://"+c.host+"/api/torrents", nil)
			r.Host = c.host
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if c.referer != "" {
				r.Header.Set("Referer", c.referer)
			}
			w := httptest.NewRecorder()
			g.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}
}

func TestSameOriginGuardHostAllowlist(t *testing.T) {
	g := SameOriginGuard([]string{"tv.local", "10.0.0.5:8080"}, okHandler())
	cases := []struct {
		name string
		host string
		want int
	}{
		{"allowed hostname (any port)", "tv.local:8080", http.StatusNoContent},
		{"allowed host:port literal", "10.0.0.5:8080", http.StatusNoContent},
		{"rebinding domain rejected", "attacker.example", http.StatusForbidden},
		{"loopback rejected when not listed", "127.0.0.1:8080", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+c.host+"/api/torrents", nil)
			r.Host = c.host
			w := httptest.NewRecorder()
			g.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}

	// Empty allowlist accepts any Host (default, no behaviour change).
	open := SameOriginGuard(nil, okHandler())
	r := httptest.NewRequest("GET", "http://whatever.example/api/torrents", nil)
	r.Host = "whatever.example"
	w := httptest.NewRecorder()
	open.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("empty allowlist status = %d, want 204", w.Code)
	}
}
