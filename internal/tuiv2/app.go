// Package tuiv2 实现 Ghost Console TUI v2 的应用骨架。
package tuiv2

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/keymap"
	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
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

	// Ctrl+C 双退保护
	lastCtrlC time.Time

	// Leader 重试与切换上一会话所需的私有运行时状态（不入 ViewState，非渲染状态）
	lastUserText  string // 最近一次发送的用户消息文本，供 Space r 重试
	prevSessionID string // 上一个会话 ID，供 Space Space 切换

	ambientStatus  *components.AmbientStatus
	agentStream    *components.AgentStream
	commandPrompt  *components.CommandPrompt
	cmdLine        *components.CmdLine
	softInspector  *components.SoftInspector
	palette        *components.Palette
	helpOverlay    *components.HelpOverlay
	sessionPicker  *components.SessionPicker
	modelPicker    *components.ModelPicker
	confirmOverlay *components.ConfirmOverlay
}

var _ tea.Model = (*App)(nil)

// NewApp 创建 TUI v2 根组件，并初始化集中式 ViewState。
func NewApp(cfg StartupConfig) tea.Model {
	viewState := state.NewViewState()
	return &App{
		client:         cfg.Client,
		state:          viewState,
		debug:          cfg.Debug,
		backend:        cfg.Backend,
		scenario:       cfg.Scenario,
		ambientStatus:  components.NewAmbientStatus(viewState),
		agentStream:    components.NewAgentStream(viewState),
		commandPrompt:  components.NewCommandPrompt(viewState),
		cmdLine:        components.NewCmdLine(viewState),
		softInspector:  components.NewSoftInspector(viewState),
		palette:        components.NewPalette(viewState),
		helpOverlay:    components.NewHelpOverlay(viewState),
		sessionPicker:  components.NewSessionPicker(viewState),
		modelPicker:    components.NewModelPicker(viewState),
		confirmOverlay: components.NewConfirmOverlay(viewState),
	}
}

// Init 通过 Gateway 客户端检查连接并加载初始 ViewState。
func (a *App) Init() tea.Cmd {
	if a.client == nil {
		return a.commandPrompt.Init()
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
		return a.handleKeyMsg(msg)
	case tea.MouseMsg:
		return a.handleMouseMsg(msg)
	case initialLoadedMsg:
		a.applyInitialLoaded(msg)
		if msg.eventCh != nil {
			return a, tea.Batch(waitEventCmd(msg.eventCh), a.commandPrompt.Init())
		}
		return a, a.commandPrompt.Init()
	case gatewayEventMsg:
		if msg.closed {
			return a, nil
		}
		beforeStreamLen := len(a.state.Stream)
		a.state = state.Reduce(a.state, msg.event)
		if len(a.state.Stream) > beforeStreamLen {
			a.state.Layout.AutoScroll = true
			a.state.Layout.ScrollOffset = 0
			// stream 增长后，已有搜索结果不再包含新内容，标记 stale 提示用户。
			if len(a.state.Search.Matches) > 0 {
				a.state.Search.Stale = true
			}
		}
		// 新 run 开始或会话相关事件时清理 Normal 子状态（搜索/Ex），避免跨 run/会话残留。
		switch msg.event.Type {
		case gateway.EventRunStarted:
			a.clearSearchAndEx()
		}
		a.bindComponents()
		if a.state.Runtime.Phase == state.RuntimePhaseError && len(a.state.Stream) > 0 {
			a.lastErr = a.state.Stream[len(a.state.Stream)-1].Content
		}
		return a, waitEventCmd(a.eventCh)
	case components.SubmitMessageMsg:
		return a, a.handleSubmitMessage(msg)
	case components.PermissionActionMsg:
		return a, a.handlePermissionAction(msg)
	case components.QuestionAnswerMsg:
		return a, a.handleQuestionAnswer(msg)
	case components.PromptCancelMsg:
		a.cancelPrompt(msg.Mode)
		return a, nil
	case components.ExCommandMsg:
		// : 命令行提交：执行命令并关闭 Ex overlay（命令打开新 overlay 者除外）。
		cmd := a.executeExCommand(msg.Command)
		if a.state.Overlay.Active == state.OverlayEx {
			a.closeOverlay()
			a.state.Ex = state.ExState{}
		}
		return a, cmd
	case components.SearchSubmitMsg:
		// / 搜索提交：执行扫描并关闭搜索 overlay 回 Normal。
		cmd := a.executeSearch(msg.Query)
		a.closeOverlay()
		a.state.Search.Active = false
		return a, cmd
	case components.CmdLineCancelMsg:
		// Esc/Ctrl+C 取消 Ex/Search 输入：关闭 overlay 回 Normal。
		a.closeOverlay()
		a.clearSearchAndEx()
		return a, nil
	case components.SlashCommandMsg:
		return a, a.handleSlashCommand(msg)
	case components.PaletteCommandMsg:
		return a, a.handlePaletteCommand(msg)
	case components.SessionSelectMsg:
		return a, a.handleSessionSelect(msg)
	case components.SessionDeleteMsg:
		return a, a.handleSessionDelete(msg)
	case components.ModelSelectMsg:
		return a, a.handleModelSelect(msg)
	case components.ConfirmYesMsg:
		return a, a.handleConfirmYes(msg)
	case components.ConfirmNoMsg:
		a.state.Confirm = state.ConfirmState{}
		a.closeOverlay()
		return a, nil
	case leaderTimeoutMsg:
		if a.state.Mode == state.LeaderMode {
			a.state.Mode = state.NormalMode
		}
		return a, nil
	case sessionSwitchedMsg:
		a.eventCh = msg.eventCh
		if msg.detail != nil {
			a.state.Stream = nil
			a.state.Runtime.Tokens = state.TokenUsage{
				Input:  msg.detail.Usage.Input,
				Output: msg.detail.Usage.Output,
				Total:  msg.detail.Usage.Total,
			}
			for _, item := range msg.detail.Stream {
				a.appendStream(streamEntryFromItem(item))
			}
		}
		// 会话历史重载会清空 Stream，因此切换提示必须在重载之后追加，
		// 否则用户无法确认是否切换成功。
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("session-switch-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("Switched to session: %s", a.activeSessionTitle()),
			Metadata:  map[string]any{"done": true},
		})
		a.bindComponents()
		if a.eventCh != nil {
			return a, waitEventCmd(a.eventCh)
		}
		return a, nil
	case sessionCreatedMsg:
		return a, a.handleSessionCreated(msg)
	}
	return a, a.routeComponents(msg)
}

