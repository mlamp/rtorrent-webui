package api

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// validateBasePath is the lexical gate: it decides whether a daemon-supplied
// base_path may be deleted, purely from the string and the configured roots —
// no filesystem, no rtorrent. These cases pin every containment rule.
func TestValidateBasePath(t *testing.T) {
	for _, tc := range []struct {
		name    string
		base    string
		roots   []string
		want    string // expected returned clean path (only when wantErr == nil)
		wantErr error  // sentinel (errors.Is); nil = must succeed
	}{
		{"contained child", "/data/dl/ubuntu", []string{"/data/dl"}, "/data/dl/ubuntu", nil},
		{"nested deeper", "/data/dl/a/b/c", []string{"/data/dl"}, "/data/dl/a/b/c", nil},
		{"second root matches", "/data/dl/x", []string{"/other", "/data/dl"}, "/data/dl/x", nil},
		{"uncleaned but contained", "/data/dl/./a/../ubuntu", []string{"/data/dl"}, "/data/dl/ubuntu", nil},
		{"trailing-dot filename stays contained", "/data/dl/foo.", []string{"/data/dl"}, "/data/dl/foo.", nil},

		{"base equals root rejected", "/data/dl", []string{"/data/dl"}, "", errPathOutsideRoots},
		{"escape via dotdot", "/data/dl/../etc", []string{"/data/dl"}, "", errPathOutsideRoots},
		{"sibling prefix", "/data/dl-evil/x", []string{"/data/dl"}, "", errPathOutsideRoots},
		{"fully outside", "/etc/passwd", []string{"/data/dl"}, "", errPathOutsideRoots},
		{"root slash refused", "/data/dl/x", []string{"/"}, "", errPathOutsideRoots},
		{"empty root skipped", "/data/dl/x", []string{""}, "", errPathOutsideRoots},
		{"base is filesystem root", "/", []string{"/data/dl"}, "", errPathOutsideRoots},

		{"empty base", "", []string{"/data/dl"}, "", errEmptyBasePath},
		{"relative base", "relative/path", []string{"/data/dl"}, "", errNotAbsolute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateBasePath(tc.base, tc.roots)
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

// resolveAndRevalidate is the symlink-aware second gate: it resolves real
// symlinks and re-checks containment against resolved roots, so a symlink that
// points outside the sandbox cannot smuggle the delete out. It must fail closed.
func TestResolveAndRevalidate(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "ubuntu")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	wantResolved, err := filepath.EvalSymlinks(child)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("contained existing path resolves", func(t *testing.T) {
		got, err := resolveAndRevalidate(child, []string{root})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got != wantResolved {
			t.Fatalf("got %q, want %q", got, wantResolved)
		}
	})

	t.Run("unresolvable root is dropped, valid root still used", func(t *testing.T) {
		got, err := resolveAndRevalidate(child, []string{filepath.Join(root, "does-not-exist"), root})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got != wantResolved {
			t.Fatalf("got %q, want %q", got, wantResolved)
		}
	})

	t.Run("all roots unresolvable fails closed", func(t *testing.T) {
		_, err := resolveAndRevalidate(child, []string{filepath.Join(root, "nope")})
		if !errors.Is(err, errPathOutsideRoots) {
			t.Fatalf("err = %v, want errPathOutsideRoots", err)
		}
	})

	t.Run("missing base reports already-gone", func(t *testing.T) {
		_, err := resolveAndRevalidate(filepath.Join(root, "ghost"), []string{root})
		if !errors.Is(err, errBasePathGone) {
			t.Fatalf("err = %v, want errBasePathGone", err)
		}
	})

	t.Run("symlink pointing outside the root is rejected", func(t *testing.T) {
		outside := t.TempDir() // a sibling tree, not under root
		link := filepath.Join(root, "escape")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		_, err := resolveAndRevalidate(link, []string{root})
		if !errors.Is(err, errPathOutsideRoots) {
			t.Fatalf("err = %v, want errPathOutsideRoots", err)
		}
	})
}
