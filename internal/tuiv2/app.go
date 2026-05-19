// Package tuiv2 实现 Ghost Console TUI v2 的应用骨架。
package tuiv2

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"
)

const (
	defaultTerminal      = "0x0"
	inspectorWideWidth   = 30
	inspectorHiddenWidth = 80
	inspectorWideMin     = 100
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

	ambientStatus *components.AmbientStatus
	agentStream   *components.AgentStream
	commandPrompt *components.CommandPrompt
	softInspector *components.SoftInspector
}

var (
	debugStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3b4261"))
)

var _ tea.Model = (*App)(nil)

// NewApp 创建 TUI v2 根组件，并初始化集中式 ViewState。
func NewApp(cfg StartupConfig) tea.Model {
	viewState := state.NewViewState()
	return &App{
		client:        cfg.Client,
		state:         viewState,
		debug:         cfg.Debug,
		backend:       cfg.Backend,
		scenario:      cfg.Scenario,
		ambientStatus: components.NewAmbientStatus(viewState),
		agentStream:   components.NewAgentStream(viewState),
		commandPrompt: components.NewCommandPrompt(viewState),
		softInspector: components.NewSoftInspector(viewState),
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
		a.applyWindowSize(msg.Width, msg.Height)
		return a, tea.ClearScreen
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
	return a, a.routeComponents(msg)
}

// View 自上而下拼接 Focus-Only 静态布局，宽屏时将 Soft Inspector 放到右侧。
func (a *App) View() string {
	lines := []string{
		a.ambientStatus.View(),
		a.separatorLine(),
	}
	if a.lastErr != "" {
		lines = append(lines, errorStyle.Render("  × "+a.lastErr))
	}
	lines = append(lines, a.mainArea(), a.separatorLine(), a.commandPrompt.View())
	if a.debug {
		lines = append(lines, "", debugStyle.Render(a.debugLine()))
	}
	return a.fitViewToTerminal(strings.Join(lines, "\n"))
}

// applyWindowSize 更新布局尺寸，并按 Focus-Only 断点计算 Soft Inspector 状态。
func (a *App) applyWindowSize(width int, height int) {
	a.state.Layout.Width = width
	a.state.Layout.Height = height
	switch {
	case width < inspectorHiddenWidth:
		a.state.Layout.ShowInspector = false
		a.state.Layout.InspectorWidth = 0
	case width < inspectorWideMin:
		a.state.Layout.ShowInspector = true
		a.state.Layout.InspectorWidth = width
	default:
		a.state.Layout.ShowInspector = true
		a.state.Layout.InspectorWidth = inspectorWideWidth
	}
}

// routeComponents 将全局消息转发给各静态布局组件。
func (a *App) routeComponents(msg tea.Msg) tea.Cmd {
	_, statusCmd := a.ambientStatus.Update(msg)
	_, streamCmd := a.agentStream.Update(msg)
	_, inspectorCmd := a.softInspector.Update(msg)
	_, promptCmd := a.commandPrompt.Update(msg)
	return tea.Batch(statusCmd, streamCmd, inspectorCmd, promptCmd)
}

// mainArea 渲染中部区域，按终端宽度决定 Inspector 右侧或纵向压缩显示。
func (a *App) mainArea() string {
	streamView := a.agentStream.View()
	if !a.state.Layout.ShowInspector {
		return streamView
	}
	inspectorView := a.softInspector.View()
	if a.state.Layout.Width >= inspectorWideMin {
		return lipgloss.JoinHorizontal(lipgloss.Top, streamView, "  ", inspectorView)
	}
	return lipgloss.JoinVertical(lipgloss.Left, streamView, "", a.separatorLine(), inspectorView)
}

// separatorLine 渲染单条细线，用于区分主要区域而不使用边框。
func (a *App) separatorLine() string {
	width := a.state.Layout.Width
	if width <= 0 {
		width = 48
	}
	return separatorStyle.Render(strings.Repeat("─", width))
}

// fitViewToTerminal 将视图约束到当前终端尺寸，避免 resize 后自动换行或旧行残留。
func (a *App) fitViewToTerminal(view string) string {
	width := a.state.Layout.Width
	height := a.state.Layout.Height
	if width <= 0 {
		return view
	}
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		lines[i] = fitLine(line, width)
	}
	if height > 0 {
		switch {
		case len(lines) > height:
			lines = lines[:height]
		case len(lines) < height:
			for len(lines) < height {
				lines = append(lines, strings.Repeat(" ", width))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// fitLine 截断并补齐单行显示宽度，保留 ANSI 样式同时防止终端自动 wrap。
func fitLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	target := width - 1
	if target <= 0 {
		return ""
	}
	fitted := ansi.Truncate(line, target, "")
	lineWidth := ansi.StringWidth(fitted)
	if lineWidth < target {
		fitted += strings.Repeat(" ", target-lineWidth)
	}
	return fitted
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
