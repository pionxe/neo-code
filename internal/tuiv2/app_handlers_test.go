package tuiv2

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/tuiv2/components"
	"neo-code/internal/tuiv2/fakegateway"
	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/state"
)

// ---- 消息处理器 ----

func TestHandleSubmitMessageAppendsAndReturnsCmd(t *testing.T) {
	app := newReadyApp(t)
	updated, cmd := app.Update(components.SubmitMessageMsg{Text: "  hello  "})
	app = updated.(*App)
	if cmd == nil {
		t.Fatal("submit with client returned nil cmd")
	}
	last := app.state.Stream[len(app.state.Stream)-1]
	if last.Content != "  hello  " || last.Metadata["role"] != "user" {
		t.Fatalf("user message not appended: %+v", last)
	}

	// 空文本 -> nil
	if _, cmd := app.Update(components.SubmitMessageMsg{Text: "   "}); cmd != nil {
		t.Fatal("empty submit should return nil cmd")
	}
}

func TestHandleSubmitMessageNoClient(t *testing.T) {
	app := NewApp(StartupConfig{Backend: "fake", Scenario: "default"}).(*App)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = updated.(*App)
	_, cmd := app.Update(components.SubmitMessageMsg{Text: "hi"})
	if cmd != nil {
		t.Fatal("submit without client should return nil cmd")
	}
}

func TestPermissionAndQuestionHandlers(t *testing.T) {
	app := newReadyApp(t)
	app.state.Runtime.RunID = "run-1"

	if _, cmd := app.Update(components.PermissionActionMsg{Decision: "y"}); cmd == nil {
		t.Fatal("permission action with client returned nil")
	}
	if _, cmd := app.Update(components.QuestionAnswerMsg{Text: "answer"}); cmd == nil {
		t.Fatal("question answer with client returned nil")
	}

	// 无 client -> nil
	noc := NewApp(StartupConfig{Backend: "fake", Scenario: "default"}).(*App)
	if _, cmd := noc.Update(components.PermissionActionMsg{Decision: "y"}); cmd != nil {
		t.Fatal("permission without client should be nil")
	}
	if _, cmd := noc.Update(components.QuestionAnswerMsg{Text: "x"}); cmd != nil {
		t.Fatal("question without client should be nil")
	}
}

func TestCancelPromptResetsInputAndLogs(t *testing.T) {
	app := newReadyApp(t)
	app.state.Input.Mode = state.InputStateModeQuestionAnswer
	app.state.Input.Text = "abc"
	app.state.Input.Options = []string{"x"}
	app.Update(components.PromptCancelMsg{Mode: state.InputStateModeQuestionAnswer})

	if app.state.Input.Mode != state.InputStateModeMessage || app.state.Input.Text != "" {
		t.Fatalf("input not reset: %+v", app.state.Input)
	}
	if app.state.Input.Options != nil {
		t.Fatalf("options not cleared: %+v", app.state.Input.Options)
	}
}

// ---- Slash / Palette 命令分发 ----

func TestSlashCommandDispatch(t *testing.T) {
	cases := map[string]func(*App) bool{
		"/session": func(a *App) bool { return a.state.Overlay.Active == "session_picker" },
		"/model":   func(a *App) bool { return a.state.Overlay.Active == "model_picker" },
		"/help":    func(a *App) bool { return a.state.Overlay.Active == "help" },
		"/mode":    func(a *App) bool { return a.state.Runtime.AgentMode == "build" },
		"/compact": func(a *App) bool { return lastContains(a, "Compact triggered") },
		"/clear":   func(a *App) bool { return len(a.state.Stream) == 0 },
		"/bogus":   func(a *App) bool { return lastContains(a, "unknown command") },
	}
	for cmd, check := range cases {
		t.Run(cmd, func(t *testing.T) {
			app := newReadyApp(t)
			app.Update(components.SlashCommandMsg{Command: cmd})
			if !check(app) {
				t.Fatalf("%s did not produce expected effect: overlay=%q stream=%v", cmd, app.state.Overlay.Active, streamContents(app))
			}
		})
	}
	// /exit / /quit -> tea.Quit
	app := newReadyApp(t)
	if _, cmd := app.Update(components.SlashCommandMsg{Command: "/exit"}); cmd == nil {
		t.Fatal("/exit should return quit cmd")
	}
}

