# ws-ex 工程化约定（Windsurf）

## 分层

### 后端

```
controller → service → model/dto
router 注册 · middleware 鉴权 · validate 校验
```

- 新 API：dto → service → controller → router
- 历史：`before_seq` 更早；`since_seq` 增量；响应带 `has_more`
- 撤回/编辑：约 2 分钟，仅本人
- 禁止 controller 内堆业务逻辑

### 前端

```
$lib/api → $lib/chat/controller → $lib/chat/components
```

- Svelte 5 runes only（`$state` / `$derived` / `$props` / `$effect`）
- 历史 cache-first + 上滑加载；贴底收新消息
- Toast/confirm：`$lib/ui/notify.svelte.ts`，禁用 `window.alert`
- UI 中文

## 改动纪律

1. 最小 diff，禁止顺手重构
2. REST/WS 字段变更同步 `dto` + `types.ts` + API client
3. 不提交 secrets；不 force-push 除非用户要求
4. 前端改完 `pnpm check`；Go 逻辑 `go test` / `go build`

## 功能 → 代码

| 功能 | 后端 | 前端 |
|------|------|------|
| 聊天/历史 | chat service, NATS | history.ts, MessageList |
| 置顶 | announcements / pins | GroupAnnouncementsBar |
| 好友 | friend_service | friends.ts, Sidebar |
| 群 | group_service | groups.ts, GroupSettings |
| 红包 | red_packet_service | RedPacketCard |
| 通话 | livekit | call.svelte.ts, CallPanel |

详见 [commands.md](./commands.md)、[coding-style.md](./coding-style.md)。
