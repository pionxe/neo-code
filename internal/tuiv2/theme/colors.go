// Package theme 定义 TUI v2 Ghost Console 的颜色、符号和样式约束。
//
// 视觉禁令（lint 级约束）：
//   - 绝对禁止在普通界面组件中使用任何 Border()；只有弹窗或命令面板可例外。
//   - 绝对禁止圆角背景块或药丸状标签（Pills）；用纯文本标签加颜色表达语义。
//   - 绝对禁止传统进度条；必须用状态符号序列表达进度或阶段。
//   - 区块只能通过缩进、颜色和单条细线区分，不使用表格竖线或后台管理面板风格。
package theme

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ThemeColors 定义 Ghost Console 的完整语义色板。
type ThemeColors struct {
	BG     lipgloss.Color
	FG     lipgloss.Color
	Subtle lipgloss.Color
	Muted  lipgloss.Color
	Border lipgloss.Color

	Accent  lipgloss.Color
	Success lipgloss.Color
	Warning lipgloss.Color
	Error   lipgloss.Color
	Info    lipgloss.Color

	ToolName  lipgloss.Color
	FilePath  lipgloss.Color
	CodeBlock lipgloss.Color
	DiffAdd   lipgloss.Color
	DiffDel   lipgloss.Color
}

// TokyoNight 是当前终端能力下的 Tokyo Night 色板，启动时会自动做 256 色降级。
var TokyoNight = TokyoNightForProfile(DetectColorProfile())

// ColorProfile 表示终端颜色能力。
type ColorProfile int

const (
	// ColorProfile256 表示使用 256 色降级色板。
	ColorProfile256 ColorProfile = iota
	// ColorProfileTrueColor 表示使用 24-bit true color 色板。
	ColorProfileTrueColor
)

// TokyoNightForProfile 返回指定颜色能力下的 Tokyo Night 色板。
func TokyoNightForProfile(profile ColorProfile) ThemeColors {
	if profile == ColorProfileTrueColor {
		return tokyoNightTrueColor()
	}
	return tokyoNight256()
}

// DetectColorProfile 根据环境变量判断是否启用 true color。
func DetectColorProfile() ColorProfile {
	return DetectColorProfileFromEnv(os.Getenv)
}

// DetectColorProfileFromEnv 使用给定 getenv 函数判断颜色能力，便于测试降级逻辑。
func DetectColorProfileFromEnv(getenv func(string) string) ColorProfile {
	colorTerm := strings.ToLower(getenv("COLORTERM"))
	if strings.Contains(colorTerm, "truecolor") || strings.Contains(colorTerm, "24bit") {
		return ColorProfileTrueColor
	}
	if strings.EqualFold(getenv("NEOCODE_TUI_TRUECOLOR"), "1") {
		return ColorProfileTrueColor
	}
	return ColorProfile256
}

func tokyoNightTrueColor() ThemeColors {
	return ThemeColors{
		BG:        lipgloss.Color("#1a1b26"),
		FG:        lipgloss.Color("#c0caf5"),
		Subtle:    lipgloss.Color("#565f89"),
		Muted:     lipgloss.Color("#414868"),
		Border:    lipgloss.Color("#565f89"),
		Accent:    lipgloss.Color("#7aa2f7"),
		Success:   lipgloss.Color("#9ece6a"),
		Warning:   lipgloss.Color("#e0af68"),
		Error:     lipgloss.Color("#f7768e"),
		Info:      lipgloss.Color("#7dcfff"),
		ToolName:  lipgloss.Color("#bb9af7"),
		FilePath:  lipgloss.Color("#7dcfff"),
		CodeBlock: lipgloss.Color("#c0caf5"),
		DiffAdd:   lipgloss.Color("#9ece6a"),
		DiffDel:   lipgloss.Color("#f7768e"),
	}
}
