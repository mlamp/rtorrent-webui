package api

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"unicode/utf8"
)

// Read-only directory browser behind the Add-Torrent "save to" combobox.
//
// GET /api/fs                 -> the resolved download roots (top level)
// GET /api/fs?path=<abs>      -> the child DIRECTORIES of an in-root path
//
// This lists the WEBUI process's own filesystem (os.ReadDir); it equals the
// daemon's view only when they share a mount (the recommended same-host
// deployment). SetBrowse gates it on that assumption. The listing is confined to
// the configured download roots by validateBrowsePath/resolveBrowsePath
// (pathsafe.go) and is strictly read-only — see the TOCTOU note at pathsafe.go.
//
// downloads.dirs are WEBUI-LOCAL paths reconciled with the daemon only by
// identical-mount convention.

const (
	// browseDirCap bounds how many child directories one listing returns.
	browseDirCap = 2000
	// browseScanCap hard-bounds how many raw directory entries we will scan,
	// regardless of how few are directories. A directory with millions of entries
	// would otherwise be an unauthenticated CPU DoS; we stop scanning here and
	// report truncated=true. (os.ReadDir(-1) is never used — it loads+sorts the
	// whole directory before any cap could apply.)
	browseScanCap = 20000
)

// fsEntry is one directory in a listing. Path is the absolute, symlink-resolved,
// UTF-8-valid path used to drill in (the next GET sends it back as ?path).
type fsEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// listDirs returns up to browseDirCap child directories of resolvedDir (already
// validated and symlink-resolved). It reads in bounded batches and emits ONLY
// real, UTF-8-named directories: symlinks (which could escape the roots and are
// not re-resolved per entry), pipes, sockets and devices are dropped, and a
// non-UTF-8 path is skipped so every emitted Path round-trips through JSON. The
// returned truncated is true when scanning stopped before the directory was
// exhausted (dir cap or scan cap hit), so the UI can prompt the user to narrow.
func listDirs(resolvedDir string, dirCap, scanCap int) (entries []fsEntry, truncated bool, err error) {
	f, err := os.Open(resolvedDir)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	scanned := 0
	for {
		batch, rerr := f.ReadDir(512)
		for _, de := range batch {
			scanned++
			// A symlink reports as ModeSymlink (never IsDir), so IsDir() alone
			// already excludes symlinked dirs; the explicit bit documents intent.
			if de.IsDir() && de.Type()&os.ModeSymlink == 0 {
				name := de.Name()
				full := filepath.Join(resolvedDir, name)
				if utf8.ValidString(full) {
					entries = append(entries, fsEntry{Name: name, Path: full})
				}
			}
			if len(entries) >= dirCap || scanned >= scanCap {
				return sortEntries(entries), true, nil
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				return sortEntries(entries), false, nil // whole directory scanned
			}
			return nil, false, rerr
		}
	}
}

func sortEntries(e []fsEntry) []fsEntry {
	sort.Slice(e, func(i, j int) bool { return e[i].Name < e[j].Name })
	return e
}

// rootEntries renders the resolved roots as the top-level listing.
func rootEntries(roots []string) []fsEntry {
	out := make([]fsEntry, 0, len(roots))
	for _, r := range roots {
		out = append(out, fsEntry{Name: filepath.Base(r), Path: r})
	}
	return out
}

func (s *Server) handleFS(w http.ResponseWriter, r *http.Request) {
	// Guard order is load-bearing: browseAllowed is checked BEFORE any filesystem
	// access, so a capability-off (e.g. unauthenticated TCP) caller triggers zero
	// syscalls. resolvedRoots() touches the filesystem, so it must come second.
	if !s.browseAllowed {
		writeErr(w, http.StatusServiceUnavailable, "browse_unavailable", "browsing unavailable")
		return
	}
	roots := s.resolvedRoots()
	if len(roots) == 0 {
		writeErr(w, http.StatusServiceUnavailable, "browse_unavailable", "browsing unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	path := r.URL.Query().Get("path")
	if path == "" {
		// Top level: the resolved roots themselves.
		writeOK(w, fsResult("", rootEntries(roots), []fsEntry{}, false))
		return
	}

	// Lexical gate first (pure, no syscalls) so a malformed/out-of-root path is
	// rejected without touching disk.
	clean, err := validateBrowsePath(path, roots)
	if err != nil {
		writeBrowseErr(w, err)
		return
	}

	// The symlink resolve + directory read can block on a hung mount; run them
	// under the request deadline and abandon the goroutine if it overruns (the
	// deadline bounds our WAIT, not a stuck syscall — adequate for local disk).
	ctx, cancel := reqCtx(r)
	defer cancel()
	type listing struct {
		resolved  string
		entries   []fsEntry
		truncated bool
		err       error
	}
	done := make(chan listing, 1)
	go func() {
		resolved, err := resolveBrowsePath(clean, roots)
		if err != nil {
			done <- listing{err: err}
			return
		}
		entries, truncated, err := listDirs(resolved, browseDirCap, browseScanCap)
		done <- listing{resolved: resolved, entries: entries, truncated: truncated, err: err}
	}()

	select {
	case <-ctx.Done():
		writeErr(w, http.StatusGatewayTimeout, "browse_timeout", "directory listing timed out")
		return
	case res := <-done:
		if res.err != nil {
			if errors.Is(res.err, errBrowseBadPath) || errors.Is(res.err, errBrowseOutside) || errors.Is(res.err, errBrowseNotFound) {
				writeBrowseErr(w, res.err)
				return
			}
			// A raw filesystem error (EACCES on an unreadable dir, ENOTDIR when a
			// root is a regular file, …). Map to a static message — never a raw
			// 500 and never the path.
			writeErr(w, http.StatusInternalServerError, "list_failed", "could not list directory")
			return
		}
		writeOK(w, fsResult(res.resolved, rootEntries(roots), res.entries, res.truncated))
	}
}

func fsResult(path string, roots, entries []fsEntry, truncated bool) map[string]any {
	// Always emit arrays, never JSON null: an empty directory yields a nil slice
	// that would marshal to `null` and break the client's entries.length / filter.
	if roots == nil {
		roots = []fsEntry{}
	}
	if entries == nil {
		entries = []fsEntry{}
	}
	return map[string]any{"path": path, "roots": roots, "entries": entries, "truncated": truncated}
}

// writeBrowseErr maps a browse sentinel to a STATIC-message HTTP error — the
// message is a literal that never interpolates the caller's path (so the
// endpoint cannot become a path/existence-reflection oracle). A 403 is returned
// identically whether or not an out-of-root target exists.
func writeBrowseErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errBrowseBadPath):
		writeErr(w, http.StatusBadRequest, "bad_path", "invalid path")
	case errors.Is(err, errBrowseNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "not found")
	default: // errBrowseOutside and any unexpected error — fail closed
		writeErr(w, http.StatusForbidden, "path_outside_roots", "path is outside the allowed roots")
	}
}
