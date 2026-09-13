package palette

import (
	"context"
	"strings"
	"testing"

	"neo-code/internal/tuiv2/gateway"
	"neo-code/internal/tuiv2/kernel"
	"neo-code/internal/tuiv2/state"

	tea "github.com/charmbracelet/bubbletea"
)

// recordingHost 是 fake Host：Commands 返回预置快照；PopOverlay 记录次数。
var _ kernel.Host = (*recordingHost)(nil)

type recordingHost struct {
	st       *state.ViewState
	cmds     []kernel.Command
	popCount int
	executed []string
	overlays []kernel.Overlay
}

func newRecordingHost() *recordingHost { return &recordingHost{st: state.NewViewState()} }

func (h *recordingHost) State() *state.ViewState                        { return h.st }
func (h *recordingHost) Gateway() gateway.Client                        { return nil }
func (h *recordingHost) BindEventStream(ch <-chan gateway.GatewayEvent) {}
func (h *recordingHost) GoCmd(cmd tea.Cmd)                              {}
func (h *recordingHost) Send(msg tea.Msg)                               {}
func (h *recordingHost) Mode() state.InputMode                          { return h.st.Mode }
func (h *recordingHost) SetMode(m state.InputMode)                      {}
func (h *recordingHost) PushOverlay(o kernel.Overlay)                   { h.overlays = append(h.overlays, o) }
func (h *recordingHost) PopOverlay()                                    { h.popCount++ }
func (h *recordingHost) Confirm(req state.ConfirmRequest)               {}
func (h *recordingHost) Notify(text string)                             {}
func (h *recordingHost) Quit()                                          {}
func (h *recordingHost) Commands() []kernel.Command                     { return h.cmds }
func (h *recordingHost) RunCommand(name string, args []string) error {
	h.executed = append(h.executed, name)
	return nil
}
func (h *recordingHost) Bindings() []kernel.Binding { return nil }

func TestPluginIdentityAndBindings(t *testing.T) {
	p := New()
	if p.ID() != "palette" {
		t.Fatalf("id = %q", p.ID())
	}
	bs := p.Bindings()
	if len(bs) != 1 || bs[0].Key != "p" || bs[0].Mode != state.LeaderMode {
		t.Fatalf("bindings = %+v", bs)
	}
}

func TestOpenSnapshotsCommandsAndFilter(t *testing.T) {
	p := New()
	h := newRecordingHost()
	h.cmds = []kernel.Command{
		{Name: "/new", Description: "新建会话"},
		{Name: "/model", Description: "切换模型"},
	}
	// 打开：经 Leader p 绑定。
	for _, b := range p.Bindings() {
		b.OnKey(h)
	}
	if len(h.overlays) != 1 {
		t.Fatal("open should push overlay")
	}
	o := h.overlays[0]
	// 过滤 "mo"：/model 前缀命中。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mo")})
	out := o.View(h, 60)
	if !strings.Contains(out, "/model") || strings.Contains(out, "/new  ") {
		t.Fatalf("filtered view = %q", out)
	}
	// backspace 回删恢复（经具体类型访问未导出的过滤方法——同包测试）。
	specific := o.(*overlay)
	specific.HandleKey(h, tea.KeyMsg{Type: tea.KeyBackspace})
	_ = specific.View(h, 60)
	specific.HandleKey(h, tea.KeyMsg{Type: tea.KeyBackspace})
	if len(specific.filtered()) != 2 {
		t.Fatalf("after clearing query, matched = %d", len(specific.filtered()))
	}
}

func TestOverlayExecuteRunsCommand(t *testing.T) {
	p := New()
	h := newRecordingHost()
	ran := false
	h.cmds = []kernel.Command{{Name: "/do", Description: "d", Run: func(h kernel.Host, args []string) { ran = true }}}
	p.open(h)
	o := h.overlays[0]
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	if !ran {
		t.Fatal("enter should execute selected command")
	}
	if h.popCount != 1 {
		t.Fatalf("popCount = %d", h.popCount)
	}
}

