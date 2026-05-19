package components

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// fitBlock 将组件输出限制在指定宽度内，避免 lipgloss 宽度布局触发自动换行。
func fitBlock(content string, width int, pad bool) string {
	if width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = fitBlockLine(line, width, pad)
	}
	return strings.Join(lines, "\n")
}

// fitBlockLine 按显示宽度截断单行，并按需补齐到安全宽度。
func fitBlockLine(line string, width int, pad bool) string {
	target := width - 1
	if target <= 0 {
		return ""
	}
	fitted := ansi.Truncate(line, target, "")
	if !pad {
		return fitted
	}
	lineWidth := ansi.StringWidth(fitted)
	if lineWidth < target {
		fitted += strings.Repeat(" ", target-lineWidth)
	}
	return fitted
}
