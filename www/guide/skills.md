---
title: Skills 使用
description: 用 SKILL.md 固化当前任务的工作流提示，让 Agent 按指定方式处理任务。
---

# Skills 使用

Skills 是可复用的工作流提示。它不会新增工具，也不会绕过权限；它只是告诉 Agent 这类任务应该怎么做。

如果规则属于整个项目，优先写进 [AGENTS.md](./agents-md)。如果只是你的个人长期偏好，优先用 `/remember`。如果需要调用真实外部工具，使用 [MCP](./mcp)。

## 什么时候用 Skills

| 你想做的事 | 建议 |
|---|---|
| 代码审查时固定先列风险 | 用 Skill |
| 改代码前固定先读测试 | 用 Skill |
| 当前任务需要特定输出结构 | 用 Skill |
| 保存团队项目规则 | 用 `AGENTS.md` |
| 保存个人长期偏好 | 用记忆 |
| 接入外部工具 | 用 MCP |

## Skills 放在哪里

Skills 会按下面顺序加载（项目优先全局）：

```text
<workspace>/.neocode/skills/
~/.neocode/skills/
```

当全局目录 `~/.neocode/skills/` 不存在时，会回退到 `~/.codex/skills/`。

推荐每个 Skill 一个目录：

```text
<workspace>/.neocode/skills/go-review/SKILL.md
~/.neocode/skills/go-review/SKILL.md
```

如果项目和全局存在同名 `id`，会使用项目版本。

## 创建一个 Skill

示例：

```md
---
id: go-review
name: Go Review
description: Review Go changes for correctness, boundaries, and tests.
---

# Go Review

## Instruction

先阅读相关实现和测试，再审查改动。优先关注行为回归、错误处理、边界条件和测试缺口。输出时先列风险，再给简短总结。
```

常用字段：

| 字段 | 说明 |
|---|---|
| `id` | Skill 标识，启用和停用时使用 |
| `name` | 展示名称 |
| `description` | 简短说明，用于列表中识别用途 |

最重要的是 `Instruction`，写清楚“先做什么、重点看什么、输出什么”。

## 启用和停用

在 NeoCode 里使用：

```text
/skills
/skill use go-review
/skill active
/skill off go-review
```

`/skill use <id>` 只影响当前会话。需要每次都生效的长期偏好，更适合写进记忆；需要跟项目一起维护的规则，更适合写进 `AGENTS.md`。

`/skills` 输出中的 `source` 字段会显示来源层级（例如 `project/local`、`global/local`），方便排查覆盖关系。

## 写好 Instruction

不推荐：

```md
## Instruction

请更认真地 review。
```

推荐：

```md
## Instruction

先阅读相关实现和测试，再审查改动。输出时先列高风险问题，再列测试缺口，最后给简短总结。不要要求无关重构。
```

## 常见问题

### `/skills` 看不到我的 Skill

检查：

- 文件是否位于 `~/.neocode/skills/`
- 文件名是否为 `SKILL.md`
- frontmatter 是否是合法 YAML
- `id` 是否重复

### Skill 启用了但效果不明显

把 `Instruction` 写得更具体。不要只写抽象要求，要写检查顺序、关注点和输出结构。

### Skill 能不能授权工具

不能。所有写文件、执行命令等操作仍然按正常权限流程确认。

## 下一步

- 不确定该用什么能力：[能力选择指南](./capability-choice)
- 想接入外部工具：[MCP 工具接入](./mcp)
- 想保存项目级规则：[AGENTS.md 项目规则](./agents-md)
