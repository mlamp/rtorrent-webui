# Contributing

Thanks for your interest! This is a single Go binary that embeds a Svelte SPA.

## Dev setup

Toolchain is pinned via [`mise`](https://mise.jdx.dev) (Go, Node, pnpm — see `mise.toml`):

```bash
mise install
mise run web-install
mise run build        # SPA -> embed -> bin/rtorrent-webui
mise run run          # serve on :8080 (uses config.example.toml)
mise run web-dev      # Vite dev server (proxies to :8080)
sh dev/up.sh          # throwaway rtorrent (TCP SCGI :5000) for local testing
```

## Before opening a PR

```bash
mise run test         # Go unit tests
mise run test-race    # race detector (CGO) — run if you touch concurrency
cd web && pnpm run check && pnpm run test:unit   # svelte-check + tsc + web unit tests
gofmt -l .            # must be empty
```

CI runs all of the above plus `govulncheck`. Please keep `gofmt` clean, add tests for new
behavior, and update the README/`config.example.toml` when you add a flag or config key.

## Commits & licensing

By contributing you agree your changes are licensed under the project's
[Apache-2.0](LICENSE). Keep commit messages focused; no co-author trailers.
