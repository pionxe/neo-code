package tuiv2

import (
	"context"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// bootstrapDoneMsg 是初始加载的 RPC 产物（内核内部消息）：
// 闭包只做 RPC 与通道建立并打包结果，状态写入 + BindEventStream
// 回到主 goroutine 的 Update 循环执行（P0-1 并发契约修复）。
type bootstrapDoneMsg struct {
	healthOK bool
	sessions []gateway.SessionSummary
	active   *gateway.SessionSummary
	detail   *gateway.SessionDetail
	eventCh  <-chan gateway.GatewayEvent
	errs     []string
}

// Bootstrap 发起初始加载：闭包只做 RPC 并打包结果为 Msg 返回，
// 状态写入由内核 Update 在主 goroutine 完成（P0-1 并发契约修复）。
func Bootstrap(ctx context.Context, client gateway.Client) tea.Cmd {
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		var errs []string
		healthOK := false
		if _, err := client.Health(ctx); err != nil {
			errs = append(errs, "health: "+err.Error())
		} else {
			healthOK = true
		}

		sessions, err := client.ListSessions(ctx)
		if err != nil {
			errs = append(errs, "list: "+err.Error())
		}

		var active *gateway.SessionSummary
		var detail *gateway.SessionDetail
		var eventCh <-chan gateway.GatewayEvent
		if len(sessions) > 0 {
			active = &sessions[0]
			detail, err = client.LoadSession(ctx, active.ID)
			if err != nil {
				errs = append(errs, "load: "+err.Error())
			}
			ch, subErr := client.SubscribeEvents(ctx, active.ID)
			if subErr != nil {
				errs = append(errs, "subscribe: "+subErr.Error())
			} else {
				eventCh = ch
			}
		}

		return bootstrapDoneMsg{
			healthOK: healthOK,
			sessions: sessions,
			active:   active,
			detail:   detail,
			eventCh:  eventCh,
			errs:     errs,
		}
	}
}

// ApplyBootstrap 在主 goroutine 内落地 bootstrapDoneMsg 的全部状态写入
// （由内核 Update 的 bootstrapDoneMsg case 调用——P0-1 修复：Cmd goroutine
// 不直接写共享状态）。
func ApplyBootstrap(st *state.ViewState, msg bootstrapDoneMsg, bindEventStream func(<-chan gateway.GatewayEvent)) {
	// 连接状态（Gateway 子槽）。
	if msg.healthOK {
		st.Gateway.Connected = true
	}
	// 会话列表（Gateway.Sessions 子槽）。
	for _, sess := range msg.sessions {
		found := false
		for _, existing := range st.Gateway.Sessions {
			if existing.ID == sess.ID {
				found = true
				break
			}
		}
		if !found {
			st.Gateway.Sessions = append(st.Gateway.Sessions, sess)
		}
	}
	// 活跃会话。
	if msg.active != nil {
		st.Gateway.ActiveSess = msg.active
	}
	// 会话详情 → Stream 条目（含 Timestamp 修复，审计 P1-3）。
	if msg.detail != nil {
		for _, item := range msg.detail.Stream {
			entry := state.StreamEntry{
				ID:       item.ID,
				Type:     string(item.Kind),
				Content:  item.Text,
				Metadata: map[string]any{"done": true, "role": item.Role},
			}
			if !item.CreatedAt.IsZero() {
				entry.Timestamp = item.CreatedAt
			}
			st.Stream = append(st.Stream, entry)
		}
		st.Layout.AutoScroll = true
		st.Layout.ScrollOffset = 0
	}
	// 事件流绑定。
	if msg.eventCh != nil {
		bindEventStream(msg.eventCh)
	}
	// 非致命错误以弱提示呈现。
	for _, e := range msg.errs {
		_ = e // 可扩展为 Notify
	}
}