// View 自上而下拼接 Focus-Only 静态布局，宽屏时将 Soft Inspector 放到右侧。
func (a *App) View() string {
	// 浮层模式下覆盖主视图
	switch a.state.Overlay.Active {
	case state.OverlayPalette:
		return a.fitViewToTerminal(a.palette.View())
	case state.OverlayHelp:
		return a.fitViewToTerminal(a.helpOverlay.View())
	case state.OverlaySessionPicker:
		return a.fitViewToTerminal(a.sessionPicker.View())
	case state.OverlayModelPicker:
		return a.fitViewToTerminal(a.modelPicker.View())
	case state.OverlayConfirm:
		return a.fitViewToTerminal(a.confirmOverlay.View())
	}
	lines := []string{
		a.ambientStatus.View(),
		a.separatorLine(),
	}
	if a.lastErr != "" {
		lines = append(lines, theme.ErrorStyle().Render("  "+theme.StatusSymbol(theme.PhaseError)+" "+a.lastErr))
	}
	// Ex/Search 输入 overlay：渲染 cmdline 输入行（替代普通 prompt 输入区），
	// 不覆盖主视图，仍保留 ambient/stream 可见。
	promptView := a.commandPrompt.View()
	if a.state.Overlay.Active == state.OverlayEx || a.state.Overlay.Active == state.OverlaySearch {
		promptView = a.cmdLine.View() + "\n" + a.commandPrompt.ModeLine()
	}
	lines = append(lines, a.mainArea(), a.separatorLine(), promptView)
	if a.debug {
		lines = append(lines, "", theme.WarningStyle().Render(a.debugLine()))
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

// routeStreamKey 将滚动按键优先交给 Agent Stream，避免与全局快捷键混淆。
func (a *App) routeStreamKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "j", "k", "g", "G":
		_, cmd := a.agentStream.Update(msg)
		return true, cmd
	default:
		return false, nil
	}
}

// handleMouseMsg 将鼠标事件分发到当前活跃的组件或浮层。
func (a *App) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// 浮层激活时，鼠标交给浮层组件
	switch a.state.Overlay.Active {
	case state.OverlayPalette:
		_, cmd := a.palette.Update(msg)
		return a, cmd
	case state.OverlaySessionPicker:
		_, cmd := a.sessionPicker.Update(msg)
		return a, cmd
	case state.OverlayModelPicker:
		_, cmd := a.modelPicker.Update(msg)
		return a, cmd
	}
	// 主视图下，滚轮事件交给 Agent Stream
	switch msg.Type {
	case tea.MouseWheelUp:
		a.state.Layout.ScrollOffset++
		a.state.Layout.AutoScroll = false
	case tea.MouseWheelDown:
		if a.state.Layout.ScrollOffset > 0 {
			a.state.Layout.ScrollOffset--
		}
		a.state.Layout.AutoScroll = a.state.Layout.ScrollOffset == 0
	}
	return a, nil
}

// handleKeyMsg 根据当前模式分发键盘消息。
func (a *App) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Esc always closes any active overlay first (global escape hatch)
	if a.state.Overlay.Active != state.OverlayNone {
		if msg.String() == "esc" {
			// 关闭 overlay 并清理 Normal 子状态（搜索/Ex 输入）。
			a.closeOverlay()
			a.clearSearchAndEx()
			return a, nil
		}
	}
	// 浮层激活时，键盘消息交给对应浮层组件处理
	switch a.state.Overlay.Active {
	case state.OverlayPalette:
		_, cmd := a.palette.Update(msg)
		return a, cmd
	case state.OverlayHelp:
		_, cmd := a.helpOverlay.Update(msg)
		return a, cmd
	case state.OverlaySessionPicker:
		_, cmd := a.sessionPicker.Update(msg)
		return a, cmd
	case state.OverlayModelPicker:
		_, cmd := a.modelPicker.Update(msg)
		return a, cmd
	case state.OverlayConfirm:
		_, cmd := a.confirmOverlay.Update(msg)
		return a, cmd
	case state.OverlayEx, state.OverlaySearch:
		// Ex/Search 输入 overlay：所有按键路由给 cmdline 组件，
		// 由它处理字符/Backspace/Enter/Esc（Esc/Ctrl+C 已在上方被全局拦截
		// 关闭 overlay，此处主要处理字符输入与提交）。
		_, cmd := a.cmdLine.Update(msg)
		return a, cmd
	}
	switch a.state.Mode {
	case state.LeaderMode:
		return a.handleLeaderKey(msg)
	case state.NormalMode:
		return a.handleNormalModeKey(msg)
	default: // InputModeInput
		return a.handleInputModeKey(msg)
	}
}

