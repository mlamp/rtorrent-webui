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

// Default returns a config with sane defaults (also the base for merge).
func Default() Config {
	return Config{
		Server:   Server{Listen: ":8080"},
		Rtorrent: Rtorrent{Socket: "/var/run/rtorrent/scgi.socket", View: "main", PollInterval: Duration(time.Second), IdleInterval: Duration(30 * time.Second), MaxInflight: 8, MaxUploadMB: 12, RPCTimeout: Duration(60 * time.Second)},
		Auth:     Auth{Mode: "none", Realm: "rtorrent-webui"},
		Insight:  Insight{GeoIPDB: "/usr/share/GeoIP/dbip-country-lite.mmdb"},
		Features: Features{RPCDenylist: []string{"execute.throw", "execute.capture", "execute.nothrow", "system.shutdown"}, RPCProxyPath: "/RPC2"},
	}
}

// Load reads a TOML file over the defaults.
func Load(path string) (Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
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
