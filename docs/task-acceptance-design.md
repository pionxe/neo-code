# Task Acceptance Design

## 主链目标
final acceptance 只保留一条主链：

`session-owned verification profile -> completion gate -> verifier gate -> acceptance decision -> terminal decision`

这条链只负责回答一件事：现在是否可以稳定结束任务。

## 结构化输入
- verifier 集合只由 `session.TaskState.VerificationProfile` 决定。
- `session.TaskState` 与 `session.TodoItem` 是唯一结构化验收输入来源。
- `Acceptance` 只用于人类阅读，不参与机器判定。
- `Artifacts`、`ContentChecks`、`Supersedes` 通过 session 契约驱动 `file_exists`、`content_match` 与 required todo replacement 语义。

## 决策规则
- completion gate 未通过：`continue`
- verifier 首个非 `pass` 为 `soft_block`：`continue`
- verifier 首个非 `pass` 为 `hard_block`：`incomplete`
- verifier 首个非 `pass` 为 `fail`：`failed`
- 全部 verifier `pass`：`accepted`

## Candidate Final
- provider 返回 final 后，usage/provider/model 会先持久化。
- assistant final 只作为 candidate final 暂存在内存。
- 只有 `accepted`、`incomplete`、`failed` 才会把 candidate final 写入 `session.Messages`。
- `continue` 时不会落盘 candidate final，只会追加 reminder。

## 约束
- 不再支持 compatibility fallback。
- 不再支持通过 `runtime.verification.enabled=false` 或 `final_intercept=false` 跳过 verifier gate。
- 不再支持 shell string verifier command。
- 不再支持基于 task 文本的 verifier policy 推断。
