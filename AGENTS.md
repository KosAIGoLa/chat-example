# AGENTS.md — ws-ex

Instructions for AI coding agents (Grok, Cursor, Windsurf, Claude, etc.).

## Project

Realtime chat monorepo:

| Path | Role |
|------|------|
| `chat-service/` | Go WebSocket + REST (Gin, NATS JetStream, Postgres, LiveKit tokens) |
| `front-chat/` | SvelteKit 5 static UI (pnpm, Tailwind 4, shadcn-svelte) |
| `docker-compose.yml` | nats · postgres · livekit · ws-server · frontend |

Human docs: [README.md](./README.md)

## Tool-specific locations

| Tool | Skills index | Engineering / rules | Invocable skill |
|------|--------------|---------------------|-----------------|
| **Grok** | [.grok/skills.md](./.grok/skills.md) | [.grok/ENGINEERING.md](./.grok/ENGINEERING.md) | `.grok/skills/ws-ex-dev/` |
| **Cursor** | [.cursor/skills.md](./.cursor/skills.md) | [.cursor/rules/engineering.mdc](./.cursor/rules/engineering.mdc) | `.cursor/skills/ws-ex-dev/` |
| **Windsurf** | [.windsurf/skills.md](./.windsurf/skills.md) | [.windsurf/rules/](./.windsurf/rules/) | `.windsurf/skills/ws-ex-dev/` |
| **Claude** | [.claude/skills.md](./.claude/skills.md) | [.claude/ENGINEERING.md](./.claude/ENGINEERING.md) + [CLAUDE.md](./CLAUDE.md) | `.claude/skills/ws-ex-dev/` |

Prefer the tool’s native path when present; keep content aligned across copies.

## Non-negotiables

1. **Minimal diffs** — no drive-by refactors  
2. **Layers** — Go: controller → service → model/dto; FE: api → controller → components  
3. **Svelte 5 runes** only in new UI code  
4. **pnpm** for `front-chat`  
5. **Contract sync** — `dto` ↔ `types.ts` ↔ API clients  
6. **No secrets** in commits; no destructive git unless asked  
7. Verify: `pnpm check` and/or `go test ./...`

## Quick commands

```bash
docker compose up -d --build
cd chat-service && go run ./cmd/server   # or: make dev (air hot reload)
cd front-chat && pnpm install && pnpm dev
cd front-chat && pnpm check
```

## Feature → code (short)

| Feature | Start here |
|---------|------------|
| History scroll | `history.ts`, `MessageList.svelte` |
| Pins | `GroupAnnouncementsBar.svelte`, pin/announcement APIs |
| Groups | `group_service.go`, `GroupSettings.svelte` |
| Red packet | `red_packet_*`, `RedPacketCard.svelte` |
| Calls | `call.svelte.ts`, `livekit_*` |

## Skill

Use **ws-ex-dev** (`/ws-ex-dev`) when implementing or debugging chat features in this repo.
