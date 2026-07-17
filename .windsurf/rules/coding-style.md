# Coding Style — ws-ex

## Go (`chat-service`)

- 模块名：`ws-ex`
- 错误：返回 `error` + 上层映射 HTTP/DTO，避免 panic
- JSON 标签与前端 `types.ts` 对齐（snake_case 常见于 API）
- 新校验放 `validate/` 并补测试
- 日志勿打印 token / 密码 / 完整密钥

## TypeScript / Svelte (`front-chat`)

- TypeScript strict；优先显式类型在公共 API
- Svelte 5：组件用 runes，不用旧版 `export let` store 混写
- 组件 props：`interface Props` + `$props()`
- 路径别名：`$lib/...`
- 样式：Tailwind 工具类；shadcn 组件在 `$lib/components/ui`
- 图标：`@lucide/svelte/icons/...`

## 命名

- 文件：Go 用 snake 或标准 package 风格；前端组件 `PascalCase.svelte`，逻辑 `kebab` 或 `camel` 模块
- WS 事件 `type` 字符串与后端 DTO 常量一致（如 `private_pin`, `group_announcement`, `typing`）

## 注释

- 只注释非显而易见的意图/约束
- 不写冗长复述代码的注释
