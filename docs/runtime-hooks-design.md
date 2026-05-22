# Runtime Hooks 设计说明

本文记录 NeoCode runtime hooks 的当前实现边界与约束，确保配置、运行时行为与可观测性一致。

## 当前阶段

当前已实现能力：

- P0：hooks core（registry / executor / timeout / panic recover / failure policy / hook events）
- P1：接入 `before_tool_call`、`after_tool_result`、`before_completion_decision`
- P2：全局 user builtin hooks（`runtime.hooks`）
- P3：repo hooks（`<workspace>/.neocode/hooks.yaml`）+ workspace trust gate（`~/.neocode/trusted-workspaces.json`）
- P4：生命周期点位扩展（permission/session/compact/subagent）+ 点位能力矩阵
- P5：internal hooks 支持 `async/async_rewake` + run 内存通知队列（ephemeral 注入）
- P6-lite：user `http/observe` hooks（仅观测回调）

当前未实现能力：

- command/prompt/agent hooks（P6）

## P2 user hooks 边界

P2 仅支持：

- `scope=user`
- `kind=builtin`
- `mode=sync`
- 挂载点：与 `HookPointCapability` 中 `UserAllowed=true` 的点位一致，当前包括：
  `before_tool_call`、`after_tool_result`、`before_completion_decision`、`accept_gate`、`after_tool_failure`、
  `session_start`、`session_end`、`user_prompt_submit`、`post_compact`、`subagent_stop`
- handler：`require_file_exists`、`warn_on_tool_call`、`add_context_note`
- `kind=http + mode=observe`：允许发送 HTTP 观测回调（不支持 block）
- `http observe` 默认不携带 metadata（`include_metadata=false`）；即使显式开启也会剥离 `result_content_preview`、`execution_error`
- `http observe` 回调端点仅允许 loopback 地址（`localhost` / `127.0.0.1` / `::1`），避免误配为公网外发
- external kinds 中 `command/prompt/agent` 在 P6-lite 阶段显式拒绝，不会半生效

当前（P3）明确不支持：

- user hook 修改 tool 输入或 tool result
- user hook 直接写入 provider-facing prompt
- repo hook 修改 tool 输入或 tool result
- repo hook 直接写入 provider-facing prompt

## P3 repo hooks 边界

repo hooks 文件路径固定为：

```text
<workspace>/.neocode/hooks.yaml
```

仅支持与 P2 相同的 builtin 子集（`kind=builtin`、`mode=sync`、`UserAllowed=true` points、3 个 handlers）。
repo hooks 暂不支持 `kind=http`，external kinds（`command/http/prompt/agent`）在 repo 侧仍显式拒绝。

执行顺序固定为：

```text
internal -> user -> repo
```

冲突规则：

- 同来源内重复 `id`：fail-fast
- 跨来源同 `id`：允许并存（通过 `source` 区分）

## 安全模型

### 上下文裁剪

user/repo hook 接收的 `HookContext` 经过白名单裁剪，仅保留最小必要字段：

- `run_id` / `session_id`
- `point` / `tool_call_id` / `tool_name`
- `is_error` / `error_class`
- `result_content_preview` / `result_metadata_present`
- `execution_error`
- `workdir`

不会暴露：

- API key / capability token
- service 指针与 provider 客户端对象
- 原始工具参数明文（`tool_arguments`）

### 点位能力矩阵（P4）

runtime 内置 `HookPointCapability` 作为唯一真源，定义每个点位是否允许 block/observe/update_input 以及是否允许 user/repo 挂载。

当前点位：

- `before_tool_call`
- `after_tool_result`
- `before_completion_decision`
- `accept_gate`
- `before_permission_decision`
- `after_tool_failure`
- `session_start`
- `session_end`
- `user_prompt_submit`
- `pre_compact`
- `post_compact`
- `subagent_start`
- `subagent_stop`

约束规则：

- `CanBlock=false` 的点位，hook 返回 `block` 会自动降级为观测结果，不中断主链。
- `CanUpdateInput` 仅作为能力建模；当前阶段不开放输入改写通道。
- `UserAllowed=false` 的点位拒绝 user/repo 挂载（配置 fail-fast）。

### trust gate

