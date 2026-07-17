---
name: ws-ex-dev
description: >
  Develop and debug the ws-ex realtime chat monorepo (Go chat-service + SvelteKit front-chat).
  Use when working on WebSocket chat, history pagination, groups, friends, pins/announcements,
  red packets, LiveKit calls/meetings, or frontend chat UI. Triggers: /ws-ex-dev, "改聊天",
  "WS 消息", "群置顶", "红包", "通话", "MessageList", "chat-service".
---

# Skill: ws-ex-dev (Cursor)

在 **ws-ex** monorepo 中实现功能、修 bug 时加载本 skill。

## 必读

- 始终生效的工程规则：`.cursor/rules/engineering.mdc`
- 根目录：`AGENTS.md`、`README.md`

## 工作流

1. 定位：后端 `controller→service→dto`；前端 `api→controller→components`
2. 契约：WS `type`、历史 `before_seq`/`since_seq`/`has_more` 前后端一致
3. 实现：最小 diff；Svelte 5 runes；中文 UI；notify 库
4. 验证：`pnpm check` 和/或 `go test ./...` / `go build ./...`

## 路径速查

| 需求 | 文件 |
|------|------|
| 消息列表/加载 | `front-chat/src/lib/chat/components/MessageList.svelte`, `controller/history.ts` |
| 置顶 | `GroupAnnouncementsBar.svelte`, pins/announcements API |
| 群 | `group_service.go`, `GroupSettings.svelte` |
| 红包 | `red_packet_*`, `RedPacketCard.svelte` |
| 通话 | `call.svelte.ts`, `livekit_*` |
| 路由 | `chat-service/router/router.go` |

## 完成标准

- 无无关改动；类型对齐；检查命令通过；向用户说明行为变化
