package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// ── erase + optional on-disk data deletion ──────────────────────────────────

// recDeleter is a fake FileDeleter: it records every path it is asked to delete
// (so a test can assert the exact, resolved path) and returns a canned error,
// without touching the real filesystem.
type recDeleter struct {
	mu     sync.Mutex
	called []string
	err    error
}

func (d *recDeleter) RemoveAll(_ context.Context, path string) error {
	d.mu.Lock()
	d.called = append(d.called, path)
	d.mu.Unlock()
	return d.err
}

func (d *recDeleter) paths() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.called...)
}

// eraseDataResp is a canned JSON-RPC reply whose string result is `path`. It
// serves BOTH calls in the data-delete flow: d.base_path decodes the string;
// d.erase (action) discards its result, so any valid success body works.
func eraseDataResp(path string) string {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": path})
	return "Status: 200 OK\r\nContent-Type: application/json\r\n\r\n" + string(body)
}

func newRemoveServer(t *testing.T, scgiAddr string, roots []string, enabled bool, del FileDeleter) *Server {
	t.Helper()
	srv := newActionServer(t, scgiAddr)
	srv.SetDirs(roots)
	if enabled {
		srv.SetDeleteWithData(true)
	}
	if del != nil {
		srv.SetFileDeleter(del)
	}
	return srv
}

func deleteReq(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, path, nil))
	return rec
}

func eraseData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode data: %v (%q)", err, rec.Body.String())
	}
	return resp.Data
}

// Default DELETE (no ?data) erases the session entry only and reports that the
// data was KEPT — the response must carry an explicit dataDeleted:false so the
// UI can tell the user truthfully.
func TestEraseKeepsDataByDefault(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp) // result:0 — a real d.erase OK
	defer fake.close()
	del := &recDeleter{}
	srv := newRemoveServer(t, fake.addr(), []string{"/data"}, true, del)

	rec := deleteReq(t, srv, "/api/torrents/ABCD")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	data := eraseData(t, rec)
	if data["erased"] != true {
		t.Errorf("erased = %v, want true", data["erased"])
	}
	if _, ok := data["dataDeleted"]; !ok {
		t.Errorf("response is missing the dataDeleted field: %v", data)
	}
	if data["dataDeleted"] != false {
		t.Errorf("dataDeleted = %v, want false", data["dataDeleted"])
	}
	if got := del.paths(); len(got) != 0 {
		t.Errorf("deleter was called %v, want never", got)
	}
}

// ?data=true is refused with 403 unless the operator opted in.
func TestEraseDataDisabledByDefault(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()
	del := &recDeleter{}
	srv := newRemoveServer(t, fake.addr(), []string{"/data"}, false, del) // enabled=false

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
	if code := errCode(t, rec); code != "data_delete_disabled" {
		t.Errorf("code = %q, want data_delete_disabled", code)
	}
	if got := del.paths(); len(got) != 0 {
		t.Errorf("deleter called %v despite disabled config", got)
	}
}

// Even with the feature enabled, -mock mode must refuse data deletion (no real
// daemon/disk to act on) rather than unlink something under the mock dirs.
func TestEraseDataDisabledInMockMode(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()
	del := &recDeleter{}
	srv := newRemoveServer(t, fake.addr(), []string{"/data"}, true, del)
	srv.SetMockMode(true)

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
	if code := errCode(t, rec); code != "data_delete_disabled" {
		t.Errorf("code = %q, want data_delete_disabled", code)
	}
	if got := del.paths(); len(got) != 0 {
		t.Errorf("deleter called %v in mock mode", got)
	}
}

// The happy path: validate the daemon's base_path, resolve symlinks, erase, then
// unlink exactly the resolved (real) path.
func TestEraseWithDataValidatesAndDeletes(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "ubuntu")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	wantResolved, err := filepath.EvalSymlinks(child)
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeSCGI(t, eraseDataResp(child))
	defer fake.close()
	del := &recDeleter{}
	srv := newRemoveServer(t, fake.addr(), []string{root}, true, del)

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if data := eraseData(t, rec); data["dataDeleted"] != true {
		t.Errorf("dataDeleted = %v, want true", data["dataDeleted"])
	}
	if got := del.paths(); len(got) != 1 || got[0] != wantResolved {
		t.Errorf("deleted %v, want [%q]", got, wantResolved)
	}
}