// handleInputModeKey 处理 Input Mode 下的键盘输入。
//
// 分层约定（plan-v4）：模式切换键在此拦截，不传给 prompt 编辑器。
// Ctrl+D 不进 MatchInputKey，由本函数按输入框是否为空决定：
//   - 输入为空 → EOF 退出程序
//   - 输入非空 → 删除光标后字符（等同 delete），委派 prompt 处理
func (a *App) handleInputModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+D 上下文分发
	if msg.String() == "ctrl+d" {
		if strings.TrimSpace(a.state.Input.Text) == "" {
			return a, tea.Quit
		}
		_, cmd := a.commandPrompt.Update(tea.KeyMsg{Type: tea.KeyDelete})
		return a, cmd
	}
	action := keymap.MatchInputKey(msg.String())
	switch action {
	case keymap.ActionCtrlC:
		return a.handleCtrlC()
	case keymap.ActionEscape:
		a.state.Mode = state.NormalMode
		return a, nil
	case keymap.ActionOpenPalette:
		a.openOverlay(state.OverlayPalette)
		return a, nil
	case keymap.ActionLogViewer:
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("log-hint-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   "Log viewer not yet available",
			Metadata:  map[string]any{"done": true},
		})
		return a, nil
	default:
		_, promptCmd := a.commandPrompt.Update(msg)
		return a, promptCmd
	}
}

// handleNormalModeKey 处理 Normal Mode 下的键盘输入。
//
// 分层约定（plan-v4）：模式切换键（i/Enter→Input、Space→Leader、:→Ex、
// /→Search）优先拦截；n/N 在搜索 Matches 非空时跳转；Ctrl+D 半页下翻；
// 其余导航键交给 stream。
func (a *App) handleNormalModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := keymap.MatchNormalKey(msg.String())
	switch action {
	case keymap.ActionCtrlC:
		return a.handleCtrlC()
	case keymap.ActionEnterInput:
		a.enterInputFromNormal()
		return a, nil
	case keymap.ActionScrollDown, keymap.ActionScrollUp,
		keymap.ActionScrollTop, keymap.ActionScrollBottom:
		_, cmd := a.agentStream.Update(msg)
		return a, cmd
	case keymap.ActionHalfPageDown, keymap.ActionHalfPageUp:
		_, cmd := a.agentStream.Update(msg)
		return a, cmd
	case keymap.ActionFullPageDown, keymap.ActionFullPageUp:
		_, cmd := a.agentStream.Update(msg)
		return a, cmd
	case keymap.ActionLeader:
		a.state.Mode = state.LeaderMode
		return a, leaderTimeoutCmd()
	case keymap.ActionQuit:
		return a, tea.Quit
	case keymap.ActionSearchForward:
		// / 进入搜索输入 overlay
		a.openSearch()
		return a, nil
	case keymap.ActionSearchNext:
		a.jumpSearchMatch(1)
		return a, nil
	case keymap.ActionSearchPrev:
		a.jumpSearchMatch(-1)
		return a, nil
	case keymap.ActionExCommand:
		// : 进入 Ex 命令行输入 overlay
		a.openEx()
		return a, nil
	default:
		_, promptCmd := a.commandPrompt.Update(msg)
		return a, promptCmd
	}
}

// handleLeaderKey 处理 Leader Key 后缀。
//
// 行为约定（plan-v4）：Leader 是独占捕获，非后缀键或超时(1s)时立即静默回到
// Normal（不泄漏给 Normal handler）。后缀键执行动作后回到 Normal，除非打开了
// 需要保持的面板（palette/session_picker/help/model_picker）。
func (a *App) handleLeaderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()
	if keyStr == "esc" || keyStr == "ctrl+c" {
		a.state.Mode = state.NormalMode
		return a, nil
	}
	action := keymap.MatchLeaderKey(keyStr)
	// 非后缀键：静默回到 Normal，不执行任何动作。
	if action == keymap.ActionNone {
		a.state.Mode = state.NormalMode
		return a, nil
	}
	a.state.Mode = state.NormalMode // Leader 后总是回到 Normal
	switch action {
	case keymap.ActionLeaderQuit:
		return a, tea.Quit
	case keymap.ActionLeaderPalette:
		a.openOverlay(state.OverlayPalette)
		return a, nil
	case keymap.ActionLeaderHelp:
		a.openOverlay(state.OverlayHelp)
		return a, nil
	case keymap.ActionLeaderSwitchSession:
		a.openOverlay(state.OverlaySessionPicker)
		return a, nil
	case keymap.ActionLeaderModelPicker:
		a.openOverlay(state.OverlayModelPicker)
		return a, nil
	case keymap.ActionLeaderNewSession:
		if a.client != nil {
			return a, createSessionCmd(a.client)
		}
		return a, nil
	case keymap.ActionLeaderFullAccess:
		return a, a.toggleFullAccess()
	case keymap.ActionLeaderLog:
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("log-hint-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   "Log viewer not yet available",
			Metadata:  map[string]any{"done": true},
		})
		return a, nil
	case keymap.ActionLeaderCancelRun:
		return a, a.cancelCurrentRun()
	case keymap.ActionLeaderRetry:
		return a, a.retryLastRun()
	case keymap.ActionLeaderLastSession:
		return a, a.switchToLastSession()
	default:
		return a, nil
	}
}

