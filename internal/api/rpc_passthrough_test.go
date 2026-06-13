package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/mlamp/rtorrent-webui/internal/config"
)

// With the DEFAULT denylist (deny-only, no allowlist), the multicall family and
// control-flow primitives must be rejected: each runs its command-string args as
// commands against rtorrent, so one call smuggles arbitrary exec past the
// execute* entries.
func TestPassthroughDefaultBlocksNestedRunners(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()
	srv := newActionServer(t, fake.addr())
	srv.EnablePassthrough(nil, config.Default().Features.RPCDenylist)

	for _, m := range []string{"d.multicall2", "system.multicall", "f.multicall", "if", "or", "and", "not", "try"} {
		rec := postJSON(t, srv, "/api/rpc", `{"method":"`+m+`","params":[""]}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403 (body %s)", m, rec.Code, rec.Body.String())
		}
		select {
		case raw := <-fake.reqs:
			_, params := capturedCall(t, raw)
			t.Fatalf("%s reached rtorrent: %q", m, params)
		case <-time.After(50 * time.Millisecond):
		}
	}
	// Plain getters stay reachable.
	if rec := postJSON(t, srv, "/api/rpc", `{"method":"d.name","params":["HASH"]}`); rec.Code != http.StatusOK {
		t.Errorf("d.name: status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}

// Denylist entries ending in '*' must block the whole method family — the
// daemon grows execute/load/method variants faster than an exact list can
// chase, and one missed variant re-opens single-call RCE.
func TestPassthroughDeniesPrefixFamilies(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()
	srv := newActionServer(t, fake.addr())
	srv.EnablePassthrough(nil, []string{"execute*", "load*", "system.shutdown*"})

	for _, m := range []string{"execute2", "execute.raw.bg", "load.start", "load.raw_start", "system.shutdown.normal"} {
		rec := postJSON(t, srv, "/api/rpc", `{"method":"`+m+`","params":[""]}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403 (body %s)", m, rec.Code, rec.Body.String())
		}
		select {
		case raw := <-fake.reqs:
			_, params := capturedCall(t, raw)
			t.Fatalf("%s reached rtorrent: %q", m, params)
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Harmless reads must still flow.
	rec := postJSON(t, srv, "/api/rpc", `{"method":"system.api_version","params":[""]}`)
	if rec.Code != http.StatusOK {
		t.Errorf("system.api_version: status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}

// Allowlist entries support the same '*' prefix form; with an allowlist set,
// everything else is rejected.
func TestPassthroughAllowlistPrefixes(t *testing.T) {
	fake := newFakeSCGI(t, loadOKResp)
	defer fake.close()
	srv := newActionServer(t, fake.addr())
	srv.EnablePassthrough([]string{"d.*", "system.api_version"}, nil)

	if rec := postJSON(t, srv, "/api/rpc", `{"method":"d.name","params":["HASH"]}`); rec.Code != http.StatusOK {
		t.Errorf("d.name: status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if rec := postJSON(t, srv, "/api/rpc", `{"method":"system.pid","params":[""]}`); rec.Code != http.StatusForbidden {
		t.Errorf("system.pid: status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
}
