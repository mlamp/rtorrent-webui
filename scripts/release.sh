#!/usr/bin/env bash
# Release rtorrent-webui under CalVer  YYYY.0M.MICRO
# (e.g. 2026.06.0). MICRO is the Nth release within the calendar month, from 0.
#
# Git tags are the SOURCE OF TRUTH for versions. The version is baked into the
# binary (-X .../internal/api.Version) AND used for the image tag, so the image
# tag, the git tag, and /api/version always agree — no more guessing.
#
# Usage:
#   scripts/release.sh                 # auto: next micro for this month
#   scripts/release.sh 2026.06.3       # explicit override
#   RTWEBUI_BUILDER=relaybuild scripts/release.sh
#
# One-time prerequisite for multi-arch (arm64) on an amd64 host:
#   docker run --privileged --rm tonistiigi/binfmt --install arm64
set -euo pipefail
cd "$(dirname "$0")/.."

IMAGE="ghcr.io/mlamp/rtorrent-webui"
BUILDER="${RTWEBUI_BUILDER:-rtwebui-release}"
PLATFORMS="${RTWEBUI_PLATFORMS:-linux/amd64,linux/arm64}"

# ── version ──────────────────────────────────────────────────────────────────
if [[ "${1:-}" =~ ^[0-9]{4}\.[0-9]{2}\.[0-9]+$ ]]; then
  VERSION="$1"
else
  YM=$(date +%Y.%m)
  LAST=$(git tag --list "${YM}.*" | sed -E "s/^${YM}\\.//" | sort -n | tail -1)
  VERSION="${YM}.$(( ${LAST:--1} + 1 ))"
fi
echo "▶ releasing ${IMAGE}:${VERSION}"

# ── guards ───────────────────────────────────────────────────────────────────
git rev-parse "${VERSION}" >/dev/null 2>&1 && { echo "✗ git tag ${VERSION} already exists" >&2; exit 1; }
[[ -z "$(git status --porcelain)" ]] || { echo "✗ working tree dirty — commit first" >&2; exit 1; }

# ── builder (multi-arch) ─────────────────────────────────────────────────────
docker buildx inspect "${BUILDER}" >/dev/null 2>&1 \
  || docker buildx create --name "${BUILDER}" --driver docker-container --bootstrap

# ── build + push ─────────────────────────────────────────────────────────────
docker buildx build --builder "${BUILDER}" --platform "${PLATFORMS}" \
  --build-arg VERSION="${VERSION}" \
  -t "${IMAGE}:${VERSION}" -t "${IMAGE}:latest" \
  --push .

# ── tag the released commit ──────────────────────────────────────────────────
git tag -a "${VERSION}" -m "rtorrent-webui ${VERSION}"
echo "✓ pushed ${IMAGE}:{${VERSION},latest}; created git tag ${VERSION}"
echo "  if a remote is configured: git push origin ${VERSION}"
