package context

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	agentsession "neo-code/internal/session"
)

// taskStateSource 负责将 durable TaskState 投影为稳定的 prompt section。
type taskStateSource struct{}

// Sections 将任务状态渲染为 provider 可读的结构化上下文。
func (taskStateSource) Sections(ctx context.Context, input BuildInput) ([]promptSection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	state := agentsession.NormalizeTaskState(input.TaskState)
	if !state.Established() {
		return nil, nil
	}

	return []promptSection{renderTaskStateSection(state)}, nil
}

// renderTaskStateSection 把任务状态转成稳定顺序的文本段，供模型恢复长期任务上下文。
func renderTaskStateSection(state agentsession.TaskState) promptSection {
	lines := make([]string, 0, 8)
	lines = append(lines, fmt.Sprintf("- goal: %s", promptTaskStateValue(state.Goal)))
	lines = append(lines, fmt.Sprintf("- progress: %s", promptTaskStateListValue(state.Progress)))
	lines = append(lines, fmt.Sprintf("- open_items: %s", promptTaskStateListValue(state.OpenItems)))
	lines = append(lines, fmt.Sprintf("- next_step: %s", promptTaskStateValue(state.NextStep)))
	lines = append(lines, fmt.Sprintf("- blockers: %s", promptTaskStateListValue(state.Blockers)))
	lines = append(lines, fmt.Sprintf("- key_artifacts: %s", promptTaskStateListValue(state.KeyArtifacts)))
	lines = append(lines, fmt.Sprintf("- decisions: %s", promptTaskStateListValue(state.Decisions)))
	lines = append(lines, fmt.Sprintf("- user_constraints: %s", promptTaskStateListValue(state.UserConstraints)))

	return promptSection{
		Title:   "Task State",
		Content: strings.Join(lines, "\n"),
	}
}

// promptTaskStateValue 统一渲染任务状态中的单值字段。
func promptTaskStateValue(value string) string {
	value = sanitizePromptTaskStateText(value)
	if value == "" {
		return "none"
	}
	return value
}

// promptTaskStateListValue 统一渲染任务状态中的列表字段。
func promptTaskStateListValue(values []string) string {
	if len(values) == 0 {
		return "none"
	}

	sanitized := make([]string, 0, len(values))
	for _, value := range values {
		value = sanitizePromptTaskStateText(value)
		if value == "" {
			continue
		}
		sanitized = append(sanitized, value)
	}
	if len(sanitized) == 0 {
		return "none"
	}
	return strings.Join(sanitized, " | ")
}

// sanitizePromptTaskStateText 将 TaskState 文本收敛为单行安全片段，避免注入额外 prompt 结构。
func sanitizePromptTaskStateText(value string) string {
	value = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), unicode.IsSpace(r):
			return ' '
		default:
			return r
		}
	}, value)
	return strings.Join(strings.Fields(value), " ")
}
