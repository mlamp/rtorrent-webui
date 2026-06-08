# rtorrent-webui

A modern, fast web UI for rtorrent — a single **Go** binary that embeds a
**Svelte 5** SPA, talks **JSON-RPC over rtorrent's SCGI socket**, and streams live
state to the browser over **SSE** from one shared poll loop. A drop-in **sidecar**
replacement for the heavy nginx + PHP + ruTorrent stack.

Image: **`ghcr.io/mlamp/rtorrent-webui:latest`**

## Features

- Live torrent table (virtualized; smooth at 1000+ torrents, ~1 update/sec)
- Add (file / magnet / URL), start, stop, pause, remove, label, priority, global throttle
- Detail panel: General, Files (per-file priority), Peers (GeoIP flags), Trackers, Speed
- Insight: traffic-history graph (persisted in SQLite), disk-space, tracker-search seam
- Dark / light (Catppuccin), responsive, keyboard-friendly
- Optional HTTP Basic auth (or trust a fronting reverse proxy); allowlisted `/api/rpc` passthrough
- Optional raw `/RPC2` XML-RPC proxy — point *arr straight at the webui, drop the nginx `scgi_pass` shim
- Single static binary, no CGO; multi-arch (amd64 / arm64)

## Requirements

- An **rtorrent** instance exposing an **SCGI** endpoint:
  - unix socket (recommended): `network.scgi.open_local = /var/run/rtorrent/scgi.socket`, **or**
  - TCP: `network.scgi.open_port = 127.0.0.1:5000`
- rtorrent **≥ 0.16.10**. Use the EPIPE-fixed image
  `ghcr.io/mlamp/rtorrent-rutorrent:0.16.10-scgifix` (or v0.16.13+) so a large
  `d.multicall` over many torrents can't peg the SCGI thread.
- Docker (for the container) — that's it. The UI speaks JSON-RPC natively; no nginx
  or PHP needed.

## Quick start

### A. Sidecar next to rtorrent (recommended)

```bash
# pull
docker login ghcr.io -u <you> --password-stdin   # token with read:packages
docker pull ghcr.io/mlamp/rtorrent-webui:latest
```

Use [`docker-compose.example.yml`](docker-compose.example.yml): both containers
share a `rtorrent-socket` volume; rtorrent must open
`network.scgi.open_local = /var/run/rtorrent/scgi.socket`. Then:

```bash
docker compose up -d
# open http://<host>:8080
```

> The container runs as a **nonroot** user — the SCGI socket and any mounted dirs
> must be readable/writable by it (match uids, or set socket perms `0o666`).

### B. Standalone against an existing rtorrent (TCP SCGI)

```bash
docker run --rm -p 8080:8080 \
  ghcr.io/mlamp/rtorrent-webui:latest \
  -rtorrent 192.168.1.10:5000
```

(Flags override the baked config — handy for a quick try.)

## Configuration

The image runs with `-config /etc/rtorrent-webui/config.toml`. Mount your own over
it, or override individual settings with flags. See
[`config.example.toml`](config.example.toml). Key sections:

| Setting | Meaning |
|---|---|
| `rtorrent.socket` | `/path.sock` or `host:port` SCGI endpoint |
| `rtorrent.poll_interval` | list poll cadence (SSE delta rate), e.g. `1s` |
| `auth.mode` | `none` (trust a reverse proxy) or `basic` |
| `auth.users` / `auth.htpasswd_file` | bcrypt credentials for basic auth |
| `downloads.dirs` | dirs for the disk-space widget |
| `insight.history_db` | SQLite path for traffic history (e.g. `/data/history.db`); stores cumulative counters, derives rates, rolls up raw 15m → 1m 24h → 1h 7d → 1d 1y |
| `insight.geoip_db` | MaxMind/DB-IP `.mmdb` for peer country flags (optional) |
| `features.rpc_passthrough` | enable `POST /api/rpc` (allow/deny lists) |
| `features.rpc_proxy` | enable the raw `/RPC2` XML-RPC proxy for *arr (unfiltered; inherits `[auth]`) |

### Auth

```bash
# generate a bcrypt hash for a password:
docker run --rm --entrypoint /usr/local/bin/rtorrent-webui ... # (or build genhash)
```

In `config.toml`:

```toml
[auth]
mode = "basic"
[[auth.users]]
  name = "admin"
  password_hash = "$2y$12$…"   # from `genhash <password>`
```

Most homelab setups instead front the UI with a reverse proxy (Authelia, Cloudflare
Access, etc.) and leave `auth.mode = "none"`.

### GeoIP peer flags

Works out of the box: the image bundles the **DB-IP Lite Country** database
(CC BY 4.0, no license key) at `/usr/share/GeoIP/dbip-country-lite.mmdb`. To use
your own MaxMind **GeoLite2-Country.mmdb** instead, mount it and set
`insight.geoip_db`. Set it to `""` to disable.

> IP geolocation data by [DB-IP](https://db-ip.com) ([CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)). See [`NOTICE`](NOTICE).

## API (for scripting)

`GET /api/torrents`, `GET /api/events` (SSE snapshot+deltas),
`GET /api/torrents/{hash}/{files,peers,trackers}`, `GET /api/{stats,diskspace,history}`,
action endpoints under `/api/torrents/{hash}/…`, and (if enabled) `POST /api/rpc`.

### `/RPC2` proxy (drop the nginx shim for *arr)

The typical rtorrent stack puts an nginx `scgi_pass` location at `/RPC2` so
Sonarr/Radarr/Lidarr can drive rtorrent over XML-RPC. Set
`features.rpc_proxy = true` and the webui serves that itself — a transparent
byte-pipe to the same SCGI socket — so you can delete the nginx container and just
repoint the *arr clients:

```
URL Base:  (blank)
Host/Port: <webui-host> : 8080
Path:      /RPC2
```

It forwards the request body verbatim (XML-RPC, or JSON-RPC if the client sends
`application/json`) and echoes rtorrent's response.

> **This is full, unfiltered control of rtorrent** (including `execute.*`), so it
> follows the same logic as the nginx setup it replaces: it inherits the `[auth]`
> section. With `auth.mode = "basic"`, *arr clients authenticate exactly as they
> would against an htpasswd-protected nginx — put the credentials in the field or
> use `http://user:pass@host:8080/RPC2`. With `auth.mode = "none"`, the endpoint
> is open, so keep it on an internal Docker network only. Unlike `/api/rpc`, there
> is **no** method allow/deny filter here — treat it as root-equivalent.

## Development

Pinned toolchain via `mise` (Go 1.26, Node 24 LTS, pnpm 11, Vite 8, Svelte 5,
TypeScript 6, Tailwind v4):

```bash
mise install
mise run web-install
mise run build        # SPA -> embed -> bin/rtorrent-webui
mise run run          # serve on :8080
mise run web-dev      # Vite dev server (proxies /api,/events,/rpc -> :8080)
mise run screenshot   # Playwright light/dark/mobile -> web/e2e/screenshots
sh dev/up.sh          # throwaway rtorrent (TCP SCGI :5000) for local testing
```

Build the image: `docker build -t rtorrent-webui .`
