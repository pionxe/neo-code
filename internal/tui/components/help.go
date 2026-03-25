package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func RenderHelp(width int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#61AFEF")).
		Bold(true).
		Render("NeoCode 帮助")

	b.WriteString(title)
	b.WriteString("\n\n")

	commands := []struct {
		cmd  string
		desc string
	}{
		{"/help", "显示帮助"},
		{"/pwd | /workspace", "显示当前工作区目录"},
		{"/apikey <env_name>", "切换 API Key 变量名"},
		{"/provider <name>", "切换模型提供商"},
		{"/switch <model>", "切换模型"},
		{"/todo [add|list]", "管理待办清单"},
		{"/run <code>", "执行代码"},
		{"/explain <code>", "解释代码"},
		{"/memory", "显示记忆统计"},
		{"/clear-memory confirm", "清空长期记忆"},
		{"/clear-context", "清空会话上下文"},
		{"/exit", "退出程序"},
	}

	cmdStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#98C379")).
		Width(22)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ABB2BF"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5C6370"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#61AFEF"))

	for _, c := range commands {
		b.WriteString(cmdStyle.Render(c.cmd))
		b.WriteString(descStyle.Render(c.desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("输入框支持光标、粘贴、滚动，F5/F8 发送"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("聊天区支持 PgUp/PgDn、鼠标滚轮，以及点击代码块 [Copy] 复制"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("取消: Ctrl+C"))

	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("按 Esc 或 /help 关闭"))

	return lipgloss.NewStyle().MaxWidth(width).Render(b.String())
}