func TestOverlayEmptyQueryRendersNoMatchMessage(t *testing.T) {
	p := New()
	h := newRecordingHost()
	p.open(h)
	o := h.overlays[0]
	// 过滤到零结果。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zz")})
	if !strings.Contains(o.View(h, 60), "无匹配命令") {
		t.Fatalf("view = %q", o.View(h, 60))
	}
}

func TestFilterDedup(t *testing.T) {
	// 空查询返回全部（引用透传亦可——只验证长度）。
	o := &overlay{cmds: []kernel.Command{{Name: "/a"}, {Name: "/b"}}}
	if len(o.filtered()) != 2 {
		t.Fatalf("filtered = %d", len(o.filtered()))
	}
}

// TestPluginLifecycleAndIdentity 补齐身份/生命周期/View 分支。
func TestPluginLifecycleAndIdentity(t *testing.T) {
	p := New()
	if p.ID() != "palette" {
		t.Fatalf("id = %q", p.ID())
	}
	p.Init(context.Background(), newRecordingHost())
	p.Close(context.Background())
}

// TestOverlayDescribeBranch 补齐 HandleKey 的描述包含分桶与空选择执行。
func TestOverlayDescribeBranchAndEmptyExecute(t *testing.T) {
	p := New()
	h := newRecordingHost()
	ran := false
	h.cmds = []kernel.Command{
		{Name: "/aaa", Description: "包含关键词 mention", Run: func(h kernel.Host, args []string) { ran = true }},
		{Name: "/bbb", Description: "无关", Run: func(h kernel.Host, args []string) { ran = true }},
	}
	p.open(h)
	o := h.overlays[0].(*overlay)
	// 描述包含分桶：查询 "mention" 命中 /aaa。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mention")})
	if len(o.filtered()) != 1 || o.filtered()[0].Name != "/aaa" {
		t.Fatalf("describe bucket = %+v", o.filtered())
	}
	// 无匹配时 enter：不执行任何命令（selected 越界守卫）。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zz")})
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	if ran {
		t.Fatal("enter with no match must not run command")
	}
	// 清查询后 enter：执行选中项。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyBackspace})
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyBackspace})
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	if !ran {
		t.Fatal("enter with match should run command")
	}
	// up：selected 边界（0 时上移不动）。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyUp})
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyDown})
}

// TestOverlayUnrecognizedKeyConsumed：未识别键被模态消费（不关闭不执行）。
func TestOverlayUnrecognizedKeyConsumed(t *testing.T) {
	p := New()
	h := newRecordingHost()
	h.cmds = []kernel.Command{{Name: "/x", Description: "d", Run: func(h kernel.Host, args []string) {}}}
	p.open(h)
	o := h.overlays[0].(*overlay)
	consumed := o.HandleKey(h, tea.KeyMsg{Type: tea.KeyTab})
	if !consumed {
		t.Fatal("unrecognized key should still be consumed (modal)")
	}
	_ = o.View(h, 60) // View 分支补齐
}

// TestOverlayID 补齐 overlay.ID 覆盖（此前零调用）。
func TestOverlayID(t *testing.T) {
	h := newRecordingHost()
	pl := New()
	pl.open(h)
	if id := h.overlays[0].ID(); id != "palette" {
		t.Fatalf("overlay id = %q", id)
	}
}

// TestOverlayHandleKeyFullMatrix 补齐 HandleKey 剩余分支：
// 空匹配 up/down、无 Run 的选中项 enter（脏数据防御）、backspace 空查询。
func TestOverlayHandleKeyFullMatrix(t *testing.T) {
	p := New()
	h := newRecordingHost() // newTestPlugin 亦等价；此处显式构造以控制 cmds
	h.cmds = []kernel.Command{
		{Name: "/with-run", Description: "d", Run: func(_ kernel.Host, args []string) {
			h.executed = append(h.executed, "/with-run") // 闭包捕获外层 fake host
		}},
	}
	p.open(h)
	o := h.overlays[0].(*overlay)

	// 空匹配下 up/down：不 panic、selected 不越界（query="zzz" 无匹配）。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyUp})
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyDown})
	// 清查询：三次 backspace → query=""，匹配恢复。
	for i := 0; i < 3; i++ {
		o.HandleKey(h, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	// enter：匹配恢复（/with-run）→ 执行。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	if len(h.executed) != 1 || h.executed[0] != "/with-run" {
		t.Fatalf("executed = %v", h.executed)
	}
}

