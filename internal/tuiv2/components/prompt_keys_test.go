package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/tuiv2/state"
)

// 覆盖 CommandPrompt 的删除、光标移动、权限/问答模式全分支。
func TestCommandPromptEditingAndModes(t *testing.T) {
	vs := promptState()
	p := NewCommandPrompt(vs)

	// insertRunes 空切片保护
	p.insertRunes(nil)
	// deleteBeforeCursor 在光标=0 时不动作
	p.deleteBeforeCursor()
	// 正常编辑：插入、移动光标、删除
	p.insertText("hello")
	p.moveCursor(-2)       // 光标到 index 3
	p.deleteBeforeCursor() // 删除 index 2 的 'l' -> "helo"，光标 2
	if vs.Input.Text != "helo" {
		t.Fatalf("after deleteBeforeCursor text=%q", vs.Input.Text)
	}
	p.deleteAtCursor() // 删除 index 2 的 'l' -> "heo"
	if vs.Input.Text != "heo" {
		t.Fatalf("after deleteAtCursor text=%q", vs.Input.Text)
	}
	// deleteAtCursor 在末尾不动作
	p.moveCursor(100)
	p.deleteAtCursor()
	// home/end
	p.Update(keyType(tea.KeyHome))
	if vs.Input.Cursor != 0 {
		t.Fatalf("home cursor=%d", vs.Input.Cursor)
	}
	p.Update(keyType(tea.KeyEnd))
	// left/right 移动
	p.Update(keyType(tea.KeyLeft))
	p.Update(keyType(tea.KeyRight))
	// delete 键
	p.Update(keyType(tea.KeyDelete))
	// clearText
	p.clearText()
	if vs.Input.Text != "" {
		t.Fatal("clearText failed")
	}
	// clampInt 边界
	if clampInt(5, 0, 3) != 3 || clampInt(-1, 0, 3) != 0 || clampInt(2, 0, 3) != 2 {
		t.Fatal("clampInt wrong")
	}
	// runeLen
	if runeLen("你好") != 2 {
		t.Fatal("runeLen wrong")
	}
}

func TestCommandPromptPermissionKeyFull(t *testing.T) {
	vs := promptState()
	vs.Input.Mode = state.InputStateModePermissionResponse
	vs.Input.Prompt = "允许写入？"
	p := NewCommandPrompt(vs)
	// 渲染权限视图
	if v := p.View(); v == "" {
		t.Fatal("permission view empty")
	}
	// y/n/d/a 决策
	for _, decision := range []string{"y", "n", "d", "a"} {
		vs2 := promptState()
		vs2.Input.Mode = state.InputStateModePermissionResponse
		pp := NewCommandPrompt(vs2)
		_, cmd := pp.Update(keyMsg(decision))
		if cmd == nil {
			t.Fatalf("%s should emit PermissionActionMsg", decision)
		}
	}
	// 大写 Y
	vs3 := promptState()
	vs3.Input.Mode = state.InputStateModePermissionResponse
	pp3 := NewCommandPrompt(vs3)
	_, cmd := pp3.Update(keyMsg("Y"))
	if cmd == nil {
		t.Fatal("Y should emit PermissionActionMsg")
	}
	// esc -> PromptCancelMsg
	vs4 := promptState()
	vs4.Input.Mode = state.InputStateModePermissionResponse
	pp4 := NewCommandPrompt(vs4)
	_, cmd = pp4.Update(keyType(tea.KeyEsc))
	if _, ok := cmd().(PromptCancelMsg); !ok {
		t.Fatal("esc should emit PromptCancelMsg")
	}
	// left/right/backspace + 可打印字符输入
	vs5 := promptState()
	vs5.Input.Mode = state.InputStateModePermissionResponse
	pp5 := NewCommandPrompt(vs5)
	pp5.Update(keyMsg("x"))
	pp5.Update(keyType(tea.KeyLeft))
	pp5.Update(keyType(tea.KeyRight))
	pp5.Update(keyType(tea.KeyBackspace))
}

func TestCommandPromptQuestionKeyFull(t *testing.T) {
	vs := promptState()
	vs.Input.Mode = state.InputStateModeQuestionAnswer
	vs.Input.Prompt = "选哪个？"
	vs.Input.Options = []string{"甲", "乙"}
	p := NewCommandPrompt(vs)
	if v := p.View(); v == "" {
		t.Fatal("question view empty")
	}
	// 空文本回车 -> nil
	if _, cmd := p.Update(keyType(tea.KeyEnter)); cmd != nil {
		t.Fatal("empty enter should be nil")
	}
	// 输入后回车 -> QuestionAnswerMsg
	p.Update(keyMsg("1"))
	_, cmd := p.Update(keyType(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("enter with text should emit QuestionAnswerMsg")
	}
	if _, ok := cmd().(QuestionAnswerMsg); !ok {
		t.Fatal("want QuestionAnswerMsg")
	}
	// esc -> PromptCancelMsg
	vs2 := promptState()
	vs2.Input.Mode = state.InputStateModeQuestionAnswer
	pp := NewCommandPrompt(vs2)
	_, cmd = pp.Update(keyType(tea.KeyEsc))
	if _, ok := cmd().(PromptCancelMsg); !ok {
		t.Fatal("esc should emit PromptCancelMsg")
	}
	// left/right/delete + 可打印输入
	pp.Update(keyMsg("z"))
	pp.Update(keyType(tea.KeyLeft))
	pp.Update(keyType(tea.KeyRight))
	pp.Update(keyType(tea.KeyDelete))
}

func TestCommandPromptInitAndCursorBlink(t *testing.T) {
	vs := promptState()
	p := NewCommandPrompt(vs)
	if cmd := p.Init(); cmd == nil {
		t.Fatal("Init should start cursor blink")
	} else {
		// CursorBlinkMsg 翻转可见性并续订
		if msg := cmd(); msg == nil {
			t.Fatal("blink cmd produced nil msg")
		}
	}
	_, cmd := p.Update(CursorBlinkMsg{})
	if cmd == nil {
		t.Fatal("CursorBlinkMsg should renew blink cmd")
	}
}

func TestCommandPromptMessageLinesHelpers(t *testing.T) {
	vs := promptState()
	vs.Input.Mode = state.InputStateModeMessage
	p := NewCommandPrompt(vs)
	// 普通 message 模式 View
	if v := p.View(); v == "" {
		t.Fatal("message view empty")
	}
	// wrapText：超宽切分
	wrapped := wrapText("短文本短文本短文本短文本短文本短文本短文本短文本", 6)
	if len(wrapped) < 2 {
		t.Fatalf("wrapText should split: %v", wrapped)
	}
	wrapText("x", 0) // width<=0 分支
	// contentWidth 回退
	vs.Layout.Width = 0
	if p.contentWidth() != 80 {
		t.Fatal("contentWidth fallback wrong")
	}
}
