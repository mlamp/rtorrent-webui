package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// baselineDeny is every method the default denylist must block: the full
// rtorrent shell-exec family (see src/command_local.cc CMD2_EXECUTE), the lua
// code-exec entry points, and all shutdown spellings. The /api/rpc passthrough
// matches exact method names, so each variant has to be listed explicitly.
var baselineDeny = []string{
	"execute", "execute2",
	"execute.throw", "execute.throw.bg",
	"execute.nothrow", "execute.nothrow.bg",
	"execute.raw", "execute.raw.bg",
	"execute.raw_nothrow", "execute.raw_nothrow.bg",
	"execute.capture", "execute.capture_nothrow",
	"lua.execute", "lua.execute.str",
	"system.shutdown", "system.shutdown.normal", "system.shutdown.quick",
}

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The default denylist must cover every exec/shutdown variant, not just the
// four historical entries (execute2 and execute.raw alone re-open shell exec).
// Coverage is judged by MATCHING (entries may be `family*` prefixes), not by
// literal list membership.
func TestDefaultDenylistCoversExecFamily(t *testing.T) {
	deny := NewMethodSet(Default().Features.RPCDenylist)
	for _, m := range baselineDeny {
		if !deny.Matches(m) {
			t.Errorf("default rpc_denylist does not match %q", m)
		}
	}
}

// Beyond the execute.* family: every command that re-parses caller-supplied
// command strings, (re)defines methods, or writes files is an equivalent
// single-call RCE through the passthrough and must be denied by default.
// Methods the UI and legitimate API consumers rely on must stay reachable.
func TestDefaultDenylistBlocksCommandRunnerFamilies(t *testing.T) {
	deny := NewMethodSet(Default().Features.RPCDenylist)
	for _, m := range []string{
		"load.normal", "load.start", "load.start_verbose", "load.raw", "load.raw_start",
		"schedule", "schedule2", "schedule_remove", "schedule_remove2",
		"import", "try_import",
		// control-flow primitives evaluate their argument command strings, so each
		// is a nesting vector (e.g. or=execute2=,touch,/x).
		"branch", "catch", "if", "not", "and", "or", "try",
		// the multicall family runs every command-string argument against each
		// target (d_multicall -> rpc::parse_command per arg) — a single
		// d.multicall2 call with an execute2= arg is RCE regardless of the
		// execute* entries, so the whole family is blocked.
		"d.multicall2", "d.multicall.filtered", "f.multicall", "p.multicall", "t.multicall", "system.multicall",
		"method.insert", "method.insert.value", "method.set", "method.set_key",
		"method.redirect", "method.redirect.static",
		// file/log writers (and the gz / add_output / dump variants the old
		// exact-ish patterns missed). log.rpc opens a caller-supplied path with
		// O_APPEND|O_CREAT; log.xmlrpc is its alias (the daemon resolves the
		// redirect only after dispatch, so both literal spellings must be denied).
		"log.execute", "log.open_file", "log.append_file",
		"log.open_gz_file", "log.append_gz_file", "log.open_file_pid", "log.add_output",
		"log.vmmap.dump", "log.rpc", "log.xmlrpc", "file.append", "ipv4_filter.dump",
	} {
		if !deny.Matches(m) {
			t.Errorf("default rpc_denylist does not match %q (single-call exec/persistence/file-write vector)", m)
		}
	}
	for _, m := range []string{
		"d.name", "d.size_bytes", "system.listMethods",
		"throttle.global_down.max_rate", "method.get", "method.has_key",
		"system.api_version", "log.messages",
		// scheduler.* are config/status, NOT the exec-bearing schedule/schedule2;
		// the schedule pattern must not over-block them.
		"scheduler.simple.added", "scheduler.max_active",
	} {
		if deny.Matches(m) {
			t.Errorf("default rpc_denylist over-blocks %q", m)
		}
	}
}

// MethodSet semantics: exact names match exactly; entries ending in '*' match
// the prefix; nothing else matches.
func TestMethodSetMatching(t *testing.T) {
	ms := NewMethodSet([]string{"execute*", "system.shutdown", "load*"})
	for m, want := range map[string]bool{
		"execute":                true,
		"execute2":               true,
		"execute.throw.bg":       true,
		"exec":                   false,
		"system.shutdown":        true,
		"system.shutdown.normal": false, // exact entry, not a prefix
		"load.start":             true,
		"loaded.thing":           true, // prefix is plain, not segment-aware — by design
		"d.name":                 false,
	} {
		if got := ms.Matches(m); got != want {
			t.Errorf("Matches(%q) = %v, want %v", m, got, want)
		}
	}
	if !NewMethodSet(nil).Empty() || NewMethodSet([]string{"x"}).Empty() {
		t.Error("Empty() wrong")
	}
}

// A user-supplied rpc_denylist must ADD to the secure defaults; it must never
// silently replace them (TOML decodes slices by replacement, not merge).
func TestLoadMergesUserDenylistOntoDefaults(t *testing.T) {
	path := writeTempConfig(t, "[features]\nrpc_denylist = [\"foo.bar\"]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	deny := cfg.Features.RPCDenylist
	if !slices.Contains(deny, "foo.bar") {
		t.Errorf("user entry foo.bar missing from merged denylist %v", deny)
	}
	ms := NewMethodSet(deny)
	for _, m := range baselineDeny {
		if !ms.Matches(m) {
			t.Errorf("merged rpc_denylist no longer matches default-blocked %q (got %v)", m, deny)
		}
	}
}

// Re-listing a default in the config must not produce duplicate entries.
func TestLoadDenylistNoDuplicates(t *testing.T) {
	path := writeTempConfig(t, "[features]\nrpc_denylist = [\"execute2\", \"foo.bar\"]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range cfg.Features.RPCDenylist {
		if seen[m] {
			t.Errorf("duplicate denylist entry %q", m)
		}
		seen[m] = true
	}
}

// A config that never mentions rpc_denylist keeps the full baseline.
func TestLoadWithoutDenylistKeepsDefaults(t *testing.T) {
	path := writeTempConfig(t, "[server]\nlisten = \":9090\"\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ms := NewMethodSet(cfg.Features.RPCDenylist)
	for _, m := range baselineDeny {
		if !ms.Matches(m) {
			t.Errorf("rpc_denylist does not match default-blocked %q", m)
		}
	}
	if cfg.Server.Listen != ":9090" {
		t.Errorf("scalar merge broken: listen = %q", cfg.Server.Listen)
	}
}