// The default download dir is an allowed deletion root too (not just the
// explicit dirs list): a torrent stored only under default_dir must delete.
func TestEraseWithDataDefaultDirIsARoot(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "season")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	wantResolved, err := filepath.EvalSymlinks(child)
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeSCGI(t, eraseDataResp(child))
	defer fake.close()
	del := &recDeleter{}
	srv := newActionServer(t, fake.addr())
	srv.SetDeleteWithData(true)
	srv.SetDefaultDir(root) // root supplied ONLY via default_dir, with no SetDirs entry
	srv.SetFileDeleter(del)

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if data := eraseData(t, rec); data["dataDeleted"] != true {
		t.Errorf("dataDeleted = %v, want true", data["dataDeleted"])
	}
	if got := del.paths(); len(got) != 1 || got[0] != wantResolved {
		t.Errorf("deleted %v, want [%q]", got, wantResolved)
	}
}

// A contained-but-already-missing base path erases cleanly and reports nothing
// deleted — never an error, never a deleter call.
func TestEraseWithDataAlreadyGone(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "ghost") // never created on disk
	fake := newFakeSCGI(t, eraseDataResp(child))
	defer fake.close()
	del := &recDeleter{}
	srv := newRemoveServer(t, fake.addr(), []string{root}, true, del)

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if data := eraseData(t, rec); data["dataDeleted"] != false {
		t.Errorf("dataDeleted = %v, want false", data["dataDeleted"])
	}
	if got := del.paths(); len(got) != 0 {
		t.Errorf("deleter called %v for an already-gone path", got)
	}
}

// A daemon path outside the configured roots is an integrity fault: refuse to
// delete AND do not erase (502), never unlink.
func TestEraseWithDataRejectsUntrustedPath(t *testing.T) {
	root := t.TempDir()
	fake := newFakeSCGI(t, eraseDataResp("/etc")) // outside root
	defer fake.close()
	del := &recDeleter{}
	srv := newRemoveServer(t, fake.addr(), []string{root}, true, del)

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (%s)", rec.Code, rec.Body.String())
	}
	if code := errCode(t, rec); code != "untrusted_base_path" {
		t.Errorf("code = %q, want untrusted_base_path", code)
	}
	if got := del.paths(); len(got) != 0 {
		t.Errorf("deleter called %v for an untrusted path", got)
	}
}

// An empty base_path (magnet without metadata yet) means there is nothing on
// disk: erase, report dataDeleted:false, never call the deleter.
func TestEraseWithDataEmptyBasePath(t *testing.T) {
	root := t.TempDir()
	fake := newFakeSCGI(t, eraseDataResp(""))
	defer fake.close()
	del := &recDeleter{}
	srv := newRemoveServer(t, fake.addr(), []string{root}, true, del)

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if data := eraseData(t, rec); data["dataDeleted"] != false {
		t.Errorf("dataDeleted = %v, want false", data["dataDeleted"])
	}
	if got := del.paths(); len(got) != 0 {
		t.Errorf("deleter called %v for an empty base_path", got)
	}
}

// If the unlink fails after the erase commits, the torrent is gone but its files
// may be orphaned: surface a 502 whose body names the resolved path for recovery.
func TestEraseWithDataPartialFailure(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "movie")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	wantResolved, _ := filepath.EvalSymlinks(child)
	fake := newFakeSCGI(t, eraseDataResp(child))
	defer fake.close()
	del := &recDeleter{err: fmt.Errorf("permission denied")}
	srv := newRemoveServer(t, fake.addr(), []string{root}, true, del)

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (%s)", rec.Code, rec.Body.String())
	}
	if code := errCode(t, rec); code != "data_delete_failed" {
		t.Errorf("code = %q, want data_delete_failed", code)
	}
	if !strings.Contains(rec.Body.String(), wantResolved) {
		t.Errorf("error body %q does not name the orphaned path %q", rec.Body.String(), wantResolved)
	}
}

