package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// BaseStyle 返回带默认前景和背景色的基础样式。
func BaseStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(TokyoNight.FG).Background(TokyoNight.BG)
}

// AccentStyle 返回强调文本样式。
func AccentStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.Accent)
}

// SuccessStyle 返回成功文本样式。
func SuccessStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.Success)
}

// WarningStyle 返回警告或等待文本样式。
func WarningStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.Warning)
}

// ErrorStyle 返回错误文本样式。
func ErrorStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.Error)
}

// MutedStyle 返回弱化文本样式。
func MutedStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.Muted)
}

// SubtleStyle 返回提示和分隔符样式。
func SubtleStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.Subtle)
}

// ToolNameStyle 返回工具名样式。
func ToolNameStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.ToolName)
}

// FilePathStyle 返回文件路径样式。
func FilePathStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.FilePath)
}

// CodeBlockStyle 返回代码块样式。
func CodeBlockStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.CodeBlock)
}

// InfoStyle 返回信息/用户消息样式（Cyan）。
func InfoStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.Info)
}

// TimestampStyle 返回时间戳样式。
func TimestampStyle() lipgloss.Style {
	return BaseStyle().Foreground(TokyoNight.Subtle)
}

// Truncate 按终端显示宽度截断字符串，并保留 ANSI 序列。
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	return ansi.Truncate(s, max, "")
}

// DisplayWidth 返回字符串的终端显示宽度。
func DisplayWidth(s string) int {
	return ansi.StringWidth(s)
}

// PadRight 将字符串补齐到指定显示宽度。
func PadRight(s string, width int) string {
	current := DisplayWidth(s)
	if current >= width {
		return s
	}
	return s + strings.Repeat(" ", width-current)
}