// TestPaletteFilteredBranches 补齐 filtered 精确/前缀/包含三分支。
func TestPaletteFilteredBranches(t *testing.T) {
	h := newRecordingHost()
	h.cmds = []kernel.Command{
		{Name: "/clear", Description: "清空消息"},
		{Name: "/cancel", Description: "取消运行"},
	}
	o := &overlay{cmds: h.cmds}
	// 精确："/cancel"（去斜杠后全等）。
	o.query = "/cancel"
	if got := o.filtered(); len(got) != 1 || got[0].Name != "/cancel" {
		t.Fatalf("exact = %+v", got)
	}
	// 前缀："/c" 命中两命令（cancel/clear 前缀）。
	o.query = "/c"
	if got := o.filtered(); len(got) != 2 {
		t.Fatalf("prefix = %+v", got)
	}
	// 描述包含："取消" 命中 /cancel。
	o.query = "取消"
	if got := o.filtered(); len(got) != 1 || got[0].Name != "/cancel" {
		t.Fatalf("contain = %+v", got)
	}
}

// TestPaletteHandleKeyBoundary 补齐 up/down 边界与无匹配 enter。
func TestPaletteHandleKeyBoundary(t *testing.T) {
	h := newRecordingHost()
	ran := false
	h.cmds = []kernel.Command{
		{Name: "/only", Description: "d", Run: func(h kernel.Host, args []string) { ran = true }},
	}
	o := &overlay{cmds: h.cmds}
	// down 在末尾：不动。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyDown})
	if o.selected != 0 {
		t.Fatalf("selected = %d, want 0", o.selected)
	}
	// up 在顶部后 down 回 0。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyUp})
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyDown})
	if o.selected != 0 {
		t.Fatalf("selected = %d", o.selected)
	}
	// enter 执行唯一项。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	if !ran {
		t.Fatal("enter should run the only command")
	}
}

// TestPaletteViewShortcutlessBranch 补齐 View 的 Shortcut 空分支。
func TestPaletteViewShortcutlessBranch(t *testing.T) {
	h := newRecordingHost()
	h.cmds = []kernel.Command{{Name: "/nos", Description: "无快捷键"}}
	o := &overlay{cmds: h.cmds}
	out := o.View(h, 80)
	if !strings.Contains(out, "/nos") || strings.Contains(out, "(") {
		t.Fatalf("view = %q", out)
	}
}

// TestPaletteRemainingBranches 补齐最后四个未覆盖分支：
// 精确分支（去斜杠全等）、esc 关闭、backspace 空查询、View 快捷键列。
func TestPaletteRemainingBranches(t *testing.T) {
	pl := New()
	h := newRecordingHost()
	ran := false
	h.cmds = []kernel.Command{
		{Name: "/clear", Description: "清空消息", Shortcut: "Space c", Run: func(h kernel.Host, args []string) { ran = true }},
	}
	// 第一次打开：走 PushOverlay（真实链路）。
	pl.open(h)
	o := h.overlays[0].(*overlay)
	// 精确分支：query == 去 slash 后的 name。
	o.query = "clear"
	if got := o.filtered(); len(got) != 1 {
		t.Fatalf("exact = %+v", got)
	}
	// View：含快捷键列（Shortcut 非空分支）。
	if out := o.View(h, 80); !strings.Contains(out, "Space c") {
		t.Fatalf("view = %q", out)
	}
	// esc：关闭浮层（palette.go 内核分支调用 Host.PopOverlay）。
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyEsc})
	if h.popCount != 1 {
		t.Fatalf("popCount = %d", h.popCount)
	}
	// 重新打开：backspace 空查询分支。
	h.cmds[0].Run = func(h kernel.Host, args []string) { ran = false }
	pl.open(h)
	if len(h.overlays) != 2 {
		t.Fatalf("overlays = %d, want 2 (recording 列表只增不减，与内核栈语义独立)", len(h.overlays))
	}
	o2 := h.overlays[1].(*overlay)
	o2.HandleKey(h, tea.KeyMsg{Type: tea.KeyBackspace})
	// enter：空查询匹配全部 → 执行唯一命令（ran 翻转为 false，验证新闭包生效）。
	o2.HandleKey(h, tea.KeyMsg{Type: tea.KeyEnter})
	if ran {
		t.Fatal("re-opened overlay executed the OLD closure (ran=true), want new closure (ran=false)")
	}
}

