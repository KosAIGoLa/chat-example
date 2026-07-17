# ---------- Build stage ----------
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
RUN CGO_ENABLED=0 go build -o /ws-server ./cmd/server

# ---------- Runtime stage ----------
FROM alpine:3.20

# tzdata is required for TimeZone=Asia/Taipei in the Postgres DSN
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /ws-server .

EXPOSE 8080

ENV SERVER_ADDR=:8080
ENV NATS_URL=nats://nats:4222
ENV DB_HOST=postgres
ENV DB_PORT=5432
ENV DB_USER=chat
ENV DB_PASSWORD=chatpass
ENV DB_NAME=chatdb
ENV JWT_SECRET=change-me-in-production
ENV SHUTDOWN_TIMEOUT=15s
ENV REDIS_ADDR=redis:6379
# REDIS_PASSWORD must be set at runtime (compose / systemd) to match redis --requirepass
ENV REDIS_PASSWORD=
ENV REDIS_DB=0
ENV REDIS_LIST_TTL=10m
# Client pools (override at runtime)
ENV DB_MAX_OPEN_CONNS=50
ENV DB_MAX_IDLE_CONNS=10
ENV DB_CONN_MAX_LIFETIME=1h
ENV DB_CONN_MAX_IDLE_TIME=10m
ENV REDIS_POOL_SIZE=50
ENV REDIS_MIN_IDLE_CONNS=5
ENV REDIS_MAX_IDLE_CONNS=20

# Compose / orchestrators send SIGTERM; app drains HTTP+WS then exits.
STOPSIGNAL SIGTERM

CMD ["./ws-server"]