// cancelCurrentRun 取消当前运行中的 Agent；无运行中任务时静默 no-op。
func (a *App) cancelCurrentRun() tea.Cmd {
	phase := a.state.Runtime.Phase
	if phase != state.RuntimePhaseRunning &&
		phase != state.RuntimePhaseWaitingPermission &&
		phase != state.RuntimePhaseWaitingUser {
		// 空闲态：静默 no-op，避免打扰用户。
		return nil
	}
	if a.client != nil {
		return cancelRunCmd(a.client, a.activeSessionID(), a.state.Runtime.RunID)
	}
	a.state.Runtime.Phase = state.RuntimePhaseCancelled
	return nil
}

// retryLastRun 重试最近一次用户输入；无历史输入时提示。
func (a *App) retryLastRun() tea.Cmd {
	if strings.TrimSpace(a.lastUserText) == "" {
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("retry-hint-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   "No previous run to retry",
			Metadata:  map[string]any{"done": true},
		})
		return nil
	}
	return a.handleSubmitMessage(components.SubmitMessageMsg{Text: a.lastUserText})
}

// switchToLastSession 切换到上一个会话；无上一会话时提示。
func (a *App) switchToLastSession() tea.Cmd {
	if a.prevSessionID == "" {
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("last-sess-hint-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   "No previous session to switch",
			Metadata:  map[string]any{"done": true},
		})
		return nil
	}
	if a.client == nil {
		return nil
	}
	return loadSessionCmd(a.client, a.prevSessionID)
}

// handleCtrlC 实现 Ctrl+C 双退保护：运行中取消、空闲双退。
func (a *App) handleCtrlC() (tea.Model, tea.Cmd) {
	phase := a.state.Runtime.Phase
	if phase == state.RuntimePhaseRunning || phase == state.RuntimePhaseWaitingPermission || phase == state.RuntimePhaseWaitingUser {
		// Agent 运行中 → 取消运行
		if a.client != nil {
			return a, cancelRunCmd(a.client, a.activeSessionID(), a.state.Runtime.RunID)
		}
		a.state.Runtime.Phase = state.RuntimePhaseCancelled
		return a, nil
	}
	// Agent 空闲 → 双退保护
	now := time.Now()
	if !a.lastCtrlC.IsZero() && now.Sub(a.lastCtrlC) < 2*time.Second {
		return a, tea.Quit
	}
	a.lastCtrlC = now
	a.appendStream(state.StreamEntry{
		ID:        fmt.Sprintf("ctrlc-hint-%d", now.UnixNano()),
		Type:      "status",
		Timestamp: now,
		Content:   "Press Ctrl+C again to quit",
		Metadata:  map[string]any{"done": true},
	})
	return a, nil
}

// leaderTimeoutMsg 用于 Leader Key 1 秒超时回退。
type leaderTimeoutMsg struct{}

// leaderTimeoutCmd 在 1 秒后发送超时消息，将 Leader 模式回退到 Normal。
func leaderTimeoutCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(_ time.Time) tea.Msg {
		return leaderTimeoutMsg{}
	})
}

// handleSubmitMessage 将用户输入交给 GatewayClient，并让后续 ACK 以事件形式回到 reducer。
func (a *App) handleSubmitMessage(msg components.SubmitMessageMsg) tea.Cmd {
	if strings.TrimSpace(msg.Text) == "" {
		return nil
	}
	// 记录最近一次用户输入，供 Leader Space r 重试使用。
	a.lastUserText = msg.Text
	// 立即将用户消息追加到 Stream 中以便渲染
	a.appendStream(state.StreamEntry{
		ID:        fmt.Sprintf("user-msg-%d", time.Now().UnixNano()),
		Type:      "message",
		Timestamp: time.Now(),
		Content:   msg.Text,
		Metadata:  map[string]any{"role": "user", "done": true},
	})
	if a.client == nil {
		return nil
	}
	sessionID := a.activeSessionID()
	return submitMessageCmd(a.client, sessionID, msg.Text)
}

// handlePermissionAction 将权限快捷键转换成 GatewayClient 权限决策 RPC。
func (a *App) handlePermissionAction(msg components.PermissionActionMsg) tea.Cmd {
	if a.client == nil {
		return nil
	}
	decision := gateway.PermissionDecision{
		SessionID: a.activeSessionID(),
		RunID:     a.state.Runtime.RunID,
		Allow:     msg.Decision == "y" || msg.Decision == "a",
		Reason:    msg.Decision,
	}
	return resolvePermissionCmd(a.client, decision)
}

