package api

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/rpc"
	"github.com/mlamp/rtorrent-webui/internal/scgi"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// fakeSCGI is a one-shot SCGI server: it records the next request it receives and
// replies with the canned CGI response, mimicking how rtorrent half-closes after
// answering.
type fakeSCGI struct {
	ln   net.Listener
	reqs chan []byte
	resp string
}

func newFakeSCGI(t *testing.T, resp string) *fakeSCGI {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeSCGI{ln: ln, reqs: make(chan []byte, 1), resp: resp}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				raw, _ := io.ReadAll(conn) // client CloseWrites after sending -> EOF here
				select {
				case f.reqs <- raw:
				default:
				}
				_, _ = conn.Write([]byte(f.resp))
			}()
		}
	}()
	return f
}

func (f *fakeSCGI) addr() string { return "tcp://" + f.ln.Addr().String() }
func (f *fakeSCGI) close()       { f.ln.Close() }

// parseSCGI splits an SCGI frame into its headers and body.
func parseSCGI(t *testing.T, raw []byte) (map[string]string, []byte) {
	t.Helper()
	i := bytes.IndexByte(raw, ':')
	if i < 0 {
		t.Fatalf("no netstring length in request")
	}
	n, err := strconv.Atoi(string(raw[:i]))
	if err != nil {
		t.Fatalf("bad netstring length: %v", err)
	}
	hbytes := raw[i+1 : i+1+n]
	body := raw[i+1+n+1:] // skip the trailing ','
	headers := map[string]string{}
	parts := bytes.Split(hbytes, []byte{0})
	for j := 0; j+1 < len(parts); j += 2 {
		headers[string(parts[j])] = string(parts[j+1])
	}
	return headers, body
}

func newProxyServer(t *testing.T, scgiAddr string) *Server {
	t.Helper()
	client := rpc.New(scgi.New(scgiAddr, 4, 2*time.Second, 2*time.Second))
	srv := New(sse.NewHub(), client, "main")
	srv.EnableRPCProxy("/RPC2")
	return srv
}

func TestRPCProxyForwardsXML(t *testing.T) {
	const reqXML = `<?xml version="1.0"?><methodCall><methodName>system.listMethods</methodName></methodCall>`
	const respXML = `<?xml version="1.0"?><methodResponse><params/></methodResponse>`
	fake := newFakeSCGI(t, "Status: 200 OK\r\nContent-Type: text/xml\r\n\r\n"+respXML)
	defer fake.close()

	srv := newProxyServer(t, fake.addr())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/RPC2", strings.NewReader(reqXML))
	req.Header.Set("Content-Type", "text/xml")
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != respXML {
		t.Errorf("response body = %q, want %q", got, respXML)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/xml") {
		t.Errorf("response Content-Type = %q, want text/xml", ct)
	}

	// The request must have reached rtorrent verbatim with the XML content-type.
	select {
	case raw := <-fake.reqs:
		headers, body := parseSCGI(t, raw)
		if headers["CONTENT_TYPE"] != "text/xml" {
			t.Errorf("forwarded CONTENT_TYPE = %q, want text/xml", headers["CONTENT_TYPE"])
		}
		if string(body) != reqXML {
			t.Errorf("forwarded body = %q, want %q", body, reqXML)
		}
		if headers["CONTENT_LENGTH"] != strconv.Itoa(len(reqXML)) {
			t.Errorf("CONTENT_LENGTH = %q, want %d", headers["CONTENT_LENGTH"], len(reqXML))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scgi server never received the request")
	}
}

func TestRPCProxyDefaultsAndMirrorsJSON(t *testing.T) {
	fake := newFakeSCGI(t, "Status: 200 OK\r\nContent-Type: application/json\r\n\r\n{\"id\":1}")
	defer fake.close()
	srv := newProxyServer(t, fake.addr())

	// No Content-Type on the request and a JSON-ish dialect chosen by the client:
	// send application/json and expect the response mirrored as JSON.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/RPC2", strings.NewReader(`{"method":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("response Content-Type = %q, want application/json", ct)
	}
	headers, _ := parseSCGI(t, <-fake.reqs)
	if headers["CONTENT_TYPE"] != "application/json" {
		t.Errorf("forwarded CONTENT_TYPE = %q, want application/json", headers["CONTENT_TYPE"])
	}
}

