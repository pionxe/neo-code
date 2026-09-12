package tuiv2

import (
	"context"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// Bootstrap 完成内核启动时的初始加载流程：
// Health → ListSessions → LoadSession 首个会话 → SubscribeEvents → 绑定事件泵。
// 此流程对应旧路径 loadInitialCmd 的行为。
func Bootstrap(ctx context.Context, k *kernel.Kernel, client gateway.Client, st *state.ViewState) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		health, err := client.Health(ctx)
		if err != nil {
			return gateway.GatewayEvent{
				Type:    gateway.EventGatewayOffline,
				Payload: map[string]any{"message": err.Error()},
			}
		}
		_ = health

		sessionList, err := client.ListSessions(ctx)
		if err != nil {
			return gateway.GatewayEvent{
				Type:    gateway.EventError,
				Payload: map[string]any{"message": err.Error()},
			}
		}
		for _, sess := range sessionList {
			st.Gateway.Sessions = append(st.Gateway.Sessions, sess)
		}

		// 加载首个会话的详细数据。
		if len(sessionList) > 0 {
			detail, err := client.LoadSession(ctx, sessionList[0].ID)
			if err == nil {
				st.Gateway.ActiveSess = &sessionList[0]
				st.Runtime.RunID = ""
				for _, item := range detail.Stream {
					st.Stream = append(st.Stream, state.StreamEntry{
						ID:       item.ID,
						Type:     string(item.Kind),
						Content:  item.Text,
						Metadata: map[string]any{"done": true, "role": item.Role},
					})
				}
				st.Layout.AutoScroll = true
				st.Layout.ScrollOffset = 0
			}

			// 订阅事件流。
			eventCh, err := client.SubscribeEvents(ctx, sessionList[0].ID)
			if err == nil {
				k.BindEventStream(eventCh)
			}
		}
		return initialLoadDoneMsg{}
	}
}

// initialLoadDoneMsg 是初始加载完成的内部信号（触发 View 重绘）。
type initialLoadDoneMsg struct{}
