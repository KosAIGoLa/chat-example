# ws-ex

Chat stack monorepo: Go backend and Svelte frontend are separate projects.

```
ws-ex/
├── chat-service/   # Go WebSocket chat API (module: ws-ex)
├── front-chat/     # SvelteKit UI + nginx reverse proxy
└── docker-compose.yml
```

## Services

| Path | Stack | Default port |
|------|--------|--------------|
| `chat-service/` | Go + Gin + NATS + Postgres | `:8080` |
| `front-chat/` | SvelteKit (static) + nginx | `:3000` → proxies `/api`, `/ws` |

## Quick start (Docker)

```bash
# from repo root
docker compose up -d --build
```

- UI: http://localhost:3000  
- API / WS: http://localhost:8080  

## Local development

### Backend (`chat-service`)

```bash
cd chat-service
# start NATS + Postgres (from repo root): docker compose up -d nats postgres
go run ./cmd/server
```

### Frontend (`front-chat`)

```bash
cd front-chat
pnpm install
pnpm dev
```

Point `VITE_API_BASE` / proxy at the Go server if not using the nginx image.
