---
title: 工具与权限
description: Agent 能做什么，什么时候需要你确认，Allow、Ask、Deny 怎么选。
---

# 工具与权限

NeoCode 通过工具与你的项目交互。只读操作通常自动执行；写入文件、编辑文件或执行有风险的命令时，会先让你确认。

## Agent 能做什么

| 能力 | 工具 | 通常是否需要确认 |
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

## 权限确认

当 Agent 请求写入文件或执行命令时，NeoCode 会弹出确认界面：

```text
◆ NEO wants to run: filesystem_write_file
  path: src/main.go
  content: (428 bytes)

  [Allow] [Ask] [Deny]
```

| 选择 | 含义 | 适合场景 |
|---|---|---|
| Allow | 允许执行，并记住相同操作的决策 | 已确认安全的连续修改 |
| Ask | 本次按默认审批流程处理，后续仍询问 | 默认选择，适合多数任务 |
| Deny | 拒绝本次执行 | 路径不对、命令危险、范围失控 |

## 怎么判断

| 场景 | 建议 |
|---|---|
| 读取文件、搜索代码 | 通常可以放行 |
| 写测试、改小范围代码 | Ask，确认路径后允许 |
| 运行项目已有测试命令 | 通常可以允许 |
| 删除文件、重置 Git、批量改写 | 要求解释，确认后再决定 |
| 涉及密钥、本地配置或不想改的目录 | Deny |

## Full Access 模式

按 `Ctrl+F` 可以进入 Full Access 风险确认流程。启用后，工具审批会自动通过。

::: warning
Full Access 会跳过审批。只在你明确了解当前任务风险、信任工作区内容、并能接受文件或命令副作用时使用。
:::

## 命令风险分类

命令不是一律审批。NeoCode 会按风险分类处理：

| 分类 | 示例 | 处理方式 |
|---|---|---|
| 只读 | `git status`、`git log`、`ls` | 自动放行 |
| 本地变更 | `git commit`、`go build` | 需要确认 |
| 远端交互 | `git push`、`git fetch` | 需要确认 |
| 破坏性 | `git reset --hard`、`rm` | 需要确认 |
| 无法判断 | 复合命令、解析失败 | 需要确认 |

## 文件操作范围

文件操作默认限制在当前工作区内，路径穿越和符号链接逃逸会被拦截。

当前工作区可以通过 Slash 指令查看：

```text
/cwd
```

## 下一步

- 想了解日常流程：[日常使用](./daily-use)
- 想理解 Slash 指令：[Slash 指令](./slash-commands)
- 想接入外部工具：[MCP 工具接入](./mcp)
