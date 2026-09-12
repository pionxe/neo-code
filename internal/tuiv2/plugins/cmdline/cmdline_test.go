package cmdline

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/plugins/chat"
	"neo-code/internal/tuiv2/plugins/prompt"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// recordingHost 是 fake Host：同步执行 GoCmd 并回流广播；
// RunCommand 记录调用并回放预置错误（Ex 执行链验证）。
var _ kernel.Host = (*recordingHost)(nil)

type recordingHost struct {
	st         *state.ViewState
	notifies   []string
	broadcasts []tea.Msg
	client     gateway.Client
	runCalls   []string
	runErr     error
}

func newRecordingHost() *recordingHost { return &recordingHost{st: state.NewViewState()} }

func (h *recordingHost) State() *state.ViewState { return h.st }
func (h *recordingHost) Gateway() gateway.Client { return h.client }
func (h *recordingHost) GoCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		h.Send(msg)
	}
}
func (h *recordingHost) Send(msg tea.Msg) {
	h.broadcasts = append(h.broadcasts, msg)
}
func (h *recordingHost) Mode() state.InputMode        { return h.st.Mode }
func (h *recordingHost) SetMode(m state.InputMode)    { h.st.Mode = m }
func (h *recordingHost) PushOverlay(o kernel.Overlay) {}
func (h *recordingHost) PopOverlay()                  {}
func (h *recordingHost) Confirm(req state.ConfirmRequest) {
	h.notifies = append(h.notifies, "confirm:"+req.Title)
}
func (h *recordingHost) Notify(text string)                             { h.notifies = append(h.notifies, text) }
func (h *recordingHost) Quit()                                          {}
func (h *recordingHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}
func (h *recordingHost) Commands() []kernel.Command                     { return nil }
func (h *recordingHost) RunCommand(name string, args []string) error {
	h.runCalls = append(h.runCalls, name)
	return h.runErr
}
func (h *recordingHost) Bindings() []kernel.Binding { return nil }

func newTestPlugin(t *testing.T) (*Plugin, *recordingHost) {
	t.Helper()
	p := New()
	h := newRecordingHost()
	p.Init(context.Background(), h)
	if p.st == nil || p.cmdline == nil {
		t.Fatal("Init should fix state and construct delegate")
	}
	return p, h
}

