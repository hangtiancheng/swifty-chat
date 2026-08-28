# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Stage 1: build the React frontend.
# VITE_API_URL is baked in at build time (Vite inlines it); pass a different
# value when the backend is reachable at another origin:
#   docker compose build --build-arg VITE_API_URL=https://chat.example.com:8000
# ---------------------------------------------------------------------------
FROM node:24-alpine AS web-builder
WORKDIR /app

RUN npm install -g pnpm@10

COPY package.json pnpm-lock.yaml .npmrc ./
RUN pnpm install --frozen-lockfile

COPY . .

ARG VITE_API_URL=http://localhost:8000
ARG VITE_WS_URL=
ENV VITE_API_URL=${VITE_API_URL} VITE_WS_URL=${VITE_WS_URL}
RUN pnpm build

# ---------------------------------------------------------------------------
# Stage 2: build the Go chat server (static binary, no CGO).
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS server-builder
WORKDIR /src

# goproxy.cn mirrors the module registry, matching the npmmirror registry in
# .npmrc; override with --build-arg GOPROXY=https://proxy.golang.org,direct.
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/swifty-chat-server ./cmd

# ---------------------------------------------------------------------------
# Runtime target "server": the API + websocket + Swiftx agent host.
# bash/git/curl are for the agent's Bash tool, which runs shell commands
# inside per-user workspaces under /app/.swiftx/chat.
# ---------------------------------------------------------------------------
FROM alpine:3.21 AS server
RUN apk add --no-cache bash git curl ca-certificates tzdata
WORKDIR /app

COPY --from=server-builder /out/swifty-chat-server /usr/local/bin/swifty-chat-server

# config.json (chat server) is bind-mounted by compose; the Swiftx provider
# config is read from $HOME/.swiftx/config.yaml, also bind-mounted.
EXPOSE 8000
CMD ["swifty-chat-server"]

# ---------------------------------------------------------------------------
# Runtime target "web": nginx serving the built SPA.
# ---------------------------------------------------------------------------
FROM nginx:1.27-alpine AS web
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=web-builder /app/dist /usr/share/nginx/html
EXPOSE 80