// handleQuestionAnswer 将 ask_user 回答交给 GatewayClient，并把完成事件交给 reducer。
func (a *App) handleQuestionAnswer(msg components.QuestionAnswerMsg) tea.Cmd {
	if a.client == nil {
		return nil
	}
	answer := gateway.UserQuestionAnswer{
		SessionID: a.activeSessionID(),
		RunID:     a.state.Runtime.RunID,
		Text:      msg.Text,
	}
	return answerQuestionCmd(a.client, answer)
}

// cancelPrompt 取消当前内联交互，只重置输入 UI 状态，不触碰后端运行状态。
func (a *App) cancelPrompt(mode string) {
	a.state.Input.Mode = state.InputStateModeMessage
	a.state.Input.Text = ""
	a.state.Input.Cursor = 0
	a.state.Input.Prompt = ""
	a.state.Input.Options = nil
	a.state.Mode = state.InputModeInput
	a.appendStream(state.StreamEntry{
		ID:        fmt.Sprintf("prompt-cancel-%d", time.Now().UnixNano()),
		Type:      "status",
		Timestamp: time.Now(),
		Content:   fmt.Sprintf("%s cancelled", emptyDash(mode)),
		Metadata:  map[string]any{"done": true},
	})
}

// openOverlay 打开指定类型的浮层，重置搜索状态。
func (a *App) openOverlay(overlayType state.OverlayType) {
	a.state.Overlay.Active = overlayType
	a.state.Overlay.Query = ""
	a.state.Overlay.Selected = 0
}

// closeOverlay 关闭当前浮层，重置搜索与选中状态。
func (a *App) closeOverlay() {
	a.state.Overlay.Active = state.OverlayNone
	a.state.Overlay.Query = ""
	a.state.Overlay.Selected = 0
}

// enterInputFromNormal 从 Normal 进入 Input Mode，并清除 Normal 专属子状态（搜索）。
func (a *App) enterInputFromNormal() {
	a.state.Mode = state.InputModeInput
	a.clearSearchAndEx()
}

// openEx 打开 : 命令行输入 overlay。
func (a *App) openEx() {
	a.state.Ex.Active = true
	a.state.Ex.Input = ""
	a.openOverlay(state.OverlayEx)
}

// openSearch 打开 / 搜索输入 overlay。
func (a *App) openSearch() {
	a.state.Search.Active = true
	a.state.Search.Query = ""
	a.openOverlay(state.OverlaySearch)
}

// clearSearchAndEx 清除搜索与 Ex 输入状态（切出 Normal 或事件触发时调用）。
func (a *App) clearSearchAndEx() {
	a.state.Search = state.SearchState{}
	a.state.Ex = state.ExState{}
}

// executeExCommand 解释并执行 : 命令（已去除前缀 ":"），返回副作用 cmd。
//
// 支持命令：q/quit/exit=退出、debug=切调试、help=开帮助、compact=触发压缩、
// mode=切换 Agent 模式。空或未知命令给出提示。
func (a *App) executeExCommand(command string) tea.Cmd {
	switch command {
	case "q", "quit", "exit":
		return tea.Quit
	case "debug":
		a.debug = !a.debug
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("debug-toggle-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("Debug: %v", a.debug),
			Metadata:  map[string]any{"done": true},
		})
		return nil
	case "help":
		a.openOverlay(state.OverlayHelp)
		return nil
	case "compact":
		return a.triggerCompact()
	case "mode":
		return a.toggleAgentMode()
	default:
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("ex-unknown-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("Unknown ex command: %s", emptyDash(command)),
			Metadata:  map[string]any{"done": true},
		})
		return nil
	}
}

// executeSearch 执行全量扫描并记录匹配索引到 Search.Matches，滚动到首个匹配。
//
// 空 query 为 no-op（关闭搜索 overlay）；无匹配给出提示。
func (a *App) executeSearch(query string) tea.Cmd {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	matches := components.RunSearch(a.state.Stream, query)
	a.state.Search.Matches = matches
	a.state.Search.MatchIndex = 0
	a.state.Search.Stale = false
	if len(matches) == 0 {
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("search-empty-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("No matches: %s", query),
			Metadata:  map[string]any{"done": true},
		})
		return nil
	}
	a.scrollToStreamIndex(matches[0])
	return nil
}

// jumpSearchMatch 在搜索匹配间循环跳转（direction=1 下一个，-1 上一个）。
//
// 无匹配时静默 no-op；到末尾/首位循环折返。
func (a *App) jumpSearchMatch(direction int) {
	matches := a.state.Search.Matches
	if len(matches) == 0 {
		return
	}
	a.state.Search.MatchIndex = (a.state.Search.MatchIndex + direction + len(matches)) % len(matches)
	a.scrollToStreamIndex(matches[a.state.Search.MatchIndex])
}

