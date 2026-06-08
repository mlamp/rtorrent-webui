# rtorrent-webui — single static Go binary embedding the Svelte SPA.

# 1) Build the SPA (once, on the native build platform — output is arch-independent)
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
RUN npm install -g pnpm@11
WORKDIR /app/web
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build   # -> /app/web/dist

# 2) Build the Go binary with the SPA embedded (native host, cross-compiled)
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags "-s -w" -o /out/rtorrent-webui ./cmd/rtorrent-webui

# 3) Minimal runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/rtorrent-webui /usr/local/bin/rtorrent-webui
COPY config.example.toml /etc/rtorrent-webui/config.toml
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/rtorrent-webui", "-config", "/etc/rtorrent-webui/config.toml"]