func TestPaletteCommandDispatch(t *testing.T) {
	cases := map[string]func(*App) bool{
		"/session":    func(a *App) bool { return a.state.Overlay.Active == "session_picker" },
		"/model":      func(a *App) bool { return a.state.Overlay.Active == "model_picker" },
		"/help":       func(a *App) bool { return a.state.Overlay.Active == "help" },
		"/mode":       func(a *App) bool { return a.state.Runtime.AgentMode == "build" },
		"/compact":    func(a *App) bool { return lastContains(a, "Compact triggered") },
		"/checkpoint": func(a *App) bool { return lastContains(a, "not yet implemented") },
	}
	for name, check := range cases {
		t.Run(name, func(t *testing.T) {
			app := newReadyApp(t)
			app.Update(components.PaletteCommandMsg{Name: name})
			if !check(app) {
				t.Fatalf("%s did not produce expected effect: overlay=%q stream=%v", name, app.state.Overlay.Active, streamContents(app))
			}
		})
	}
	app := newReadyApp(t)
	if _, cmd := app.Update(components.PaletteCommandMsg{Name: "/exit"}); cmd == nil {
		t.Fatal("/exit should return quit cmd")
	}
}

// ---- Ctrl+C 双退保护 ----

func TestHandleCtrlCIdleHintAndDoubleQuit(t *testing.T) {
	app := newReadyApp(t)
	// 空闲单次 -> 提示
	_, cmd := app.handleCtrlC()
	if cmd != nil || !lastContains(app, "Press Ctrl+C again to quit") {
		t.Fatalf("idle single Ctrl+C should hint, cmd=%v stream=%v", cmd, streamContents(app))
	}
	// 运行中 -> 取消
	app.state.Runtime.Phase = state.RuntimePhaseRunning
	_, cmd = app.handleCtrlC()
	if cmd == nil {
		t.Fatal("running Ctrl+C should return cancel cmd")
	}
}

func TestHandleCtrlCCancelWithoutClient(t *testing.T) {
	app := NewApp(StartupConfig{Backend: "fake", Scenario: "default"}).(*App)
	app.state.Runtime.Phase = state.RuntimePhaseWaitingPermission
	_, _ = app.handleCtrlC()
	if app.state.Runtime.Phase != state.RuntimePhaseCancelled {
		t.Fatalf("phase=%s, want cancelled", app.state.Runtime.Phase)
	}
}

// ---- 会话/模型/确认 ----

func TestHandleSessionCreatedErrorAndSuccess(t *testing.T) {
	// 错误路径
	app := newReadyApp(t)
	app.Update(sessionCreatedMsg{err: errSentinel("boom")})
	if !lastContains(app, "Failed to create session") {
		t.Fatalf("error path not logged: %v", streamContents(app))
	}
	// 成功路径
	app = newReadyApp(t)
	_, cmd := app.Update(sessionCreatedMsg{Session: &gateway.SessionSummary{ID: "new-1", Title: "New"}})
	if app.state.Gateway.ActiveSess == nil || app.state.Gateway.ActiveSess.ID != "new-1" {
		t.Fatal("active session not set on create")
	}
	if !lastContains(app, "New session created") {
		t.Fatalf("create not logged: %v", streamContents(app))
	}
	if cmd == nil {
		t.Fatal("create with client should return load cmd")
	}
}

func TestHandleSessionDeleteOpensConfirm(t *testing.T) {
	app := newReadyApp(t)
	app.Update(components.SessionDeleteMsg{SessionID: "s-x"})
	if app.state.Overlay.Active != "confirm" || app.state.Confirm.Action != "delete_session" {
		t.Fatalf("confirm not opened: overlay=%q confirm=%+v", app.state.Overlay.Active, app.state.Confirm)
	}
	// 无 client -> 忽略
	noc := NewApp(StartupConfig{Backend: "fake", Scenario: "default"}).(*App)
	if _, cmd := noc.Update(components.SessionDeleteMsg{SessionID: "s-x"}); cmd != nil {
		t.Fatal("delete without client should be nil")
	}
}

func TestHandleModelSelectSwitches(t *testing.T) {
	app := newReadyApp(t)
	app.Update(components.ModelSelectMsg{ModelID: "neo-fake-fast", ModelName: "Neo Fake Fast"})
	if app.state.Gateway.ActiveModel != "neo-fake-fast" || !lastContains(app, "Model switched") {
		t.Fatalf("model not switched: model=%q stream=%v", app.state.Gateway.ActiveModel, streamContents(app))
	}
}

