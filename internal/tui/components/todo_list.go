package components

import (
	"fmt"
	"strings"

	"go-llm-demo/internal/tui/services"
	"go-llm-demo/internal/tui/todo"

	"github.com/charmbracelet/lipgloss"
)

type TodoList struct {
	Todos   []services.Todo
	Cursor  int
	Width   int
	Focused bool
}

func (tl TodoList) Render() string {
	if len(tl.Todos) == 0 {
		style := lipgloss.NewStyle().
			Foreground(todo.ColorDim).
			Italic(true).
			Padding(1, 2)
		return style.Render(todo.EmptyText)
	}

	var b strings.Builder
	titleStyle := lipgloss.NewStyle().
		Foreground(todo.ColorTitle).
		Bold(true).
		PaddingBottom(1)

	b.WriteString(titleStyle.Render(todo.TitleText))
	b.WriteString("\n")

	for i, t := range tl.Todos {
		cursor := todo.IconNoCursor
		itemStyle := lipgloss.NewStyle()
		if i == tl.Cursor && tl.Focused {
			cursor = todo.IconCursor
			itemStyle = itemStyle.Background(todo.ColorSelection).Foreground(lipgloss.Color("#FFFFFF"))
		}

		statusIcon := todo.IconPending
		statusStyle := lipgloss.NewStyle().Foreground(todo.ColorPending)
		switch t.Status {
		case services.TodoInProgress:
			statusIcon = todo.IconInProgress
			statusStyle = lipgloss.NewStyle().Foreground(todo.ColorInProgress)
		case services.TodoCompleted:
			statusIcon = todo.IconCompleted
			statusStyle = lipgloss.NewStyle().Foreground(todo.ColorCompleted)
		}

		priorityLabel := ""
		priorityStyle := lipgloss.NewStyle()
		switch t.Priority {
		case services.TodoPriorityHigh:
			priorityLabel = fmt.Sprintf(" (%s)", todo.PriorityHigh)
			priorityStyle = priorityStyle.Foreground(todo.ColorPriorityHigh).Bold(true)
		case services.TodoPriorityMedium:
			priorityLabel = fmt.Sprintf(" (%s)", todo.PriorityMedium)
			priorityStyle = priorityStyle.Foreground(todo.ColorPending)
		case services.TodoPriorityLow:
			priorityLabel = fmt.Sprintf(" (%s)", todo.PriorityLow)
			priorityStyle = priorityStyle.Foreground(todo.ColorDim)
		}

		content := fmt.Sprintf("%s%s %s%s", cursor, statusStyle.Render(statusIcon), t.Content, priorityStyle.Render(priorityLabel))
		b.WriteString(itemStyle.Width(tl.Width).Render(content))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(todo.ColorDim).Italic(true)
	b.WriteString(helpStyle.Render(todo.HelpFooterText))

	return b.String()
}
