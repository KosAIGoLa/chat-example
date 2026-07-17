---
name: ws-ex-dev
description: >
  Develop and debug the ws-ex realtime chat monorepo (Go chat-service + SvelteKit front-chat).
  Use when working on WebSocket chat, history pagination, groups, friends, pins/announcements,
  red packets, LiveKit calls/meetings, or frontend chat UI. Triggers: /ws-ex-dev, "改聊天",
  "WS 消息", "群置顶", "红包", "通话", "MessageList", "chat-service".
---

# Skill: ws-ex-dev (Claude)

## 必读

- 根 `CLAUDE.md`、`.claude/ENGINEERING.md`
- 人类文档 `README.md`

## 工作流

1. 定位：后端分层 / 前端 api→controller→components  
2. 契约：DTO、types、WS `type` 一致  
3. 实现：最小 diff；Svelte 5；中文 UI  
4. 验证：`pnpm check`、`go test`/`go build`

## 路径速查

| 需求 | 文件 |
|------|------|
| 消息列表 | `MessageList.svelte`, `history.ts` |
| 置顶 | `GroupAnnouncementsBar.svelte` |
| 群 | `group_service.go`, `GroupSettings.svelte` |
| 红包 | `red_packet_*`, `RedPacketCard.svelte` |
| 通话 | `call.svelte.ts`, `livekit_*` |
