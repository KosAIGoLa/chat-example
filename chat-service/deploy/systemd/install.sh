#!/usr/bin/env bash
# Install ws-server binary + systemd unit (run as root).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PREFIX="${PREFIX:-/opt/ws-ex/chat-service}"
UNIT_SRC="$(cd "$(dirname "$0")" && pwd)/ws-server.service"
ENV_EXAMPLE="$(cd "$(dirname "$0")" && pwd)/ws-server.env.example"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "run as root: sudo $0" >&2
  exit 1
fi

id -u ws-chat &>/dev/null || useradd --system --home "$PREFIX" --shell /usr/sbin/nologin ws-chat

install -d -o ws-chat -g ws-chat -m 755 "$PREFIX" "$PREFIX/data" "$PREFIX/data/voice"
install -d -m 755 /etc/ws-ex

echo "building binary…"
(cd "$ROOT" && CGO_ENABLED=0 go build -o "$PREFIX/ws-server" ./cmd/server)
chown ws-chat:ws-chat "$PREFIX/ws-server"
chmod 755 "$PREFIX/ws-server"

if [[ ! -f /etc/ws-ex/ws-server.env ]]; then
  install -m 640 "$ENV_EXAMPLE" /etc/ws-ex/ws-server.env
  chown root:ws-chat /etc/ws-ex/ws-server.env
  echo "wrote /etc/ws-ex/ws-server.env — edit secrets before start"
fi

install -m 644 "$UNIT_SRC" /etc/systemd/system/ws-server.service
systemctl daemon-reload
systemctl enable ws-server.service

echo
echo "Installed. Next:"
echo "  sudo edit /etc/ws-ex/ws-server.env"
echo "  sudo systemctl start ws-server"
echo "  sudo systemctl status ws-server"
echo "  journalctl -u ws-server -f"
