// Package tuiv2 实现 Ghost Console TUI v2 的应用骨架。
package tuiv2

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"
)

const (
	surfaceName     = "ghost-console"
	defaultTerminal = "0x0"
)

// StartupConfig 承载 TUI v2 独立入口解析出的启动参数和 Gateway 客户端。
type StartupConfig struct {
	Backend  string
	Scenario string
	Debug    bool
	Client   gateway.Client
}

// App 是 TUI v2 的根组件，负责持有 Gateway 客户端、集中式 ViewState 和顶层消息路由。
type App struct {
	client gateway.Client
	state  *state.ViewState
	debug  bool

	backend  string
	scenario string
	eventCh  <-chan gateway.GatewayEvent
	lastErr  string

	ambientStatus *AmbientStatus
	agentStream   *AgentStream
	commandPrompt *CommandPrompt
	softInspector *SoftInspector
}

// AmbientStatus 是后续阶段接入的顶部环境状态组件占位。
type AmbientStatus struct{}

// AgentStream 是后续阶段接入的 Agent 流组件占位。
type AgentStream struct{}

// CommandPrompt 是后续阶段接入的命令输入组件占位。
type CommandPrompt struct{}

// SoftInspector 是后续阶段接入的软检查器组件占位。
type SoftInspector struct{}

var (
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	idleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	debugStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	promptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7"))
	errorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
)

var _ tea.Model = (*App)(nil)

// NewApp 创建 TUI v2 根组件，并初始化集中式 ViewState。
func NewApp(cfg StartupConfig) tea.Model {
	return &App{
		client:        cfg.Client,
		state:         state.NewViewState(),
		debug:         cfg.Debug,
		backend:       cfg.Backend,
		scenario:      cfg.Scenario,
		ambientStatus: &AmbientStatus{},
		agentStream:   &AgentStream{},
		commandPrompt: &CommandPrompt{},
		softInspector: &SoftInspector{},
	}
}

// Init 通过 Gateway 客户端检查连接并加载初始 ViewState。
func (a *App) Init() tea.Cmd {
	if a.client == nil {
		return nil
	}
	return loadInitialCmd(a.client)
}

// Update 处理全局消息，并把 Gateway 结果映射到集中式 ViewState。
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.state.Layout.Width = msg.Width
		a.state.Layout.Height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return a, tea.Quit
		}
	case initialLoadedMsg:
		a.applyInitialLoaded(msg)
		if msg.eventCh != nil {
			return a, waitEventCmd(msg.eventCh)
		}
	case gatewayEventMsg:
		if msg.closed {
			return a, nil
		}
		a.applyGatewayEvent(msg.event)
		return a, waitEventCmd(a.eventCh)
	}
	return a, nil
}

// View 自上而下拼接子组件占位视图，目前直接输出 ViewState 关键字段。
func (a *App) View() string {
	lines := []string{
		a.statusLine(),
		"",
		a.viewStateLine(),
		a.gatewayLine(),
		a.runtimeLine(),
		a.layoutLine(),
		a.streamLine(),
		"",
		promptStyle.Render("› "),
	}
	if a.lastErr != "" {
		lines = append(lines[:2], append([]string{errorStyle.Render("  × " + a.lastErr)}, lines[2:]...)...)
	}
	if a.debug {
		lines = append(lines, "", debugStyle.Render(a.debugLine()))
	}
	return strings.Join(lines, "\n")
}

// applyInitialLoaded 将 Gateway 初始 RPC 结果写入 ViewState。
func (a *App) applyInitialLoaded(msg initialLoadedMsg) {
	a.lastErr = msg.errText
	a.state.Gateway.Connected = msg.connected
	a.state.Gateway.Sessions = append([]gateway.SessionSummary(nil), msg.sessions...)
	a.state.Gateway.Models = append([]gateway.ModelInfo(nil), msg.models...)
	a.state.Gateway.ActiveModel = msg.activeModel
	a.eventCh = msg.eventCh
	if msg.errText != "" {
		a.state.Runtime.Phase = state.RuntimePhaseError
	}
	if len(msg.sessions) > 0 {
		active := msg.sessions[0]
		a.state.Gateway.ActiveSess = &active
	}
	if msg.detail != nil {
		a.state.Runtime.Tokens = state.TokenUsage{
			Input:  msg.detail.Usage.Input,
			Output: msg.detail.Usage.Output,
			Total:  msg.detail.Usage.Total,
		}
		for _, item := range msg.detail.Stream {
			a.appendStream(streamEntryFromItem(item))
		}
	}
}

