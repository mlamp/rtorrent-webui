#!/bin/sh
# Start a throwaway rtorrent (patched image) with TCP SCGI on 127.0.0.1:5000
# for webui development. Override the image with RT_IMAGE=...
set -e
cd "$(dirname "$0")/.."
IMAGE="${RT_IMAGE:-rtorrent-scgi:fixed}"
docker rm -f rt-dev >/dev/null 2>&1 || true
docker run -d --name rt-dev -p 127.0.0.1:5000:5000 \
  -v "$PWD/dev/rtorrent.rc:/root/.rtorrent.rc:ro" \
  --entrypoint sh "$IMAGE" \
  -c "mkdir -p /tmp/rt-session /tmp/rt-dl && exec rtorrent" >/dev/null
for _ in $(seq 1 30); do
  (echo >/dev/tcp/127.0.0.1/5000) 2>/dev/null && break
  sleep 0.3
done
echo "rt-dev up on 127.0.0.1:5000 ($IMAGE)"
