package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// validateBrowsePath is validateBasePath with the single divergence that a path
// equal to a root is ACCEPTED (you must be able to list a root). These cases are
// the pathsafe_test.go matrix with base==root flipped reject->accept, plus the
// browse-specific bad-path/trailing-slash cases.
func TestValidateBrowsePath(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		roots   []string
		want    string
		wantErr error
	}{
		{"contained child", "/data/dl/ubuntu", []string{"/data/dl"}, "/data/dl/ubuntu", nil},
		{"nested deeper", "/data/dl/a/b/c", []string{"/data/dl"}, "/data/dl/a/b/c", nil},
		{"second root matches", "/data/dl/x", []string{"/other", "/data/dl"}, "/data/dl/x", nil},
		{"uncleaned but contained", "/data/dl/./a/../ubuntu", []string{"/data/dl"}, "/data/dl/ubuntu", nil},
		{"path IS the root (accepted, unlike delete)", "/data/dl", []string{"/data/dl"}, "/data/dl", nil},
		{"root with trailing slash", "/data/dl/", []string{"/data/dl"}, "/data/dl", nil},
		{"child with trailing slash", "/data/dl/ubuntu/", []string{"/data/dl"}, "/data/dl/ubuntu", nil},

		{"escape via dotdot", "/data/dl/../etc", []string{"/data/dl"}, "", errBrowseOutside},
		{"sibling prefix", "/data/dl-evil/x", []string{"/data/dl"}, "", errBrowseOutside},
		{"fully outside", "/etc/passwd", []string{"/data/dl"}, "", errBrowseOutside},
		{"root slash refused", "/data/dl/x", []string{"/"}, "", errBrowseOutside},
		{"empty root skipped", "/data/dl/x", []string{""}, "", errBrowseOutside},

		{"filesystem root rejected", "/", []string{"/data/dl"}, "", errBrowseBadPath},
		{"empty path", "", []string{"/data/dl"}, "", errBrowseBadPath},
		{"relative path", "relative/path", []string{"/data/dl"}, "", errBrowseBadPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateBrowsePath(tc.path, tc.roots)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err = %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// resolveBrowsePath resolves symlinks on both sides and re-contains against the
// resolved roots; it permits path==root, reports a missing in-root path as
// errBrowseNotFound, and fails closed.
func TestResolveBrowsePath(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "ubuntu")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	wantChild, _ := filepath.EvalSymlinks(child)
	wantRoot, _ := filepath.EvalSymlinks(root)

	t.Run("contained child resolves", func(t *testing.T) {
		got, err := resolveBrowsePath(child, []string{root})
		if err != nil || got != wantChild {
			t.Fatalf("got %q,%v want %q,nil", got, err, wantChild)
		}
	})
	t.Run("path is the root (accepted)", func(t *testing.T) {
		got, err := resolveBrowsePath(root, []string{root})
		if err != nil || got != wantRoot {
			t.Fatalf("got %q,%v want %q,nil", got, err, wantRoot)
		}
	})
	t.Run("symlinked root resolves to its target", func(t *testing.T) {
		target := t.TempDir()
		if err := os.MkdirAll(filepath.Join(target, "movies"), 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(t.TempDir(), "dl") // /tmp/X/dl -> target
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		got, err := resolveBrowsePath(filepath.Join(link, "movies"), []string{link})
		want, _ := filepath.EvalSymlinks(filepath.Join(target, "movies"))
		if err != nil || got != want {
			t.Fatalf("got %q,%v want %q,nil", got, err, want)
		}
	})
	t.Run("missing in-root path is not_found", func(t *testing.T) {
		_, err := resolveBrowsePath(filepath.Join(root, "ghost"), []string{root})
		if !errors.Is(err, errBrowseNotFound) {
			t.Fatalf("err = %v, want errBrowseNotFound", err)
		}
	})
	t.Run("symlink pointing outside the root is rejected", func(t *testing.T) {
		outside := t.TempDir()
		link := filepath.Join(root, "escape")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		_, err := resolveBrowsePath(link, []string{root})
		if !errors.Is(err, errBrowseOutside) {
			t.Fatalf("err = %v, want errBrowseOutside", err)
		}
	})
	t.Run("all roots unresolvable fails closed", func(t *testing.T) {
		_, err := resolveBrowsePath(child, []string{filepath.Join(root, "nope")})
		if !errors.Is(err, errBrowseOutside) {
			t.Fatalf("err = %v, want errBrowseOutside", err)
		}
	})
}

func TestListDirs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"alpha", "bravo", "charlie"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A regular file and a symlink-to-dir must NOT appear; a non-UTF-8 name skipped.
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "alpha"), filepath.Join(root, "zlink")); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		if err := os.Mkdir(filepath.Join(root, "bad\xffname"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("dirs only, sorted, symlink+file+nonutf8 excluded", func(t *testing.T) {
		entries, truncated, err := listDirs(root, browseDirCap, browseScanCap)
		if err != nil || truncated {
			t.Fatalf("err=%v truncated=%v", err, truncated)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name)
		}
		if strings.Join(names, ",") != "alpha,bravo,charlie" {
			t.Fatalf("names = %v, want [alpha bravo charlie]", names)
		}
		if entries[0].Path != filepath.Join(root, "alpha") {
			t.Fatalf("path = %q", entries[0].Path)
		}
	})

	t.Run("truncated when dir cap is hit", func(t *testing.T) {
		entries, truncated, err := listDirs(root, 2, browseScanCap)
		if err != nil || !truncated || len(entries) != 2 {
			t.Fatalf("entries=%d truncated=%v err=%v, want 2/true/nil", len(entries), truncated, err)
		}
	})

	t.Run("truncated when scan cap is hit", func(t *testing.T) {
		// scanCap=1 stops after a single raw entry regardless of dir count.
		_, truncated, err := listDirs(root, browseDirCap, 1)
		if err != nil || !truncated {
			t.Fatalf("truncated=%v err=%v, want true/nil", truncated, err)
		}
	})

	t.Run("ENOTDIR when target is a regular file", func(t *testing.T) {
		_, _, err := listDirs(filepath.Join(root, "notes.txt"), browseDirCap, browseScanCap)
		if err == nil {
			t.Fatal("want a filesystem error for a non-directory, got nil")
		}
	})
}

