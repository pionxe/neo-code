# Context Compact

本文说明 NeoCode 当前的上下文压缩策略、预算触发链路和 compact 协议。

## 总览

当前 compact 只承担“压缩 transcript”的职责，不再负责预算判断。预算控制已独立为 `context.budget`：

- `manual`：用户通过 `/compact` 主动触发
- `proactive`：发送前输入预算超限时触发
- `reactive`：provider 返回 `context_too_long` 时触发

三种模式共用同一条 compact 执行管线，但触发源不同。

## 配置

```yaml
context:
  compact:
    manual_strategy: keep_recent
    manual_keep_recent_messages: 10
    micro_compact_retained_tool_spans: 6
    read_time_max_message_spans: 24
    max_summary_chars: 1200
    micro_compact_disabled: false
  budget:
    prompt_budget: 0
    reserve_tokens: 13000
    fallback_prompt_budget: 100000
    max_reactive_compacts: 3
```

### `context.compact`

- `manual_strategy`
  控制手动 compact 的策略，支持 `keep_recent` 和 `full_replace`。
- `manual_keep_recent_messages`
  在 `keep_recent` 模式下保留的最近消息数，并按 tool call / tool result 的原子块整体保留。
- `read_time_max_message_spans`
  控制 `context.Builder` 读时 trim 可保留的 message span 上限。
- `micro_compact_retained_tool_spans`
  控制 read-time micro compact 默认保留原始内容的最近可压缩工具块数量。
- `max_summary_chars`
  控制 compact summary 的最大字符数。
- `micro_compact_disabled`
  控制是否关闭默认启用的 read-time micro compact。

### `context.budget`

- `prompt_budget`
  显式输入预算；`> 0` 时直接使用，`0` 表示自动推导。
- `reserve_tokens`
  自动推导预算时，为输出、tool call、system prompt 预留的缓冲。
- `fallback_prompt_budget`
  模型窗口不可用时的保底输入预算。
- `max_reactive_compacts`
  单次 run 内 reactive compact 的最大尝试次数。

## 预算闭环

当前发送链路固定为：

```text
BuildRequest -> FreezeSnapshot -> EstimateInput -> DecideBudget -> (allow | compact | stop)
```

关键规则：

- `context.Builder` 只构建 provider-facing request，不再返回旧的 builder 压缩建议布尔值。
- provider 发送前一定先做输入 token estimate。
- estimate 首次超预算时，runtime 执行一次 `proactive` compact，然后重建 request 并重新估算。
- compact 后仍超预算且 `gate_policy=gateable` 时，runtime 停止本次 run，并返回 `STOP_BUDGET_EXCEEDED`。
- compact 后仍超预算但 `gate_policy=advisory` 时，runtime 继续发送请求，不直接硬停。
- provider 返回 `context_too_long` 时，runtime 触发 `reactive` compact，并重新进入同一预算闭环。

## compact 如何压缩

compact runner 会先写入完整 transcript，再生成 durable `TaskState` 与面向人类阅读的 `display_summary`。

自动链路下的保留规则固定为：

- 最近一条显式用户消息所在 span 永远保留原文
- 最近尾部消息原样保留
- 更早历史归档为一条 `[compact_summary]`

这意味着：

- 当前轮用户刚输入的问题不会被摘要替换
- 被压缩的是更早的历史消息，而不是当前交互的最近尾部

## 执行链路

1. TUI 识别 `/compact` 并调用 `runtime.Compact(...)`。
2. runtime 发出 `compact_start` 事件。
3. compact runner 写入原始 transcript。
4. compact runner 根据策略构造归档消息与保留消息，并过滤旧 `[compact_summary]`，避免“摘要的摘要”。
5. summary generator 调用模型生成完整 `task_state` 与 `display_summary`。
6. runner 校验 summary 结构与长度，必要时截断，并更新 `task_state.last_updated_at`。
7. compact 成功后，runtime 回写会话消息与 `TaskState`，重置 token totals 和 `HasUnknownUsage`，并发出 `compact_applied`。
8. compact 失败时发出 `compact_error`。

## 生成协议

compact generator 必须只返回一个 JSON 对象，顶层固定包含：

```json
{
  "task_state": {
    "goal": "",
    "progress": [],
    "open_items": [],
    "next_step": "",
    "blockers": [],
    "key_artifacts": [],
    "decisions": [],
    "user_constraints": []
  },
  "display_summary": "[compact_summary]\n..."
}
```

约束：

- `task_state` 表示 compact 后的完整 durable task state，不是增量 patch
- `task_state` 只允许固定字段
- `display_summary` 必须使用 `[compact_summary]` 协议

`display_summary` 结构如下：

```text
[compact_summary]

done:
- ...

in_progress:
- ...

decisions:
- ...

code_changes:
- ...

constraints:
- ...
```

## 事件

compact 相关 runtime 事件包括：

- `compact_start`
- `compact_applied`
- `compact_error`

`compact_applied` payload 包含：

- `applied`
- `before_chars`
- `before_tokens`
- `after_chars`
- `saved_ratio`
- `trigger_mode`
- `transcript_id`
- `transcript_path`
