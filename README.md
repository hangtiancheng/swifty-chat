<div align="center">

# Swifty Chat

**A self-hosted, real-time chat platform — direct & group messaging, audio/video
calls, resumable file transfer, and a private AI coding agent per user.**

Built on a React 19 + Vite frontend and a Go backend on top of the
[`swifty.go`](https://github.com/hangtiancheng/swifty.go) stack.

![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=white)
![TypeScript](https://img.shields.io/badge/TypeScript-6.0-3178C6?logo=typescript&logoColor=white)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![MongoDB](https://img.shields.io/badge/MongoDB-7-47A248?logo=mongodb&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-yellow)

</div>

---

## Overview

Swifty Chat is a full-stack instant-messaging server and web client. It covers
the core IM feature set — accounts, contacts, groups, sessions, rich messages
and WebRTC calls — and goes one step further by hosting **Swifty**, an AI coding
agent, as a first-class chat participant. Every signed-in user gets their own
isolated agent workspace, streamed live into the conversation.

```
┌──────────────┐     HTTP + WS      ┌────────────────────┐      ┌──────────┐
│   Web (SPA)  │ ─────────────────> │   Chat Server (Go) │ <──> │ MongoDB  │
│  React 19    │                    │  swifty_http/orm   │      └──────────┘
└──────┬───────┘                    │  swifty_cache      │
       │ /agent/ws                  └─────────┬──────────┘
       V                                      │ spawns
┌──────────────┐                    ┌─────────V──────────┐
│  Agent UI    │ <────────────────> │  Swifty Agent Hub  │
│ streaming    │   token / tools    │  1 agent per user  │
└──────────────┘                    └────────────────────┘
```

## Features

### Messaging

- **Direct & group conversations** with auto-created/restored sessions, unread
  counts, last-message previews and activity ordering.
- **Rich message types** — text, image, file, video, AV signaling and system
  notifications.
- **Rich composer** powered by TipTap, with streaming Markdown rendering
  (code, math, Mermaid diagrams) via Streamdown.
- **Contacts** with tags, custom note names, online presence, blacklists and
  keyword search across users and groups.

### Groups

- Create groups with initial members + a welcome message, invite/remove
  members, and a member list carrying join time and last-speak time.
- Configurable join modes (direct entry vs. approval), leave and dismiss flows.

### Calls & Files

- **Audio & video calls** — 1v1 and group mesh — signaled over the `/wss`
  channel; the server tracks call rooms and busy state.
- **Chunked file uploads** with instant upload (dedup by hash) and resume:
  `/file/verify` → `/file/upload-chunk` → `/file/merge` (chunks ≤ 10 MiB).

### AI Agent (Swifty)

- **One private agent per user**, living in an isolated workspace under
  `.swifty/chat/<uid>`.
- Live streaming of token deltas, thinking, tool calls and permission prompts
  over a dedicated `/agent/ws` socket; finalized replies are written back into
  the chat transcript so they survive reloads and show up in session previews.
- Slash-command menu, permission gating and interactive question prompts in the
  composer.

### Operations

- **Admin console** (`/manager`) for user/group management: enable/disable,
  set admin, batch delete and status control.
- **Live cache dashboard** (`/dashboard`) streaming in-process cache stats over
  `/dashboard/ws`.
- **Installable PWA** — add-to-home-screen with auto-updating service worker.

## Tech Stack

| Layer     | Technology                                                                                                                                                      |
| --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Frontend  | React 19, Vite 8, TypeScript, Tailwind CSS v4, shadcn/ui, Zustand, TanStack Query/Form/Virtual, TipTap, Streamdown                                              |
| Backend   | Go 1.26, [`swifty_http`](https://github.com/hangtiancheng/swifty.go) (HTTP + WebSocket), `swifty_orm` (MongoDB), `swifty_cache` (in-process read-through cache) |
| Storage   | MongoDB 7                                                                                                                                                       |
| Agent     | Swifty (Go agent embedded in the chat server via `agent_hub`)                                                                                                   |
| Packaging | Docker (multi-stage), nginx, PWA                                                                                                                                |

## Getting Started

### Prerequisites

- **Node.js ≥ 24** and **pnpm** (frontend)
- **Go ≥ 1.26** (backend)
- **MongoDB ≥ 7** (a running instance)

### Local development

```bash
# 1. Backend — reads ./server/config.json
cd server
cp config.example.jsonc config.json   # then edit mongo URI / jwt secret
go run ./cmd                           # listens on :8000

# 2. Frontend — from the repo root
cp .env.example .env                   # VITE_API_URL=http://localhost:8000
pnpm install
pnpm dev                               # Vite dev server
```

### With Docker

The compose file builds a multi-stage image and runs three services — `mongo`,
`server` and `web` (nginx serving the SPA):

```bash
# Swifty agent provider config (required by the server container)
cp docker/swifty.config.example.yaml docker/swifty.config.yaml

# Point the web bundle at wherever the browser can reach the server
VITE_API_URL=http://localhost:8000 docker compose up --build
```

| Service  | Port (default)         | Notes                                                |
| -------- | ---------------------- | ---------------------------------------------------- |
| `server` | `8000` (`SERVER_PORT`) | API + WebSocket + Swifty agent host                  |
| `web`    | `8081` (`WEB_PORT`)    | nginx serving the built SPA                          |
| `mongo`  | internal               | MongoDB 7, data persisted in the `mongo-data` volume |

`VITE_API_URL` is **baked into the web bundle at build time** — rebuild the
`web` image after changing it.

## Configuration

### Backend — `server/config.json`

```jsonc
{
  "app": { "host": "0.0.0.0", "port": 8000 },
  "mongo": { "uri": "mongodb://localhost:27017", "database": "swifty_chat" },
  "cache": { "maxBytes": 67108864, "expiration": 300 },
  "static": {
    "avatarPath": "./static/avatars",
    "filePath": "./static/files",
    "chunkPath": "./static/chunks",
  },
  "auth": { "jwtSecret": "change-me", "tokenExpireHours": 336 },
}
```

### Frontend — `.env`

| Variable       | Description                                                                                                                                  |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `VITE_API_URL` | Origin of the Go backend (default `http://localhost:8000`)                                                                                   |
| `VITE_WS_URL`  | Optional. Defaults to `VITE_API_URL` with the scheme swapped to `ws`. Set only when the WebSocket endpoint is proxied to a different origin. |

## API Surface

All endpoints are `POST` unless noted. Every route except `/login`, `/register`
and `/user/update-password` requires a JWT in the `Authorization` header; admin
routes additionally require `is_admin`.

| Group       | Examples                                                                       |
| ----------- | ------------------------------------------------------------------------------ |
| `/user`     | `update-user-info`, `search-user`, `get-user-info`, `set-admin` (admin)        |
| `/group`    | `create-group`, `invite-group-members`, `leave-group`, `dismiss-group`         |
| `/session`  | `open-session`, `get-user-session-list`, `mark-session-read`, `delete-session` |
| `/contact`  | `apply-contact`, `pass-contact-apply`, `add-tag`, `black-contact`              |
| `/message`  | `get-message-list`, `get-group-message-list`, `upload-file`, `upload-avatar`   |
| `/file`     | `verify`, `upload-chunk`, `merge`                                              |
| `/chatroom` | `get-online-users`, `get-callers`                                              |

WebSocket channels (all `GET`):

| Channel         | Purpose                                                    |
| --------------- | ---------------------------------------------------------- |
| `/wss`          | Main realtime channel — messages, presence, call signaling |
| `/agent/ws`     | Swifty agent streaming (tokens, tools, permission prompts) |
| `/dashboard/ws` | Live cache dashboard stats                                 |

## Project Structure

```
swifty-chat/
├── src/                     # React frontend
│   ├── pages/               # Routes: chat, sessions, contacts, manager, dashboard, auth
│   ├── components/          # UI: composer, message bubbles, agent cards, dialogs
│   ├── store/               # Zustand stores: auth, ws, call, agent, dashboard
│   ├── service/             # HTTP client, schemas, queries, chunked upload
│   └── workers/             # file-hash web worker (instant-upload dedup)
├── server/                  # Go backend
│   ├── cmd/                 # main.go (chat server), swifty/ (terminal agent)
│   └── internal/
│       ├── router/          # Route table
│       ├── handler/         # HTTP + WebSocket handlers
│       ├── service/         # Business logic + ChatServer + AgentHub wiring
│       ├── agent_hub/       # Per-user Swifty agent host & protocol
│       ├── dao/             # MongoDB access, cache, indexes, transactions
│       ├── model/           # Data models
│       └── swifty/          # Swifty agent engine (tools, mcp, compact, …)
├── docker/                  # nginx conf, docker configs, swifty config example
├── Dockerfile               # Multi-stage: web-builder → server-builder → server / web
└── docker-compose.yml       # mongo + server + web
```

## Deployment Constraints

> **Read before exposing this to a public network.**

- **Single instance only.** The message bus is an in-process channel; the
  WebSocket connection table, call rooms and the cache all live in process
  memory. Messages routed to a user connected to another instance would never
  be delivered. Plan capacity for one instance.
- **No TLS.** The server speaks plain HTTP/WS. Terminate TLS at a gateway
  (nginx, caddy, …) in front of it for anything beyond an internal network.
- **`/user/update-password` is unauthenticated by design** (legacy
  forgot-password parity — no email/SMS verification exists). Anyone knowing a
  telephone number can reset that account's password. Front it with a
  verification step before exposing it publicly.
- **WebSocket endpoints do not validate tokens** — `client_id` is trusted on
  `/wss` and `/dashboard/ws`. Restrict access in production.
- **MongoDB transactions require a replica set.** On a standalone `mongod` the
  server automatically falls back to sequential (non-transactional) writes.

## Scripts

```bash
pnpm dev          # Vite dev server
pnpm build        # tsc -b && vite build
pnpm lint         # eslint .
pnpm format       # prettier
pnpm preview      # preview the production build
```

Backend (`server/`):

```bash
make dev          # hot-reload the chat server with air
make build        # build the chat server into ./tmp
```

## License

Released under the MIT License (see the copyright headers in each source file).
