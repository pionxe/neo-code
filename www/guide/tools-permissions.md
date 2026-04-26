---
title: 工具与权限
description: Agent 能做什么，什么时候需要你确认，怎么选。
---

# 工具与权限

## Agent 能做什么

Agent 通过工具与你的项目交互。只读操作自动执行，写入和命令操作需要你确认。

| 能力 | 工具 | 需要你确认 |
|---|---|---|
| 读取文件 | `filesystem_read_file` | 否 |
| 搜索文件内容 | `filesystem_grep` | 否 |
| 搜索文件路径 | `filesystem_glob` | 否 |
| 写入文件 | `filesystem_write_file` | 是 |
| 编辑文件 | `filesystem_edit` | 是 |
| 执行命令 | `bash` | 视风险而定 |
| 抓取网页 | `webfetch` | 否 |
| 管理任务列表 | `todo_write` | 否 |
| 保存/读取/删除记忆 | `memo_*` | 否 |
| 启动子代理 | `spawn_subagent` | 否 |

通过 MCP 配置注册的外部工具也会出现在列表中，命名空间为 `mcp.<server-id>.<tool>`。接入方式见 [MCP 工具接入](./mcp)。

## 权限审批

当 Agent 请求写入文件或执行命令时，NeoCode 会弹出确认界面：

```text
◆ NEO wants to run: filesystem_write_file
  path: src/main.go
  content: (428 bytes)

  [Allow] [Ask] [Deny]
```

- **Allow**：允许执行，且记住决策——后续相同操作不再询问
- **Ask**：每次都询问（默认）
- **Deny**：拒绝本次执行

### 怎么选

| 你的场景 | 建议 |
|---|---|
| 连续重构，已确认安全 | Allow — 减少打断 |
| 阅读未知仓库，先观察 | Ask — 每次确认 |
| 涉及不想改的目录或高风险命令 | Deny — 直接阻断 |

### Full Access 模式

按 `!` 键可以启用 Full Access，跳过所有权限审批。

::: warning
Full Access 会跳过所有审批，包括破坏性操作。确认你了解风险后再启用。
:::

## 命令风险分类

Bash 命令不是一律审批——NeoCode 会按风险分类处理：

| 分类 | 示例 | 处理方式 |
|---|---|---|
| 只读 | `git status`、`git log`、`ls` | 自动放行 |
| 本地变更 | `git commit`、`go build` | 需要确认 |
| 远端交互 | `git push`、`git fetch` | 需要确认 |
| 破坏性 | `git reset --hard`、`rm` | 需要确认 |
| 无法判断 | 复合命令、解析失败 | 需要确认 |

## 文件操作范围

所有文件操作限制在当前工作区内，路径穿越和符号链接逃逸会被拦截。

## 下一步

- 想配置工具参数：[配置指南](./configuration)
- 想了解日常操作：[日常使用](./daily-use)
- 想接入外部工具：[MCP 工具接入](./mcp)
