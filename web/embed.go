// Package web embeds the built Svelte SPA (web/dist) and serves it with an
// SPA index.html fallback. The Vite build writes to ./dist before `go build`.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded SPA assets rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}

// SPAHandler serves embedded static assets, falling back to index.html for any
// path that doesn't resolve to a file (client-side routing). Hashed assets get
// a long cache; index.html is served no-cache so new builds are picked up.
func SPAHandler() http.Handler {
	root := FS()
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if upath == "" {
			upath = "index.html"
		}
		if f, err := root.Open(upath); err == nil {
			info, err := f.Stat()
			f.Close()
			// Directories fall through to the SPA shell; serving them via
			// FileServer would emit a redirect or a generated listing.
			if err == nil && !info.IsDir() {
				if strings.HasPrefix(upath, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else if upath == "index.html" {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Not a real file → serve the SPA shell.
		w.Header().Set("Cache-Control", "no-cache")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
