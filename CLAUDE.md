# CLAUDE.md — Claude Code 项目指引

本文件是 Claude Code 在本仓库中工作的快速入口。完整且最高优先级的协作规则以
`AGENTS.md` 为准；如果两份文档出现冲突，先遵守 `AGENTS.md`，再同步修正本文。

## 项目定位

NeoCode 是一个 Go 实现的编码 Agent。当前架构已经完成控制面与数据面拆分，主链路必须保持为：

```text
用户输入(TUI) -> 网关中继(Gateway) -> Agent 推理(Runtime) -> 工具调用(Tools) -> 结果回传 -> UI 展示
```

做任何修改时，优先确认主链路仍可运行、职责边界仍清晰、行为可通过测试验证。

## 当前实现重点

- `internal/tui` 只负责 Bubble Tea 状态机、渲染和事件消费，不直接调用 provider，不执行工具。
- `internal/gateway` 负责协议路由、JSON-RPC 归一化、权限与事件中继，是 TUI 与 Runtime 的边界。
- `internal/runtime` 负责 ReAct 循环、事件派发、工具调用编排、token ledger、compact 触发和停止条件。
- `internal/context` 负责 system prompt、AGENTS.md、Task State、Todo State、Skills、Memo、消息裁剪和 micro compact。
- `internal/provider` 只收敛模型厂商差异，包括请求组装、流式解析、usage 读取和输入 token 估算。
- `internal/tools` 负责工具 schema、注册表、参数校验、执行和结果格式。
- `internal/session` 负责会话领域模型以及 JSON / SQLite 持久化。
- `internal/config` 负责配置加载、校验、provider/model 选择、环境变量名和 context budget 配置。
- `internal/app` 只做应用装配与依赖注入，不承载业务规则。

## 架构边界

- 不要跨层直连；新增能力默认沿 `TUI -> Gateway -> Runtime -> Provider / Tool Manager` 接入。
- 不要把 provider 厂商字段、错误格式或协议细节泄漏到 runtime、tui 或上层调用方。
- 不要在 TUI 或 Runtime 中内嵌可被模型调用的具体工具逻辑；工具能力必须进入 `internal/tools`。
- 不要把会话状态、消息历史、工具调用记录散落到 UI；这些状态由 runtime/session 管理。
- 新设计已确定时，不要为了“可能兼容旧版本”保留旧分支、旧协议兜底或碎片化实现。

## Budget 与 Compact

当前上下文预算统一使用 `context.budget`，不再使用旧的 `context.auto_compact` 运行时语义。

- `context.compact` 只描述 compact 策略和 read-time micro compact 行为。
- `context.budget` 描述 prompt budget、reserve tokens、fallback budget 和 reactive compact 次数。
- runtime 在发送 provider 请求前冻结 turn snapshot，并调用 provider 的 `EstimateInputTokens`。
- budget 控制面的闭环是：

```text
BuildRequest -> FreezeSnapshot -> EstimateInput -> DecideBudget -> allow | compact | stop
```

- 首次超预算时执行 `proactive` compact；compact 后仍超预算时停止本次 run，并发出
  `STOP_BUDGET_EXCEEDED`。
- provider 返回 `context_too_long` 时进入 `reactive` compact，再重新进入预算闭环。
- context builder 不负责预算判断，也不返回旧的 auto compact 建议。

## Runtime 事件协议

TUI 只消费当前 runtime/gateway 事件协议。停止原因使用：

- `STOP_COMPLETED`
- `STOP_USER_INTERRUPT`
- `STOP_FATAL_ERROR`
- `STOP_BUDGET_EXCEEDED`

预算和 token 相关事件包括：

- `budget_checked`
- `ledger_reconciled`
- `token_usage`

`token_usage` 需要携带本轮 usage、来源标签、会话累计值和 `has_unknown_usage`。不要复用旧的
`usage` 协议，也不要在 TUI 中猜测 provider usage 语义。

## 配置与安全

- 主配置路径为 `~/.neocode/config.yaml`。
- API Key 只通过环境变量读取，配置文件只保存环境变量名，不保存明文密钥。
- `workdir` 通过启动参数或运行时上下文传入，不写入主配置。
- custom provider 使用 `~/.neocode/providers/<provider-name>/provider.yaml`。
- `context.auto_compact` 只允许被迁移到 `context.budget`；主解析器仍只接受当前结构。
- `filesystem` 工具必须限制在工作目录内。
- `bash` 工具必须限制超时、输出长度，并避免交互式阻塞命令。
- `webfetch` 工具必须限制协议范围、响应大小和内容类型。

## 修改代码时

- 先判断改动属于 config、context、runtime、provider、tools、session、gateway、tui 还是 app。
- 优先做最小闭环改动，不做无关重构。
- 涉及 provider 协议差异时，只改 provider 层或其明确契约。
- 涉及模型可调用能力时，先补 tools 契约，再由 runtime 接入。
- 涉及配置结构、事件协议、目录职责或命令时，同步更新 `docs/`、`README.md` 或本文。
- 非测试文件新增函数时，紧邻函数定义写中文注释，说明职责和关键行为。
- 所有中文文件按 UTF-8 无 BOM 读写，发现乱码先确认编码再修改。

## 测试重点

改动应同步补测试，优先覆盖：

- 配置默认值、校验、迁移和错误包装。
- provider 请求组装、stream 解析、tool call 解析、usage 与错误映射。
- runtime 最大轮数、停止原因、tool result 回灌、compact、budget、token ledger 和事件派发。
- context build 输入输出契约、AGENTS.md 加载、micro compact、消息裁剪边界。
- tools 参数校验、权限、超时、输出裁剪和错误格式。
- session JSON / SQLite 持久化、schema migration、token totals 和 `HasUnknownUsage`。
- TUI 对 gateway/runtime 当前事件协议的映射和展示状态。

## 常用命令

```bash
go run ./cmd/neocode
go build ./...
go test ./...
gofmt -w ./cmd ./internal
```

## 提交前检查

- 主链路仍是 `TUI -> Gateway -> Runtime -> Tools -> UI`。
- 新增能力没有跨层接线，也没有把厂商差异泄漏到上层。
- 配置、事件、schema 和文档使用当前结构，不保留旧协议兜底。
- 测试覆盖本次改动的正常路径、边界条件、异常分支和回归场景。
- `git status` 中没有密钥、本地配置、临时目录或无关文件混入。