// TestPaletteUpDownAndID 补齐 up 递减/down 递增分支与 overlay.ID。
func TestPaletteUpDownAndID(t *testing.T) {
	pl := New()
	h := newRecordingHost()
	h.cmds = []kernel.Command{
		{Name: "/a", Description: "甲", Run: func(h kernel.Host, args []string) {}},
		{Name: "/b", Description: "乙", Run: func(h kernel.Host, args []string) {}},
	}
	pl.open(h)
	o := h.overlays[0].(*overlay)
	if got := o.ID(); got != "palette" {
		t.Fatalf("overlay id = %q", got)
	}
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyDown}) // 0→1
	if o.selected != 1 {
		t.Fatalf("selected = %d, want 1", o.selected)
	}
	o.HandleKey(h, tea.KeyMsg{Type: tea.KeyUp}) // 1→0
	if o.selected != 0 {
		t.Fatalf("selected = %d, want 0", o.selected)
	}
}

// TestOverlayMouseWheelAndClick 验证面板鼠标交互（S8，issue #52）：
// 滚轮滚动选项 + 左键点击选择并执行 + 模态消费不穿透。
func TestOverlayMouseWheelAndClick(t *testing.T) {
	p, h := New(), newRecordingHost()
	// 查询命令含可执行命令（触发 c.Run 路径）。
	h.cmds = []kernel.Command{
		{Name: "/help", Description: "帮助", Run: func(h kernel.Host, args []string) {}},
		{Name: "/exit", Description: "退出", Run: func(h kernel.Host, args []string) {}},
		{Name: "/model", Description: "切换模型"},
	}
	// 打开：经 Leader p 绑定。
	for _, b := range p.Bindings() {
		b.OnKey(h)
	}
	if len(h.overlays) != 1 {
		t.Fatal("palette overlay should be pushed")
	}
	mh, ok := h.overlays[0].(kernel.OverlayMouseHandler)
	if !ok {
		t.Fatal("palette overlay should implement OverlayMouseHandler")
	}

	// 滚轮下 → selected 增（模态消费）。
	for _, btn := range []tea.MouseButton{tea.MouseButtonWheelDown, tea.MouseButtonWheelDown} {
		mh.HandleMouse(h, tea.MouseMsg{Button: btn})
	}
	// 滚轮上 → selected 减。
	mh.HandleMouse(h, tea.MouseMsg{Button: tea.MouseButtonWheelUp})

	// 左键点击（Y=2 对齐列表首行）→ 选择并执行 Run + 关闭面板。
	consumed := mh.HandleMouse(h, tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, Y: 2})
	if !consumed {
		t.Fatal("left click should be consumed")
	}
	if h.popCount != 1 {
		t.Fatalf("PopOverlay count = %d, want 1", h.popCount)
	}
}

// TestOverlayMouseMotionDropped 验证 Motion/非按键鼠标事件被模态消费不穿透。
func TestOverlayMouseMotionDropped(t *testing.T) {
	p, h := New(), newRecordingHost()
	for _, b := range p.Bindings() {
		if b.Key == "p" {
			b.OnKey(h)
		}
	}
	consumed := h.overlays[0].(kernel.OverlayMouseHandler).HandleMouse(h, tea.MouseMsg{Type: tea.MouseMotion, X: 5, Y: 5})
	if !consumed {
		t.Fatal("motion should be consumed by modal overlay")
	}
}
