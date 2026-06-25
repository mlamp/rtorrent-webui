# rtorrent-webui — single static Go binary embedding the Svelte SPA.

# 0) GeoIP: DB-IP Lite Country (CC BY 4.0, no license key) — bundled so peer
#    country flags work out of the box. Bump DBIP_DATE to refresh.
#    NOTE: DB-IP purges old monthly files, so rebuilding an *older* image tag can
#    404 once its DBIP_DATE rolls off. For a reproducible old-tag rebuild, pass a
#    still-available DBIP_DATE, or vendor the .mmdb and COPY it instead of curling.
FROM --platform=$BUILDPLATFORM alpine:3.24 AS geoip
ARG DBIP_DATE=2026-06
RUN apk add --no-cache curl \
 && curl -fSL "https://download.db-ip.com/free/dbip-country-lite-${DBIP_DATE}.mmdb.gz" -o /tmp/db.gz \
 && gunzip /tmp/db.gz \
 && mv /tmp/db /dbip-country-lite.mmdb

# 1) Build the SPA (once, on the native build platform — output is arch-independent)
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
RUN npm install -g pnpm@11
WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build   # -> /app/web/dist
# Stage the bundled font's OFL-1.1 text at a stable path (node_modules/@fontsource is a pnpm symlink).
RUN cp "$(readlink -f node_modules/@fontsource/jetbrains-mono/LICENSE)" /OFL-1.1.txt

# 2) Build the Go binary with the SPA embedded (native host, cross-compiled)
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w -X github.com/mlamp/rtorrent-webui/internal/api.Version=${VERSION}" \
    -o /out/rtorrent-webui ./cmd/rtorrent-webui

# 3) Minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/mlamp/rtorrent-webui" \
      org.opencontainers.image.licenses="Apache-2.0"
COPY --from=build /out/rtorrent-webui /usr/local/bin/rtorrent-webui
COPY --from=geoip /dbip-country-lite.mmdb /usr/share/GeoIP/dbip-country-lite.mmdb
COPY config.example.toml /etc/rtorrent-webui/config.toml
# License + attribution compliance (Apache-2.0 + bundled deps incl. JetBrains Mono OFL-1.1, DB-IP CC BY 4.0).
COPY NOTICE /NOTICE
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /usr/share/licenses/rtorrent-webui/
COPY --from=web /OFL-1.1.txt /usr/share/licenses/OFL-1.1.txt
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/rtorrent-webui", "-config", "/etc/rtorrent-webui/config.toml"]
