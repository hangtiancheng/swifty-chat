# swifty_chat

A chat server built on the swifty.go stack: swifty_http (HTTP + WebSocket),
swifty_orm (MongoDB) and swifty_cache (in-process read-through cache).

## Run

```bash
go run ./cmd                          # backend, reads ./config.json
cd ../apps/swifty-chat && pnpm dev    # frontend dev server
```

## Capabilities

- Telephone/password accounts with salted SHA-256 hashes and HS256 JWT
  tokens (`auth.jwtSecret` in config.json). All POST endpoints except
  `/login`, `/register` and `/user/update-password` require the token in the
  `Authorization` header; admin endpoints additionally require `is_admin`.
- Sessions with unread counts, last-message previews and activity ordering;
  `/session/mark-session-read` clears unread. Direct and group messages
  auto-create/restore the receivers' sessions.
- Contacts with tags (`/contact/get-tag-list`, `/contact/add-tag`), note
  names (`/contact/update-contact`), online presence and keyword search
  (`/user/search-user`, `/group/search-group`).
- Groups with initial members + welcome message at creation, member
  invitation (`/group/invite-group-members`) and a member list carrying join
  time and last-speak time.
- Message types: 0 text, 1 image, 2 file, 3 AV signaling, 4 video,
  5 system notification (content = topic: contact/group/apply/session/online).
- Audio & video calls (1v1 and group mesh) signaled over the `/wss` channel
  via type-3 frames; the server tracks call rooms and busy state
  (`/chatroom/get-callers` lists room members).
- Chunked uploads with instant upload and resume: `/file/verify`,
  `/file/upload-chunk`, `/file/merge` (chunks capped at 10 MiB each).

## Deployment constraints

- **Single instance only.** The message bus is an in-process channel, the
  WebSocket connection table, call rooms and the cache all live in process
  memory. Messages sent to a user connected to another instance would never
  be delivered. Plan capacity for one instance.
- **No TLS.** The server speaks plain HTTP/WS. For anything beyond an
  internal network, terminate TLS at a gateway (nginx, caddy, ...) in front
  of it.
- `/user/update-password` is unauthenticated by design (legacy
  forgot-password parity: no email/SMS verification exists). Anyone knowing a
  telephone number can reset that account's password — front it with a
  verification step before exposing it publicly.
- The WebSocket endpoints (`/wss`, `/dashboard/ws`) do not validate tokens;
  `client_id` is trusted. Restrict access in production.
- **MongoDB transactions** require a replica set. On a standalone mongod the
  server automatically falls back to sequential (non-transactional) writes.
