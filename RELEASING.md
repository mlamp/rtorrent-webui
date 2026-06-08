# Releasing

`rtorrent-webui` uses **CalVer**: `YYYY.0M.MICRO`

- `YYYY.0M` — release year and zero-padded month (e.g. `2026.06`).
- `MICRO` — the Nth release *within that month*, starting at **0** (resets monthly).

Examples: `2026.06.0`, `2026.06.1`, … then `2026.07.0`.

**Git tags are the source of truth.** The version is baked into the binary at build
time (`-ldflags -X github.com/mlamp/rtorrent-webui/internal/api.Version=<v>`, via the
Dockerfile `VERSION` arg) and used for the image tag, so the **git tag, image tag, and
`/api/version` always agree**. Never hand-pick a version — the release script derives it.

> History note: tags `0.1.0…0.2.6` were the pre-CalVer semver line. CalVer (`2026.06.x`)
> supersedes it; `2026.x` sorts above `0.x`, and `:latest` tracks the newest CalVer build.

## Cut a release

```sh
mise run release          # or: scripts/release.sh
```

This:
1. computes the next `YYYY.0M.MICRO` from existing git tags,
2. refuses to run on a dirty tree or an existing tag,
3. builds **multi-arch** (`linux/amd64,linux/arm64`) with the version baked in,
4. pushes `ghcr.io/mlamp/rtorrent-webui:<version>` **and** `:latest`,
5. creates the annotated git tag `<version>`.

Override the version explicitly if needed: `scripts/release.sh 2026.06.4`.

If/when a git remote is configured, push the tag too: `git push origin <version>`.

## One-time host setup (multi-arch on an amd64 box)

```sh
docker run --privileged --rm tonistiigi/binfmt --install arm64   # arm64 emulation
```

The script creates its own buildx builder (`rtwebui-release`, docker-container driver)
on first run. Set `RTWEBUI_BUILDER` to reuse an existing one, or `RTWEBUI_PLATFORMS` to
change targets (e.g. `linux/amd64` only).

## Check the current version

```sh
mise run version          # git describe --tags --always --dirty
```