repo hooks 默认不执行，仅 trusted workspace 会加载执行。

trust store 固定路径：

```text
~/.neocode/trusted-workspaces.json
```

容错行为（统一降级为 untrusted，且不阻断启动）：

- 文件缺失
- 空文件
- JSON 损坏
- 结构不匹配

上述异常会发出事件：`repo_hooks_trust_store_invalid`。

### 路径约束

`require_file_exists` 对 `params.path` 强制执行工作目录边界检查：

- 相对路径按当前运行 workdir 解析
- 绝对路径必须位于 workdir 内
- symlink 路径会进行 realpath 校验，禁止绕过

## 可观测性

runtime 会透传 hooks 生命周期事件：

- `hook_started`
- `hook_finished`
- `hook_failed`
- `hook_blocked`
- `repo_hooks_discovered`
- `repo_hooks_loaded`
- `repo_hooks_skipped_untrusted`
- `repo_hooks_trust_store_invalid`

`hook_finished/hook_failed` 包含 `message` 字段，用于承载 warning/note 文本。  
hook 事件额外携带 `source` 字段；展示层建议使用 `<source>:<id>`。  
user/repo hook 的 `message` 会进入 runtime 的 annotation buffer（运行态内存缓冲），用于后续观测与诊断。

## 示例配置

- 全局 user builtin hooks：`~/.neocode/config.yaml` -> `runtime.hooks.items`
- 仓库级 repo builtin hooks：`<workspace>/.neocode/hooks.yaml`
- 示例文件：`docs/examples/hooks.yaml`

## 失败策略

配置层支持：

- `warn_only`
- `fail_open`
- `fail_closed`

运行时映射：

- `warn_only` -> `fail_open`
- `fail_open` -> `fail_open`
- `fail_closed` -> `fail_closed`

其中 `warn_only/fail_open` 不阻断主链，仅记录失败；`fail_closed` 触发阻断。

## Runtime 事件契约

runtime 事件在三端之间传递，任一端遗漏不会触发编译错误，仅在运行时表现为"事件丢失"或"未知事件被透传"。契约检查器通过 CI 测试强制三端一致性。

### 事件流转路径

```text
runtime (events.go) → gateway protocol encode → gateway_stream_client decode → TUI update handler consume
```

### 新增 runtime event 三步清单

当新增一个 runtime event 时，必须完成以下三步：

**Step 1：定义事件常量与 payload**

在 `internal/runtime/events.go`（或 `events_subagent.go`）中添加 `Event*` 常量和对应的 payload 结构体。

```go
// events.go
const EventMyNewEvent EventType = "my_new_event"

type MyNewEventPayload struct {
    Field string `json:"field"`
}
```

**Step 2：添加 gateway decode 分支**

在 `internal/tui/services/gateway_stream_client.go` 的 `restoreRuntimePayload` 函数中添加对应的 case 分支：

```go
case EventMyNewEvent:
    return decodeRuntimePayload[MyNewEventPayload](payload)
```

同时在 `internal/tui/services/runtime_contract.go` 中：
- 添加 `EventMyNewEvent` 常量定义
- 在 `contractRegistry` 中注册，设置 `RequireConsumer` 为 `true`（需要 TUI 消费）或 `false`（透传安全）

```go
// runtime_contract.go
const EventMyNewEvent EventType = "my_new_event"

// contractRegistry 中添加：
EventMyNewEvent: {RequireConsumer: true},
```

**Step 3：添加 TUI 消费者**

在 `internal/tui/core/app/update.go` 的事件处理 switch 中添加对应分支，确保事件被正确消费。

### CI 契约检查

以下测试用例在 CI 中强制执行事件契约一致性：

- `TestRuntimeEventContractConsistency`：扫描 runtime 事件常量，验证 `RequireConsumer=true` 的事件已注册
- `TestGatewayDecodeBranchConsistency`：验证 gateway decode 分支中的事件都在 contractRegistry 中注册
- `TestRequireConsumerMustHaveDecodeBranch`：验证 `RequireConsumer=true` 的事件必须有 decode 分支

若 CI 失败，检查以上三步是否遗漏。未注册的事件默认允许透传（`RequireConsumer=false`），不会导致 CI 失败。
