# ws-ex 工程化指南（Grok）

> 本文件为 **Grok / xAI** 专案工程约定。技能索引见 [skills.md](./skills.md)；可调用技能在 [skills/](./skills/)。

## 1. 专案定位

**ws-ex** 是实时聊天 monorepo：

| 路径 | 技术栈 | 端口 |
|------|--------|------|
| `chat-service/` | Go · Gin · NATS JetStream · Postgres · LiveKit tokens | `:8080` |
| `front-chat/` | SvelteKit 5（static）· Tailwind 4 · shadcn-svelte · nginx | `:3000` |
| `docker-compose.yml` | nats · postgres · livekit · ws-server · frontend | — |

## 2. 架构约束

```
Browser (Svelte) ──HTTPS/WS──► nginx ──/api /ws──► chat-service
                     └──/rtc──► LiveKit SFU
chat-service ──JetStream+KV──► NATS
             ──SQL──► Postgres
```

- **消息路径**：WS 帧 → 校验/加密 → 分配 `id`+`seq` → 私聊直达 / 群 NATS fan-out → JetStream 历史（~180 天，`before_seq` / `since_seq` 分页）。
- **控制事件**：`recall` / `edit` / presence / meeting → Core NATS `chat.event.*`。
- **群角色**：`owner`（解散、设管理员）> `admin`（改名/头像）> `member`。

## 3. 后端分层（必须遵守）

```
controller/  →  HTTP 入参、鉴权上下文、DTO 响应
service/     →  业务逻辑、NATS、钱包、群/好友
model/       →  GORM 实体
dto/         →  请求/响应与 WS 事件结构
validate/    →  查询/路径参数校验
middleware/  →  JWT 等
router/      →  路由注册
```

- 新 API：先 `dto` → `service` → `controller` → `router`。
- 私聊/群历史：`before_seq` / `before_ts` 加载更早；`since_seq` 增量同步。
- 撤回/编辑：约 **2 分钟** 窗口，仅本人。
- 勿在 controller 写业务；勿绕过 `validate` 直接解析不可信输入。

## 4. 前端分层（必须遵守）

```
$lib/api/*                 → REST / WS 客户端
$lib/chat/controller/*     → 领域逻辑（history、groups、messaging…）
$lib/chat/chat.svelte.ts   → 控制器装配与对外 API
$lib/chat/components/*     → UI
$lib/components/ui/*       → shadcn 基础组件
```

- **Svelte 5**：`$state` / `$derived` / `$props` / `$effect`；勿混用 Svelte 4 stores 模式。
- 历史：cache-first + 上滑加载更早；贴底自动收新消息。
- 置顶：群公告 + 私聊 pins；顶栏轮播 + 弹窗列表搜索。
- UI 文案：中文为主；toast/confirm 用 `$lib/ui/notify.svelte.ts`，勿用 `window.alert`。

## 5. 常用命令

```bash
# 全栈
docker compose up -d --build

# 后端
cd chat-service && go run ./cmd/server
cd chat-service && go test ./...

# 前端
cd front-chat && pnpm install && pnpm dev
cd front-chat && pnpm check && pnpm lint && pnpm test
cd front-chat && pnpm build
```

- 前端包管理：**pnpm**（勿用 npm/yarn 改 lockfile）。
- LiveKit LAN：`export LIVEKIT_NODE_IP=<局域网IP>` 后再 compose up。

## 6. 改动纪律

1. **最小 diff**：只改任务相关文件；禁止顺手大重构。
2. **前后端契约**：WS/REST 字段变更需同时改 `dto` + `front-chat` types + 调用方。
3. **安全**：JWT 保护 `/api/*`；媒体上传限类型/大小；勿日志打印 token/密钥。
4. **测试**：Go 改 `validate`/crypto 补单测；前端关键路径跑 `pnpm check`。
5. **提交**：完整句子说明「为什么」；不主动 commit 除非用户要求。

## 7. 功能地图（实现时查表）

| 功能 | 后端重心 | 前端重心 |
|------|----------|----------|
| 私聊/群聊 | `chat_service`, hub, NATS | `messaging`, `MessageList` |
| 历史分页 | `GetMessageHistory`, JetStream | `history.ts`, `MessageList` |
| 好友/黑名单 | `friend_service` | `friends.ts`, Sidebar |
| 群角色/解散 | `group_service` | `groups.ts`, `GroupSettings` |
| 置顶 | announcements + private pins | `GroupAnnouncementsBar` |
| 红包 | `red_packet_service`, wallet | `RedPacketCard`, dialog |
| 通话/会议 | LiveKit tokens + WS 信令 | `call.svelte.ts`, `CallPanel` |
| 输入中 | ephemeral typing WS | `typing.ts`, `typing-ui` |

## 8. 禁止事项

- 不提交 `.env` 密钥、真实 production secrets。
- 不写 exploit/PoC 攻击代码。
- 不擅自 force-push / `git reset --hard` / 删除远端分支。
- 不把 `node_modules`、构建产物当源码改。
