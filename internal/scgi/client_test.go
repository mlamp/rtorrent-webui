package scgi

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shortDir returns a short-lived temp dir; t.TempDir() paths can exceed the
// 108-byte AF_UNIX sun_path limit, so we make our own short one under os.TempDir.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "scgi")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// A dead socket path (nothing ever listens) must, after the retry budget is
// exhausted, return an error classifiable as ErrUnreachable — the contract the
// API relies on to answer 503 "rtorrent_unreachable" instead of a hard error.
func TestDoUnreachableReturnsErrUnreachable(t *testing.T) {
	saved := connectBudget
	connectBudget = 200 * time.Millisecond // keep the test fast
	defer func() { connectBudget = saved }()

	c := New(filepath.Join(shortDir(t), "nope.sock"), 1, 100*time.Millisecond, time.Second)
	_, err := c.Do(context.Background(), "application/json", []byte(`{}`))
	if err == nil {
		t.Fatal("expected an error dialing a dead socket")
	}
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("error not classifiable as ErrUnreachable: %v", err)
	}
}

// The listener comes up ~250ms AFTER the request starts (a fast daemon restart).
// The dial retry must ride over the initial connection-refused and succeed.
func TestDoRetryRidesLateListener(t *testing.T) {
	sock := filepath.Join(shortDir(t), "rt.sock")
	c := New(sock, 1, time.Second, 2*time.Second)

	type result struct {
		body []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		b, err := c.Do(context.Background(), "application/json", []byte(`{"x":1}`))
		done <- result{b, err}
	}()

	time.Sleep(250 * time.Millisecond) // first dial attempts get ECONNREFUSED

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Read(make([]byte, 4096)) // drain the request
		_, _ = conn.Write([]byte("Status: 200 OK\r\nContent-Type: application/json\r\nContent-Length: 8\r\n\r\n{\"ok\":1}"))
	}()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Do failed despite the listener coming up within budget: %v", r.err)
		}
		if string(r.body) != `{"ok":1}` {
			t.Fatalf("unexpected body: %q", r.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Do did not complete in time")
	}
}