func TestHandleConfirmYesDeletesSession(t *testing.T) {
	app := newReadyApp(t)
	app.openConfirm("Delete Session", "msg", "delete_session", map[string]any{"session_id": "sess-9"})
	_, cmd := app.Update(components.ConfirmYesMsg{})
	if app.state.Overlay.Active != "" {
		t.Fatalf("confirm should close, active=%q", app.state.Overlay.Active)
	}
	if cmd == nil {
		t.Fatal("confirm yes on delete_session should return delete cmd")
	}
}

// ---- toggle / compact ----

func TestToggleAgentModeToggleFullAccessTriggerCompact(t *testing.T) {
	app := newReadyApp(t)
	app.toggleAgentMode()
	if app.state.Runtime.AgentMode != "build" {
		t.Fatalf("agent mode=%s, want build", app.state.Runtime.AgentMode)
	}
	app.toggleAgentMode()
	if app.state.Runtime.AgentMode != "plan" {
		t.Fatalf("agent mode=%s, want plan", app.state.Runtime.AgentMode)
	}
	app.toggleFullAccess()
	if !app.state.Runtime.FullAccess {
		t.Fatal("full access should be on")
	}
	app.triggerCompact()
	if !lastContains(app, "Compact triggered") {
		t.Fatal("compact not triggered")
	}
}

// ---- tea.Cmd 工厂 ----

func TestCmdFactories(t *testing.T) {
	client, err := fakegateway.NewFakeClient(fakegateway.ScenarioDefault)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "session-ghost-console"

	if msg := submitMessageCmd(client, sessionID, "hi")(); msg == nil {
		t.Fatal("submitMessageCmd returned nil")
	}
	if msg := resolvePermissionCmd(client, gateway.PermissionDecision{Allow: true, Reason: "y"})(); msg == nil {
		t.Fatal("resolvePermissionCmd returned nil")
	}
	if msg := answerQuestionCmd(client, gateway.UserQuestionAnswer{Text: "a"})(); msg == nil {
		t.Fatal("answerQuestionCmd returned nil")
	}
	if msg := cancelRunCmd(client, sessionID, "run-1")(); msg == nil {
		t.Fatal("cancelRunCmd returned nil")
	}
	if msg := createSessionCmd(client)(); msg == nil {
		t.Fatal("createSessionCmd returned nil")
	}
	if msg := deleteSessionCmd(client, sessionID)(); msg == nil {
		t.Fatal("deleteSessionCmd returned nil")
	}
	if msg := loadSessionCmd(client, sessionID)(); msg == nil {
		t.Fatal("loadSessionCmd returned nil")
	}
	// errorEvent 包装
	if ge := errorEvent(errSentinel("x")); ge.Type != gateway.EventError {
		t.Fatalf("errorEvent wrong: %+v", ge)
	}
}

func TestCmdFactoriesErrorPath(t *testing.T) {
	// 关闭客户端 -> 各 RPC 返回 errorEvent
	client, _ := fakegateway.NewFakeClient(fakegateway.ScenarioDefault)
	_ = client.Close()

	for _, c := range []tea.Cmd{
		submitMessageCmd(client, "s", "x"),
		resolvePermissionCmd(client, gateway.PermissionDecision{}),
		answerQuestionCmd(client, gateway.UserQuestionAnswer{}),
		cancelRunCmd(client, "s", "r"),
		loadSessionCmd(client, "s"),
	} {
		msg := c()
		ge, ok := msg.(gatewayEventMsg)
		if !ok {
			t.Fatalf("expected gatewayEventMsg on closed client, got %T", msg)
		}
		if ge.event.Type != gateway.EventError {
			t.Fatalf("expected EventError, got %s", ge.event.Type)
		}
	}
}

func TestLoadInitialCmdOffline(t *testing.T) {
	client, _ := fakegateway.NewFakeClient(fakegateway.ScenarioGatewayOffline)
	msg := loadInitialCmd(client)()
	loaded, ok := msg.(initialLoadedMsg)
	if !ok {
		t.Fatalf("expected initialLoadedMsg, got %T", msg)
	}
	if loaded.errText == "" {
		t.Fatal("offline scenario should set errText")
	}
}

// ---- 模式键分发 ----

