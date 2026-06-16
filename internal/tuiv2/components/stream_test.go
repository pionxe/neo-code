package components

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"neo-code/internal/tuiv2/state"
	"neo-code/internal/tuiv2/theme"
)

func TestAgentStreamRendersEntryTypes(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	viewState := state.NewViewState()
	viewState.Layout.Width = 120
	viewState.Layout.Height = 40
	viewState.Stream = []state.StreamEntry{
		{ID: "m", Type: "message", Content: "hello", Timestamp: now},
		{ID: "ts", Type: "tool_start", ToolName: "read_file", Content: "main.go", Timestamp: now},
		{ID: "to", Type: "tool_output", Content: "12: package main", Timestamp: now},
		{ID: "te", Type: "tool_end", ToolName: "read_file", Timestamp: now},
		{ID: "p", Type: "permission", Content: "allow tool.write_file? [y/n]", Timestamp: now},
		{ID: "q", Type: "question", Content: "choose: 1. A  2. B", Timestamp: now},
		{ID: "s", Type: "status", Content: "connected to ghost-console", Timestamp: now},
		{ID: "e", Type: "error", Content: "connection refused", Timestamp: now},
	}

	view := NewAgentStream(viewState).View()
	for _, want := range []string{
		"hello",
		theme.StreamPrefix("tool_start") + " tool.read_file",
		"main.go",
		theme.AccentBar() + " 12: package main",
		theme.StreamPrefix("tool_end") + " tool.read_file",
		theme.StreamPrefix("permission_requested") + " allow tool.write_file? [y/n]",
		theme.Separator() + " choose: 1. A  2. B",
		theme.StreamPrefix("status") + " connected to ghost-console",
		theme.StreamPrefix("error") + " connection refused",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
	if strings.Contains(view, "┌") || strings.Contains(view, "┐") || strings.Contains(view, "└") || strings.Contains(view, "┘") {
		t.Fatalf("View() contains border rune:\n%s", view)
	}
}

func TestAgentStreamManualAndAutoScroll(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 80
	viewState.Layout.Height = 12
	viewState.Stream = numberedEntries(20)
	stream := NewAgentStream(viewState)

	if !viewState.Layout.AutoScroll {
		t.Fatal("AutoScroll default = false, want true")
	}
	_, _ = stream.Update(keyMsg("k"))
	if viewState.Layout.AutoScroll {
		t.Fatal("AutoScroll after k = true, want false")
	}
	if viewState.Layout.ScrollOffset == 0 {
		t.Fatal("ScrollOffset after k = 0, want > 0")
	}
	_, _ = stream.Update(keyMsg("j"))
	if !viewState.Layout.AutoScroll || viewState.Layout.ScrollOffset != 0 {
		t.Fatalf("after j at bottom AutoScroll=%t offset=%d, want true/0", viewState.Layout.AutoScroll, viewState.Layout.ScrollOffset)
	}
	_, _ = stream.Update(keyMsg("g"))
	if viewState.Layout.AutoScroll || viewState.Layout.ScrollOffset == 0 {
		t.Fatalf("after g AutoScroll=%t offset=%d, want false/>0", viewState.Layout.AutoScroll, viewState.Layout.ScrollOffset)
	}
	_, _ = stream.Update(keyMsg("G"))
	if !viewState.Layout.AutoScroll || viewState.Layout.ScrollOffset != 0 {
		t.Fatalf("after G AutoScroll=%t offset=%d, want true/0", viewState.Layout.AutoScroll, viewState.Layout.ScrollOffset)
	}
}

func TestAgentStreamWidthIsSafe(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 40
	viewState.Layout.Height = 10
	viewState.Stream = []state.StreamEntry{{ID: "long", Type: "message", Content: "这是一段很长很长的中英文 mixed content that must not wrap"}}

	for index, line := range strings.Split(NewAgentStream(viewState).View(), "\n") {
		if width := theme.DisplayWidth(line); width > 39 {
			t.Fatalf("line %d width = %d, want <= 39: %q", index, width, line)
		}
	}
}

func TestAgentStreamLargeStreamRenderBudget(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 100
	viewState.Layout.Height = 30
	viewState.Stream = numberedEntries(1200)
	stream := NewAgentStream(viewState)

	start := time.Now()
	view := stream.View()
	if elapsed := time.Since(start); elapsed > 16*time.Millisecond {
		t.Fatalf("View() elapsed = %v, want <= 16ms", elapsed)
	}
	if !strings.Contains(view, "line 1199") {
		t.Fatalf("View() does not include tail entry:\n%s", view)
	}
}

func TestAgentStreamTimestampGap(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 100
	viewState.Layout.Height = 20
	first := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	viewState.Stream = []state.StreamEntry{
		{ID: "one", Type: "message", Content: "one", Timestamp: first},
		{ID: "two", Type: "message", Content: "two", Timestamp: first.Add(6 * time.Minute)},
	}
	view := NewAgentStream(viewState).View()
	if !strings.Contains(view, "12:06") {
		t.Fatalf("View() missing timestamp gap:\n%s", view)
	}
}

func TestAgentStreamMultilineMessageAlignsUnderLabel(t *testing.T) {
	viewState := state.NewViewState()
	viewState.Layout.Width = 80
	viewState.Layout.Height = 20
	viewState.Stream = []state.StreamEntry{
		{ID: "m", Type: "message", Content: "第一行\n第二行内容\n第三行", Metadata: map[string]any{"role": "user"}},
	}
	stream := NewAgentStream(viewState)

	lines := stream.renderMessage(viewState.Stream[0])
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = ansi.Strip(l)
	}
	if len(plain) != 3 {
		t.Fatalf("rendered lines = %d, want 3: %v", len(plain), plain)
	}
	// 首行 = "  you " + 内容（you 占 3 列，前后各 2/1 空格，共 6 列）。
	wantIndent := theme.DisplayWidth("  you ")
	if !strings.HasPrefix(plain[0], "  you ") {
		t.Fatalf("line 0 = %q, want prefix %q", plain[0], "  you ")
	}
	// 续行必须是 wantIndent 个前导空格，使正文与首行正文逐列对齐。
	for i, line := range plain[1:] {
		leading := len(line) - len(strings.TrimLeft(line, " "))
		if leading != wantIndent {
			t.Fatalf("continuation line %d leading = %d, want %d: %q", i+1, leading, wantIndent, line)
		}
	}
}

func numberedEntries(count int) []state.StreamEntry {
	entries := make([]state.StreamEntry, 0, count)
	for i := 0; i < count; i++ {
		entries = append(entries, state.StreamEntry{
			ID:      fmt.Sprintf("entry-%d", i),
			Type:    "message",
			Content: fmt.Sprintf("line %d", i),
		})
	}
	return entries
}

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}
