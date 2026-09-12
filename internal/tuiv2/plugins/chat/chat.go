// Package chat 是 TUI v2 的对话流插件（issue #23）：拥有 Stream 与 Runtime 槽，
// 订阅对话类 Gateway 事件就地迁移状态，渲染行为流区域，并提供滚动键位与
// 对话命令。它是第一个消费 kernel 契约的生产级插件。
//
// 槽纪律（ADR-009 / issue #23 修订 v2）：
//   - 写：Stream、Runtime、Layout.ScrollOffset/AutoScroll；
//   - 双轨期临时越权（显式登记，prompt 迁移 PR 时移交）：permission_*、
//     ask_user 系、run_cancelled 六类事件经 state.ReduceConversation 临时写
//     Input 槽；
//   - 读：Gateway（会话/模型）、Mode（不做写入）。
package chat

import (
	"context"
	"time"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Plugin 是对话流插件。
type Plugin struct {
	st       *state.ViewState        // 全局唯一状态（Init 时固定指针，ADR-001）
	stream   *components.AgentStream // 行为流渲染委托（构造一次终身绑定）
	client   gateway.Client          // Gateway 契约（Host.Gateway 透传）
	lastText string                  // 最近一次用户提交文本（/retry 用；prompt 迁移前恒空）
}

// New 创建 chat 插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "chat" }

// Init 固定状态指针、构造渲染委托并记录 Gateway 客户端。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	p.st = h.State()
	p.stream = components.NewAgentStream(p.st)
	p.client = h.Gateway()
}

// Close 释放资源（当前无外部资源，保留生命周期对称性）。
func (p *Plugin) Close(ctx context.Context) {}

// React 订阅广播：仅处理 Gateway 事件（对话白名单 + gateway_offline bespoke），
// 并承接流增长的滚动复位衍生行为。非事件消息一律忽略（prompt 提交等
// 消息类型随对应插件迁移后在此扩展）。
func (p *Plugin) React(h kernel.Host, msg tea.Msg) {
	ev, ok := msg.(gateway.GatewayEvent)
	if !ok {
		return
	}
	before := len(p.st.Stream)
	if ev.Type == gateway.EventGatewayOffline {
		// bespoke 分支（不进 ReduceConversation 白名单）：Runtime/Stream 归 chat，
		// Gateway.Connected 归将来 health 插件，此处不碰。
		p.st.Runtime.Phase = state.RuntimePhaseError
		p.appendError(ev)
	} else {
		state.ReduceConversation(p.st, ev)
	}
	if len(p.st.Stream) > before {
		// 流增长衍生行为（自有字段）：自动跟尾。
		p.st.Layout.AutoScroll = true
		p.st.Layout.ScrollOffset = 0
	}
}

// appendError 追加错误条目（gateway_offline bespoke 专用，替代 Reduce 内联逻辑）。
func (p *Plugin) appendError(ev gateway.GatewayEvent) {
	p.st.Stream = append(p.st.Stream, state.StreamEntry{
		ID:        ev.RunID,
		Type:      "error",
		Timestamp: time.Now(),
		Content:   stringOf(ev.Payload, "message", "error"),
		Metadata:  map[string]any{"done": true},
	})
}

// stringOf 按候选键取字符串（chat 内局部工具，避免引入 state 私有函数）。
func stringOf(payload map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := payload[k].(string); ok {
			return v
		}
	}
	return ""
}

// Region 返回行为流区域。
func (p *Plugin) Region() kernel.RegionID { return kernel.RegionStream }

// Render 委托 AgentStream 渲染（组件读 Layout.Width 为宽度真相源，
// Render 的 width 参数本 PR 忽略——S7 布局包接管断点时统一）。
func (p *Plugin) Render(h kernel.Host, width int) string {
	if p.stream == nil {
		return ""
	}
	return p.stream.View()
}