func TestNormalModeKeyDispatch(t *testing.T) {
	app := newReadyApp(t)
	app.state.Mode = state.NormalMode

	// i -> 进入 InputMode
	updated, _ := app.Update(keyRunes("i"))
	app = updated.(*App)
	if app.state.Mode != state.InputModeInput {
		t.Fatalf("after i: mode=%v, want input", app.state.Mode)
	}

	// q -> 退出
	app = newReadyApp(t)
	app.state.Mode = state.NormalMode
	if _, cmd := app.Update(keyRunes("q")); cmd == nil {
		t.Fatal("q should return quit cmd")
	}

	// Space -> Leader
	app = newReadyApp(t)
	app.state.Mode = state.NormalMode
	updated, _ = app.Update(keyRunes(" "))
	app = updated.(*App)
	if app.state.Mode != state.LeaderMode {
		t.Fatalf("after space: mode=%v, want leader", app.state.Mode)
	}

	// ? / : 在 NormalMode 下是预留动作（空操作），不应崩溃也不应开浮层
	app = newReadyApp(t)
	app.state.Mode = state.NormalMode
	app.Update(keyRunes("?"))
	app.Update(keyRunes(":"))

	// 滚动键交给 stream（不 panic 即可）
	app = newReadyApp(t)
	app.state.Mode = state.NormalMode
	app.Update(keyRunes("j"))
	app.Update(keyRunes("k"))
	app.Update(keyRunes("g"))
	app.Update(keyRunes("G"))
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
}

func TestLeaderKeyDispatch(t *testing.T) {
	cases := map[string]string{
		"p": "palette",
		"s": "session_picker",
		"h": "help",
	}
	for key, wantOverlay := range cases {
		t.Run(key, func(t *testing.T) {
			app := newReadyApp(t)
			app.state.Mode = state.LeaderMode
			updated, _ := app.Update(keyRunes(key))
			app = updated.(*App)
			if app.state.Overlay.Active != wantOverlay {
				t.Fatalf("leader %s: overlay=%q, want %q", key, app.state.Overlay.Active, wantOverlay)
			}
			if app.state.Mode != state.NormalMode {
				t.Fatalf("leader suffix should reset to normal, mode=%v", app.state.Mode)
			}
		})
	}
	// m -> toggle mode, f -> full access, c -> compact, l -> log（均返回 nil）
	for _, key := range []string{"m", "f", "c", "l"} {
		app := newReadyApp(t)
		app.state.Mode = state.LeaderMode
		if _, cmd := app.Update(keyRunes(key)); cmd != nil {
			t.Fatalf("leader %s should return nil cmd", key)
		}
	}
	// n -> 创建会话（有 client 时返回 create cmd）
	app := newReadyApp(t)
	app.state.Mode = state.LeaderMode
	if _, cmd := app.Update(keyRunes("n")); cmd == nil {
		t.Fatal("leader n should return create cmd")
	}
	// esc -> 回到 normal
	app = newReadyApp(t)
	app.state.Mode = state.LeaderMode
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*App)
	if app.state.Mode != state.NormalMode {
		t.Fatalf("leader esc should reset to normal, mode=%v", app.state.Mode)
	}
}

func TestInputModeKeyDispatch(t *testing.T) {
	app := newReadyApp(t)
	// esc -> normal
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*App)
	if app.state.Mode != state.NormalMode {
		t.Fatalf("esc: mode=%v", app.state.Mode)
	}
	// ctrl+p -> palette
	app = newReadyApp(t)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	app = updated.(*App)
	if app.state.Overlay.Active != "palette" {
		t.Fatalf("ctrl+p: overlay=%q", app.state.Overlay.Active)
	}
	// ctrl+l -> 日志提示
	app = newReadyApp(t)
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if !lastContains(app, "Log viewer not yet available") {
		t.Fatalf("ctrl+l not logged: %v", streamContents(app))
	}
}

func TestRouteStreamKey(t *testing.T) {
	app := newReadyApp(t)
	if ok, _ := app.routeStreamKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}); !ok {
		t.Fatal("j should route to stream")
	}
	if ok, _ := app.routeStreamKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}); ok {
		t.Fatal("x should not route to stream")
	}
}

// ---- 鼠标 ----

