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

# Compose / orchestrators send SIGTERM; app drains HTTP+WS then exits.
STOPSIGNAL SIGTERM

CMD ["./ws-server"]
