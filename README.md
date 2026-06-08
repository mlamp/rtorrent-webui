# rtorrent-webui

A modern, fast web UI for rtorrent — a single **Go** binary that embeds a
**Svelte 5** SPA, talks **JSON-RPC over rtorrent's SCGI unix socket**, and pushes
live state to the browser over **SSE** from one shared poller. Designed as a
drop-in **sidecar** replacement for the heavy nginx + PHP + ruTorrent stack.

> Status: under construction. See the plan/milestones. M-setup (tooling + themed
> shell + Playwright visual loop) is done.

## Stack (pinned via `mise.toml`)

- Go 1.26 · Node 24 LTS · pnpm 11
- Vite 8 (Rolldown) · Svelte 5 (runes) · TypeScript 6 · Tailwind v4
- shadcn-svelte (Bits UI) · Catppuccin Latte/Mocha theme · dark/light (mode-watcher)
- Playwright (visual screenshots + E2E) · Vitest (units)

## Develop

```bash
mise install           # install pinned toolchain
mise run web-install   # pnpm install (frontend deps)
mise run build         # build SPA → embed → Go binary at bin/rtorrent-webui
mise run run           # run the server (serves SPA + API on :8080)
mise run web-dev       # Vite dev server (proxies /api,/events,/rpc → :8080)
mise run screenshot    # Playwright: capture light/dark/mobile to web/e2e/screenshots
mise run test          # go test
```

Build order matters: the Go binary embeds `web/dist`, so the SPA is built first
(the `build` task handles this via a dependency).

## Layout

```
cmd/rtorrent-webui/    main (server entrypoint)
internal/              scgi · rpc · model · poll · sse · api · insight · config (added per milestone)
web/                   Svelte 5 + Vite SPA (built to web/dist, embedded by Go)
web/e2e/               Playwright specs + screenshots
```