// applyGatewayEvent 将 Gateway 事件转换为追加式 StreamEntry，并更新运行阶段。
func (a *App) applyGatewayEvent(event gateway.GatewayEvent) {
	switch event.Type {
	case gateway.EventRunStarted, gateway.EventAssistantDelta, gateway.EventToolStarted:
		a.state.Runtime.Phase = state.RuntimePhaseRunning
	case gateway.EventPermissionRequested:
		a.state.Runtime.Phase = state.RuntimePhaseWaitingPermission
		a.state.Input.Mode = state.InputStateModePermissionResponse
		a.state.Input.Prompt = payloadText(event.Payload)
	case gateway.EventUserQuestionRequested:
		a.state.Runtime.Phase = state.RuntimePhaseWaitingUser
		a.state.Input.Mode = state.InputStateModeQuestionAnswer
		a.state.Input.Prompt = payloadText(event.Payload)
	case gateway.EventRunCancelled:
		a.state.Runtime.Phase = state.RuntimePhaseCancelled
	case gateway.EventRunFinished, gateway.EventToolFinished:
		a.state.Runtime.Phase = state.RuntimePhaseIdle
	case gateway.EventGatewayOffline, gateway.EventError:
		a.state.Runtime.Phase = state.RuntimePhaseError
		a.lastErr = payloadText(event.Payload)
	}
	if event.RunID != "" {
		a.state.Runtime.RunID = event.RunID
	}
	a.appendStream(streamEntryFromEvent(event))
}

// appendStream 以追加新 entry 的方式维护不可变 StreamEntry 序列。
func (a *App) appendStream(entry state.StreamEntry) {
	a.state.Stream = append(a.state.Stream, entry)
}

// statusLine 渲染 Ghost Console 顶部状态，保持无边框并用状态符号表达运行态。
func (a *App) statusLine() string {
	parts := []string{
		statusStyle.Render("NEOCODE"),
		a.renderStatus(),
		mutedStyle.Render(a.backend),
		mutedStyle.Render(surfaceName),
	}
	return strings.Join(parts, "   ")
}

// renderStatus 根据 ViewState 的 Runtime phase 渲染顶部状态符号。
func (a *App) renderStatus() string {
	switch a.state.Runtime.Phase {
	case state.RuntimePhaseRunning, state.RuntimePhaseWaitingPermission, state.RuntimePhaseWaitingUser:
		return statusStyle.Render("◉ " + a.state.Runtime.Phase)
	case state.RuntimePhaseError:
		return errorStyle.Render("× " + a.state.Runtime.Phase)
	default:
		return idleStyle.Render("○ " + a.state.Runtime.Phase)
	}
}

// viewStateLine 渲染 ViewState 顶层占位摘要。
func (a *App) viewStateLine() string {
	return mutedStyle.Render(fmt.Sprintf(
		"ViewState mode:%s input:%s cursor:%d",
		inputModeName(a.state.Mode),
		a.state.Input.Mode,
		a.state.Input.Cursor,
	))
}

// gatewayLine 渲染 GatewayState 占位摘要。
func (a *App) gatewayLine() string {
	active := "-"
	if a.state.Gateway.ActiveSess != nil {
		active = a.state.Gateway.ActiveSess.Title
	}
	return mutedStyle.Render(fmt.Sprintf(
		"Gateway connected:%t sessions:%d active:%s models:%d active_model:%s",
		a.state.Gateway.Connected,
		len(a.state.Gateway.Sessions),
		active,
		len(a.state.Gateway.Models),
		a.state.Gateway.ActiveModel,
	))
}

// runtimeLine 渲染 RuntimeState 占位摘要。
func (a *App) runtimeLine() string {
	return mutedStyle.Render(fmt.Sprintf(
		"Runtime phase:%s run:%s tokens:%d/%d/%d",
		a.state.Runtime.Phase,
		emptyDash(a.state.Runtime.RunID),
		a.state.Runtime.Tokens.Input,
		a.state.Runtime.Tokens.Output,
		a.state.Runtime.Tokens.Total,
	))
}

// layoutLine 渲染 LayoutState 占位摘要。
func (a *App) layoutLine() string {
	return mutedStyle.Render(fmt.Sprintf(
		"Layout size:%dx%d inspector:%t/%d",
		a.state.Layout.Width,
		a.state.Layout.Height,
		a.state.Layout.ShowInspector,
		a.state.Layout.InspectorWidth,
	))
}

