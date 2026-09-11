package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/tuiv2/state"
)

// paletteState 构造带尺寸的 ViewState，供 palette 测试使用。
func paletteState() *state.ViewState {
	vs := state.NewViewState()
	vs.Layout.Width = 80
	vs.Layout.Height = 24
	return vs
}

// TestPaletteMatchExactPrefixSubstring 验证确定性分桶：精确>前缀>子串>描述子串。
func TestPaletteMatchExactPrefixSubstring(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	p := NewPalette(vs)

	// 精确匹配 mode：/mode 精确，/model 前缀
	vs.Overlay.Query = "mode"
	matched := p.matchedItems()
	if len(matched) == 0 {
		t.Fatal("mode should match")
	}
	// /mode 精确应排第一
	if matched[0].Name != "/mode" {
		t.Fatalf("exact match /mode should be first, got %s (full=%v)", matched[0].Name, matchedNames(matched))
	}

	// 前缀匹配 mod：/mode 和 /model 都前缀匹配
	vs.Overlay.Query = "mod"
	matched = p.matchedItems()
	foundModel := false
	foundMode := false
	for _, m := range matched {
		if m.Name == "/model" {
			foundModel = true
		}
		if m.Name == "/mode" {
			foundMode = true
		}
	}
	if !foundModel || !foundMode {
		t.Fatalf("mod should match /model and /mode, got %v", matchedNames(matched))
	}
}

// TestPaletteModeDoesNotMatchModelAsExact 验证搜索 mode 时 /model 不会出现在精确桶。
// 回归保护：防 fuzzy 误命中问题的精神延续（确定性匹配下 mode≠model）。
func TestPaletteModeDoesNotMatchModelAsExact(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	vs.Overlay.Query = "mode"
	p := NewPalette(vs)
	matched := p.matchedItems()
	if len(matched) == 0 {
		t.Fatal("mode should match")
	}
	if matched[0].Name != "/mode" {
		t.Fatalf("exact match should be /mode, got %s", matched[0].Name)
	}
}

// TestPaletteEmptyQueryReturnsAllSorted 验证空查询返回全部按 Category 排序。
func TestPaletteEmptyQueryReturnsAllSorted(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	vs.Overlay.Query = ""
	p := NewPalette(vs)
	matched := p.matchedItems()
	if len(matched) != 15 {
		t.Fatalf("empty query should return all 15, got %d", len(matched))
	}
	// Category 单调非递减
	for i := 1; i < len(matched); i++ {
		if matched[i].Category < matched[i-1].Category {
			t.Fatalf("Category not sorted at %d", i)
		}
	}
}

// TestPaletteSelectionEmitsAction 验证 Enter 选中 emit 带 Action 的 PaletteCommandMsg。
func TestPaletteSelectionEmitsAction(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	vs.Overlay.Query = "exit"
	p := NewPalette(vs)
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit PaletteCommandMsg")
	}
	msg := cmd()
	pm, ok := msg.(PaletteCommandMsg)
	if !ok {
		t.Fatalf("want PaletteCommandMsg, got %T", msg)
	}
	if pm.Action != PaletteActionExit {
		t.Fatalf("action=%s, want exit", pm.Action)
	}
	if pm.Name != "/exit" {
		t.Fatalf("name=%s, want /exit", pm.Name)
	}
	if vs.Overlay.Active != state.OverlayNone {
		t.Fatal("palette should close after enter")
	}
}

// TestPaletteBackspaceUnicode 验证 palette Backspace 用 deleteLastRune 正确处理多字节 UTF-8。
func TestPaletteBackspaceUnicode(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	vs.Overlay.Query = "你好"
	p := NewPalette(vs)
	p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if vs.Overlay.Query != "你" {
		t.Fatalf("backspace query=%q, want 你", vs.Overlay.Query)
	}
	if !isValidUTF8(vs.Overlay.Query) {
		t.Fatalf("backspace produced invalid UTF-8: %q", vs.Overlay.Query)
	}
	// 连续删到空
	p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if vs.Overlay.Query != "" {
		t.Fatalf("2nd backspace query=%q, want empty", vs.Overlay.Query)
	}
}

// TestPaletteNavigationUpDownBoundary 验证导航边界（顶部不回绕、底部不越界）。
func TestPaletteNavigationUpDownBoundary(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	vs.Overlay.Query = ""
	p := NewPalette(vs)
	matched := p.matchedItems()
	// 顶部 up 不回绕
	p.Update(tea.KeyMsg{Type: tea.KeyUp})
	if vs.Overlay.Selected != 0 {
		t.Fatalf("up at top should stay 0, got %d", vs.Overlay.Selected)
	}
	// down 前进
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	if vs.Overlay.Selected != 1 {
		t.Fatalf("down should advance to 1, got %d", vs.Overlay.Selected)
	}
	// 连续 down 到底，再 down 不越界
	vs.Overlay.Selected = len(matched) - 1
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	if vs.Overlay.Selected != len(matched)-1 {
		t.Fatalf("down at bottom should not overflow, got %d want %d", vs.Overlay.Selected, len(matched)-1)
	}
}

// TestPaletteCtrlJKNavigation 验证 ctrl+j/k 导航（j/k 已改为查询字符）。
func TestPaletteCtrlJKNavigation(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	vs.Overlay.Query = ""
	p := NewPalette(vs)
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if vs.Overlay.Selected != 1 {
		t.Fatalf("ctrl+j should navigate down, got %d", vs.Overlay.Selected)
	}
	p.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if vs.Overlay.Selected != 0 {
		t.Fatalf("ctrl+k should navigate up, got %d", vs.Overlay.Selected)
	}
}

// TestPaletteQueryContainsJKAsChar 验证 j/k 作为查询字符（不触发导航）。
func TestPaletteQueryContainsJKAsChar(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	vs.Overlay.Query = ""
	p := NewPalette(vs)
	// 搜索 "checkpoint" 含 k，不应被导航吞掉
	for _, r := range "checkpoint" {
		p.Update(keyRunesMsg(string(r)))
	}
	if vs.Overlay.Query != "checkpoint" {
		t.Fatalf("query=%q, want checkpoint (k should not be swallowed)", vs.Overlay.Query)
	}
}

// TestPaletteViewRendersShortcut 验证 View 渲染含快捷键列。
func TestPaletteViewRendersShortcut(t *testing.T) {
	vs := paletteState()
	vs.Overlay.Active = state.OverlayPalette
	vs.Overlay.Query = "new"
	p := NewPalette(vs)
	v := p.View()
	if v == "" {
		t.Fatal("view empty")
	}
	// /new 的快捷键是 Space n，应出现在渲染输出
	if !containsShortcut(v, "Space n") {
		t.Fatalf("view should render shortcut 'Space n' for /new, got:\n%s", v)
	}
}

// matchedNames 提取命令名列表用于断言。
func matchedNames(items []PaletteItem) []string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	return names
}

// isValidUTF8 检查字符串是否为有效 UTF-8。
func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}

// containsShortcut 检查渲染输出是否含快捷键文本（容忍 ANSI 转义）。
func containsShortcut(s, sub string) bool {
	stripped := strings.NewReplacer("\x1b[0m", "", "\x1b[1m", "").Replace(s)
	return strings.Contains(stripped, sub)
}

// keyRunesMsg 构造携带 rune 的 KeyMsg（用于逐字符输入测试）。
func keyRunesMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
