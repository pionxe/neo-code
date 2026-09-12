package kernel

import (
	"strings"
	"testing"

	"neo-code/internal/tuiv2/state"
)

func TestBindingRegistryRejectsDuplicate(t *testing.T) {
	r := &bindingRegistry{}
	if err := r.add("a", Binding{Mode: state.NormalMode, Key: "j", OnKey: func(h Host) {}}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	err := r.add("b", Binding{Mode: state.NormalMode, Key: "j", OnKey: func(h Host) {}, Description: "b 的 j"})
	if err == nil || !strings.Contains(err.Error(), "binding conflict") {
		t.Fatalf("err = %v, want binding conflict", err)
	}
}

func TestBindingRegistryLookupAndAll(t *testing.T) {
	r := &bindingRegistry{}
	n := Binding{Mode: state.NormalMode, Key: "j", OnKey: func(h Host) {}}
	i := Binding{Mode: state.InputModeInput, Key: "enter", OnKey: func(h Host) {}}
	l := Binding{Mode: state.LeaderMode, Key: "p", OnKey: func(h Host) {}}
	for _, b := range []Binding{n, i, l} {
		if err := r.add("t", b); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if _, ok := r.lookup(state.NormalMode, "j"); !ok {
		t.Fatal("normal j should be found")
	}
	if _, ok := r.lookup(state.NormalMode, "k"); ok {
		t.Fatal("normal k should not be found")
	}
	if got := len(r.all()); got != 3 {
		t.Fatalf("all = %d, want 3", got)
	}
}

func TestCommandRegistryAddAndLookup(t *testing.T) {
	r := &commandRegistry{}
	run := func(h Host, args []string) {}
	cmds := []Command{
		{Name: "/new", Aliases: []string{"new"}, Category: "B", Run: run},
		{Name: "/help", Aliases: []string{"help"}, Category: "A", Run: run},
	}
	for _, c := range cmds {
		if err := r.add("t", c); err != nil {
			t.Fatalf("add %s: %v", c.Name, err)
		}
	}
	if c, ok := r.lookup("/new"); !ok || c.Name != "/new" {
		t.Fatalf("lookup by name failed")
	}
	if c, ok := r.lookup("help"); !ok || c.Name != "/help" {
		t.Fatalf("lookup by alias failed")
	}
	if _, ok := r.lookup("/nope"); ok {
		t.Fatal("unknown command should not resolve")
	}
	// 面板排序：Category 优先（A 在 B 前），同 Category 按 Name。
	got := r.sorted()
	if got[0].Name != "/help" || got[1].Name != "/new" {
		t.Fatalf("sorted = [%s %s], want [/help /new]", got[0].Name, got[1].Name)
	}
}

func TestCommandRegistryRejectsConflicts(t *testing.T) {
	r := &commandRegistry{}
	run := func(h Host, args []string) {}
	if err := r.add("a", Command{Name: "/x", Run: run}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := r.add("b", Command{Name: "/x", Run: run}); err == nil {
		t.Fatal("duplicate name should fail")
	}
	// 先登记 /y（别名 x）成功，再登记另一命令的别名 x → 冲突失败。
	if err := r.add("b", Command{Name: "/y", Aliases: []string{"x"}, Run: run}); err != nil {
		t.Fatalf("add /y: %v", err)
	}
	if err := r.add("c", Command{Name: "/z", Aliases: []string{"x"}, Run: run}); err == nil {
		t.Fatal("alias conflict should fail")
	}
	if err := r.add("b", Command{Name: "", Run: run}); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := r.add("b", Command{Name: "/w"}); err == nil {
		t.Fatal("nil Run should fail")
	}
}

func TestDeriveShortcut(t *testing.T) {
	bindings := &bindingRegistry{}
	nop := func(h Host) {}
	if err := bindings.add("t", Binding{Mode: state.LeaderMode, Key: "n", Command: "/new", OnKey: nop}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := bindings.add("t", Binding{Mode: state.NormalMode, Key: "N", Command: "/new", OnKey: nop}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := deriveShortcut("/new", bindings); got != "N, Space n" {
		t.Fatalf("shortcut = %q, want %q", got, "N, Space n")
	}
	if got := deriveShortcut("/none", bindings); got != "" {
		t.Fatalf("shortcut for unbound = %q, want empty", got)
	}
}

func TestOverlayStackPushPopTop(t *testing.T) {
	s := &overlayStack{}
	s.pop() // 栈空 pop 必须是空操作
	if s.depth() != 0 || s.top() != nil {
		t.Fatal("empty stack invariant broken")
	}
	s.push(namedOverlay{id: "a"})
	s.push(namedOverlay{id: "b"})
	if s.depth() != 2 || s.top().ID() != "b" {
		t.Fatalf("stack = depth %d top %s", s.depth(), s.top().ID())
	}
	s.pop()
	if s.top().ID() != "a" {
		t.Fatalf("top after pop = %s, want a", s.top().ID())
	}
}

// namedOverlay 是带标识的最小浮层桩。
type namedOverlay struct{ id string }

func (o namedOverlay) ID() string { return o.id }
func (o namedOverlay) HandleKey(h Host, key string) bool {
	return key != "esc"
}
func (o namedOverlay) View(h Host, width int) string { return "overlay:" + o.id }

func TestComposeFoldsEmptyAndMissingRegions(t *testing.T) {
	renderers := map[RegionID]RegionRenderer{
		RegionStatusBar: staticRegion{region: RegionStatusBar, out: "STATUS"},
		RegionStream:    staticRegion{region: RegionStream, out: ""}, // 空渲染 → 折叠
		RegionPrompt:    staticRegion{region: RegionPrompt, out: "PROMPT"},
		// RegionInspector 未注册 → 跳过
	}
	h := &testFakeHost{}
	got := Compose(h, renderers, 100)
	want := "STATUS\nPROMPT"
	if got != want {
		t.Fatalf("compose = %q, want %q", got, want)
	}
}

// staticRegion 是固定输出的区域渲染桩。
type staticRegion struct {
	region RegionID
	out    string
}

func (s staticRegion) Region() RegionID                { return s.region }
func (s staticRegion) Render(h Host, width int) string { return s.out }

func TestModeMachineLeaderGenerationalTimeout(t *testing.T) {
	m := &modeMachine{mode: state.InputModeInput, timeout: 1}
	if cmd := m.setMode(state.InputModeInput); cmd != nil {
		t.Fatal("same mode should arm nothing")
	}
	cmd := m.setMode(state.NormalMode)
	if m.mode != state.NormalMode || cmd != nil {
		t.Fatalf("enter normal: mode=%v cmd=%v", m.mode, cmd)
	}
	cmd = m.setMode(state.LeaderMode)
	if cmd == nil {
		t.Fatal("enter leader should arm timeout cmd")
	}
	// 旧代际超时：忽略。
	m.onLeaderTimeout(leaderTimeoutMsg{gen: m.leaderGen - 1})
	if m.mode != state.LeaderMode {
		t.Fatal("stale timeout should not leave leader")
	}
	// 当前代际超时：回落 Normal。
	msg := cmd().(leaderTimeoutMsg)
	m.onLeaderTimeout(msg)
	if m.mode != state.NormalMode {
		t.Fatalf("mode = %v, want normal after timeout", m.mode)
	}
	// 非 Leader 模式下的超时：忽略。
	m.onLeaderTimeout(leaderTimeoutMsg{gen: 999})
	if m.mode != state.NormalMode {
		t.Fatal("timeout outside leader should be ignored")
	}
}
