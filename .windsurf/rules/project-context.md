# ws-ex Project Context

## Overview

Realtime chat monorepo:

| Path | Stack | Port |
|------|--------|------|
| `chat-service/` | Go · Gin · NATS JetStream · Postgres · LiveKit | `:8080` |
| `front-chat/` | SvelteKit 5 · Tailwind 4 · shadcn-svelte · nginx | `:3000` |
| `docker-compose.yml` | nats · postgres · livekit · ws-server · frontend | — |

## Architecture

```
Browser ──HTTPS/WS──► nginx ──/api /ws──► chat-service
              └──/rtc──► LiveKit SFU
chat-service ──JetStream──► NATS
             ──SQL──► Postgres
```

- Private DM + group durable rooms
- Message seq + JetStream history (~180 days)
- Roles: owner / admin / member
- Features: typing, recall/edit (2 min), pins, red packets, private call, group meeting

## Key directories

### Backend (`chat-service/`)

- `controller/` — HTTP handlers
- `service/` — business + NATS + LiveKit
- `model/` — GORM
- `dto/` — API/WS shapes
- `router/` — Gin routes
- `validate/` — input validation
- `cmd/server/` — entrypoint

### Frontend (`front-chat/`)

- `src/lib/api/` — REST + WS clients
- `src/lib/chat/controller/` — domain logic
- `src/lib/chat/components/` — chat UI
- `src/lib/components/ui/` — shadcn primitives
- `src/routes/` — SvelteKit pages

## Package managers

- Go modules in `chat-service/`
- **pnpm** in `front-chat/` (required)
