package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/rpc"
	"github.com/mlamp/rtorrent-webui/internal/scgi"
	"github.com/mlamp/rtorrent-webui/internal/sse"
)

// loadOKResp is a canned JSON-RPC success for load.normal / load.start.
const loadOKResp = "Status: 200 OK\r\nContent-Type: application/json\r\n\r\n" +
	`{"jsonrpc":"2.0","id":1,"result":0}`

const testMagnet = "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func newActionServer(t *testing.T, scgiAddr string) *Server {
	t.Helper()
	return New(sse.NewHub(), rpc.New(scgi.New(scgiAddr, 4, 2*time.Second, 2*time.Second)), "main")
}

// capturedCall decodes the JSON-RPC call inside a captured SCGI frame and
// returns its method plus all string params.
func capturedCall(t *testing.T, raw []byte) (method string, params []string) {
	t.Helper()
	_, body := parseSCGI(t, raw)
	var req struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode forwarded rpc body: %v (%.200q)", err, body)
	}
	for _, p := range req.Params {
		var s string
		if err := json.Unmarshal(p, &s); err != nil {
			t.Fatalf("param %q is not a string", p)
		}
		params = append(params, s)
	}
	return req.Method, params
}

func postJSON(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error body: %v (%q)", err, rec.Body.String())
	}
	return resp.Error.Code
}

// Label/directory values ride to rtorrent inside load.* command strings that the
// daemon re-parses with its own grammar (',' splits args, ';' splits commands).
// They must arrive as quoted string literals so benign punctuation survives.
func TestAddQuotesLabelAndDirectory(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()
	srv := newActionServer(t, fake.addr())

	rec := postJSON(t, srv, "/api/torrents", fmt.Sprintf(
		`{"magnet":%q,"label":"tv, weekly","directory":"/data/Films, 2024 \"HD\""}`, testMagnet))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}

	select {
	case raw := <-fake.reqs:
		method, params := capturedCall(t, raw)
		if method != "load.normal" {
			t.Errorf("method = %q, want load.normal", method)
		}
		want := []string{
			`d.custom1.set="tv, weekly"`,
			`d.directory.set="/data/Films, 2024 \"HD\""`,
		}
		got := params[2:] // params[0] is the target, params[1] the magnet
		if len(got) != len(want) {
			t.Fatalf("trailing commands = %q, want %q", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("command[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scgi server never received the request")
	}
}

// rtorrent executes any parsed argument string that begins with '$' as a command
// (command substitution) — quoting does not neutralize it. Such values must be
// rejected before anything is sent to the daemon.
func TestAddRejectsCommandSubstitutionValues(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantCode string
	}{
		{"label", fmt.Sprintf(`{"magnet":%q,"label":"$execute2=,touch,/tmp/pwned"}`, testMagnet), "invalid_label"},
		{"directory", fmt.Sprintf(`{"magnet":%q,"directory":"$execute.throw=sh"}`, testMagnet), "invalid_directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeSCGI(t, loadOKResp)
			defer fake.close()
			srv := newActionServer(t, fake.addr())

			rec := postJSON(t, srv, "/api/torrents", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			} else if code := errCode(t, rec); code != tc.wantCode {
				t.Errorf("error code = %q, want %q", code, tc.wantCode)
			}
			select {
			case raw := <-fake.reqs:
				_, params := capturedCall(t, raw)
				t.Fatalf("injection reached rtorrent: %q", params)
			case <-time.After(100 * time.Millisecond):
			}
		})
	}
}

// A daemon that is down must surface as 503 rtorrent_unreachable (the UI treats
// that as a transient "reconnecting" state), not a hard 502 rpc_error.
func TestActionsMapUnreachableTo503(t *testing.T) {
	// Nothing listens here; a short connect budget makes the dial exhaust the
	// BUDGET (returning ErrUnreachable) deterministically and fast — not racing a
	// request-context deadline, which would surface as a generic 502 instead.
	cl := scgi.New("tcp://127.0.0.1:1", 4, 50*time.Millisecond, 200*time.Millisecond)
	cl.SetConnectBudget(150 * time.Millisecond)
	srv := New(sse.NewHub(), rpc.New(cl), "main")
	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/torrents/ABCD/stop", ""},
		{http.MethodPost, "/api/torrents", fmt.Sprintf(`{"magnet":%q}`, testMagnet)},
		{http.MethodDelete, "/api/torrents/ABCD", ""},
		{http.MethodPut, "/api/torrents/ABCD/label", `{"label":"x"}`},
		{http.MethodPut, "/api/torrents/ABCD/priority", `{"priority":1}`},
		{http.MethodPut, "/api/torrents/ABCD/directory", `{"directory":"/x"}`},
		{http.MethodPut, "/api/throttle", `{"down":0,"up":0}`},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503 (body %s)", rec.Code, rec.Body.String())
			} else if code := errCode(t, rec); code != "rtorrent_unreachable" {
				t.Errorf("error code = %q, want rtorrent_unreachable", code)
			}
		})
	}
}

func postTorrentFile(t *testing.T, srv *Server, size int) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("torrent", "big.torrent")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(bytes.Repeat([]byte{0xd8}, size)); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	mw.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// The max_upload_mb config option must actually govern the upload cap.
func TestAddUploadCapConfigurable(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()

	t.Run("raised cap accepts 13MiB", func(t *testing.T) {
		srv := newActionServer(t, fake.addr())
		srv.SetMaxUploadBytes(20 << 20)
		if rec := postTorrentFile(t, srv, 13<<20); rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
		}
	})
	t.Run("tightened cap rejects 2MiB", func(t *testing.T) {
		srv := newActionServer(t, fake.addr())
		srv.SetMaxUploadBytes(1 << 20)
		if rec := postTorrentFile(t, srv, 2<<20); rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want 413 (body %s)", rec.Code, rec.Body.String())
		}
	})
	t.Run("default cap still rejects 13MiB", func(t *testing.T) {
		srv := newActionServer(t, fake.addr())
		if rec := postTorrentFile(t, srv, 13<<20); rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want 413 (body %s)", rec.Code, rec.Body.String())
		}
	})
}

// The 201 add-success response must say application/json; setting the header
// after WriteHeader leaves Go's sniffer labelling the JSON body text/plain.
func TestAddSuccessContentType(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()
	srv := newActionServer(t, fake.addr())

	rec := postJSON(t, srv, "/api/torrents", fmt.Sprintf(`{"magnet":%q}`, testMagnet))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
