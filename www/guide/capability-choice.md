---
title: 能力选择指南
description: 判断什么时候用记忆、AGENTS.md、Skills 或 MCP。
---

# 能力选择指南

NeoCode 有几种容易混淆的能力：记忆、`AGENTS.md`、Skills 和 MCP。它们解决的问题不同。

## 一句话判断

| 你想做的事 | 用什么 |
|---|---|
| 保存个人长期偏好 | 记忆 |
| 让整个项目长期遵守规则 | `AGENTS.md` |
| 让当前任务按固定流程执行 | Skills |
| 让 Agent 调用真实外部工具 | MCP |

## 记忆

记忆适合保存跨会话都成立的个人偏好或稳定事实。

```text
/remember 我习惯使用 powershell
/remember 本项目测试命令是 go test ./...
```

不要用记忆保存密钥、临时需求或一次性上下文。

## AGENTS.md

`AGENTS.md` 适合写项目级规则，跟仓库一起维护。它对参与这个项目的 Agent 都有帮助。

```md
# Project Rules

- 中文文档继续使用中文
- 修改 Go 代码后运行 `go test ./...`
- 不要把 API Key 写入配置文件
```

如果规则只对你个人成立，用记忆更合适。如果规则只对当前任务成立，直接在会话里说或启用 Skill 更合适。

## Skills

Skills 适合约束当前任务的工作方式。例如代码审查时先列风险，或修改前先读测试。

```text
/skills
/skill use go-review
/skill active
/skill off go-review
```

Skill 不会新增工具，也不会绕过权限确认。

## MCP

MCP 适合接入真实外部工具，例如内部文档搜索、Issue 查询、数据库只读查询或团队自动化脚本。

如果你只是想让 Agent 按固定流程工作，不需要 MCP。MCP 更适合“Agent 必须调用某个外部能力才能完成任务”的场景。

## 常见选择

| 场景 | 推荐 |
|---|---|
| “以后都默认用中文回复我” | 记忆 |
| “这个仓库不要提交 API Key” | `AGENTS.md` |
| “这次 review 先列风险再总结” | Skill 或当前会话说明 |
| “查询公司内部文档系统” | MCP |
| “当前 bug 只改一个文件” | 当前会话说明 |

## 下一步

- 保存个人偏好：[日常使用](./daily-use)
- 编写项目规则：[AGENTS.md 项目规则](./agents-md)
- 使用 Skills：[Skills 使用](./skills)
- 接入外部工具：[MCP 工具接入](./mcp)
