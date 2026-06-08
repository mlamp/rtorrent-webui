package api

import "net/http"

// BasicAuth wraps a handler with HTTP Basic auth. GET /healthz is always open so
// container health checks work without credentials. The exemption is scoped to
// GET deliberately: a non-GET /healthz must still authenticate, so nothing
// mounted at that path (e.g. a misconfigured rpc_proxy_path) can ride the health
// check's open door to bypass auth.
func BasicAuth(realm string, verify func(user, pass string) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || !verify(user, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
