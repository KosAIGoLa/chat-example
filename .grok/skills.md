# ws-ex Grok Skills 索引

> 专案级技能目录：`.grok/skills/<name>/SKILL.md`  
> 工程约定：[.grok/ENGINEERING.md](./ENGINEERING.md) · 根目录 [AGENTS.md](../AGENTS.md)

## 已安装技能

| 技能 | 路径 | 触发场景 |
|------|------|----------|
| **ws-ex-dev** | [skills/ws-ex-dev/SKILL.md](./skills/ws-ex-dev/SKILL.md) | 在本 monorepo 改 chat/WS/群/红包/通话等功能；`/ws-ex-dev` |

## 使用方式

- **斜杠**：`/ws-ex-dev`
- **菜单**：`/skills ws-ex-dev`
- **自动**：描述匹配 `description` 时 Grok 会加载该 skill

## 新增技能

1. `mkdir -p .grok/skills/<name>`
2. 编写 `SKILL.md`（YAML frontmatter：`name` + `description`）
3. 在本文件表格中登记一行

```yaml
---
name: my-skill
description: >
  做什么。触发词：…。Use when the user runs /my-skill.
---
```

## 与其它工具的对应

| 工具 | Skills 位置 | 工程化文档 |
|------|-------------|------------|
| Grok | `.grok/skills/` | `.grok/ENGINEERING.md` |
| Cursor | `.cursor/skills/` + `.cursor/rules/` | `.cursor/rules/engineering.mdc` |
| Windsurf | `.windsurf/skills/` | `.windsurf/rules/` |
| Claude | `.claude/skills/` + 根 `CLAUDE.md` | `.claude/ENGINEERING.md` |
