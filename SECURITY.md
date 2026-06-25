# Security Policy

## Reporting a vulnerability

Please report security issues privately via GitHub **Private Vulnerability Reporting**:
the repository's **Security** tab → **Report a vulnerability**. Do not open a public issue
for a vulnerability. We aim to acknowledge within a few days.

## Supported versions

The latest CalVer release / the `:latest` image is supported. Fixes ship in a new release.

## Threat model — read before exposing this

`rtorrent-webui` is a control plane for rtorrent. Treat access to it as access to rtorrent.

- **The shipped default is `auth.mode = "none"`.** Anyone who can reach the port can add,
  start, stop, recheck, relabel, re-path, throttle, and delete torrents (and delete on-disk
  data if `delete_with_data` is enabled). **Bind the published port to loopback**
  (`-p 127.0.0.1:8080:8080`), front it with an authenticating reverse proxy, or set
  `auth.mode = "basic"` before exposing it.
- **`/RPC2` (when enabled) is unfiltered, root-equivalent control of rtorrent** (including
  `execute.*`). It inherits `[auth]` but nothing else — never expose it without auth or a
  trusted network.
- **`/api/rpc` (when enabled)** enforces an un-removable RCE-equivalent denylist, but a
  denylist cannot make an arbitrary-RPC passthrough safe against an untrusted caller (the
  multicall/control-flow families nest execution). For untrusted exposure use
  `rpc_allowlist` (deny-by-default) and do not allowlist multicall/control-flow.
- **Basic auth does not, by itself, stop CSRF** (a browser replays cached credentials).
  Same-origin CSRF protection is therefore **always on**: state-changing requests carrying a
  cross-origin `Origin`/`Referer` are rejected.
- **DNS rebinding:** a same-origin check cannot stop a rebound domain. Set
  `server.allowed_hosts` (or front with a proxy that pins `Host`) when exposing beyond
  loopback without auth.
- **Download path confinement:** `directory` on add / `PUT .../directory` accepts arbitrary
  filesystem paths the daemon can write to (only a leading `$` is rejected). Treat operators
  with write access to these endpoints as trusted.

## Controls in place

- Distroless image, runs as nonroot uid 65532; no shell, no package manager, no baked
  credentials.
- Passwords hashed with bcrypt (cost 12); `-genhash` helper.
- Always-on same-origin CSRF guard; optional `server.allowed_hosts` (DNS-rebinding defense).
- `GET /healthz` is the only auth-exempt route — scoped to GET so nothing mounted there can
  ride the health-check's open door.
- HTTP server timeouts (`ReadHeaderTimeout`/`ReadTimeout`/`IdleTimeout`) bound slow-client
  attacks; SSE is exempted from write deadlines.
- On-disk data deletion is off by default and uses symlink-safe `os.RemoveAll`.
- CI runs `govulncheck`.
