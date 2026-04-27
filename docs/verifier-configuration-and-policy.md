# Verifier Configuration And Policy

## 配置来源
- verifier 的执行参数只来自 `~/.neocode/config.yaml` 中的 `runtime.verification.verifiers.<name>`。
- final acceptance 要运行哪组 verifier，不再由配置里的 task policy 或任务文本推断决定，而是只由 session 持有的 `TaskState.VerificationProfile` 决定。

## VerificationProfile 映射
- `task_only` -> `todo_convergence`
- `create_file` / `docs` -> `todo_convergence`, `file_exists`, `content_match`
- `config` -> `todo_convergence`, `file_exists`, `content_match`, `command_success`
- `edit_code` -> `todo_convergence`, `git_diff`, `build`, `test`, `typecheck`
- `fix_bug` -> `todo_convergence`, `git_diff`, `test`, `build`, `typecheck`
- `refactor` -> `todo_convergence`, `git_diff`, `build`, `test`, `lint`, `typecheck`

## 命令模型
- 命令型 verifier 只接受 `command: ["argv0", "argv1", ...]`。
- runtime 直接执行 argv，不再经过 `powershell -Command`、`sh -lc` 或其他 shell string 兼容层。
- 旧配置里的 string command 只会在“简单空白分隔且不含 shell 语义”时自动迁移；带引号、管道、重定向、子命令替换等写法会被显式拒绝，并要求手工改成 argv。

## 结果语义
- verifier 只允许返回 `pass`、`soft_block`、`hard_block`、`fail`。
- orchestrator 按顺序执行 verifier，并在首个非 `pass` 处短路。
- `pass` 结果不得携带 `ErrorClass`。
- `fail_open`、`fail_closed`、`enabled`、`required` 等旧策略字段已移除，不再对结果做事后改写。

## Loader 与迁移
- `Loader.Load()` 会在 strict decode 前对 verification schema 做内存态预处理，用于删除废弃字段并安全迁移 legacy command string。
- loader 不会自动改写磁盘文件。
- `go run ./cmd/neocode migrate context-budget` 仍是显式落盘升级入口，会保留 `.bak` 备份。
