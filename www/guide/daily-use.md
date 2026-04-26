---
title: 日常使用
description: 工作区、会话、记忆、Skills 和子代理——每天会用到的操作。
---

# 日常使用

## 工作区

工作区决定 NeoCode 能读取和修改哪个项目。

```text
/cwd                     # 查看当前工作区
/cwd /path/to/project    # 切换到另一个项目
```

也可以启动时指定：

```bash
neocode --workdir /path/to/project
```

## 会话管理

### 切换会话

```text
/session                 # 打开会话选择器
```

### 压缩长会话

对话很长时，旧上下文可能干扰当前任务。先执行：

```text
/compact
```

### 什么时候新建会话

| 场景 | 建议 |
|---|---|
| 开始一个不相关的新任务 | 新建会话 |
| 继续完善刚才的功能 | 继续当前会话 |
| 切换到另一个项目 | 新建会话并切换工作区 |
| 回复开始重复或跑偏 | 先 `/compact`，不行再新建会话 |

## 记忆

记忆适合保存跨会话都成立的偏好或项目事实。

```text
/memo                              # 查看所有记忆
/remember 我习惯用 powershell       # 保存一条记忆
/forget powershell                  # 删除匹配的记忆
```

适合保存：

- 你常用的 Shell、测试命令或代码风格偏好
- 当前项目的固定事实，例如主要语言、包管理器、测试入口
- 不希望每次重复说明的个人偏好

不适合保存：

- 临时任务要求
- 密钥、Token 或密码
- 只对当前会话有效的上下文

## Skills

Skills 用来让 Agent 在当前会话按特定流程工作，例如“先读测试再改代码”或“审查时先列风险”。

常用命令：

```text
/skills                  # 查看可用 Skills
/skill use go-review     # 启用某个 Skill
/skill off go-review     # 停用某个 Skill
/skill active            # 查看当前已启用的 Skills
```

一句话判断：

- 长期偏好或项目事实：用记忆
- 当前任务的工作方式：用 Skill
- 真实外部工具能力：用 [MCP](./mcp)

## 子代理

复杂任务里，NeoCode 可能会让子代理并行处理搜索、审查或验证。你通常不需要手动管理它们。

如果你希望它主动拆分任务，可以直接说：

```text
请先用 researcher 角色梳理相关代码，再让 reviewer 角色检查方案风险。
```

## 常用命令速查

| 命令 | 作用 |
|---|---|
| `/help` | 查看所有命令 |
| `/provider` | 切换 Provider |
| `/model` | 切换模型 |
| `/cwd` | 查看或切换工作区 |
| `/session` | 切换会话 |
| `/compact` | 压缩长会话 |
| `/memo` | 查看记忆 |
| `/remember <text>` | 保存记忆 |
| `/forget <keyword>` | 删除记忆 |
| `/skills` | 查看 Skills |
| `/exit` | 退出 |

## 下一步

- 想配置模型和 Provider：[配置指南](./configuration)
- 想了解 Agent 能做什么、权限怎么选：[工具与权限](./tools-permissions)
- 想编写或启用 Skill：[Skills 使用](./skills)
- 遇到问题：[排障与常见问题](./troubleshooting)
