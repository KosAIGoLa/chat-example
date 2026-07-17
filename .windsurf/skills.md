# ws-ex Windsurf Skills 索引

> 专案技能：`.windsurf/skills/<name>/SKILL.md`  
> 工程规则：`.windsurf/rules/`（见 [rules/index.md](./rules/index.md)）

## 已安装技能

| 技能 | 路径 | 用途 |
|------|------|------|
| **ws-ex-dev** | [skills/ws-ex-dev/SKILL.md](./skills/ws-ex-dev/SKILL.md) | Go + SvelteKit 实时聊天全栈开发 |

## Windsurf 约定

- **Rules**：`.windsurf/rules/*.md` — Cascade 持久上下文
- **Skills**：`.windsurf/skills/<name>/SKILL.md` — 可调用工作流
- **Workflows**（可选）：`.windsurf/workflows/` — 多步流程

## 新增技能

```bash
mkdir -p .windsurf/skills/<name>
# 编写 SKILL.md
```

在本文件登记，并视需要在 `rules/index.md` 交叉引用。

## 跨工具对照

| 工具 | Skills | 工程化 |
|------|--------|--------|
| Windsurf | `.windsurf/skills/` | `.windsurf/rules/` |
| Grok | `.grok/skills/` | `.grok/ENGINEERING.md` |
| Cursor | `.cursor/skills/` | `.cursor/rules/*.mdc` |
| Claude | `.claude/skills/` | `CLAUDE.md` + `.claude/ENGINEERING.md` |
