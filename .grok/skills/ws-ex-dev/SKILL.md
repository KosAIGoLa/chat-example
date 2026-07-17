---
name: ws-ex-dev
description: >
  Develop and debug the ws-ex realtime chat monorepo (Go chat-service + SvelteKit front-chat).
  Use when working on WebSocket chat, history pagination, groups, friends, pins/announcements,
  red packets, LiveKit calls/meetings, or frontend chat UI. Triggers: /ws-ex-dev, "改聊天",
  "WS 消息", "群置顶", "红包", "通话", "MessageList", "chat-service".
metadata:
  short-description: "ws-ex monorepo development"
---

# Skill: ws-ex-dev

在 **ws-ex** monorepo 中实现功能、修 bug、做小范围重构时遵循本 skill。

## 必读

1. 工程约定：`.grok/ENGINEERING.md`（或根 `AGENTS.md`）
2. 人类文档：根 `README.md`
3. 改 UI 前先定位：`front-chat/src/lib/chat/components/` + `controller/`
4. 改协议前先定位：`chat-service/dto/` + `front-chat/src/lib/chat/types.ts`

## 工作流

1. **定位范围**
   - 后端：`controller` → `service` → `model`/`dto`
   - 前端：`api/*` → `controller/*` → `components/*`
2. **保持契约**
   - WS `type` 字段与 DTO 一致
   - 历史分页：`before_seq` / `since_seq` / `has_more`
3. **实现**
   - 最小 diff；中文 UI 文案
   - Svelte 5 runes only
4. **验证**
   - 前端：`cd front-chat && pnpm check`
   - 后端相关：`cd chat-service && go test ./...`（或至少编译 `go build ./...`）

## 关键路径速查

| 需求 | 优先打开 |
|------|----------|
| 消息列表 / 上滑加载 | `MessageList.svelte`, `history.ts` |
| 置顶 | `GroupAnnouncementsBar.svelte`, group/friend pin API |
| 发送/加密 | `messaging.ts`, `crypto.ts`, `msg_crypto.go` |
| 群设置 | `GroupSettings.svelte`, `group_service.go` |
| 红包 | `red_packet_*`, `RedPacketCard.svelte` |
| 通话 | `call.svelte.ts`, `livekit_*` |
| 路由 | `chat-service/router/router.go` |

## 完成标准

- [ ] 无无关文件改动
- [ ] 类型/接口前后端对齐
- [ ] `pnpm check` 或 `go test`/`go build` 通过（视改动面）
- [ ] 用户可见行为有简短说明（中文可）