// scrollToStreamIndex 滚动 stream 使指定全局 entry 索引尽量可见。
//
// 由于 state.Stream 是 append-only 且全量在内存，这里基于目标索引估算
// 滚动偏移（粗略：将目标定位到视口中部），足够满足跳转可见需求。
func (a *App) scrollToStreamIndex(targetIndex int) {
	if targetIndex < 0 || targetIndex >= len(a.state.Stream) {
		return
	}
	// 粗略估计：stream 行数约为 entry 数的倍数，这里直接用 entry 索引作为
	// 偏移参考，关闭自动滚动并尝试把目标带到视口。精确视口定位由 stream
	// 渲染时的 visibleLines 兜底（超出范围会被 clamp）。
	a.state.Layout.AutoScroll = false
	// 反向估算：偏移越大表示越靠顶部。目标越靠后(索引大)越接近底部，偏移越小。
	estimated := len(a.state.Stream) - targetIndex
	if estimated < 0 {
		estimated = 0
	}
	a.state.Layout.ScrollOffset = estimated
}

// handlePaletteCommand 处理命令面板选择的命令。
func (a *App) handlePaletteCommand(msg components.PaletteCommandMsg) tea.Cmd {
	switch msg.Name {
	case "/exit":
		return tea.Quit
	case "/help":
		a.openOverlay(state.OverlayHelp)
		return nil
	case "/session":
		a.openOverlay(state.OverlaySessionPicker)
		return nil
	case "/model":
		a.openOverlay(state.OverlayModelPicker)
		return nil
	case "/mode":
		return a.toggleAgentMode()
	case "/compact":
		return a.triggerCompact()
	case "/clear":
		a.state.Stream = nil
		a.bindComponents()
		return nil
	default:
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("cmd-%s-%d", msg.Name, time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("command %s not yet implemented", msg.Name),
			Metadata:  map[string]any{"done": true},
		})
	}
	return nil
}

// handleSlashCommand 处理 Slash 命令输入。
func (a *App) handleSlashCommand(msg components.SlashCommandMsg) tea.Cmd {
	switch msg.Command {
	case "/exit", "/quit":
		return tea.Quit
	case "/help":
		a.openOverlay(state.OverlayHelp)
		return nil
	case "/session":
		a.openOverlay(state.OverlaySessionPicker)
		return nil
	case "/model":
		a.openOverlay(state.OverlayModelPicker)
		return nil
	case "/mode":
		return a.toggleAgentMode()
	case "/compact":
		return a.triggerCompact()
	case "/clear":
		a.state.Stream = nil
		a.bindComponents()
		return nil
	default:
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("slash-%s-%d", msg.Command, time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("unknown command: %s", msg.Command),
			Metadata:  map[string]any{"done": true},
		})
	}
	return nil
}

// handleSessionSelect 处理会话切换操作。
//
// 切换前先把当前活动会话 ID 存入 prevSessionID，供 Leader Space Space 回切。
func (a *App) handleSessionSelect(msg components.SessionSelectMsg) tea.Cmd {
	if a.client == nil {
		return nil
	}
	if current := a.activeSessionID(); current != "" && current != msg.Session.ID {
		a.prevSessionID = current
	}
	a.state.Gateway.ActiveSess = &msg.Session
	return loadSessionCmd(a.client, msg.Session.ID)
}

// handleSessionDelete 通过确认弹窗处理会话删除操作。
func (a *App) handleSessionDelete(msg components.SessionDeleteMsg) tea.Cmd {
	if a.client == nil {
		return nil
	}
	a.openConfirm(
		"Delete Session",
		fmt.Sprintf("Are you sure you want to delete this session?"),
		"delete_session",
		map[string]any{"session_id": msg.SessionID},
	)
	return nil
}

// handleSessionCreated 处理新会话创建完成。
func (a *App) handleSessionCreated(msg sessionCreatedMsg) tea.Cmd {
	if msg.err != nil {
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("session-err-%d", time.Now().UnixNano()),
			Type:      "error",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("Failed to create session: %s", msg.err),
			Metadata:  map[string]any{"done": true},
		})
		return nil
	}
	if msg.Session != nil {
		a.state.Gateway.ActiveSess = msg.Session
		a.appendStream(state.StreamEntry{
			ID:        fmt.Sprintf("session-created-%d", time.Now().UnixNano()),
			Type:      "status",
			Timestamp: time.Now(),
			Content:   fmt.Sprintf("New session created: %s", msg.Session.Title),
			Metadata:  map[string]any{"done": true},
		})
		if a.client != nil {
			return loadSessionCmd(a.client, msg.Session.ID)
		}
	}
	return nil
}

// handleModelSelect 处理模型切换操作。
func (a *App) handleModelSelect(msg components.ModelSelectMsg) tea.Cmd {
	if a.client != nil {
		sessionID := a.activeSessionID()
		if err := a.client.SetModel(context.Background(), sessionID, msg.ModelID); err != nil {
			a.appendStream(state.StreamEntry{
				ID:        fmt.Sprintf("model-err-%d", time.Now().UnixNano()),
				Type:      "error",
				Timestamp: time.Now(),
				Content:   fmt.Sprintf("Failed to switch model: %s", err),
				Metadata:  map[string]any{"done": true},
			})
			return nil
		}
	}
	if a.state.Gateway.ActiveSess != nil {
		a.state.Gateway.ActiveSess.Model = msg.ModelID
	}
	a.state.Gateway.ActiveModel = msg.ModelID
	a.appendStream(state.StreamEntry{
		ID:        fmt.Sprintf("model-switch-%d", time.Now().UnixNano()),
		Type:      "status",
		Timestamp: time.Now(),
		Content:   fmt.Sprintf("Model switched to %s", msg.ModelName),
		Metadata:  map[string]any{"done": true},
	})
	return nil
}

