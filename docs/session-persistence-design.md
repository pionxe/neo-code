# Session 持久化设计

## 模块职责与边界

- `internal/session` 是会话领域模型、存储抽象与 JSON 持久化实现的唯一归属层。
- `internal/runtime` 负责决定保存时机、恢复会话状态和编排主循环，不承载文件存储细节。
- `internal/tui` 只消费 runtime 暴露的会话数据，不直接读写会话文件。

## 存储策略

NeoCode 当前使用本地 JSON 文件持久化会话，以保持实现简单、可调试且跨平台可移植。

- 默认目录按工作区隔离：`~/.neocode/projects/<workspace-hash>/sessions/`
- 工作区哈希基于启动阶段确定的工作区根目录生成
- `session.Workdir` 表示会话最近一次运行实际使用的目录，由启动 `workdir` 或请求级覆盖值写回，但不参与分桶
- 旧的全局 `~/.neocode/sessions/` 开发期数据不迁移、不回读

## 数据模型

`internal/session.Session` 持久化以下核心字段：

- `schema_version`
- `id`、`title`
- `provider`、`model`
- `created_at`、`updated_at`
- `workdir`
- `task_state`
- `todos`
- `messages`
- `token_input_total`
- `token_output_total`

其中：

- `schema_version` 为开发期强校验字段；当前实现只接受当前版本，不兼容旧 session 文件
- `provider` / `model` 记录最近一次成功运行会话时使用的配置，供 compact 等流程优先复用
- `task_state` 是会话级 durable task state，由 runtime 维护、session 持久化、context 只读投影
- `token_input_total` / `token_output_total` 分别表示会话累计输入与输出 token
- token 字段仍使用 `omitempty`，但不再承担旧版 session JSON 兼容职责

`internal/session.Summary` 只保留会话列表渲染所需的轻量字段，不加载完整消息历史。

`task_state` 固定包含以下字段：

- `goal`
- `progress`
- `open_items`
- `next_step`
- `blockers`
- `key_artifacts`
- `decisions`
- `user_constraints`
- `last_updated_at`

`todos` 固定包含以下要点：
- `id`
- `content`
- `status`
- `dependencies`
- `created_at`
- `updated_at`
- 可选 `priority`

其中 `status` 当前固定为：
- `pending`
- `in_progress`
- `completed`

同时，当 session JSON 缺失 `todos` 字段时，`Load` 会按空 Todo 列表兼容加载。

## 读写行为

- `Save` 使用“临时文件 + 原子替换”写入完整会话 JSON
- `Load` 在用户真正进入某个会话时读取完整历史，并严格要求 `schema_version` 与 `task_state` 字段存在
- `ListSummaries` 只解析摘要字段，并按 `updated_at` 倒序返回；不合法的旧 session 文件会被直接跳过

## Token 计数持久化

- runtime 在 provider 调用完成后更新 session 的累计 token 字段
- 会话保存时，token 计数随 session 一起持久化
- 会话重新加载时，runtime 从 session 恢复累计 token
- 自动 compact 成功后，runtime 会重置累计 token，并将重置后的值持久化

## TaskState 与 compact

- `TaskState` 是继续执行多轮任务时的唯一 durable truth，不依赖聊天消息本身长期保存
- compact 成功后，runtime 会同时回写 `session.TaskState` 和压缩后的 `session.Messages`
- `messages` 中的 `[compact_summary]` 只是展示层，不再是唯一续航载体
- context 构建时会优先注入 `TaskState`，再注入 memo、最近消息和必要工具结果
- 只有当 `TaskState` 已建立后，读时 micro compact 才允许清理旧的可重建 tool payload

## 保存时机

- 用户消息提交后保存
- assistant 完整回复后保存
- 每个工具结果完成后保存
- 避免在高频 UI 刷新路径中直接做磁盘 I/O

## 并发约束

- `internal/session` 的存储实现自行保护共享访问
- 保存时机统一由 runtime 决定，TUI 不直接触发磁盘写入

## 演进约束

- 新增存储实现时，应优先在 `internal/session` 内扩展并通过接口注入
- 不应把持久化逻辑重新分散到 `runtime`、`tui` 或其他上层模块
