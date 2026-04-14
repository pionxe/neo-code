# Session Todo 设计说明

本文档补充说明 `internal/session` 中 Todo 的数据模型、持久化语义与边界约束。

## 设计目标

- Todo 归属于 `Session`，不单独引入新的持久化子系统。
- Todo 只表示结构化待办状态，不替代现有 `TaskState`。
- Todo 的校验、规范化和基础增删改查统一收敛在 `internal/session`。

## 数据模型

`Session` 新增 `todos` 字段，对应 `[]TodoItem`。

单个 `TodoItem` 目前包含：

- `id`
- `content`
- `status`
- `dependencies`
- `created_at`
- `updated_at`
- 可选 `priority`

其中 `status` 固定为以下三个值：

- `pending`
- `in_progress`
- `completed`

## 持久化语义

- Todo 跟随 `Session` 一起通过现有 JSONStore 保存和加载。
- `Save` 前会对 Todo 执行统一规范化与校验：
  - `id`、`content` 去空白
  - 空状态默认收敛为 `pending`
  - `dependencies` 去空白、去重、保持顺序
  - 拒绝重复 ID
  - 拒绝自依赖
  - 拒绝引用不存在的依赖项
- `Load` 允许 session JSON 缺失 `todos` 字段，并按空 Todo 列表处理。

## 与 TaskState 的关系

- `TaskState` 仍是 runtime/context 用于 compact 与续航的 durable summary。
- `Todo` 是更细粒度的结构化状态，不直接注入 context，不写入消息历史。
- 如果未来需要收敛两者关系，应通过单独演进，让 `TaskState` 从 `Todo` 派生摘要，而不是直接复用同一字段。
