# ws-ex Cursor Skills 索引

> 专案规则：`.cursor/rules/`  
> 专案技能：`.cursor/skills/<name>/SKILL.md`  
> 工程约定（always apply）：[rules/engineering.mdc](./rules/engineering.mdc)

## 已安装技能

| 技能 | 路径 | 用途 |
|------|------|------|
| **ws-ex-dev** | [skills/ws-ex-dev/SKILL.md](./skills/ws-ex-dev/SKILL.md) | monorepo 聊天全栈开发 |

## Cursor 使用说明

- **Rules**：`.cursor/rules/*.mdc` 在匹配 globs / `alwaysApply` 时注入上下文。
- **Skills**：Agent Skills 放在 `.cursor/skills/<name>/SKILL.md`（与 Claude/Grok 同构）。
- 人类可读总览也可看根目录 [AGENTS.md](../AGENTS.md)。

## 新增技能

```bash
mkdir -p .cursor/skills/<name>
# 编写 SKILL.md（name + description frontmatter）
```

并在本表登记。

## 跨工具对照

| 工具 | 位置 |
|------|------|
| Cursor rules | `.cursor/rules/*.mdc` |
| Cursor skills | `.cursor/skills/` |
| Grok | `.grok/skills/` + `.grok/ENGINEERING.md` |
| Windsurf | `.windsurf/skills/` + `.windsurf/rules/` |
| Claude | `.claude/skills/` + `CLAUDE.md` |
