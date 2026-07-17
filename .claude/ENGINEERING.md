# ws-ex 工程化指南（Claude）

> Claude Code 会优先读根目录 [CLAUDE.md](../CLAUDE.md)。本文件为完整工程约定；技能见 [skills.md](./skills.md)。

## 专案

| 路径 | 技术 | 端口 |
|------|------|------|
| `chat-service/` | Go · Gin · NATS · Postgres · LiveKit | `:8080` |
| `front-chat/` | SvelteKit 5 · Tailwind 4 · shadcn · pnpm | `:3000` |

## 架构

- 消息：WS → 校验/加密 → `id`+`seq` → 私聊/群 fan-out → JetStream 历史
- 分页：`before_seq` 更早，`since_seq` 增量，`has_more`
- 群角色：owner > admin > member
- 撤回/编辑：约 2 分钟窗口

## 分层

**后端：** `controller` → `service` → `model`/`dto`；`router`；`validate`；`middleware`

**前端：** `$lib/api` → `$lib/chat/controller` → `$lib/chat/components`；UI  primitives 在 `$lib/components/ui`

## 命令

```bash
docker compose up -d --build
cd chat-service && go run ./cmd/server && go test ./...
cd front-chat && pnpm install && pnpm dev && pnpm check
```

## 纪律

1. 最小 diff  
2. 前后端契约同步  
3. Svelte 5 runes only  
4. 中文 UI；`notify.svelte.ts` 代替 `alert`  
5. 不提交密钥；不擅自破坏性 git 操作  
6. 验证：`pnpm check` / `go test`  

## 功能地图

| 功能 | 后端 | 前端 |
|------|------|------|
| 历史 | history API, JetStream | history.ts, MessageList |
| 置顶 | announcements / pins | GroupAnnouncementsBar |
| 群/好友 | group/friend service | GroupSettings, Sidebar |
| 红包 | red_packet + wallet | RedPacketCard |
| 通话 | livekit | call.svelte.ts |