func TestRPCProxyDefaultContentTypeIsXML(t *testing.T) {
	fake := newFakeSCGI(t, "Status: 200 OK\r\nContent-Type: text/xml\r\n\r\n<ok/>")
	defer fake.close()
	srv := newProxyServer(t, fake.addr())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/RPC2", strings.NewReader(`<methodCall/>`))
	// Deliberately no Content-Type header.
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	headers, _ := parseSCGI(t, <-fake.reqs)
	if headers["CONTENT_TYPE"] != "text/xml" {
		t.Errorf("forwarded CONTENT_TYPE = %q, want text/xml (default)", headers["CONTENT_TYPE"])
	}
}

func TestRPCProxyRejectsNonPost(t *testing.T) {
	fake := newFakeSCGI(t, "Status: 200 OK\r\n\r\nx")
	defer fake.close()
	srv := newProxyServer(t, fake.addr())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/RPC2", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /RPC2 status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "POST" {
		t.Errorf("Allow header = %q, want POST", allow)
	}
}

func TestRPCProxyDisabledByDefault(t *testing.T) {
	// Without EnableRPCProxy, /RPC2 must not be a live endpoint (falls through to
	// the SPA catch-all, never a 200 XML proxy response).
	srv := New(sse.NewHub(), rpc.New(scgi.New("tcp://127.0.0.1:1", 1, time.Second, time.Second)), "main")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/RPC2", strings.NewReader(`<methodCall/>`))
	req.Header.Set("Content-Type", "text/xml")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && strings.HasPrefix(rec.Header().Get("Content-Type"), "text/xml") {
		t.Errorf("/RPC2 answered as a proxy while disabled (code %d, ct %q)", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestEnableRPCProxyPathFallback(t *testing.T) {
	// Degenerate, reserved, and subtree-forming paths all fall back to the /RPC2
	// default. Reserved (/healthz, /api/*) must never host the unfiltered proxy;
	// a trailing slash would make ServeMux 307-redirect the canonical path. Each
	// call needs a fresh mux — registering the same pattern twice panics.
	fallback := []string{
		"",          // empty
		"/",         // root collides with the SPA catch-all
		"RPC2",      // not rooted
		"/RPC2/",    // trailing slash -> subtree pattern
		"/RPC2///",  // multiple trailing slashes
		"/healthz",  // auth-exempt path -> would be an auth bypass
		"/api/rpc",  // shadows the JSON passthrough
		"/api/torrents",
	}
	for _, in := range fallback {
		srv := New(sse.NewHub(), rpc.New(scgi.New("tcp://127.0.0.1:1", 1, time.Second, time.Second)), "main")
		if got := srv.EnableRPCProxy(in); got != "/RPC2" {
			t.Errorf("EnableRPCProxy(%q) resolved to %q, want /RPC2", in, got)
		}
	}

	// A valid custom path is honored verbatim.
	srv := New(sse.NewHub(), rpc.New(scgi.New("tcp://127.0.0.1:1", 1, time.Second, time.Second)), "main")
	if got := srv.EnableRPCProxy("/myrpc"); got != "/myrpc" {
		t.Errorf("EnableRPCProxy(/myrpc) = %q, want /myrpc", got)
	}
}

// TestHealthzAuthExemptGETOnly locks in the fix for the proxy-on-/healthz auth
// bypass: the BasicAuth /healthz carve-out applies to GET only, so a non-GET to
// /healthz (e.g. an unfiltered proxy a misconfig mounted there) still needs auth.
func TestHealthzAuthExemptGETOnly(t *testing.T) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	h := BasicAuth("test", func(u, p string) bool { return u == "u" && p == "p" }, next)

	do := func(method string, withCreds bool) (int, bool) {
		reached = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/healthz", nil)
		if withCreds {
			req.SetBasicAuth("u", "p")
		}
		h.ServeHTTP(rec, req)
		return rec.Code, reached
	}

	if code, ok := do(http.MethodGet, false); !ok || code != http.StatusOK {
		t.Errorf("GET /healthz (no creds): code=%d reached=%v, want 200/true (exempt)", code, ok)
	}
	if code, ok := do(http.MethodPost, false); ok || code != http.StatusUnauthorized {
		t.Errorf("POST /healthz (no creds): code=%d reached=%v, want 401/false (auth required)", code, ok)
	}
	if code, ok := do(http.MethodPost, true); !ok || code != http.StatusOK {
		t.Errorf("POST /healthz (with creds): code=%d reached=%v, want 200/true", code, ok)
	}
}
