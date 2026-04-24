package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// fullAccessPromptOption 描述 Full Access 风险确认弹窗中的一个选项。
type fullAccessPromptOption struct {
	Label  string
	Hint   string
	Enable bool
}

var fullAccessPromptOptions = []fullAccessPromptOption{
	{
		Label:  "Yes",
		Hint:   "Enable full access and auto-approve all tool requests",
		Enable: true,
	},
	{
		Label:  "No",
		Hint:   "Keep normal approval flow",
		Enable: false,
	},
}

// fullAccessPromptState 保存 Full Access 风险确认弹窗状态。
type fullAccessPromptState struct {
	Selected int
}

// normalizeFullAccessPromptSelection 将选项下标收敛到合法范围，避免越界读取。
func normalizeFullAccessPromptSelection(selected int) int {
	if len(fullAccessPromptOptions) == 0 {
		return 0
	}
	if selected < 0 {
		return len(fullAccessPromptOptions) - 1
	}
	if selected >= len(fullAccessPromptOptions) {
		return 0
	}
	return selected
}

// fullAccessPromptOptionAt 返回指定下标对应的 Full Access 风险确认选项。
func fullAccessPromptOptionAt(selected int) fullAccessPromptOption {
	index := normalizeFullAccessPromptSelection(selected)
	return fullAccessPromptOptions[index]
}

// parseFullAccessPromptShortcut 将 y/n 快捷输入映射为启用或取消动作。
func parseFullAccessPromptShortcut(input string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y", "yes", "enable":
		return true, true
	case "n", "no", "cancel":
		return false, true
	default:
		return false, false
	}
}

// formatFullAccessPromptLines 构造 Full Access 风险确认弹窗文案。
func formatFullAccessPromptLines(state fullAccessPromptState) []string {
	normalizedIdx := normalizeFullAccessPromptSelection(state.Selected)
	lines := []string{
		"Enable Full Access Mode?",
		"Risk: all tool permission requests will be auto-approved for this session.",
		"Press Y/N to approve, or use Up/Down + Enter.",
	}

	for index, item := range fullAccessPromptOptions {
		prefix := "  "
		if normalizedIdx == index {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s  - %s", prefix, item.Label, item.Hint))
	}
	return lines
}

// renderFullAccessPrompt 渲染 Full Access 风险确认弹窗。
func (a App) renderFullAccessPrompt() string {
	if a.pendingFullAccessPrompt == nil {
		return a.input.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, formatFullAccessPromptLines(*a.pendingFullAccessPrompt)...)
}
