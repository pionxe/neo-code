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
- P6：user/repo `command` hooks（stdin/stdout JSON 协议）

当前未实现能力：

- prompt/agent hooks（P6）

## P2 user hooks 边界

P2 仅支持：

- `scope=user`
- `kind=builtin`
- `mode=sync`
- 挂载点：与 `HookPointCapability` 中 `UserAllowed=true` 的点位一致，当前包括：
  `before_tool_call`、`after_tool_result`、`before_completion_decision`、`accept_gate`、`after_tool_failure`、
  `session_start`、`session_end`、`user_prompt_submit`、`post_compact`、`subagent_stop`
- handler：`require_file_exists`、`warn_on_tool_call`、`add_context_note`
- `match`：统一 matcher DSL（字段间 AND、同字段多值 OR），支持：
  - `tool_name`：精确匹配（`string` 或 `[]string`）
  - `tool_name_regex`：正则匹配（`string` 或 `[]string`，单条最长 256）
  - `arguments_contains`：参数预览包含匹配（`[]string`）
- `kind=http + mode=observe`：允许发送 HTTP 观测回调（不支持 block）
- `http observe` 默认不携带 metadata（`include_metadata=false`）；即使显式开启也会剥离 `result_content_preview`、`execution_error`
- `http observe` 回调端点仅允许 loopback 地址（`localhost` / `127.0.0.1` / `::1`），避免误配为公网外发
- `kind=command + mode=sync`：允许执行外部命令，通过 stdin/stdout JSON 协议通信（详见下方 P6 章节）
- external kinds 中 `prompt/agent` 仍显式拒绝

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
- `tool_arguments_preview`（脱敏+截断后的参数预览）
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
- `CanUpdateInput` 在 `user_prompt_submit` 点位已开放：command hook 可通过 stdout JSON 的 `update_input` 字段改写用户输入。
- `UserAllowed=false` 的点位拒绝 user/repo 挂载（配置 fail-fast）。
- matcher 字段会按点位能力矩阵做 fail-fast：不支持的维度会在配置加载阶段直接报错。

### matcher 点位维度矩阵（#684）

| point | tool_name | tool_name_regex | arguments_contains |
|---|---|---|---|
| `before_tool_call` | ✅ | ✅ | ✅ |
| `after_tool_result` | ✅ | ✅ | ❌ |
| `after_tool_failure` | ✅ | ✅ | ✅ |
| `before_permission_decision` | ✅ | ✅ | ❌ |
| 其他点位 | ❌ | ❌ | ❌ |

说明：

- `arguments_contains` 基于 `tool_arguments_preview` 字段匹配，不读取 `tool_arguments` 原文。
- `warn_on_tool_call` 当前要求显式配置 `match`；旧参数 `params.tool_name/tool_names` 不再承担匹配语义。

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

## P6 command hooks

`kind=command` 允许 user/repo scope 通过外部可执行脚本参与 hook 链。

### stdin 协议

外部命令通过 stdin 接收单行 JSON：

```json
{
  "payload_version": "1",
  "hook_id": "my-hook",
  "point": "before_tool_call",
  "run_id": "run_abc123",
  "session_id": "sess_abc123",
  "metadata": {
    "tool_name": "bash",
    "workdir": "/path/to/workspace"
  }
}
```

- `payload_version`：协议版本号，当前固定 `"1"`，变更 stdin 结构时递增
- `hook_id`：hook 配置中的 `id`
- `point`：触发点位名称
- `metadata`：经白名单裁剪后的上下文字段（与 builtin/http hook 相同的 allowlist）

### stdout 协议

外部命令通过 stdout 返回单行 JSON：

```json
{
  "status": "pass",
  "message": "optional message",
  "update_input": {"text": "rewritten prompt"},
  "annotations": ["note1", "note2"]
}
```

- `status`：必填，`pass` / `block` / `failed`
- `message`：可选，进入 hook event 和 annotation buffer
- `update_input`：仅 `CanUpdateInput=true` 的点位（当前仅 `user_prompt_submit`）允许；格式 `{"text": "..."}` 替换用户输入文本
- `annotations`：字符串数组，进入 runtime annotation buffer

### stdout 退化模式

如果 stdout 不是合法 JSON，handler 退化为 exit code 模式：

- exit 0 → `pass`
- exit 1 或 2 → `block`
- 其他 → `failed`

原始 stdout 文本作为 `message`。此模式兼容简单脚本（如 `echo "ok"; exit 0`）。

### 执行模式

#### argv 模式（默认）

`params.command` 为字符串数组，直接 exec 不经 shell：

```yaml
kind: command
params:
  command:
    - python3
    - /path/to/hook.py
```

