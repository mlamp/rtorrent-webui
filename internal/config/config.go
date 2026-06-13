// Package config loads the rtorrent-webui TOML configuration.
package config

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

// Duration wraps time.Duration so TOML can use strings like "1s" / "24h".
type Duration time.Duration

func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}
func (d Duration) D() time.Duration { return time.Duration(d) }

type Config struct {
	Server    Server    `toml:"server"`
	Rtorrent  Rtorrent  `toml:"rtorrent"`
	Auth      Auth      `toml:"auth"`
	Downloads Downloads `toml:"downloads"`
	Insight   Insight   `toml:"insight"`
	Features  Features  `toml:"features"`
}

type Server struct {
	Listen string `toml:"listen"`
	Name   string `toml:"name"` // optional instance label shown in the browser tab title + on-screen brand
}

type Rtorrent struct {
	Socket       string   `toml:"socket"`
	View         string   `toml:"view"`
	PollInterval Duration `toml:"poll_interval"`      // live cadence while a client is watching
	IdleInterval Duration `toml:"idle_poll_interval"` // background cadence for history when idle
	MaxInflight  int      `toml:"max_inflight"`
	MaxUploadMB  int      `toml:"max_upload_mb"`
	RPCTimeout   Duration `toml:"rpc_timeout"` // whole-request deadline (nginx scgi_read_timeout parity); generous so big multicalls aren't abandoned mid-flight
}

type Auth struct {
	Mode         string `toml:"mode"` // "none" | "basic"
	Realm        string `toml:"realm"`
	HtpasswdFile string `toml:"htpasswd_file"`
	Users        []User `toml:"users"`
}

type User struct {
	Name         string `toml:"name"`
	PasswordHash string `toml:"password_hash"` // bcrypt
}

type Downloads struct {
	Dirs       []string `toml:"dirs"`
	DefaultDir string   `toml:"default_dir"`
}

type Insight struct {
	GeoIPDB       string `toml:"geoip_db"`
	HistoryDB     string `toml:"history_db"`
	SearchEnabled bool   `toml:"search_enabled"`
}

type Features struct {
	RPCPassthrough bool     `toml:"rpc_passthrough"`
	RPCAllowlist   []string `toml:"rpc_allowlist"`
	RPCDenylist    []string `toml:"rpc_denylist"`
	RPCProxy       bool     `toml:"rpc_proxy"`      // raw XML-RPC byte-pipe (replaces nginx scgi_pass for *arr)
	RPCProxyPath   string   `toml:"rpc_proxy_path"` // mount point; defaults to "/RPC2"
	DeleteWithData bool     `toml:"delete_with_data"`
}

// defaultRPCDenylist is the default safety baseline for the /api/rpc
// passthrough (entries ending in '*' deny the whole family — see MethodSet).
// It blocks every known command class that executes caller-supplied commands
// or code, (re)defines methods, runs/writes files, or stops the daemon.
//
// This is BEST-EFFORT foot-gun protection, NOT a security boundary against an
// untrusted caller. rtorrent's command language nests execution: the multicall
// family and the control-flow primitives evaluate their string arguments as
// commands, so a single allowed call can smuggle exec (e.g. d.multicall2 with
// an "execute2=…" command string). A denylist therefore cannot make the
// passthrough safe in general — these are RCE-EQUIVALENT and are denied here,
// but for untrusted exposure use rpc_allowlist (deny-by-default) and do NOT
// allowlist multicall/control-flow. The /RPC2 proxy (rpc_proxy) is a deliberate
// unfiltered byte-pipe for *arr clients — enabling it grants full control.
//
// User rpc_denylist entries are merged on top in Load and can never remove
// these. Torrent adds belong on POST /api/torrents, which validates and escapes
// label/directory; raw load.* through the passthrough would accept arbitrary
// trailing commands, so the family is denied.
func defaultRPCDenylist() []string {
	return []string{
		// command/code execution
		"execute*", "lua.execute*",
		// re-parse and run caller-supplied command strings
		"load*", // trailing load.* params are executed commands
		// scheduler entries whose command-string arg is executed (NOT the
		// scheduler.* config/status getters)
		"schedule", "schedule2", "schedule.remove", "schedule_remove", "schedule_remove2",
		"import", "try_import", // run command files from disk
		// control-flow primitives evaluate their argument command strings
		"branch", "catch", "if", "not", "and", "or", "try",
		// multicall runs each command-string arg against every target — RCE
		// equivalent (d_multicall -> rpc::parse_command per arg)
		"d.multicall*", "f.multicall*", "p.multicall*", "t.multicall*", "system.multicall*",
		// method (re)definition — turns later innocuous-looking calls into exec
		"method.insert*", "method.set", "method.set_key", "method.redirect*",
		// log/file writers and log-driven execution (open*/append* cover the
		// gz/pid/.flush variants; plus the dump and direct file-append commands).
		// log.rpc opens a caller-supplied path (O_APPEND|O_CREAT); log.xmlrpc is
		// its alias — the daemon resolves the redirect only after dispatch, so the
		// passthrough must deny both literal spellings.
		"log.execute", "log.open*", "log.append*", "log.add_output", "log.vmmap.dump",
		"log.rpc", "log.xmlrpc", "file.append", "ipv4_filter.dump",
		// daemon shutdown
		"system.shutdown*",
	}
}

// Default returns a config with sane defaults (also the base for merge).
func Default() Config {
	return Config{
		Server:   Server{Listen: ":8080"},
		Rtorrent: Rtorrent{Socket: "/var/run/rtorrent/scgi.socket", View: "main", PollInterval: Duration(time.Second), IdleInterval: Duration(30 * time.Second), MaxInflight: 8, MaxUploadMB: 12, RPCTimeout: Duration(60 * time.Second)},
		Auth:     Auth{Mode: "none", Realm: "rtorrent-webui"},
		Insight:  Insight{GeoIPDB: "/usr/share/GeoIP/dbip-country-lite.mmdb"},
		Features: Features{RPCDenylist: defaultRPCDenylist(), RPCProxyPath: "/RPC2"},
	}
}

// Load reads a TOML file over the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	// TOML replaces slices instead of merging them, so a user rpc_denylist
	// would silently drop the security baseline; re-merge so entries only add.
	cfg.Features.RPCDenylist = mergeDenylist(defaultRPCDenylist(), cfg.Features.RPCDenylist)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// mergeDenylist returns base plus any extra entries not already present.
func mergeDenylist(base, extra []string) []string {
	seen := make(map[string]bool, len(base))
	out := make([]string, 0, len(base)+len(extra))
	for _, m := range base {
		seen[m] = true
		out = append(out, m)
	}
	for _, m := range extra {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func (c Config) Validate() error {
	switch c.Auth.Mode {
	case "", "none", "basic":
	default:
		return fmt.Errorf("auth.mode must be none or basic, got %q", c.Auth.Mode)
	}
	if c.Rtorrent.Socket == "" {
		return fmt.Errorf("rtorrent.socket is required")
	}
	return nil
}
