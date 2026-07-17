# chat-service

Go WebSocket chat backend (module path: `ws-ex`).

## Layout

```
chat-service/
├── cmd/server/       # thin main (loads config, runs app)
├── internal/app/     # config + dependency wiring
├── controller/
├── database/
├── dto/
├── middleware/
├── model/
├── router/
├── service/
├── .air.toml         # hot reload (air)
├── Makefile
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
export SHUTDOWN_TIMEOUT=15s   # graceful stop budget (SIGINT/SIGTERM)

# one-shot
go run ./cmd/server
# or
make run
```

## Hot reload (local dev)

Uses [air](https://github.com/air-verse/air). On save of `.go` files, rebuilds and restarts.

```bash
# from chat-service/
make dev
```

If `air` is not installed, `make dev` falls back to `go run github.com/air-verse/air@v1.61.7`.

Optional permanent install:

```bash
go install github.com/air-verse/air@v1.61.7
```

## Graceful shutdown

On **SIGINT** / **SIGTERM** (and systemd `systemctl stop`):

1. Stop accepting new HTTP / WebSocket upgrades  
2. Close local WS clients (presence → offline; clients can reconnect)  
3. `http.Server.Shutdown` with timeout (`SHUTDOWN_TIMEOUT`, default `15s`)  
4. Close NATS + DB pool  

```bash
# local test
go run ./cmd/server
# another terminal
kill -TERM $(pgrep -f 'tmp/ws-server|cmd/server')
```

## systemd

Unit + env example under `deploy/systemd/`:

```bash
# one-shot install (root): build binary, unit, env skeleton
sudo ./deploy/systemd/install.sh

sudoedit /etc/ws-ex/ws-server.env
sudo systemctl start ws-server
sudo systemctl status ws-server
journalctl -u ws-server -f

# graceful stop / restart
sudo systemctl stop ws-server
sudo systemctl restart ws-server
```

| Path | Purpose |
|------|---------|
| `deploy/systemd/ws-server.service` | unit (`KillSignal=SIGTERM`, `TimeoutStopSec=25`) |
| `deploy/systemd/ws-server.env.example` | → `/etc/ws-ex/ws-server.env` |
| `deploy/systemd/install.sh` | build + enable |

Default install prefix: `/opt/ws-ex/chat-service` (override with `PREFIX=...`).

## Docker

Built from repo root:

```bash
docker compose build ws-server
docker compose up -d ws-server
```

Image context is `./chat-service`. Docker/Compose also send SIGTERM on stop; the same graceful path runs inside the container.