// handleConfirmYes 处理确认弹窗的确认操作。
func (a *App) handleConfirmYes(msg components.ConfirmYesMsg) tea.Cmd {
	confirm := a.state.Confirm
	a.state.Confirm = state.ConfirmState{}
	a.closeOverlay()
	switch confirm.Action {
	case "delete_session":
		sessionID, _ := confirm.Data["session_id"].(string)
		if sessionID != "" && a.client != nil {
			return deleteSessionCmd(a.client, sessionID)
		}
	}
	return nil
}

// openConfirm 打开确认弹窗。
func (a *App) openConfirm(title, message, action string, data map[string]any) {
	a.state.Confirm = state.ConfirmState{
		Title:   title,
		Message: message,
		Action:  action,
		Data:    data,
	}
	a.state.Overlay.Active = state.OverlayConfirm
	a.state.Overlay.Query = ""
	a.state.Overlay.Selected = 0
}

// activeSessionID 返回当前会话 ID，缺失时使用空字符串让 GatewayClient 自行决定错误语义。
func (a *App) activeSessionID() string {
	if a.state.Gateway.ActiveSess != nil {
		return a.state.Gateway.ActiveSess.ID
	}
	return ""
}

// activeSessionTitle 返回当前会话标题，缺失时回退到会话 ID 或占位文本。
func (a *App) activeSessionTitle() string {
	if a.state.Gateway.ActiveSess != nil {
		if a.state.Gateway.ActiveSess.Title != "" {
			return a.state.Gateway.ActiveSess.Title
		}
		return a.state.Gateway.ActiveSess.ID
	}
	return "untitled"
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
	return theme.SubtleStyle().Render(strings.Repeat("─", width))
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
				lines = append(lines, strings.Repeat(" ", width-1))
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
	fitted := theme.Truncate(line, target)
	lineWidth := theme.DisplayWidth(fitted)
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

// appendStream 以追加新 entry 的方式维护不可变 StreamEntry 序列。
func (a *App) appendStream(entry state.StreamEntry) {
	a.state.Stream = append(a.state.Stream, entry)
}

// bindComponents 将子组件重新绑定到当前 ViewState 指针。
// 注意：state.Reduce 每次返回新的 *ViewState，a.state 会被替换，因此所有
// 子组件（含浮层：palette / help / sessionPicker / modelPicker / confirmOverlay）
// 都必须在这里重新绑定，否则会持有旧指针，导致浮层交互改到废弃状态上、
// 出现"回车不关闭面板、跳回第一项"等问题。
func (a *App) bindComponents() {
	a.ambientStatus = components.NewAmbientStatus(a.state)
	a.agentStream = components.NewAgentStream(a.state)
	a.commandPrompt = components.NewCommandPrompt(a.state)
	a.cmdLine = components.NewCmdLine(a.state)
	a.softInspector = components.NewSoftInspector(a.state)
	a.palette = components.NewPalette(a.state)
	a.helpOverlay = components.NewHelpOverlay(a.state)
	a.sessionPicker = components.NewSessionPicker(a.state)
	a.modelPicker = components.NewModelPicker(a.state)
	a.confirmOverlay = components.NewConfirmOverlay(a.state)
}

// toggleAgentMode 切换 Agent 模式 (build/plan) 并追加状态提示。
func (a *App) toggleAgentMode() tea.Cmd {
	mode := "plan"
	if a.state.Runtime.AgentMode == "plan" || a.state.Runtime.AgentMode == "" {
		mode = "build"
	}
	a.state.Runtime.AgentMode = mode
	a.appendStream(state.StreamEntry{
		ID:        fmt.Sprintf("mode-toggle-%d", time.Now().UnixNano()),
		Type:      "status",
		Timestamp: time.Now(),
		Content:   fmt.Sprintf("Agent mode: %s", mode),
		Metadata:  map[string]any{"done": true},
	})
	return nil
}

// toggleFullAccess 切换 Full Access 模式并追加状态提示。
func (a *App) toggleFullAccess() tea.Cmd {
	a.state.Runtime.FullAccess = !a.state.Runtime.FullAccess
	label := "off"
	if a.state.Runtime.FullAccess {
		label = "on"
	}
	a.appendStream(state.StreamEntry{
		ID:        fmt.Sprintf("access-toggle-%d", time.Now().UnixNano()),
		Type:      "status",
		Timestamp: time.Now(),
		Content:   fmt.Sprintf("Full access: %s", label),
		Metadata:  map[string]any{"done": true},
	})
	return nil
}

// triggerCompact 触发手动 compact 并追加状态提示。
func (a *App) triggerCompact() tea.Cmd {
	a.appendStream(state.StreamEntry{
		ID:        fmt.Sprintf("compact-%d", time.Now().UnixNano()),
		Type:      "status",
		Timestamp: time.Now(),
		Content:   "Compact triggered",
		Metadata:  map[string]any{"done": true},
	})
	return nil
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
			"done":   true,
		},
	}
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