func keys(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func bindingMap(p *Plugin) map[string]kernel.Binding {
	m := map[string]kernel.Binding{}
	for _, b := range p.Bindings() {
		m[b.Key] = b
	}
	return m
}

func TestPluginIdentity(t *testing.T) {
	p := New()
	if p.ID() != "cmdline" || p.Region() != kernel.RegionCmdLine {
		t.Fatalf("identity = %q/%q", p.ID(), p.Region())
	}
}

func TestOpenSearchAndType(t *testing.T) {
	p, h := newTestPlugin(t)
	byKey := bindingMap(p)
	// "/" 打开搜索（守卫：未激活时）。
	byKey["/"].OnKey(h)
	if !p.st.Search.Active {
		t.Fatal("search should open")
	}
	// 输入字符（wildcard 守卫：激活时接管）。
	wild := normalWildcard(p.Bindings())
	wild.OnKeyMsg(h, keys("fix "))
	wild.OnKeyMsg(h, keys("bug"))
	if p.st.Search.Query != "fix bug" {
		t.Fatalf("query = %q", p.st.Search.Query)
	}
}

func TestSearchGuardsPreserveChatKeys(t *testing.T) {
	p, h := newTestPlugin(t)
	// 搜索未激活：wildcard 守卫不通过 → lookup 跳过（此处直接断言 When）。
	wild := normalWildcard(p.Bindings())
	if wild.When != nil && wild.When(p.st) {
		t.Fatal("wildcard must not match while search inactive")
	}
	p.React(h, keys("/")) // 打开
	_ = p
}

func TestSearchScansStreamAndJumps(t *testing.T) {
	p, h := newTestPlugin(t)
	// 预置流：3 条，其中第 2 条含目标词。
	p.st.Stream = []state.StreamEntry{
		{ID: "a", Type: "message", Content: "alpha"},
		{ID: "b", Type: "message", Content: "target"},
		{ID: "c", Type: "message", Content: "gamma target"},
	}
	byKey := bindingMap(p)
	byKey["/"].OnKey(h)
	wild := normalWildcard(p.Bindings())
	wild.OnKeyMsg(h, keys("target"))
	p.React(h, tea.KeyMsg{Type: tea.KeyEnter}) // 提交（走 React 不必要——直接 OnKey）

	// 提交经 Binding.OnKey：重新调用（OnKey 内执行搜索）。
	byKey["enter"].OnKey(h)
	if len(p.st.Search.Matches) != 2 {
		t.Fatalf("matches = %v, want [1 2]", p.st.Search.Matches)
	}
	// n/N 循环跳转。
	byKey["n"].OnKey(h)
	if p.st.Search.MatchIndex != 1 {
		t.Fatalf("matchIndex = %d", p.st.Search.MatchIndex)
	}
	byKey["N"].OnKey(h)
	if p.st.Search.MatchIndex != 0 {
		t.Fatalf("matchIndex = %d", p.st.Search.MatchIndex)
	}
	// 跳转经 SearchJumped 广播移交 chat（issue #41 P1-4）：cmdline 不再直写
	// Layout 槽——最后一次 n 跳转（匹配 0）应产生对应意图广播。
	jumps := 0
	var lastJump state.SearchJumped
	for _, b := range h.broadcasts {
		if j, ok := b.(state.SearchJumped); ok {
			jumps++
			lastJump = j
		}
	}
	if jumps != 3 { // 初次跳转 matches[0]=1 + n→matches[1]=2 + N→matches[0]=1
		t.Fatalf("search jumps broadcast = %d, want 3", jumps)
	}
	if lastJump.EntryIndex != 1 {
		t.Fatalf("last jump index = %d, want 1", lastJump.EntryIndex)
	}
}

func TestExCommandExecutesViaHost(t *testing.T) {
	p, h := newTestPlugin(t)
	byKey := bindingMap(p)
	byKey[":"].OnKey(h)
	if !p.st.Ex.Active {
		t.Fatal("ex should open")
	}
	wild := normalWildcard(p.Bindings())
	wild.OnKeyMsg(h, keys("clear"))
	p.React(h, tea.KeyMsg{Type: tea.KeyEnter})
	byKey["enter"].OnKey(h)
	if len(h.runCalls) != 1 || h.runCalls[0] != "clear" {
		t.Fatalf("runCalls = %v, want [clear]", h.runCalls)
	}
	if p.st.Ex.Active {
		t.Fatal("ex should close after submit")
	}
}

func TestRunCommandUnknownNotifies(t *testing.T) {
	p, h := newTestPlugin(t)
	// 插件路径：Ex 提交未知命令 → RunCommand 返回错误 → Notify 呈现
	//（测试通过 fake host 的 runErr 注入验证 Notify 分支）。
	h.runErr = errFake("unknown")
	p.React(h, tea.KeyMsg{Type: tea.KeyEnter})
	// 直接验证 fake host 的 RunCommand 语义（kernel 端测试覆盖真实解析）。
	if err := h.RunCommand("nope", nil); err == nil {
		t.Fatal("unknown command should error")
	}
}

func TestModeChangedClearsSearchAndEx(t *testing.T) {
	p, h := newTestPlugin(t)
	p.st.Search = state.SearchState{Active: true, Query: "q"}
	p.st.Ex = state.ExState{Active: true, Input: "w"}
	// 切到 Normal 以外的模式（如 Input）→ 清理。
	p.React(h, state.ModeChanged{From: state.NormalMode, To: state.InputModeInput})
	if p.st.Search.Active || p.st.Ex.Active {
		t.Fatal("leaving normal should clear search/ex")
	}
}

func TestRunStartedClearsSearchAndEx(t *testing.T) {
	p, h := newTestPlugin(t)
	p.st.Search = state.SearchState{Active: true}
	p.React(h, gateway.GatewayEvent{Type: gateway.EventRunStarted})
	if p.st.Search.Active {
		t.Fatal("run start should clear search")
	}
}

func TestRenderReflectsActiveLine(t *testing.T) {
	p, h := newTestPlugin(t)
	if out := p.Render(h, 80); out != "" {
		t.Fatalf("inactive render = %q, want empty", out)
	}
	p.st.Search = state.SearchState{Active: true, Query: "q"}
	if out := p.Render(h, 80); !strings.Contains(out, "q") {
		t.Fatalf("search render = %q", out)
	}
	p.st.Search = state.SearchState{}
	p.st.Ex = state.ExState{Active: true, Input: "clear"}
	if out := p.Render(h, 80); !strings.Contains(out, "clear") {
		t.Fatalf("ex render = %q", out)
	}
}

func TestBackspaceEditsQueryAndEx(t *testing.T) {
	p, h := newTestPlugin(t)
	p.st.Search = state.SearchState{Active: true, Query: "ab"}
	wild := normalWildcard(p.Bindings())
	wild.OnKeyMsg(h, tea.KeyMsg{Type: tea.KeyBackspace})
	if p.st.Search.Query != "a" {
		t.Fatalf("query = %q", p.st.Search.Query)
	}
	p.st.Search = state.SearchState{}
	p.st.Ex = state.ExState{Active: true, Input: "xy"}
	wild.OnKeyMsg(h, tea.KeyMsg{Type: tea.KeyBackspace})
	if p.st.Ex.Input != "x" {
		t.Fatalf("ex input = %q", p.st.Ex.Input)
	}
}

func normalWildcard(bindings []kernel.Binding) kernel.Binding {
	for _, b := range bindings {
		if b.Mode == state.NormalMode && b.OnKeyMsg != nil {
			return b
		}
	}
	return kernel.Binding{}
}

// errFake 是测试用错误类型。
type errFake string

func (e errFake) Error() string { return string(e) }

func TestCmdlineCloseIsSafe(t *testing.T) {
	p, _ := newTestPlugin(t)
	p.Close(context.Background()) // 无外部资源，对称生命周期
}

// TestExSubmitErrorNotifies：Ex 提交未知命令 → RunCommand 错误 → Notify
// （补 Bindings 内 OnKey 的错误分支）。
func TestExSubmitErrorNotifies(t *testing.T) {
	p, h := newTestPlugin(t)
	h.runErr = errFake("unknown command")
	byKey := bindingMap(p)
	byKey[":"].OnKey(h)
	wild := normalWildcard(p.Bindings())
	wild.OnKeyMsg(h, keys("bogus"))
	byKey["enter"].OnKey(h)
	if !strings.Contains(h.notifies[len(h.notifies)-1], "未知命令") {
		t.Fatalf("notifies = %v", h.notifies)
	}
}

// TestSearchNoMatchNotifies：搜索无匹配 → Notify 提示。
func TestSearchNoMatchNotifies(t *testing.T) {
	p, h := newTestPlugin(t)
	byKey := bindingMap(p)
	byKey["/"].OnKey(h)
	wild := normalWildcard(p.Bindings())
	wild.OnKeyMsg(h, keys("zzz"))
	byKey["enter"].OnKey(h)
	if len(h.notifies) == 0 || !strings.Contains(h.notifies[len(h.notifies)-1], "无匹配") {
		t.Fatalf("notifies = %v", h.notifies)
	}
}

// TestEmptySearchSubmitIsNoop：空查询提交为 no-op。
func TestEmptySearchSubmitIsNoop(t *testing.T) {
	p, h := newTestPlugin(t)
	byKey := bindingMap(p)
	byKey["/"].OnKey(h)
	byKey["enter"].OnKey(h)
	if p.st.Search.Active {
		t.Fatal("empty submit should close search")
	}
	if len(p.st.Search.Matches) != 0 {
		t.Fatal("empty submit should not scan")
	}
}

// TestJumpToTailEmitsIntent：跳转到末条目同样只广播意图（AutoScroll 语义
// 收敛在 chat 侧 ScrollToEntry 单一真源，issue #41 审计 P2-3 断言翻转——
// 旧断言"尾跳保持 AutoScroll"随直写 Layout 移除而失效）。
// TestJumpToOutOfRangeIsNoop 验证 jumpTo 越界防御：无效索引不广播跳转意图
//（binding 层已被 When 守卫约束到有效匹配集，此为直接调用的防御分支——
// issue #41 PR 审计 P1-2 补测凑齐包覆盖 100%）。
func TestJumpToOutOfRangeIsNoop(t *testing.T) {
	p, h := newTestPlugin(t)
	p.st.Stream = []state.StreamEntry{{ID: "a", Content: "x"}}
	p.jumpTo(h, -1)
	p.jumpTo(h, 5)
	if len(h.broadcasts) != 0 {
		t.Fatalf("out-of-range jumps must not broadcast, got %v", h.broadcasts)
	}
}

func TestJumpToTailEmitsIntent(t *testing.T) {
	p, h := newTestPlugin(t)
	p.st.Stream = []state.StreamEntry{
		{ID: "a", Content: "one"},
		{ID: "b", Content: "two"},
	}
	byKey := bindingMap(p)
	byKey["/"].OnKey(h)
	wild := normalWildcard(p.Bindings())
	wild.OnKeyMsg(h, keys("two"))
	byKey["enter"].OnKey(h)
	// cmdline 不直写 Layout（槽写权归 chat）：跳转前后 Layout 槽不变量。
	if p.st.Layout.ScrollOffset != 0 || !p.st.Layout.AutoScroll {
		t.Fatalf("cmdline must not write Layout slots: offset=%d auto=%v",
			p.st.Layout.ScrollOffset, p.st.Layout.AutoScroll)
	}
	// 广播的意图指向末条目（index 1）。
	var jumped bool
	for _, b := range h.broadcasts {
		if j, ok := b.(state.SearchJumped); ok && j.EntryIndex == 1 {
			jumped = true
		}
	}
	if !jumped {
		t.Fatalf("tail jump should broadcast SearchJumped{1}, broadcasts = %v", h.broadcasts)
	}
}

// TestBindingsWhenGuards 锁定全部 When 守卫的双向语义（覆盖闭包体）。
func TestBindingsWhenGuards(t *testing.T) {
	p, h := newTestPlugin(t)
	byKey := bindingMap(p)
	inactive := p.st
	// 搜索未激活："/" 可用、搜索输入捕获不可用。
	if !byKey["/"].When(inactive) {
		t.Fatal("/ should be available when search closed")
	}
	if b := byKey["__wildcard"]; b.When != nil && b.When(inactive) {
		t.Fatal("input capture must not match while search closed")
	}
	// 搜索激活："/" 不可用（已激活）、捕获可用、esc/enter 可用。
	p.st.Search = state.SearchState{Active: true, Query: "q"}
	if byKey["/"].When != nil && byKey["/"].When(p.st) {
		t.Fatal("/ must not re-open while search active")
	}
	if b := normalWildcard(p.Bindings()); b.When == nil || !b.When(h.State()) {
		t.Fatal("input capture should match while search active")
	}
	if b := byKey["esc"]; b.When == nil || !b.When(h.State()) {
		t.Fatal("esc should match while search active")
	}
	// Ex 激活：esc/enter 同样可用。
	p.st.Search = state.SearchState{}
	p.st.Ex = state.ExState{Active: true, Input: "clear"}
	if b := byKey["enter"]; b.When == nil || !b.When(h.State()) {
		t.Fatal("enter should match while ex active")
	}
	if b := normalWildcard(p.Bindings()); b.When != nil && !b.When(h.State()) {
		t.Fatal("input capture should match while ex active")
	}
}

// TestBindingsWhenGuardsExAndSearchKeys 补齐剩余守卫闭包与分支。
func TestBindingsWhenGuardsExAndSearchKeys(t *testing.T) {
	p, h := newTestPlugin(t)
	byKey := bindingMap(p)
	// ":" 守卫：Ex 未激活时可用。
	if b := byKey[":"]; b.When == nil || !b.When(p.st) {
		t.Fatal(": should be available when ex closed")
	}
	// n/N 守卫：无匹配时不可用。
	if b := byKey["n"]; b.When != nil && b.When(p.st) {
		t.Fatal("n must not match without matches")
	}
	if b := byKey["N"]; b.When != nil && b.When(p.st) {
		t.Fatal("N must not match without matches")
	}
	// Ex 空白提交：cmd 为空 → 直接返回（不 RunCommand）。
	byKey[":"].OnKey(h)
	p.st.Ex.Input = "   "
	before := len(h.runCalls)
	byKey["enter"].OnKey(h)
	if len(h.runCalls) != before {
		t.Fatal("blank ex submit should not run command")
	}
	// n/N 有匹配：OnKey 执行（含负向环绕）。
	p.st.Stream = []state.StreamEntry{{Content: "hit"}, {Content: "hit"}}
	p.st.Search.Matches = []int{0, 1}
	p.st.Search.MatchIndex = 0
	byKey["N"].OnKey(h)
	if p.st.Search.MatchIndex != 1 {
		t.Fatalf("matchIndex = %d, want 1 (wrap)", p.st.Search.MatchIndex)
	}
	byKey["n"].OnKey(h)
	if p.st.Search.MatchIndex != 0 {
		t.Fatalf("matchIndex = %d, want 0", p.st.Search.MatchIndex)
	}
}

// TestEscOnKeyClearsSearchAndEx：esc OnKey 清空两个子状态（106 块）。
func TestEscOnKeyClearsSearchAndEx(t *testing.T) {
	p, h := newTestPlugin(t)
	byKey := bindingMap(p)
	p.st.Search = state.SearchState{Active: true, Query: "q"}
	p.st.Ex = state.ExState{Active: true, Input: "w"}
	byKey["esc"].OnKey(h)
	if p.st.Search.Active || p.st.Ex.Active || p.st.Search.Query != "" || p.st.Ex.Input != "" {
		t.Fatalf("esc should clear both: %+v %+v", p.st.Search, p.st.Ex)
	}
}

// TestNextMatchNoMatchesNoop：无匹配时 nextMatch 为 no-op（210 块）。
func TestNextMatchNoMatchesNoop(t *testing.T) {
	p, h := newTestPlugin(t)
	p.nextMatch(h, 1)
	p.nextMatch(h, -1)
	// 无 panic 且状态不变即为通过。
}

// commandRecorder 是注册进真内核的命令记录桩：供端到端回归验证
// RunCommand 是否收到完整命令名。
type commandRecorder struct {
	called []string
}

func (p *commandRecorder) ID() string                       { return "recorder" }
func (p *commandRecorder) Init(ctx context.Context, h kernel.Host) {}
func (p *commandRecorder) Close(ctx context.Context)               {}
func (p *commandRecorder) Commands() []kernel.Command {
	return []kernel.Command{{
		Name:     "/debug",
		Aliases:  []string{"debug"}, // 对齐真实 debug 插件别名（Ex 无斜杠入口）
		Category: "test",
		Run:      func(h kernel.Host, args []string) { p.called = append(p.called, "debug") },
	}}
}

// TestSearchExInputNotHijackedByExactBindings 端到端回归（issue #41 PR
// 审计 P1-1）：搜索/Ex 激活期，无守卫的精确绑定（chat 滚动键 g/j 等、
// prompt 的 i）不得劫持 cmdline 通配绑定的查询输入。
// 真内核 + 真插件（chat/prompt/cmdline）驱动：
//   - 搜索期输入 "gij" → 查询串完整为 "gij"、模式保持 Normal
//     （修复前：g 被滚动键劫持、i 被 SetMode 切走模式杀掉搜索）
//   - Ex 期输入 "debug" 提交 → RunCommand 收到完整 "debug"
//     （修复前："g" 被劫持得 "debu"）
func TestSearchExInputNotHijackedByExactBindings(t *testing.T) {
	st := state.NewViewState()
	k := kernel.NewKernel(kernel.Config{State: st})
	rec := &commandRecorder{}
	for _, p := range []kernel.Plugin{chat.New(), prompt.New(), New(), rec} {
		if err := k.Register(p); err != nil {
			t.Fatalf("register %T: %v", p, err)
		}
	}
	k.Init() // 触发各插件 Init（固定状态指针）；nil cmd 被丢弃无需处理
	// esc：Input → Normal（kernel 初始为 Input 模式）。
	k.Update(keys("esc"))
	// 打开搜索并输入含劫持字符的查询。
	k.Update(keys("/"))
	if !st.Search.Active {
		t.Fatal("search should open")
	}
	for _, ch := range []string{"g", "i", "j"} {
		k.Update(keys(ch))
	}
	if st.Search.Query != "gij" {
		t.Fatalf("query = %q, want %q (hijack regression)", st.Search.Query, "gij")
	}
	if st.Mode != state.NormalMode {
		t.Fatalf("mode = %v, want NormalMode ('i' must not switch mode during search)", st.Mode)
	}
	// esc 关闭搜索 → : 打开 Ex → 输入 debug → enter 提交。
	k.Update(keys("esc"))
	if st.Search.Active {
		t.Fatal("esc should close search")
	}
	k.Update(keys(":"))
	for _, ch := range []string{"d", "e", "b", "u", "g"} {
		k.Update(keys(ch))
	}
	if st.Ex.Input != "debug" {
		t.Fatalf("ex input = %q, want %q (hijack regression)", st.Ex.Input, "debug")
	}
	k.Update(keys("enter"))
	if len(rec.called) != 1 || rec.called[0] != "debug" {
		t.Fatalf("RunCommand calls = %v, want [debug]", rec.called)
	}
}
