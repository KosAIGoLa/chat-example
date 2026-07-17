# Commands — ws-ex

## Docker（推荐）

```bash
# 仓库根目录
docker compose up -d --build

# LiveKit 局域网 ICE
export LIVEKIT_NODE_IP=192.168.x.x
docker compose up -d
```

- UI: http://localhost:3000  
- API/WS: http://localhost:8080  

## 后端本地

```bash
# 依赖：nats postgres livekit
docker compose up -d nats postgres livekit

cd chat-service
go run ./cmd/server
go test ./...
go build -o /dev/null ./...
```

## 前端本地

```bash
cd front-chat
pnpm install
pnpm dev
pnpm check
pnpm lint
pnpm format
pnpm test
pnpm build
```

包管理：**仅 pnpm**。

## 健康检查参考

- NATS monitor: `http://localhost:8222/healthz`
- Postgres: compose healthcheck `pg_isready`
- LiveKit: host `:7880`（浏览器信令经 nginx `/rtc`）
