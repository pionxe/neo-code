// Package sessions 是 TUI v2 的会话管理插件（issue #27 S3-2）：
// 拥有 Gateway.Sessions/ActiveSess 子槽，提供会话切换/新建/删除、
// 会话选择器浮层，并订阅 GatewayRecovered 做断连恢复重绑
// （S6 起 health_changed 的 Connected 写入已移交 health 插件）。
//
// 事件流重绑定：LoadSession 后经 Host.BindEventStream 换代订阅
// （重绑定协议：代际号使旧泵产物失效，旧通道由本插件 cancel 关闭）。
package sessions

import (
	"context"
	"fmt"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Plugin 是会话管理插件。
type Plugin struct {
	st        *state.ViewState          // 全局唯一状态（Init 时固定指针，ADR-001）
	client    gateway.Client            // Gateway 契约
	picker    *components.SessionPicker // 会话选择器渲染/交互委托
	prevID    string                    // 上一会话 ID（Space Space 切换用，非渲染态）
	streamCtx context.CancelFunc        // 当前事件订阅的取消函数（换代时关闭旧通道）
}

// New 创建会话插件实例。
func New() *Plugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return "sessions" }

// Init 固定状态指针、构造选择器委托并记录客户端。
func (p *Plugin) Init(ctx context.Context, h kernel.Host) {
	p.st = h.State()
	p.client = h.Gateway()
	p.picker = components.NewSessionPicker(p.st)
}

// Close 释放当前事件订阅（若存在）。
func (p *Plugin) Close(ctx context.Context) {
	p.closeStream()
}

// closeStream 取消当前事件订阅上下文（旧通道关闭 → 旧泵返回陈旧 closed 被丢弃）。
func (p *Plugin) closeStream() {
	if p.streamCtx != nil {
		p.streamCtx()
		p.streamCtx = nil
	}
}

// sessionStreamReadyMsg 是 LoadSession+重订阅的 RPC 产物（内部消息）：
// 闭包只做 RPC 与通道建立，Host 副作用（closeStream/BindEventStream/
// Notify/PopOverlay）全部回到 React 路径执行（P0-1 并发契约修复）。
type sessionStreamReadyMsg struct {
	session gateway.SessionSummary
	detail  *gateway.SessionDetail // nil = Load 失败
	ch      <-chan gateway.GatewayEvent
	cancel  context.CancelFunc // 新流订阅的取消函数（React 收尾时登记）
	loadErr error
}

// recoveryStreamReadyMsg 是恢复重绑的订阅产物（无 Load/无广播——守卫④）。
type recoveryStreamReadyMsg struct {
	ch     <-chan gateway.GatewayEvent
	cancel context.CancelFunc
}

// recoveryRebindFailedMsg 是恢复重绑订阅失败（经 Notify 呈现）。
type recoveryRebindFailedMsg struct{ err error }