#### shell 模式

`params.command` 为字符串且 `params.shell: true`，通过 `sh -c`（Unix）/ `powershell -Command`（Windows）执行：

```yaml
kind: command
params:
  command: "python3 /path/to/hook.py"
  shell: true
```

单字符串 `params.command` 不设置 `params.shell: true` 会触发配置校验错误。

### 环境变量

命令进程仅注入以下环境变量，不继承宿主环境：

| 变量 | 值 |
|------|------|
| `NEOCODE_HOOK_HOOK_ID` | hook 的 `id` |
| `NEOCODE_HOOK_POINT` | 触发点位（如 `before_tool_call`） |
| `NEOCODE_HOOK_PAYLOAD_VERSION` | `"1"` |

Windows 额外注入 `SystemRoot`、`SystemDrive`、`USERPROFILE`（从宿主环境读取），以确保 TLS 证书加载和运行时基础功能正常工作。

### 执行约束

- workdir = 当前 run 的 workspace（`cmd.Dir = workdir`）
- 超时 = hook 配置的 `timeout_sec`（默认 2s）
- 并发限制 = executor 的 `max_in_flight`（默认 128）
- repo scope command hook 受 trust gate 保护
- stdout 大小限制 = 1 MiB；超出视为 `failed`

### stderr 处理

外部命令的 stderr 与 stdout 分离捕获。stderr 不会混入 `message` 字段，仅在命令执行失败（非零 exit code）且 stdout 无可用 message 时，stderr 内容才作为 fallback 追加到结果中。此设计确保 hook 协议输出（stdout JSON）不受调试输出（stderr）干扰。

### stdin 字段说明

- `run_id` / `session_id` 同时出现在 payload 顶层和 `metadata` 中。**顶层字段为权威来源**，`metadata` 中的同名字段为冗余副本（与 builtin/http hook 的 metadata allowlist 一致）。外部脚本应优先读取顶层字段。
- `payload_version` 当前固定为 `"1"`，变更 stdin 结构时递增。

### update_input 与 block 交互

当 hook 返回 `status: "block"` 时，`update_input` 不会被应用。阻断优先于输入改写——hook 链在检测到 block 后立即终止，不进入 `applyCommandHookUpdateInput` 逻辑。

### 安全：exit code 优先于 JSON status

当命令以非零 exit code 退出时，stdout 中 JSON 声称的 `status` 字段被忽略。exit code 的映射优先：

- exit 1/2 → `block`
- 其他非零 → `failed`

此规则防止恶意脚本通过 `{"status":"pass"}` 掩盖实际失败。JSON 中的 `message` 和 `annotations` 仍会被提取（如果 stdout 是合法 JSON）。

### 示例

#### Python

```python
#!/usr/bin/env python3
import json, sys

payload = json.loads(sys.stdin.readline())
if payload["metadata"].get("tool_name") == "bash":
    json.dump({"status": "block", "message": "bash not allowed"}, sys.stdout)
else:
    json.dump({"status": "pass"}, sys.stdout)
print()
```

#### Bash

```bash
#!/bin/bash
read -r line
tool=$(echo "$line" | jq -r '.metadata.tool_name // empty')
if [ "$tool" = "rm" ]; then
  echo '{"status":"block","message":"rm is blocked"}'
else
  echo '{"status":"pass"}'
fi
```

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

在 `internal/tui/core/app/update.go` 的 `runtimeEventHandlerRegistry` 中添加对应 handler：

```go
// update.go - runtimeEventHandlerRegistry 中添加：
tuiservices.EventMyNewEvent: runtimeEventMyNewEventHandler,
```

### CI 契约检查

以下测试用例在 CI 中强制执行事件契约一致性：

- `TestRuntimeEventContractConsistency`：扫描 runtime 事件常量，未注册且不在 `legacyPassthroughEvents` 中的事件会导致 CI 失败
- `TestGatewayDecodeBranchConsistency`：验证 gateway decode 分支中的事件都在 contractRegistry 中注册
- `TestRequireConsumerMustHaveDecodeBranch`：验证 `RequireConsumer=true` 的事件必须有 gateway decode 分支
- `TestRequireConsumerMustHaveTUIConsumer`：验证 `RequireConsumer=true` 的事件必须在 `runtimeEventHandlerRegistry` 中有 handler

若 CI 失败，检查以上三步是否遗漏。

### 遗留透传事件

`legacyPassthroughEvents` 是已知的遗留透传事件允许列表，这些事件在 contractRegistry 建立之前已存在，允许不注册。新增的 runtime Event* 常量必须显式注册到 contractRegistry，否则 CI 失败。