// A delete that exceeds the budget surfaces the same recoverable 502.
func TestEraseWithDataDeleteTimeout(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "iso")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := newFakeSCGI(t, eraseDataResp(child))
	defer fake.close()
	del := &recDeleter{err: context.DeadlineExceeded}
	srv := newRemoveServer(t, fake.addr(), []string{root}, true, del)

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (%s)", rec.Code, rec.Body.String())
	}
	if code := errCode(t, rec); code != "data_delete_failed" {
		t.Errorf("code = %q, want data_delete_failed", code)
	}
}

// The safety story rests on reading base_path WHILE the torrent still exists —
// i.e. d.base_path must precede d.erase. Pin that ordering.
func TestEraseBasePathBeforeErase(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "show")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := newOrderedSCGI(t, eraseDataResp(child))
	defer fake.close()
	srv := newRemoveServer(t, fake.addr(), []string{root}, true, &recDeleter{})

	if rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	methods := fake.methods(t)
	want := []string{"d.base_path", "d.erase"}
	if len(methods) != len(want) || methods[0] != want[0] || methods[1] != want[1] {
		t.Fatalf("call order = %v, want %v", methods, want)
	}
}

// The capability gate is stateless and per-request: concurrent ?data=true calls
// against a server with the feature OFF are all rejected, none reach the deleter.
func TestEraseBulkGateIsPerRequest(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()
	del := &recDeleter{}
	srv := newRemoveServer(t, fake.addr(), []string{"/data"}, false, del)

	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := deleteReq(t, srv, fmt.Sprintf("/api/torrents/H%d?data=true", i))
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()
	for i, c := range codes {
		if c != http.StatusForbidden {
			t.Errorf("request %d status = %d, want 403", i, c)
		}
	}
	if got := del.paths(); len(got) != 0 {
		t.Errorf("deleter called %v under a disabled gate", got)
	}
}

// A dial failure on the data-delete path maps to 503 (transient), like every
// other action, not a hard 502.
func TestEraseWithDataUnreachable(t *testing.T) {
	cl := scgi.New("tcp://127.0.0.1:1", 4, 50*time.Millisecond, 200*time.Millisecond)
	cl.SetConnectBudget(150 * time.Millisecond)
	srv := New(sse.NewHub(), rpc.New(cl), "main")
	srv.SetDeleteWithData(true)
	srv.SetDirs([]string{"/data"})

	rec := deleteReq(t, srv, "/api/torrents/ABCD?data=true")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (%s)", rec.Code, rec.Body.String())
	}
	if code := errCode(t, rec); code != "rtorrent_unreachable" {
		t.Errorf("code = %q, want rtorrent_unreachable", code)
	}
}

// osDeleter is the production FileDeleter; every handler test injects a fake, so
// this is the only coverage that the real watchdog actually unlinks a tree and
// reports success through its select.
func TestOSDeleterRemovesRealTree(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "f.bin"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "a")
	if err := (osDeleter{}).RemoveAll(context.Background(), target); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("tree still present after RemoveAll: stat err = %v", err)
	}
}

// orderedSCGI captures EVERY forwarded SCGI frame in arrival order (the shared
// newFakeSCGI keeps only the first), so a test can assert call ordering. It
// replies the same canned resp to every connection.
type orderedSCGI struct {
	ln   net.Listener
	mu   sync.Mutex
	raw  [][]byte
	resp string
}

func newOrderedSCGI(t *testing.T, resp string) *orderedSCGI {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &orderedSCGI{ln: ln, resp: resp}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				raw, _ := io.ReadAll(conn)
				f.mu.Lock()
				f.raw = append(f.raw, raw)
				f.mu.Unlock()
				_, _ = conn.Write([]byte(f.resp))
			}()
		}
	}()
	return f
}

func (f *orderedSCGI) addr() string { return "tcp://" + f.ln.Addr().String() }
func (f *orderedSCGI) close()       { f.ln.Close() }

func (f *orderedSCGI) methods(t *testing.T) []string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	ms := make([]string, 0, len(f.raw))
	for _, raw := range f.raw {
		m, _ := capturedCall(t, raw)
		ms = append(ms, m)
	}
	return ms
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