// React 订阅广播：Gateway 域事件（ApplyGatewayForEvent）、picker 产出消息
// （选择/删除）、确认结果（删除执行）。
func (p *Plugin) React(h kernel.Host, msg tea.Msg) {
	switch m := msg.(type) {
	case gateway.GatewayEvent:
		// 精确化（审计 P2-①）：sessions 写 session_* 三类；model_changed
		// 归 models；health_changed 归 health 插件（S6 移交，issue #48）。
		switch m.Type {
		case gateway.EventSessionCreated, gateway.EventSessionDeleted,
			gateway.EventSessionUpdated:
			state.ApplyGatewayForEvent(p.st, m)
			if m.Type == gateway.EventSessionDeleted {
				h.Notify("会话已删除")
			}
		}
	case components.SessionSelectMsg:
		p.handleSelect(h, m)
	case state.ConfirmResult:
		if m.Action == "delete_session" && m.Yes {
			if id, ok := m.Data["id"].(string); ok {
				h.Send(state.SessionDeleted{ID: id})
			}
		}
	case state.SessionDeleted:
		// 本地合成事件由本插件消费（复用 reducer 既有分支，不新写移除逻辑）。
		state.ApplyGatewayForEvent(p.st, gateway.GatewayEvent{
			Type:    gateway.EventSessionDeleted,
			Payload: map[string]any{"id": m.ID},
		})
		h.Notify("会话已删除")
	case sessionStreamReadyMsg:
		// 换代收尾（Update 循环内执行 Host 副作用）：先关旧流再绑新流；
		// Load 失败（Detail=nil）经 Notify 呈现，不绑定新流。
		// （浮层已在 enter 时关闭——弹栈语义统一在 HandleKey。）
		p.closeStream()
		p.streamCtx = m.cancel
		if m.ch != nil {
			h.BindEventStream(m.ch)
		}
		if m.loadErr != nil {
			h.Notify("切换会话失败：" + m.loadErr.Error())
		}
		h.Send(state.SessionLoaded{Session: m.session, Detail: m.detail})
	case state.GatewayRecovered:
		// health 插件恢复广播（fail→success 边沿）：断连期间事件流死亡，
		// 重绑当前活跃会话的订阅。四守卫（S6 审计裁定，issue #48）：
		// ①边沿由 health 保证；②无活跃会话跳过；③运行态跳过（重绑换窗
		// 期在途事件丢失）；④重绑不广播 SessionLoaded（恢复不得清空
		// 对话流——与换会话语义分离，故不复用 sessionStreamReadyMsg）。
		if p.st.Gateway.ActiveSess == nil {
			return
		}
		if p.st.Runtime.Phase == state.RuntimePhaseRunning {
			return
		}
		if p.client == nil {
			return
		}
		sessionID := p.st.Gateway.ActiveSess.ID
		client := p.client
		h.GoCmd(func() tea.Msg {
			subCtx, cancel := context.WithCancel(context.Background())
			eventCh, subErr := client.SubscribeEvents(subCtx, sessionID)
			if subErr != nil {
				cancel()
				return recoveryRebindFailedMsg{err: subErr}
			}
			return recoveryStreamReadyMsg{ch: eventCh, cancel: cancel}
		})
	case recoveryStreamReadyMsg:
		// 恢复重绑收尾：换流但不广播 SessionLoaded（守卫④）。
		p.closeStream()
		p.streamCtx = m.cancel
		h.BindEventStream(m.ch)
	case recoveryRebindFailedMsg:
		// 恢复重绑失败经 Notify 呈现（对齐 sessionStreamReady 失败路径）。
		h.Notify("事件流重绑失败：" + m.err.Error())
	case components.SessionDeleteMsg:
		// 危险操作走内核确认服务（ADR-012），结果按 ID 回流。
		title := m.SessionID
		if sess := p.st.Gateway.ActiveSess; sess != nil && sess.ID == m.SessionID {
			title = sess.Title
		}
		h.Confirm(state.ConfirmRequest{
			Title:   "⚠ 删除会话",
			Message: fmt.Sprintf("确定删除 %s？此操作不可撤销。", title),
			Action:  "delete_session",
			Data:    map[string]any{"id": m.SessionID},
		})
		h.PopOverlay() // 进入确认流程，选择器关闭（P0-2 弹栈语义）
	}
}

// handleSelect 处理会话选择：更新活跃会话 → 闭包执行 LoadSession + 新流
// 订阅（**只返回 Msg，无任何 Host 副作用**——P0-1 并发契约）→
// React 的 sessionStreamReadyMsg 分支完成换代收尾与广播。
func (p *Plugin) handleSelect(h kernel.Host, msg components.SessionSelectMsg) {
	if p.client == nil {
		return
	}
	if current := p.activeSessionID(); current != "" && current != msg.Session.ID {
		p.prevID = current
	}
	p.st.Gateway.ActiveSess = &msg.Session
	sessionID := msg.Session.ID
	client := p.client
	h.GoCmd(func() tea.Msg {
		detail, loadErr := client.LoadSession(context.Background(), sessionID)
		if loadErr != nil {
			return sessionStreamReadyMsg{session: msg.Session, loadErr: loadErr}
		}
		// 先建新流：订阅成功后才在 React 收尾关闭旧流（审计 P1-① 顺序）。
		subCtx, cancel := context.WithCancel(context.Background())
		eventCh, subErr := client.SubscribeEvents(subCtx, sessionID)
		if subErr != nil {
			cancel()
			return sessionStreamReadyMsg{session: msg.Session, loadErr: subErr}
		}
		return sessionStreamReadyMsg{session: msg.Session, detail: detail, ch: eventCh, cancel: cancel}
	})
}

// activeSessionID 返回活跃会话 ID（无则空串）。
func (p *Plugin) activeSessionID() string {
	if p.st.Gateway.ActiveSess != nil {
		return p.st.Gateway.ActiveSess.ID
	}
	return ""
}