// --- handler taxonomy --------------------------------------------------------

func browseServer(t *testing.T, roots ...string) *Server {
	t.Helper()
	s := New(sse.NewHub(), nil, "main")
	s.SetBrowse(true)
	s.SetDirs(roots)
	return s
}

func getFS(t *testing.T, s *Server, query string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/fs"+query, nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

func TestHandleFS(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "movies")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, _ := filepath.EvalSymlinks(root)

	t.Run("capability off returns 503 with NO filesystem access", func(t *testing.T) {
		s := New(sse.NewHub(), nil, "main")
		s.SetDirs([]string{root}) // roots exist, but browse not armed
		// browseAllowed=false must short-circuit before resolvedRoots(); use a
		// path that would error if it ever reached the FS gate.
		rec, _ := getFS(t, s, "?path=/etc")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("code = %d, want 503", rec.Code)
		}
	})

	t.Run("no resolvable roots returns 503", func(t *testing.T) {
		s := New(sse.NewHub(), nil, "main")
		s.SetBrowse(true) // armed, but no dirs
		rec, _ := getFS(t, s, "")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("code = %d, want 503", rec.Code)
		}
	})

	t.Run("empty path lists resolved roots", func(t *testing.T) {
		s := browseServer(t, root)
		rec, body := getFS(t, s, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200", rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
		}
		roots := body["data"].(map[string]any)["roots"].([]any)
		if len(roots) != 1 || roots[0].(map[string]any)["path"] != resolvedRoot {
			t.Fatalf("roots = %v, want one root %q", roots, resolvedRoot)
		}
	})

	t.Run("path lists child dirs", func(t *testing.T) {
		s := browseServer(t, root)
		rec, body := getFS(t, s, "?path="+root)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200", rec.Code)
		}
		entries := body["data"].(map[string]any)["entries"].([]any)
		if len(entries) != 1 || entries[0].(map[string]any)["name"] != "movies" {
			t.Fatalf("entries = %v, want [movies]", entries)
		}
	})

	t.Run("empty directory yields entries [] not null", func(t *testing.T) {
		s := browseServer(t, root)
		empty := filepath.Join(root, "empty")
		if err := os.Mkdir(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		rec, _ := getFS(t, s, "?path="+empty)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200", rec.Code)
		}
		// Must be the literal JSON array [], never null (which breaks the client).
		if !strings.Contains(rec.Body.String(), `"entries":[]`) {
			t.Fatalf("empty dir must serialize entries as [], got: %s", rec.Body.String())
		}
	})

	t.Run("out-of-root path is 403 and leaks NO path", func(t *testing.T) {
		s := browseServer(t, root)
		rec, _ := getFS(t, s, "?path=/etc/shadow")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("code = %d, want 403", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "/etc") {
			t.Fatalf("response leaked the requested path: %s", rec.Body.String())
		}
	})

	t.Run("out-of-root response is identical whether target exists", func(t *testing.T) {
		s := browseServer(t, root)
		recA, _ := getFS(t, s, "?path=/etc")      // exists
		recB, _ := getFS(t, s, "?path=/nope/zzz") // does not exist
		if recA.Code != http.StatusForbidden || recB.Code != http.StatusForbidden {
			t.Fatalf("codes = %d,%d, want 403,403 (no existence oracle)", recA.Code, recB.Code)
		}
		if recA.Body.String() != recB.Body.String() {
			t.Fatalf("bodies differ — existence oracle:\n%s\n%s", recA.Body.String(), recB.Body.String())
		}
	})

	t.Run("in-root missing path is 404", func(t *testing.T) {
		s := browseServer(t, root)
		rec, _ := getFS(t, s, "?path="+filepath.Join(root, "ghost"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("code = %d, want 404", rec.Code)
		}
	})

	t.Run("relative path is 400", func(t *testing.T) {
		s := browseServer(t, root)
		rec, _ := getFS(t, s, "?path=relative")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code = %d, want 400", rec.Code)
		}
	})

	t.Run("an in-root symlink is omitted from the listing but its target is reachable by path", func(t *testing.T) {
		s := browseServer(t, root)
		// /root/link -> /root/movies (in-root target)
		link := filepath.Join(root, "link")
		_ = os.Symlink(child, link)
		_, body := getFS(t, s, "?path="+root)
		entries := body["data"].(map[string]any)["entries"].([]any)
		for _, e := range entries {
			if e.(map[string]any)["name"] == "link" {
				t.Fatal("symlink entry must be omitted from the listing")
			}
		}
		// But the real target is browsable by typing its path.
		rec, _ := getFS(t, s, "?path="+child)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d, want 200 for the symlink target by path", rec.Code)
		}
	})

	t.Run("parallel requests are race-free", func(t *testing.T) {
		t.Parallel()
		s := browseServer(t, root)
		rec, _ := getFS(t, s, "?path="+root)
		if rec.Code != http.StatusOK {
			t.Fatalf("code = %d", rec.Code)
		}
	})
}

// /api/config browse flag is the AND of (browseAllowed) and (>=1 resolvable root).
func TestHandleConfigAdvertisesBrowse(t *testing.T) {
	tmp := t.TempDir()
	for _, tc := range []struct {
		name  string
		armed bool
		dirs  []string
		want  bool
	}{
		{"armed with a resolvable root", true, []string{tmp}, true},
		{"armed but no roots", true, nil, false},
		{"roots present but not armed", false, []string{tmp}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(sse.NewHub(), nil, "main")
			s.SetBrowse(tc.armed)
			s.SetDirs(tc.dirs)
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
			var got struct {
				Data struct {
					Browse bool `json:"browse"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Data.Browse != tc.want {
				t.Fatalf("browse = %v, want %v", got.Data.Browse, tc.want)
			}
		})
	}
}
