# Hooks CLI

`neocode hook` 提供面向 Runtime Hooks 的本地调试入口：

- `neocode hook lint [path]`
- `neocode hook dry-run [path] --hook <id> --fixture <path>`
- `neocode hook trace --run-id <id>`

## lint

默认扫描两处：

- `~/.neocode/config.yaml` 中的 `runtime.hooks.items`
- `<workspace>/.neocode/hooks.yaml` 中的 `hooks.items`

可显式传入单个文件路径：

```bash
neocode hook lint
neocode hook lint .neocode/hooks.yaml
```

退出码：

- `0`：无问题
- `1`：存在 lint findings
- `2`：读取、解析或内部错误

## dry-run

使用 fixture 驱动单个 hook：

```bash
neocode hook dry-run --hook warn-bash --fixture fixture.yaml
neocode hook dry-run --hook repo-guard --fixture fixture.json --repo
neocode hook dry-run .neocode/hooks.yaml --hook repo-guard --fixture fixture.json
```

fixture 支持 YAML / JSON，字段以 hook payload schema 为准，最小示例：

```yaml
payload_version: "1"
point: before_tool_call
run_id: run-1
session_id: session-1
metadata:
  tool_name: bash
  tool_call_id: call-1
  tool_arguments_preview: echo hello
  workdir: /workspace
```

查找 hook 时的默认行为：

- 默认同时扫描 user hooks 与 repo hooks。
- 同名 hook 同时存在时，优先选择 user hook。
- 若要强制执行 repo hook，可使用 `--repo`，或直接把 repo hooks 文件路径作为 `[path]` 传入。
- fixture 的 `point` 必须与目标 hook 的 `point` 完全一致，否则 `dry-run` 直接失败。

输出中会固定打印：

- `status: pass|block|failed`
- `block: true|false`
- `duration_ms: <n>`
- 命中的 `message` / `annotations`

退出码：

- `0`：结果为 `pass`
- `3`：结果为 `block`
- `4`：结果为 `failed`
- `2`：fixture / hook 解析失败

## trace

在真实 runtime 路径上打开 `--trace-hooks` 后，hook 相关 runtime 事件会持久化到当前 workspace：

```bash
neocode gateway --trace-hooks
neocode hook trace --run-id run_123
```

trace 文件位置：

```text
~/.neocode/projects/<workspace-hash>/hook-traces/<run-id>.jsonl
```

`hook trace` 会按时间顺序回放：

- `hook_started`
- `hook_finished`
- `hook_failed`
- `hook_blocked`

并在末尾输出按 `hook_id` 聚合的简单耗时统计。

补充说明：

- `hook trace --run-id` 只读取当前 workspace 对应目录下的 trace 文件，不做跨项目全局搜索。
- workspace 优先取 `--workdir`，未传时回退到当前进程工作目录。

退出码：

- `0`：成功输出 trace
- `1`：未找到 trace
- `2`：trace 文件损坏或读取失败
