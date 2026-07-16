# chat-service

Go WebSocket chat backend (module path: `ws-ex`).

## Layout

```
chat-service/
├── cmd/server/     # entrypoint
├── controller/
├── database/
├── dto/
├── middleware/
├── model/
├── router/
├── service/
├── Dockerfile
├── go.mod
└── go.sum
```

## Run

```bash
# env (defaults shown)
export NATS_URL=nats://127.0.0.1:4222
export DB_HOST=127.0.0.1
export JWT_SECRET=change-me
export MSG_CRYPTO_KEY=change-me-crypto
export MEDIA_DIR=./data/voice
export SERVER_ADDR=:8080

go run ./cmd/server
```

## Docker

Built from repo root:

```bash
docker compose build ws-server
docker compose up -d ws-server
```

Image context is `./chat-service`.