// streamLine 渲染 Stream 列表占位摘要。
func (a *App) streamLine() string {
	last := "-"
	if len(a.state.Stream) > 0 {
		last = a.state.Stream[len(a.state.Stream)-1].Content
	}
	return mutedStyle.Render(fmt.Sprintf("Stream entries:%d last:%s", len(a.state.Stream), last))
}

// debugLine 渲染调试模式下的最小运行信息。
func (a *App) debugLine() string {
	size := defaultTerminal
	if a.state.Layout.Width > 0 || a.state.Layout.Height > 0 {
		size = fmt.Sprintf("%dx%d", a.state.Layout.Width, a.state.Layout.Height)
	}
	return fmt.Sprintf(
		"[debug] mode:%s  scenario:%s  events:%d  size:%s",
		inputModeName(a.state.Mode),
		a.scenario,
		len(a.state.Stream),
		size,
	)
}

// streamEntryFromItem 将会话历史 DTO 映射为不可变 StreamEntry。
func streamEntryFromItem(item gateway.StreamItem) state.StreamEntry {
	return state.StreamEntry{
		ID:        item.ID,
		Type:      item.Kind,
		Timestamp: item.CreatedAt,
		Content:   item.Text,
		Metadata: map[string]any{
			"role":   item.Role,
			"status": item.Status,
		},
	}
}

// streamEntryFromEvent 将 Gateway 事件 DTO 映射为不可变 StreamEntry。
func streamEntryFromEvent(event gateway.GatewayEvent) state.StreamEntry {
	return state.StreamEntry{
		ID:        fmt.Sprintf("%s:%s:%d", event.Type, event.RunID, len(event.Payload)),
		Type:      string(event.Type),
		Timestamp: event.At,
		Content:   payloadText(event.Payload),
		ToolName:  fmt.Sprint(event.Payload["tool"]),
		Metadata:  clonePayload(event.Payload),
	}
}

// payloadText 从事件 payload 中提取最适合显示的摘要文本。
func payloadText(payload map[string]any) string {
	for _, key := range []string{"text", "message", "phase", "tool", "question"} {
		if value, ok := payload[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

// clonePayload 复制事件 payload，避免 StreamEntry 与原事件共享可变 map。
func clonePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	clone := make(map[string]any, len(payload))
	for key, value := range payload {
		clone[key] = value
	}
	return clone
}

// inputModeName 将输入模式转换为占位视图中的稳定文本。
func inputModeName(mode state.InputMode) string {
	switch mode {
	case state.NormalMode:
		return "normal"
	case state.LeaderMode:
		return "leader"
	default:
		return "input"
	}
}

// emptyDash 在占位视图中用短横线表示空值。
func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

type initialLoadedMsg struct {
	connected   bool
	sessions    []gateway.SessionSummary
	detail      *gateway.SessionDetail
	models      []gateway.ModelInfo
	activeModel string
	eventCh     <-chan gateway.GatewayEvent
	errText     string
}

type gatewayEventMsg struct {
	event  gateway.GatewayEvent
	closed bool
}

// loadInitialCmd 通过 Gateway 客户端加载初始状态，并建立首个会话的事件订阅。
func loadInitialCmd(client gateway.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		msg := initialLoadedMsg{}
		if _, err := client.Health(ctx); err != nil {
			msg.errText = err.Error()
			return msg
		}
		msg.connected = true
		sessions, err := client.ListSessions(ctx)
		if err != nil {
			msg.errText = err.Error()
			return msg
		}
		msg.sessions = sessions
		models, err := client.ListModels(ctx)
		if err != nil {
			msg.errText = err.Error()
			return msg
		}
		msg.models = models
		if len(sessions) == 0 {
			return msg
		}
		activeModel, err := client.GetModel(ctx, sessions[0].ID)
		if err != nil {
			msg.errText = err.Error()
			return msg
		}
		msg.activeModel = activeModel
		detail, err := client.LoadSession(ctx, sessions[0].ID)
		if err != nil {
			msg.errText = err.Error()
			return msg
		}
		msg.detail = detail
		eventCh, err := client.SubscribeEvents(ctx, sessions[0].ID)
		if err != nil {
			msg.errText = err.Error()
			return msg
		}
		msg.eventCh = eventCh
		return msg
	}
}

// waitEventCmd 等待 Gateway 事件 channel 的下一条事件，保持异步事件逐条进入 Update。
func waitEventCmd(events <-chan gateway.GatewayEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		return gatewayEventMsg{event: event, closed: !ok}
	}
}
