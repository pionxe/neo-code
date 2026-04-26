---
title: MCP 工具接入
description: 把外部 MCP server 接入 NeoCode，让 Agent 安全调用你的工具。
---

# MCP 工具接入

MCP 适合把已有工具接进 NeoCode，例如内部文档搜索、Issue 查询、数据库只读查询或团队自动化脚本。

## 什么时候用 MCP

| 你想做的事 | 建议 |
|---|---|
| 让 Agent 调用一个真实外部工具 | 用 MCP |
| 查询公司文档、任务系统或私有平台 | 用 MCP |
| 只是让 Agent 按固定流程工作 | 用 [Skills](./skills) |
| 只是保存个人偏好或项目事实 | 用 `/remember` 记忆 |

## 最小配置

把 MCP server 写进：

```text
~/.neocode/config.yaml
```

示例：

```yaml
tools:
  mcp:
    servers:
      - id: docs
        enabled: true
        source: stdio
        stdio:
          command: node
          args:
            - ./mcp-server.js
          workdir: ./mcp
        env:
          - name: MCP_TOKEN
            value_env: MCP_TOKEN
```

常用字段：

| 字段 | 说明 |
|---|---|
| `id` | MCP server 的名称，会出现在工具名中 |
| `enabled` | 是否启用 |
| `source` | 当前使用 `stdio` |
| `stdio.command` | 启动 MCP server 的命令 |
| `stdio.args` | 命令参数 |
| `stdio.workdir` | MCP server 的工作目录 |
| `env` | 传给 MCP server 的环境变量 |

::: tip
密钥建议放在系统环境变量里，然后用 `value_env` 引用。不要把 token、API Key 或密码直接写进 `config.yaml`。
:::

## 控制工具可见范围

如果一个 MCP server 暴露了很多工具，可以只开放需要的工具：

```yaml
tools:
  mcp:
    exposure:
      allowlist:
        - mcp.docs.search
      denylist:
        - mcp.docs.delete*
```

常用规则：

| 配置 | 作用 |
|---|---|
| `allowlist` | 只让匹配工具对 Agent 可见 |
| `denylist` | 隐藏匹配工具，优先级更高 |

## 验证是否可用

启动 NeoCode 后，先问：

```text
请列出你当前可用的工具。
```

看到 `mcp.docs.search` 这类工具名后，再让 Agent 做一次真实查询：

```text
请使用 mcp.docs.search 搜索 "release process"，并总结结果。
```

工具参数以你的 MCP server 文档为准。

## 常见问题

### 看不到 MCP 工具

按顺序检查：

- `enabled` 是否为 `true`
- `stdio.command` 在当前终端里是否可执行
- `stdio.workdir` 是否指向正确目录
- `env.value_env` 引用的系统环境变量是否已设置
- `allowlist` 或 `denylist` 是否把工具过滤掉了

### 启动时报环境变量为空

如果配置了：

```yaml
env:
  - name: MCP_TOKEN
    value_env: MCP_TOKEN
```

就必须先在启动 NeoCode 的同一个终端里设置 `MCP_TOKEN`。

### 工具能看到，但调用失败

优先检查 MCP server 自己的日志和工具参数。多数调用失败都来自参数不符合 server 要求，或 server 依赖的外部服务不可用。

## 下一步

- 想控制 Agent 的工作流：[Skills 使用](./skills)
- 想理解权限审批：[工具与权限](./tools-permissions)
- 想查看常见配置：[配置指南](./configuration)
