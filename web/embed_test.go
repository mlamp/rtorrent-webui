package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// get runs a GET request for path through h and returns the recorder.
func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// spaShell returns the embedded index.html, the canonical SPA shell body.
func spaShell(t *testing.T) string {
	t.Helper()
	b, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}
	return string(b)
}

// index.html must be served no-cache on every path that yields it — including
// the direct "/" hit — so intermediaries never pin a stale shell after a deploy.
func TestSPAHandlerIndexNoCache(t *testing.T) {
	h := SPAHandler()
	for _, p := range []string{"/", "/index.html"} {
		rec := get(t, h, p)
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("GET %s: Cache-Control = %q, want %q", p, got, "no-cache")
		}
	}
}

// Directory paths must take the SPA fallback rather than reach
// http.FileServer, which would emit a 301 (/assets) or an auto-generated
// listing of the bundle files (/assets/).
func TestSPAHandlerDirectoryServesSPAShell(t *testing.T) {
	h := SPAHandler()
	shell := spaShell(t)
	for _, p := range []string{"/assets", "/assets/"} {
		rec := get(t, h, p)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want %d", p, rec.Code, http.StatusOK)
		}
		if body := rec.Body.String(); body != shell {
			t.Errorf("GET %s: body is not the SPA shell (got %d bytes, starts %.60q)", p, len(body), body)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("GET %s: Cache-Control = %q, want %q", p, got, "no-cache")
		}
	}
}

// Hashed bundle files keep their long immutable cache header.
func TestSPAHandlerHashedAssetImmutable(t *testing.T) {
	entries, err := fs.ReadDir(FS(), "assets")
	if err != nil {
		t.Fatalf("read embedded assets dir: %v", err)
	}
	var name string
	for _, e := range entries {
		if !e.IsDir() {
			name = e.Name()
			break
		}
	}
	if name == "" {
		t.Fatal("no files under embedded assets/")
	}
	rec := get(t, SPAHandler(), "/assets/"+name)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /assets/%s: status = %d, want %d", name, rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Errorf("GET /assets/%s: Cache-Control = %q, want immutable", name, got)
	}
}

// Unknown deep-link routes fall back to the SPA shell with no-cache.
func TestSPAHandlerDeepLinkFallback(t *testing.T) {
	rec := get(t, SPAHandler(), "/settings/peers")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings/peers: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != spaShell(t) {
		t.Errorf("GET /settings/peers: body is not the SPA shell (%d bytes)", len(body))
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("GET /settings/peers: Cache-Control = %q, want %q", got, "no-cache")
	}
}
