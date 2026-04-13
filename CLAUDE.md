# CLAUDE.md — Claude Code 项目规则

本文件是 Claude Code 在此仓库中工作时的行为指引。完整的项目协作规则请参见 `AGENTS.md`。

## 项目概览

NeoCode Coding Agent MVP — 一个 Go 实现的 AI 编码助手，主链路为：
`用户输入 -> Agent 推理 -> 调用工具 -> 获取结果 -> 继续推理 -> UI 展示`

## 关键目录

| 目录 | 职责 |
|------|------|
| `cmd/neocode` | CLI 入口 |
| `internal/app` | 应用装配与 bootstrap |
| `internal/config` | 配置模型、YAML 加载、校验 |
| `internal/provider` | 模型厂商适配器（差异收敛在此层） |
| `internal/runtime` | ReAct 主循环、事件流、Prompt 编排 |
| `internal/session` | 会话领域模型与 JSON 持久化 |
| `internal/tools` | 工具契约、注册表与具体实现 |
| `internal/tui` | Bubble Tea TUI 状态机与渲染 |
| `internal/context` | 上下文构建、压缩决策 |
| `docs` | 架构与设计文档 |

## 必须遵守的规则

### 架构边界
- **不跨层直连**：遵循 `TUI -> Runtime -> Provider / Tool Manager` 主链路
- **不泄漏厂商差异**：模型协议差异收敛在 `internal/provider` 内
- **不内嵌工具逻辑**：所有可被模型调用的能力必须进入 `internal/tools`
- **不散落状态**：会话状态、消息历史、工具调用记录由 `runtime` 管理

### 编码规范
- Go 惯用风格，制表符缩进，单行约 120 字符
- `PascalCase` 导出，`camelCase` 未导出
- 新增函数必须附带中文注释，说明职责与关键行为
- 不硬编码路径、URL、模型名、超时等，通过配置或常量注入
- 不硬编码业务语义字符串，收敛到共享常量或类型定义

### 安全
- 不写入明文 API Key
- 配置只保存环境变量名
- `filesystem` 工具限制在工作目录内
- `bash` 工具限制超时与输出长度
- 本地运行数据、会话数据不入库

### 测试
- 整体测试覆盖率以 **100%** 为硬性目标
- 改动必须同步补齐测试：正常路径 + 边界条件 + 异常分支 + 回归场景
- 优先覆盖：配置校验、provider 转换、tool 参数校验、runtime 停止条件、事件派发

### 文档
- 沿用目标文档已有语言（中文为主则续用中文）
- 实现与文档冲突时必须修正至少一个

### 提交前检查
- 确认职责分工未被破坏
- 确认新增能力接到正确层级
- `go build ./...` && `go test ./...` && `gofmt -w ./cmd ./internal`
- 检查 `git status`，确保无密钥或临时文件混入

## 常用命令

```bash
go run ./cmd/neocode       # 启动应用
go build ./...              # 编译
go test ./...               # 运行测试
gofmt -w ./cmd ./internal   # 格式化
```
