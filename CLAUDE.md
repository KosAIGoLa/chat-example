# CLAUDE.md — ws-ex

Project memory for **Claude Code**. Detailed engineering: [.claude/ENGINEERING.md](./.claude/ENGINEERING.md). Multi-agent map: [AGENTS.md](./AGENTS.md).

## What this repo is

Realtime chat: **Go** backend (`chat-service`) + **SvelteKit** frontend (`front-chat`), with NATS JetStream, Postgres, LiveKit.

## Before coding

1. Read [.claude/ENGINEERING.md](./.claude/ENGINEERING.md) for layers and rules  
2. For feature work, load skill [.claude/skills/ws-ex-dev/SKILL.md](./.claude/skills/ws-ex-dev/SKILL.md)  
3. Prefer minimal diffs; keep REST/WS contracts aligned  

## Stack at a glance

- Backend: Gin, GORM/Postgres, NATS, JWT, LiveKit tokens  
- Frontend: Svelte 5, SvelteKit static adapter, Tailwind 4, shadcn-svelte, **pnpm**  
- Ports: API/WS `:8080`, UI `:3000`  

## Commands

```bash
docker compose up -d --build
cd chat-service && go run ./cmd/server && go test ./...
cd front-chat && pnpm install && pnpm dev && pnpm check
```

## Do / Don’t

**Do:** controller→service→dto; api→controller→components; Chinese UI; `notify.svelte.ts`  

**Don’t:** Svelte 4 patterns in new code; npm lockfile churn; commit `.env` secrets; unrelated refactors  

## Skills index

See [.claude/skills.md](./.claude/skills.md).