func TestHandleMouseMsgMainViewAndOverlays(t *testing.T) {
	// 主视图滚轮：MouseWheelUp 增加 offset，MouseWheelDown 减少（内容不足也允许推进）
	app := newReadyApp(t)
	app.Update(tea.MouseMsg{Type: tea.MouseWheelUp})
	if app.state.Layout.ScrollOffset == 0 {
		t.Fatal("MouseWheelUp should increase scroll offset")
	}
	app.Update(tea.MouseMsg{Type: tea.MouseWheelDown})

	// 浮层鼠标分发不 panic：组件按 Button 判定，故设置 Button
	for _, active := range []string{"palette", "session_picker", "model_picker"} {
		app := newReadyApp(t)
		app.openOverlay(active)
		app.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
		app.Update(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: 4})
	}
}

// ---- View 各路径 ----

func TestViewOverlayAndMainPaths(t *testing.T) {
	overlays := []string{"palette", "help", "session_picker", "model_picker", "confirm"}
	for _, ov := range overlays {
		t.Run(ov, func(t *testing.T) {
			app := newReadyApp(t)
			app.openOverlay(ov)
			if app.state.Overlay.Active != ov {
				app.openOverlay(ov)
			}
			view := app.View()
			if view == "" {
				t.Fatalf("%s view empty", ov)
			}
		})
	}

	// 主视图：error 行 + debug 行
	app := newReadyApp(t)
	app.lastErr = "boom"
	app.debug = true
	view := app.View()
	if !strings.Contains(view, "boom") {
		t.Fatal("error line not rendered")
	}
	if !strings.Contains(view, "[debug]") {
		t.Fatal("debug line not rendered")
	}
}

func TestViewInspectorLayoutBreakpoints(t *testing.T) {
	for _, w := range []int{79, 90, 120} {
		app := newReadyApp(t)
		app.Update(tea.WindowSizeMsg{Width: w, Height: 30})
		if view := app.View(); view == "" {
			t.Fatalf("width %d view empty", w)
		}
	}
}

func TestViewZeroSizeFallback(t *testing.T) {
	app := newReadyApp(t)
	app.state.Layout.Width = 0
	app.state.Layout.Height = 0
	if view := app.View(); view == "" {
		t.Fatal("zero-size view empty")
	}
}

// ---- 辅助 ----

func TestUtilityHelpers(t *testing.T) {
	if emptyDash("") != "-" || emptyDash("x") != "x" {
		t.Fatal("emptyDash wrong")
	}
	if inputModeName(state.NormalMode) != "normal" || inputModeName(state.LeaderMode) != "leader" || inputModeName(state.InputModeInput) != "input" {
		t.Fatal("inputModeName wrong")
	}
	app := newReadyApp(t)
	if app.activeSessionID() == "" {
		t.Fatal("activeSessionID should be non-empty after load")
	}
	if app.activeSessionTitle() == "untitled" {
		// 默认场景首个会话有标题
		t.Fatalf("activeSessionTitle=%q, want real title", app.activeSessionTitle())
	}
	// 无活动会话
	app.state.Gateway.ActiveSess = nil
	if app.activeSessionID() != "" || app.activeSessionTitle() != "untitled" {
		t.Fatal("nil session fallback wrong")
	}
	// 无标题会话 -> 回退到 ID
	app.state.Gateway.ActiveSess = &gateway.SessionSummary{ID: "id-only"}
	if app.activeSessionTitle() != "id-only" {
		t.Fatalf("title fallback wrong: %q", app.activeSessionTitle())
	}
}

func TestSeparatorAndFitLine(t *testing.T) {
	if s := separatorLineHelper(10); !strings.Contains(s, "─") {
		t.Fatal("separatorLine missing dash")
	}
	if fitLine("abc", 0) != "abc" {
		t.Fatal("fitLine width<=0 should return as-is")
	}
	if fitLine("abc", 1) != "" {
		t.Fatal("fitLine target<=0 should return empty")
	}
}

// ---- 测试辅助函数 ----

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

func keyRunes(s string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func lastContains(app *App, sub string) bool {
	if len(app.state.Stream) == 0 {
		return false
	}
	return strings.Contains(app.state.Stream[len(app.state.Stream)-1].Content, sub)
}

func streamContents(app *App) []string {
	out := make([]string, len(app.state.Stream))
	for i, e := range app.state.Stream {
		out[i] = e.Content
	}
	return out
}

// separatorLineHelper 以给定宽度渲染分隔线（覆盖 width<=0 分支）。
func separatorLineHelper(width int) string {
	app := NewApp(StartupConfig{Backend: "fake", Scenario: "default"}).(*App)
	app.state.Layout.Width = width
	return app.separatorLine()
}
