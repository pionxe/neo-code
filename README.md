# NeoCode

> 基于 Go + Bubble Tea 的本地 AI Coding Agent

## NeoCode 是什么
NeoCode 是一个在终端中运行的 AI 编程助手，围绕这条主链路工作：

`用户输入 -> Agent 推理 -> 调用工具 -> 获取结果 -> 继续推理 -> UI 展示`

适合希望在本地工作流里完成代码理解、修改、调试与自动化操作的开发者。

## 有什么能力
- 终端原生 TUI 交互体验（Bubble Tea）
- 多 Provider / Model 切换（如 OpenAI、Gemini、OpenLL、Qiniu）
- 工具调用与结果回灌（文件、命令等能力）
- 上下文压缩（`/compact`）与长会话维持
- 工作目录隔离（`--workdir`、`/cwd`）
- 会话持久化与恢复
- Gateway 本地控制面（IPC + HTTP/WS/SSE）

## 怎么用（快速开始）

### 1) 环境要求
- Go `1.25+`
- 可用 API Key（按你使用的 provider 配置环境变量）

### 2) 一键安装
macOS / Linux:
```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/neo-code/main/scripts/install.sh | bash
```

Windows PowerShell:
```powershell
irm https://raw.githubusercontent.com/1024XEngineer/neo-code/main/scripts/install.ps1 | iex
```

### 3) 从源码运行
```bash
git clone https://github.com/1024XEngineer/neo-code.git
cd neo-code
go run ./cmd/neocode
```

启动 Gateway：
```bash
go run ./cmd/neocode gateway
```

指定网络监听地址（默认 `127.0.0.1:8080`）：
```bash
go run ./cmd/neocode gateway --http-listen 127.0.0.1:8080
```

URL Scheme 分发（当前为网关唤醒入口）：
```bash
go run ./cmd/neocode url-dispatch --url "neocode://review?path=README.md"
```

注意：当前 MVP 仅支持 `review` 动作，且必须带 `path` 参数，其他动作会在网关侧被拒绝。

### 4) 首次使用常用命令
- `/help`
- `/provider`
- `/model`
- `/compact`
- `/status`
- `/cwd`
- `/memo`
- `/remember`
- `/forget`
- `& <command>`

## 配置入口
- 主配置：`~/.neocode/config.yaml`
- Provider 配置：`~/.neocode/providers/<provider-name>/provider.yaml`

配置原则：
- API Key 通过环境变量注入，不写入配置文件明文
- `--workdir` 仅影响当前进程，不回写配置

## 文档导航
- [配置指南](docs/guides/configuration.md)
- [扩展 Provider](docs/guides/adding-providers.md)
- [Runtime/Provider 事件流](docs/runtime-provider-event-flow.md)
- [Session 持久化设计](docs/session-persistence-design.md)
- [Context Compact 说明](docs/context-compact.md)
- [Tools 与 TUI 集成](docs/tools-and-tui-integration.md)
- [MCP 配置指南](docs/guides/mcp-configuration.md)
- [更新与升级](docs/guides/update.md)
- [Gateway 详细设计](docs/gateway-detailed-design.md)

## Gateway 运行与安全
- Silent Auth：网关启动时自动读取 `~/.neocode/auth.json`，缺失会自动生成 token
- 认证链路：`Auth -> ACL -> Dispatch`
- 免鉴权端点：`GET /healthz`、`GET /version`
- 需鉴权端点：`GET /metrics`、`GET /metrics.json`（`Authorization: Bearer <token>`）
- 网络端点：`POST /rpc`、`GET /ws`、`GET /sse`

Origin 安全限制：
- 对带 `Origin` 的浏览器请求，默认仅允许 `http://localhost`、`http://127.0.0.1`、`http://[::1]` 和 `app://` 前缀
- 非允许来源会被拦截并返回 `403`
- 不携带 `Origin`（如 cURL/Postman/本地后端脚本）默认放行

## Gateway JSON-RPC 方法（当前实现）
- `gateway.authenticate`
- `gateway.ping`
- `gateway.bindStream`
- `gateway.run`（Accepted-ACK，异步执行）
- `gateway.compact`
- `gateway.cancel`（按 `run_id` 精确取消，`run_id` 必填）
- `gateway.listSessions`
- `gateway.loadSession`
- `gateway.resolvePermission`
- `wake.openUrl`
- `gateway.event`（notification）

## 如何参与
1. 先在 [Issues](https://github.com/1024XEngineer/neo-code/issues) 沟通问题或需求
2. Fork 仓库并创建分支
3. 开发完成后执行：
```bash
gofmt -w ./cmd ./internal
go test ./...
go build ./...
```
4. 提交 PR，说明改动目的、影响范围和验证方式

贡献约束：
- 不提交密钥
- 不提交本地运行数据或临时文件

## License
MIT
