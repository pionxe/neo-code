---
title: 安装与首次运行
description: 从安装到第一条对话的完整路径，3 分钟跑起来。
---

# 安装与首次运行

## 1. 环境要求

- 操作系统：macOS、Linux 或 Windows
- 终端环境：系统默认终端、PowerShell 或其他兼容终端
- 至少一个可用的 API Key（OpenAI、Gemini、OpenLL、Qiniu 或 ModelScope）

## 2. 安装

### 一键安装（推荐）

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/neo-code/main/scripts/install.sh | bash
```

Windows PowerShell：

```powershell
irm https://raw.githubusercontent.com/1024XEngineer/neo-code/main/scripts/install.ps1 | iex
```

## 3. 设置 API Key

NeoCode 只从环境变量读取 API Key，不会写入配置文件。

macOS / Linux：

```bash
export OPENAI_API_KEY="your_key_here"
```

Windows PowerShell：

```powershell
$env:OPENAI_API_KEY = "your_key_here"
```

其他 Provider 的环境变量名：

| Provider | 环境变量 |
|---|---|
| OpenAI | `OPENAI_API_KEY` |
| Gemini | `GEMINI_API_KEY` |
| OpenLL | `AI_API_KEY` |
| Qiniu | `QINIU_API_KEY` |
| ModelScope | `MODELSCOPE_API_KEY` |

## 4. 启动

```bash
neocode
```

启动后你会看到终端界面，底部是输入框，直接输入文字即可开始对话。

如果要直接打开指定工作区：

```bash
neocode --workdir /path/to/your/project
```

## 5. 第一次对话

不知道从哪里开始？可以先试这两句：

```text
请阅读当前项目目录结构，给出模块职责摘要
```

```text
请根据刚才的项目结构，帮我找出测试入口和主要业务入口
```

Agent 会自动调用文件读取和搜索工具。当它需要写入文件或执行命令时，界面会先弹出权限确认。

## 6. 常用 `/` 命令

进入 NeoCode 后，可用 `/` 命令执行常见操作：

| 命令 | 作用 |
|---|---|
| `/help` | 查看所有命令 |
| `/provider` | 切换 Provider |
| `/provider add` | 添加自定义 Provider |
| `/model` | 切换模型 |
| `/compact` | 压缩长会话上下文 |
| `/clear` | 清空当前草稿 |
| `/cwd [path]` | 查看/切换工作区 |
| `/session` | 切换会话 |
| `/memo` | 查看记忆索引 |
| `/remember <text>` | 保存记忆 |
| `/forget <keyword>` | 删除记忆 |
| `/skills` | 查看可用 Skills |
| `/exit` | 退出 |

## 安装有问题？

看 [排障与常见问题](./troubleshooting)

## 开发者：从源码构建

如果你要阅读代码、调试功能或参与开发，可以从源码构建运行。需要 Go 1.25+ 环境。

```bash
git clone https://github.com/1024XEngineer/neo-code.git
cd neo-code
go build ./...
go run ./cmd/neocode
```

如果你只想稳定使用，优先使用上面的一键安装方式。源码构建更适合阅读代码、调试功能或参与开发。

## 下一步

- 想看更多使用场景：[使用示例](./examples)
- 想切换模型或加自定义 Provider：[配置指南](./configuration)
- 想了解日常操作：[日常使用](./daily-use)
