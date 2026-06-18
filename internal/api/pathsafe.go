package api

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Path-safety gates for the optional "delete files from disk" path. The base
// path we are about to unlink is supplied by the DAEMON (d.base_path), not the
// client — a buggy rtorrent could hand back "/" or an arbitrary path, so we
// refuse to delete anything that is not provably contained inside one of the
// configured download roots. These are FAIL-CLOSED by construction.
//
// Threat model: this defends against a BUGGY daemon returning a bad string and
// against symlinks inside the tree (see resolveAndRevalidate). It does NOT claim
// to defeat a MALICIOUS daemon that can race the filesystem between the
// containment check and the unlink (TOCTOU) — that would need openat/O_NOFOLLOW
// and is out of scope. Linux-targeted.
var (
	errEmptyBasePath    = errors.New("base_path is empty (no data on disk yet)")
	errNotAbsolute      = errors.New("base_path is not absolute")
	errPathOutsideRoots = errors.New("base_path is outside the configured download roots")
	errBasePathGone     = errors.New("base_path no longer exists on disk")
)

// validateBasePath is the pure, lexical containment gate: no filesystem access,
// no rtorrent. It returns the cleaned path when `base` is provably a strict
// descendant of at least one usable root, else a sentinel describing why not.
//
// Roots equal to "" or "/" are skipped (refusing "/" as a root prevents an
// over-broad sandbox). A base equal to a root is rejected — we never RemoveAll a
// whole download root.
func validateBasePath(base string, roots []string) (string, error) {
	if base == "" {
		return "", errEmptyBasePath
	}
	if !filepath.IsAbs(base) {
		return "", errNotAbsolute
	}
	clean := filepath.Clean(base)
	if clean == "/" || clean == "." {
		return "", errPathOutsideRoots
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		rootClean := filepath.Clean(root)
		if rootClean == "/" || rootClean == "." {
			continue // refuse an over-broad root
		}
		rel, err := filepath.Rel(rootClean, clean)
		if err != nil {
			continue
		}
		// Strict containment: rel must be a real sub-path. "." means base IS the
		// root (rejected); ".." / "../…" means it escapes; an absolute rel means
		// different volumes (Rel cannot relate them).
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		return clean, nil
	}
	return "", errPathOutsideRoots
}

// resolveAndRevalidate is the symlink-aware second gate. It resolves real
// symlinks in both the candidate path and the roots, then re-checks containment
// against the RESOLVED roots, so a symlink inside the tree that points outside
// cannot redirect the delete. Fail-closed:
//   - any root that cannot be resolved is DROPPED (never lexical-fallback); if
//     none survive, the path is treated as outside the sandbox.
//   - a base that no longer exists returns errBasePathGone (benign: already
//     gone, the caller just erases).
//   - any other resolve failure, or a resolved path that escapes, returns
//     errPathOutsideRoots.
//
// `clean` is expected to already have passed validateBasePath.
func resolveAndRevalidate(clean string, roots []string) (string, error) {
	var resolvedRoots []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		rr, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err != nil {
			continue // drop unresolvable roots — never fall back to the lexical path
		}
		resolvedRoots = append(resolvedRoots, rr)
	}
	if len(resolvedRoots) == 0 {
		return "", errPathOutsideRoots
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errBasePathGone
		}
		return "", errPathOutsideRoots
	}
	if _, err := validateBasePath(resolved, resolvedRoots); err != nil {
		return "", errPathOutsideRoots
	}
	return resolved, nil
}

// --- Browse gate ----------------------------------------------------------
//
// The READ-ONLY directory browser (GET /api/fs) reuses the same fail-closed
// containment philosophy as the delete gate above, with ONE deliberate
// divergence: a path equal to a configured root is ACCEPTED — you must be able
// to list a root itself — whereas the delete gate rejects it (never RemoveAll a
// whole root). These functions never feed a mutating syscall; a returned path is
// still re-validated through this gate before any further use. Linux-targeted.
//
// Error sentinels carry NO path interpolation (like errPathOutsideRoots), so the
// handler can map them to static messages and never reflect a caller path.
var (
	errBrowseBadPath  = errors.New("path is empty, relative, or the filesystem root")
	errBrowseOutside  = errors.New("path is outside the configured download roots")
	errBrowseNotFound = errors.New("path does not exist inside the configured download roots")
)

// validateBrowsePath is the lexical containment gate for the browser. It is
// validateBasePath with the single change that rel=="." (path IS a root) is
// accepted. No filesystem access. Returns the cleaned path on success.
func validateBrowsePath(path string, roots []string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", errBrowseBadPath
	}
	clean := filepath.Clean(path)
	if clean == "/" || clean == "." {
		return "", errBrowseBadPath
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		rootClean := filepath.Clean(root)
		if rootClean == "/" || rootClean == "." {
			continue // refuse an over-broad root
		}
		rel, err := filepath.Rel(rootClean, clean)
		if err != nil {
			continue
		}
		// Containment, permitting rel=="." (the root itself). ".." / "../…"
		// escapes; an absolute rel means different volumes. NEVER use
		// strings.HasPrefix on the raw paths — "/data/dl-evil" must not match
		// root "/data/dl"; filepath.Rel is what makes the sibling-prefix safe.
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		return clean, nil
	}
	return "", errBrowseOutside
}

// resolveRoots returns the EvalSymlinks-resolved, de-duplicated form of each
// usable root, dropping empty / "/" / unresolvable entries. Fail-closed: an
// unresolvable root is never lexical-fallback'd. Order follows the input.
func resolveRoots(roots []string) []string {
	seen := make(map[string]bool, len(roots))
	var out []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		rc := filepath.Clean(root)
		if rc == "/" || rc == "." {
			continue
		}
		rr, err := filepath.EvalSymlinks(rc)
		if err != nil {
			continue // drop unresolvable roots — never fall back to the lexical path
		}
		if !seen[rr] {
			seen[rr] = true
			out = append(out, rr)
		}
	}
	return out
}

// resolveBrowsePath is the symlink-aware second gate for the browser. It mirrors
// resolveAndRevalidate but (a) permits the resolved path to equal a resolved root
// and (b) reports a missing in-root path as errBrowseNotFound (404 — the
// read-only analogue of the delete gate's benign errBasePathGone). `clean` is
// expected to have passed validateBrowsePath. Fail-closed.
func resolveBrowsePath(clean string, roots []string) (string, error) {
	resolvedRoots := resolveRoots(roots)
	if len(resolvedRoots) == 0 {
		return "", errBrowseOutside
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errBrowseNotFound
		}
		return "", errBrowseOutside
	}
	if _, err := validateBrowsePath(resolved, resolvedRoots); err != nil {
		return "", errBrowseOutside
	}
	return resolved, nil
}
