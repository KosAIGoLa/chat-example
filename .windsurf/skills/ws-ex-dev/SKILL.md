---
name: ws-ex-dev
description: >
  Develop and debug the ws-ex realtime chat monorepo (Go chat-service + SvelteKit front-chat).
  Use when working on WebSocket chat, history pagination, groups, friends, pins/announcements,
  red packets, LiveKit calls/meetings, or frontend chat UI. Triggers: /ws-ex-dev, "改聊天",
  "WS 消息", "群置顶", "红包", "通话", "MessageList", "chat-service".
---

# Skill: ws-ex-dev (Windsurf)

## 必读

- Rules：`.windsurf/rules/`（尤其 `engineering.md`、`project-context.md`）
- 根：`AGENTS.md`、`README.md`

## 工作流

1. 读相关 rule + 定位代码
2. 保持 DTO / types / API 契约
3. 最小实现（Svelte 5 + Go 分层）
4. `pnpm check` 和/或 `go test`/`go build`

## 路径速查

| 需求 | 文件 |
|------|------|
| 消息列表 | `MessageList.svelte`, `history.ts` |
| 置顶 | `GroupAnnouncementsBar.svelte` |
| 群 | `group_service.go`, `GroupSettings.svelte` |
| 红包 | `red_packet_*`, `RedPacketCard.svelte` |
| 通话 | `call.svelte.ts`, `livekit_*` |
| 路由 | `router/router.go` |