// submitMessageCmd 调用 GatewayClient 发送用户消息，并把 ACK 转成 reducer 可消费事件。
func submitMessageCmd(client gateway.Client, sessionID string, text string) tea.Cmd {
	return func() tea.Msg {
		ack, err := client.SendMessage(context.Background(), sessionID, text)
		if err != nil {
			return gatewayEventMsg{event: errorEvent(err)}
		}
		return gatewayEventMsg{event: gateway.GatewayEvent{
			Type:      gateway.EventRunStarted,
			SessionID: ack.SessionID,
			RunID:     ack.RunID,
			Payload:   map[string]any{"message": ack.Message, "accepted": ack.Accepted},
			At:        time.Now(),
		}}
	}
}

// resolvePermissionCmd 调用 GatewayClient 提交权限决策，并把完成结果转成 GatewayEvent。
func resolvePermissionCmd(client gateway.Client, decision gateway.PermissionDecision) tea.Cmd {
	return func() tea.Msg {
		if err := client.ResolvePermission(context.Background(), decision); err != nil {
			return gatewayEventMsg{event: errorEvent(err)}
		}
		text := "permission denied"
		if decision.Allow {
			text = "permission allowed"
		}
		return gatewayEventMsg{event: gateway.GatewayEvent{
			Type:      gateway.EventPermissionResolved,
			SessionID: decision.SessionID,
			RunID:     decision.RunID,
			Payload:   map[string]any{"decision": decision.Reason, "message": text},
			At:        time.Now(),
		}}
	}
}

// answerQuestionCmd 调用 GatewayClient 提交 ask_user 回答，并把完成结果转成 GatewayEvent。
func answerQuestionCmd(client gateway.Client, answer gateway.UserQuestionAnswer) tea.Cmd {
	return func() tea.Msg {
		if err := client.AnswerUserQuestion(context.Background(), answer); err != nil {
			return gatewayEventMsg{event: errorEvent(err)}
		}
		return gatewayEventMsg{event: gateway.GatewayEvent{
			Type:      gateway.EventUserQuestionAnswered,
			SessionID: answer.SessionID,
			RunID:     answer.RunID,
			Payload:   map[string]any{"answer": answer.Text, "message": "answer submitted"},
			At:        time.Now(),
		}}
	}
}

// errorEvent 将 GatewayClient RPC 错误包装成统一错误事件。
func errorEvent(err error) gateway.GatewayEvent {
	return gateway.GatewayEvent{
		Type:    gateway.EventError,
		Payload: map[string]any{"message": err.Error()},
		At:      time.Now(),
	}
}

// cancelRunCmd 调用 GatewayClient 取消运行中的 Agent，并把完成结果转成 GatewayEvent。
func cancelRunCmd(client gateway.Client, sessionID string, runID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.CancelRun(context.Background(), sessionID, runID); err != nil {
			return gatewayEventMsg{event: errorEvent(err)}
		}
		return gatewayEventMsg{event: gateway.GatewayEvent{
			Type:      gateway.EventRunCancelled,
			SessionID: sessionID,
			RunID:     runID,
			Payload:   map[string]any{"message": "run cancelled by user"},
			At:        time.Now(),
		}}
	}
}

// loadSessionCmd 切换到指定会话并建立新的事件订阅。
func loadSessionCmd(client gateway.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		detail, err := client.LoadSession(context.Background(), sessionID)
		if err != nil {
			return gatewayEventMsg{event: errorEvent(err)}
		}
		eventCh, err := client.SubscribeEvents(context.Background(), sessionID)
		if err != nil {
			return gatewayEventMsg{event: errorEvent(err)}
		}
		return sessionSwitchedMsg{sessionID: sessionID, detail: detail, eventCh: eventCh}
	}
}

// deleteSessionCmd 调用 GatewayClient 删除会话。
func deleteSessionCmd(client gateway.Client, sessionID string) tea.Cmd {
	return func() tea.Msg {
		// Gateway Client 接口暂无 DeleteSession，此处预留
		return gatewayEventMsg{event: gateway.GatewayEvent{
			Type:      gateway.EventSessionDeleted,
			SessionID: sessionID,
			Payload:   map[string]any{"id": sessionID, "message": "session deleted"},
			At:        time.Now(),
		}}
	}
}

// sessionSwitchedMsg 表示会话切换完成。
type sessionSwitchedMsg struct {
	sessionID string
	detail    *gateway.SessionDetail
	eventCh   <-chan gateway.GatewayEvent
}

// sessionCreatedMsg 表示新会话创建完成。
type sessionCreatedMsg struct {
	Session *gateway.SessionSummary
	err     error
}

// createSessionCmd 通过 GatewayClient 创建新会话。
func createSessionCmd(client gateway.Client) tea.Cmd {
	return func() tea.Msg {
		summary, err := client.CreateSession(context.Background())
		if err != nil {
			return sessionCreatedMsg{err: err}
		}
		return sessionCreatedMsg{Session: summary}
	}
}
