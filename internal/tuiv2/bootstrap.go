package tuiv2

import (
	"context"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// bootstrapReactor 实现 kernel.Reactor：在主 goroutine 内消费
// bootstrapDoneMsg 并落地全部状态写入（P0-1 并发契约修复的收口）。
// 注册进 kernel 后，Kernel.Update → drain → React 在主 goroutine 调用，
// 消除 tea.Cmd goroutine 直写共享状态的数据竞争。
type bootstrapReactor struct {
	client gateway.Client
}

func newBootstrapReactor(client gateway.Client) *bootstrapReactor {
	return &bootstrapReactor{client: client}
}

// React 处理 bootstrapDoneMsg：调用 ApplyBootstrap 落状态、绑定事件流、
// 经 Notify 呈现错误（不丢弃、不静默——审计第 3 轮 P1-3 收尾）。
func (r *bootstrapReactor) ID() string { return "bootstrap" }

// Init 经 GoCmd 发起初始加载（kernel.Init 链自动调用）。
func (r *bootstrapReactor) Init(ctx context.Context, h kernel.Host) {
	if r.client != nil {
		h.GoCmd(Bootstrap(ctx, r.client))
	}
}
func (r *bootstrapReactor) Close(ctx context.Context) {}
func (r *bootstrapReactor) React(h kernel.Host, msg tea.Msg) {
	bd, ok := msg.(bootstrapDoneMsg)
	if !ok {
		return
	}
	ApplyBootstrap(h.State(), bd, h.BindEventStream)
	for _, e := range bd.errs {
		h.Notify("初始加载警告：" + e)
	}
}

// bootstrapDoneMsg 是初始加载的 RPC 产物（内核内部消息）：
// 闭包只做 RPC 与通道建立并打包结果，状态写入 + BindEventStream
// 回到主 goroutine 的 Update 循环执行（P0-1 并发契约修复）。
type bootstrapDoneMsg struct {
	healthOK bool
	sessions []gateway.SessionSummary
	active   *gateway.SessionSummary
	detail   *gateway.SessionDetail
	models   []gateway.ModelInfo
	eventCh  <-chan gateway.GatewayEvent
	errs     []string
}

// Bootstrap 发起初始加载：闭包只做 RPC 并打包结果为 Msg 返回，
// Host 副作用（状态写入 + BindEventStream）由 bootstrapReactor.React
// 在主 goroutine 内完成（P0-1 并发契约）。
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
		sessionList, err := client.ListSessions(ctx)
		if err != nil {
			errs = append(errs, "list: "+err.Error())
		}
		var active *gateway.SessionSummary
		var detail *gateway.SessionDetail
		var eventCh <-chan gateway.GatewayEvent
		if len(sessionList) > 0 {
			active = &sessionList[0]
			// 先声明外层 detail，避免 := 遮蔽导致 LoadSession 结果丢弃（审计 P0-1）。
			var loadErr error
			detail, loadErr = client.LoadSession(ctx, active.ID)
			if loadErr != nil {
				errs = append(errs, "load: "+loadErr.Error())
			}
			ch, subErr := client.SubscribeEvents(ctx, active.ID)
			if subErr != nil {
				errs = append(errs, "subscribe: "+subErr.Error())
			} else {
				eventCh = ch
			}
		}
		// GetModel 取服务端真值（审计第 6 轮 P1-②）。
		if serverModel, gmErr := client.GetModel(ctx, active.ID); gmErr == nil && serverModel != "" {
			active.Model = serverModel
		}
		// 补充模型列表（审计第 3 轮 P1-2：kernel 路径唯一 ListModels 调用点）。
		models, modelsErr := client.ListModels(ctx)
		if modelsErr != nil {
			errs = append(errs, "models: "+modelsErr.Error())
		}
		return bootstrapDoneMsg{
			healthOK: healthOK,
			sessions: sessionList,
			active:   active,
			detail:   detail,
			models:   models,
			eventCh:  eventCh,
			errs:     errs,
		}
	}
}

// ApplyBootstrap 在主 goroutine 内落地 bootstrapDoneMsg 的全部状态写入
// （由 bootstrapReactor.React 调用——kernel.Update drain 循环保证单线程）。
func ApplyBootstrap(st *state.ViewState, msg bootstrapDoneMsg, bindEventStream func(<-chan gateway.GatewayEvent)) {
	if msg.healthOK {
		st.Gateway.Connected = true
	}
	for _, sess := range msg.sessions {
		found := false
		for _, e := range st.Gateway.Sessions {
			if e.ID == sess.ID {
				found = true
				break
			}
		}
		if !found {
			st.Gateway.Sessions = append(st.Gateway.Sessions, sess)
		}
	}
	if msg.active != nil {
		active := *msg.active
		st.Gateway.ActiveSess = &active
	}
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
		// Token 用量落地（对齐旧路径 app_view.go:111）。
		st.Runtime.Tokens = state.TokenUsage{
			Input:  msg.detail.Usage.Input,
			Output: msg.detail.Usage.Output,
			Total:  msg.detail.Usage.Total,
		}
	}
	// 模型列表落地（审计 P1-2 补位）。
	st.Gateway.Models = append(st.Gateway.Models, msg.models...)
	if len(msg.models) > 0 && st.Gateway.ActiveModel == "" {
		st.Gateway.ActiveModel = msg.models[0].ID
	}
	// 事件流绑定。
	if msg.eventCh != nil {
		bindEventStream(msg.eventCh)
	}
}
