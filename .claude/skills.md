# ws-ex Claude Skills 索引

> 专案技能：`.claude/skills/<name>/SKILL.md`  
> 工程化：[ENGINEERING.md](./ENGINEERING.md)  
> Claude Code 入口：根目录 [CLAUDE.md](../CLAUDE.md)

## 已安装技能

| 技能 | 路径 | 用途 |
|------|------|------|
| **ws-ex-dev** | [skills/ws-ex-dev/SKILL.md](./skills/ws-ex-dev/SKILL.md) | monorepo 实时聊天全栈开发 |

## Claude Code 用法

- 启动时读取根目录 `CLAUDE.md`
- 技能按需加载：`.claude/skills/*/SKILL.md`
- 与 Grok/Cursor/Windsurf 内容对齐，避免各工具分叉

## 新增技能

```bash
mkdir -p .claude/skills/<name>
# 编写 SKILL.md（YAML: name, description）
```

更新本表与 `CLAUDE.md` 中的 skills 列表（如有）。

## 跨工具对照

| 工具 | Skills | 工程化 |
|------|--------|--------|
| Claude | `.claude/skills/` | `CLAUDE.md` + `.claude/ENGINEERING.md` |
| Grok | `.grok/skills/` | `.grok/ENGINEERING.md` |
| Cursor | `.cursor/skills/` | `.cursor/rules/*.mdc` |
| Windsurf | `.windsurf/skills/` | `.windsurf/rules/` |
