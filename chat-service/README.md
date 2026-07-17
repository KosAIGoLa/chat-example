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
export REDIS_ADDR=127.0.0.1:6379   # empty = disable list cache
export REDIS_PASSWORD=chatredis_change_me   # must match redis --requirepass
export REDIS_LIST_TTL=10m

# Postgres / Redis client pools (optional; defaults shown)
export DB_MAX_OPEN_CONNS=50
export DB_MAX_IDLE_CONNS=10
export DB_CONN_MAX_LIFETIME=1h
export DB_CONN_MAX_IDLE_TIME=10m
export REDIS_POOL_SIZE=50
export REDIS_MIN_IDLE_CONNS=5
export REDIS_MAX_IDLE_CONNS=20

# one-shot
go run ./cmd/server
# or
make run
```

## Connection pools

| Target | Env | Default | Notes |
|--------|-----|---------|--------|
| Postgres open | `DB_MAX_OPEN_CONNS` | 50 | Concurrent DB connections per process |
| Postgres idle | `DB_MAX_IDLE_CONNS` | 10 | Kept warm in pool |
| Postgres lifetime | `DB_CONN_MAX_LIFETIME` | 1h | Recycle long-lived conns |
| Postgres idle time | `DB_CONN_MAX_IDLE_TIME` | 10m | Close unused idle conns |
| Redis pool | `REDIS_POOL_SIZE` | 50 | Max connections in go-redis pool |
| Redis min idle | `REDIS_MIN_IDLE_CONNS` | 5 | Warm idle conns |
| Redis max idle | `REDIS_MAX_IDLE_CONNS` | 20 | Cap idle pool |
| Redis pool wait | `REDIS_POOL_TIMEOUT` | 4s | Wait for free conn |
| Redis I/O | `REDIS_DIAL/READ/WRITE_TIMEOUT` | 3s / 2s / 2s | Per-op timeouts |

Rule of thumb: `DB_MAX_OPEN_CONNS × service_replicas < Postgres max_connections` (compose sets `max_connections=200`).

Compose also tunes server-side Redis (`maxmemory` / `allkeys-lru`) and Postgres buffers — see `docker-compose.yml`.

## Redis list cache

Read-through for hot lists (no CUD → Redis; CUD → invalidate → next list refills):

| List | Key pattern | Invalidate on |
|------|-------------|----------------|
| Friends | `list:friends:{uid}` | accept / remove / block / unblock |
| Incoming / outgoing invites | `list:friends:incoming\|outgoing:{uid}` | send / accept / reject |
| Blacklist | `list:blacklist:{uid}` | block / unblock |
| My groups | `list:groups:mine:{uid}` | create / join / leave / dissolve / rename / avatar / role |
| Group members | `list:group:members:{gid}` | join / leave / dissolve / role |
| Group announcements | `list:group:ann:{gid}` | pin / unpin |
| Private pins | `list:private:pins:{a}:{b}` | pin / unpin / unfriend |

`online` flags on friends/members are **not** trusted from cache — refreshed from the hub on every read.

**Password required:** Redis is started with `--requirepass`. Set the same value in:

- root `.env` → `REDIS_PASSWORD=...` (compose injects into `redis` + `ws-server`)
- or export `REDIS_PASSWORD` when running Go locally

Default (local only): `chatredis_change_me` — **change in production**.

```bash
# local redis (uses REDIS_PASSWORD from .env / compose default)
docker compose up -d redis

# manual redis with password
docker run -p 6379:6379 redis:8-alpine \
  redis-server --appendonly yes --requirepass "$REDIS_PASSWORD"

# check auth
redis-cli -a "$REDIS_PASSWORD" --no-auth-warning ping
```

- **关闭缓存**：`REDIS_ADDR=`（空）  
- **密码错误 / 连不上**：日志告警后降级为纯 DB，服务仍可启动  
- TTL 默认 `10m`（`REDIS_LIST_TTL`）

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

## NATS JetStream

JetStream is **enabled** via repo-root [`nats.conf`](../nats.conf) (compose mounts it into the `nats` service):

| Setting | Value | Purpose |
|---------|--------|---------|
| `jetstream.store_dir` | `/data` | Persistent volume `nats-data` |
| `max_file_store` | `10GB` | Server file budget for streams |
| `max_memory_store` | `512MB` | KV / memory streams |
| Stream `CHAT_MESSAGES` | `NATS_STREAM_MAX_BYTES` (default `1G`) | App soft cap ≤ server budget |

Monitor: http://localhost:8222/jsz · http://localhost:8222/varz

Local NATS without compose:

```bash
nats-server -c nats.conf
# or: nats-server -js -sd ./data/nats -m 8222
```

## Docker

Built from repo root:

```bash
docker compose build ws-server
docker compose up -d ws-server
```

Image context is `./chat-service`. Docker/Compose also send SIGTERM on stop; the same graceful path runs inside the container.
