package components

import "github.com/charmbracelet/lipgloss"

type InputBox struct {
	Body       string
	Generating bool
	Status     string
}

func (i InputBox) Render() string {
	helpText := "[Enter换行 F5/F8发送 PgUp/PgDn滚动]"
	if !i.Generating {
		helpText = "[Enter换行 F5/F8发送 Ctrl+V粘贴 鼠标点[Copy]复制 PgUp/PgDn滚动]"
	}

	statusText := i.Status
	if statusText == "" {
		statusText = "就绪"
	}

	status := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#61AFEF")).
		Render(statusText)

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#5C6370")).
		Render(helpText)

	return i.Body + "\n" + status + "\n" + footer
}
