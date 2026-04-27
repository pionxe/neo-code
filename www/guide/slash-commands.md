---
title: Slash 指令
description: 了解 NeoCode 里以 / 开头的本地控制命令，以及每个命令适合什么时候用。
---

# Slash 指令

Slash 指令是 NeoCode 的本地控制命令。它们以 `/` 开头，用来切换模型、管理会话、压缩上下文、保存记忆或打开帮助。

它和普通聊天输入不同：普通输入会交给 Agent 推理，Slash 指令会先由 NeoCode 本地界面处理。比如 `请帮我解释 /compact 的作用` 是普通问题，`/compact` 是立即执行上下文压缩。

## 如何使用

在输入框里输入 `/` 会出现命令建议。继续输入关键字可以筛选，例如 `/pro` 会匹配 Provider 相关命令。

常用方式：

```text
/help
/remember 我习惯先看测试再改代码
```

## 帮助与退出

| 命令 | 用途 | 示例 |
|---|---|---|
| `/help` | 查看当前可用的 Slash 指令 | `/help` |
| `/clear` | 清空当前输入草稿，不影响会话历史 | `/clear` |
| `/exit` | 退出 NeoCode | `/exit` |

## 工作区、会话与上下文

| 命令 | 用途 | 示例 |
|---|---|---|
| `/session` | 打开会话选择器 | `/session` |
| `/compact` | 压缩当前长会话上下文 | `/compact` |

工作区决定 Agent 能读取和修改哪个项目。会话适合承载一个连续任务。上下文变长后，如果回答开始重复、跑偏或混入旧任务，可以先执行 `/compact`。

## 记忆

| 命令 | 用途 | 示例 |
|---|---|---|
| `/memo` | 查看已保存的记忆 | `/memo` |
| `/remember <text>` | 保存长期偏好或稳定事实 | `/remember 本项目测试命令是 go test ./...` |
| `/forget <keyword>` | 删除匹配关键字的记忆 | `/forget powershell` |

记忆适合保存跨会话都成立的信息，不适合保存密钥、临时需求或只对当前任务有用的上下文。

## Skills

| 命令 | 用途 | 示例 |
|---|---|---|
| `/skills` | 查看可用 Skills 和当前会话激活状态 | `/skills` |
| `/skill use <id>` | 在当前会话启用一个 Skill | `/skill use go-review` |
| `/skill off <id>` | 在当前会话停用一个 Skill | `/skill off go-review` |
| `/skill active` | 查看当前会话已启用的 Skills | `/skill active` |

Skills 用来约束当前任务的工作方式，例如“先读测试再改代码”或“审查时先列风险”。它不会新增工具，也不会跳过权限确认。

## Provider 与模型

| 命令 | 用途 | 示例 |
|---|---|---|
| `/provider` | 打开 Provider 选择器 | `/provider` |
| `/provider add` | 添加 OpenAI 兼容的自定义 Provider | `/provider add` |
| `/model` | 打开模型选择器 | `/model` |

Provider 是模型服务来源，例如 OpenAI、Gemini 或自定义兼容服务。模型是该 Provider 下实际使用的模型。API Key 只从环境变量读取，不应写进配置文件。

## 常见误区

- Slash 指令不是提示词。想让 Agent 执行任务时，直接用自然语言描述目标。
- `/clear` 只清空当前输入草稿，不会清空历史会话。
- `/compact` 会压缩上下文，但不能替代清晰的任务描述。压缩后继续任务时，最好补一句当前目标。
- `/remember` 不适合保存密钥、Token、密码或一次性任务要求。

## 下一步

- 想理解工作区和会话：[会话、上下文与工作区](./context-session-workspace)
- 想保存项目规则：[AGENTS.md 项目规则](./agents-md)
- 想判断记忆、Skills、MCP 怎么选：[能力选择指南](./capability-choice)
