// Package chat 是 TUI v2 的对话流插件（issue #23）：拥有 Stream 与 Runtime 槽，
// 订阅对话类 Gateway 事件就地迁移状态，渲染行为流区域，并提供滚动键位与
// 对话命令。它是第一个消费 kernel 契约的生产级插件。
//
// 槽纪律（ADR-009 / issue #23 修订 v2）：
//   - 写：Stream、Runtime（除 AgentMode 子槽）、Layout.ScrollOffset/AutoScroll；
//   - Runtime.AgentMode 子槽写权归 prompt 插件（/mode 命令，issue #41 接线登记）；
//   - 不写 Input 槽（写权归 prompt 插件，经 ApplyInputForEvent）：六类
//     含 Input 部分的事件走 ReduceWithoutInput 跳过（issue #25 移交完成）；
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
// 并承接流增长的滚动复位衍生行为与用户提交记录（UserSubmitted → lastText
// + role=user 流条目，打通 /retry——issue #25 修订 v2 审计 P1-2）。
func (p *Plugin) React(h kernel.Host, msg tea.Msg) {
	switch m := msg.(type) {
	case gateway.GatewayEvent:
		p.handleGatewayEvent(m)
	case state.SessionLoaded:
		// sessions 插件的切换广播：重载 Stream 槽（Detail 历史转条目；
		// 无 Detail = Load 失败，清空并以错误条目提示）。
		p.st.Stream = nil
		if m.Detail != nil {
			for _, item := range m.Detail.Stream {
				p.st.Stream = append(p.st.Stream, state.StreamEntry{
					ID:        item.ID,
					Type:      string(item.Kind),
					Timestamp: item.CreatedAt,
					Content:   item.Text,
					Metadata:  map[string]any{"done": true, "role": item.Role},
				})
			}
		}
		p.st.Layout.AutoScroll = true
		p.st.Layout.ScrollOffset = 0
	case state.UserSubmitted:
		// prompt 插件的提交广播：更新重试文本并追加用户流条目
		//（旧路径 app.go:429 行为等价）。流增长 → 滚动复位
		//（与 gateway 事件流增长同一不变量，审计 P2-①）。
		p.lastText = m.Text
		p.st.Stream = append(p.st.Stream, state.StreamEntry{
			ID:        "user-" + time.Now().Format("150405.000000000"),
			Type:      "message",
			Timestamp: time.Now(),
			Content:   m.Text,
			Metadata:  map[string]any{"done": true, "role": "user"},
		})
		p.st.Layout.AutoScroll = true
		p.st.Layout.ScrollOffset = 0
	case state.SearchJumped:
		// cmdline 插件的搜索跳转意图（issue #41 审计 P1-4 接线）：
		// 经 ScrollToEntry 落滚动——行级定位 + 视口 clamp 的单一真源，
		// 消除 cmdline 旧朴素公式以条目数冒充渲染行数的维度错配。
		// 越界 no-op 由 ScrollToEntry 内建；AutoScroll 语义随组件统一
		// （跳转即固定视口定位并关闭自动跟随，含尾条目）。
		p.stream.ScrollToEntry(m.EntryIndex)
	}
}

// handleGatewayEvent 处理对话类事件：gateway_offline bespoke +
// ReduceWithoutInput 白名单（跳过 Input 槽——Input 写权归 prompt 插件）。
func (p *Plugin) handleGatewayEvent(ev gateway.GatewayEvent) {
	before := len(p.st.Stream)
	if ev.Type == gateway.EventGatewayOffline {
		// bespoke 分支（不进白名单）：Runtime/Stream 归 chat，
		// Gateway.Connected 归 health 插件（S6 移交完成，issue #48），此处不碰。
		p.st.Runtime.Phase = state.RuntimePhaseError
		p.appendError(ev)
	} else {
		state.ReduceWithoutInput(p.st, ev)
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
// Render 的 width 参数透传 stream（S7 真相源统一，issue #50）。
func (p *Plugin) Render(h kernel.Host, width int) string {
	if p.stream == nil {
		return ""
	}
	return p.stream.View(width)
}
