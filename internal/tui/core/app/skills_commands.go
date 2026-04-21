package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	agentruntime "neo-code/internal/runtime"
	tuiservices "neo-code/internal/tui/services"
)

const unsupportedSkillActionReason = "unsupported_action_in_gateway_mode"

// skillCommandResultMsg 承载 skills 相关 slash 命令的异步执行结果。
type skillCommandResultMsg struct {
	Notice string
	Err    error
}

// handleSkillsCommand 处理 `/skills`，输出当前可用技能列表与会话激活状态。
func (a *App) handleSkillsCommand() tea.Cmd {
	sessionID := strings.TrimSpace(a.state.ActiveSessionID)
	return tuiservices.RunLocalCommandCmd(
		func(ctx context.Context) (string, error) {
			states, err := a.runtime.ListAvailableSkills(ctx, sessionID)
			if err != nil {
				return "", normalizeSkillCommandError(err)
			}
			return formatAvailableSkills(states, sessionID), nil
		},
		func(notice string, err error) tea.Msg {
			return skillCommandResultMsg{Notice: notice, Err: err}
		},
	)
}

// handleSkillCommand 解析 `/skill ...` 子命令，并分发到 use/off/active。
func (a *App) handleSkillCommand(rest string) tea.Cmd {
	action, argument := splitFirstWord(strings.TrimSpace(rest))
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "use":
		return a.handleSkillUseCommand(argument)
	case "off":
		return a.handleSkillOffCommand(argument)
	case "active":
		if strings.TrimSpace(argument) != "" {
			errText := fmt.Sprintf("usage: %s", slashUsageSkillActive)
			a.state.ExecutionError = errText
			a.state.StatusText = errText
			a.appendInlineMessage(roleError, errText)
			a.rebuildTranscript()
			return nil
		}
		return a.handleSkillActiveCommand()
	default:
		errText := "usage: /skill use <id> | /skill off <id> | /skill active"
		a.state.ExecutionError = errText
		a.state.StatusText = errText
		a.appendInlineMessage(roleError, errText)
		a.rebuildTranscript()
		return nil
	}
}

// handleSkillUseCommand 在当前会话激活指定 skill。
func (a *App) handleSkillUseCommand(skillID string) tea.Cmd {
	sessionID, ok := a.requireActiveSessionForSkillCommand()
	if !ok {
		return nil
	}
	normalizedSkillID := strings.TrimSpace(skillID)
	if normalizedSkillID == "" || isSkillUsagePlaceholder(normalizedSkillID) {
		errText := fmt.Sprintf("usage: %s", slashUsageSkillUse)
		a.state.ExecutionError = errText
		a.state.StatusText = errText
		a.appendInlineMessage(roleError, errText)
		a.rebuildTranscript()
		return nil
	}

	return tuiservices.RunLocalCommandCmd(
		func(ctx context.Context) (string, error) {
			if err := a.runtime.ActivateSessionSkill(ctx, sessionID, normalizedSkillID); err != nil {
				return "", normalizeSkillCommandError(err)
			}
			return fmt.Sprintf("Skill activated: %s", normalizedSkillID), nil
		},
		func(notice string, err error) tea.Msg {
			return skillCommandResultMsg{Notice: notice, Err: err}
		},
	)
}

// handleSkillOffCommand 在当前会话停用指定 skill。
func (a *App) handleSkillOffCommand(skillID string) tea.Cmd {
	sessionID, ok := a.requireActiveSessionForSkillCommand()
	if !ok {
		return nil
	}
	normalizedSkillID := strings.TrimSpace(skillID)
	if normalizedSkillID == "" || isSkillUsagePlaceholder(normalizedSkillID) {
		errText := fmt.Sprintf("usage: %s", slashUsageSkillOff)
		a.state.ExecutionError = errText
		a.state.StatusText = errText
		a.appendInlineMessage(roleError, errText)
		a.rebuildTranscript()
		return nil
	}

	return tuiservices.RunLocalCommandCmd(
		func(ctx context.Context) (string, error) {
			if err := a.runtime.DeactivateSessionSkill(ctx, sessionID, normalizedSkillID); err != nil {
				return "", normalizeSkillCommandError(err)
			}
			return fmt.Sprintf("Skill deactivated: %s", normalizedSkillID), nil
		},
		func(notice string, err error) tea.Msg {
			return skillCommandResultMsg{Notice: notice, Err: err}
		},
	)
}

// isSkillUsagePlaceholder 判断入参是否还是 help 文案中的占位符（例如 <id>）。
func isSkillUsagePlaceholder(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "<") && strings.HasSuffix(trimmed, ">")
}

// handleSkillActiveCommand 输出当前会话激活技能状态（含缺失项标记）。
func (a *App) handleSkillActiveCommand() tea.Cmd {
	sessionID, ok := a.requireActiveSessionForSkillCommand()
	if !ok {
		return nil
	}
	return tuiservices.RunLocalCommandCmd(
		func(ctx context.Context) (string, error) {
			states, err := a.runtime.ListSessionSkills(ctx, sessionID)
			if err != nil {
				return "", normalizeSkillCommandError(err)
			}
			return formatSessionSkills(states), nil
		},
		func(notice string, err error) tea.Msg {
			return skillCommandResultMsg{Notice: notice, Err: err}
		},
	)
}

// requireActiveSessionForSkillCommand 校验 skills 会话命令所需的 session 上下文是否存在。
func (a *App) requireActiveSessionForSkillCommand() (string, bool) {
	sessionID := strings.TrimSpace(a.state.ActiveSessionID)
	if sessionID != "" {
		return sessionID, true
	}
	errText := "skill command requires an active session; send one message first or switch session via /session"
	a.state.ExecutionError = errText
	a.state.StatusText = errText
	a.appendInlineMessage(roleError, errText)
	a.rebuildTranscript()
	return "", false
}

// normalizeSkillCommandError 将 gateway 不支持等底层错误映射为可读的命令反馈。
func normalizeSkillCommandError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), unsupportedSkillActionReason) {
		return errors.New("gateway 模式暂不支持 skills 管理，请切换到 local runtime")
	}
	return err
}

// formatAvailableSkills 渲染 `/skills` 输出，包含可见技能清单与当前激活标记。
func formatAvailableSkills(states []agentruntime.AvailableSkillState, sessionID string) string {
	if len(states) == 0 {
		return "No skills found in local registry."
	}
	rows := make([]string, 0, len(states)+2)
	header := "Available skills:"
	if strings.TrimSpace(sessionID) != "" {
		header += " (active marks from current session)"
	}
	rows = append(rows, header)
	for _, state := range states {
		scope := strings.TrimSpace(string(state.Descriptor.Scope))
		if scope == "" {
			scope = "explicit"
		}
		status := "inactive"
		if state.Active {
			status = "active"
		}
		description := strings.TrimSpace(state.Descriptor.Description)
		if description == "" {
			description = "-"
		}
		rows = append(rows, fmt.Sprintf(
			"- %s [%s] scope=%s source=%s version=%s | %s",
			state.Descriptor.ID,
			status,
			scope,
			state.Descriptor.Source.Kind,
			strings.TrimSpace(state.Descriptor.Version),
			description,
		))
	}
	return strings.Join(rows, "\n")
}

// formatSessionSkills 渲染 `/skill active` 输出，并明确缺失技能状态。
func formatSessionSkills(states []agentruntime.SessionSkillState) string {
	if len(states) == 0 {
		return "No active skills in current session."
	}
	normalized := append([]agentruntime.SessionSkillState(nil), states...)
	sort.Slice(normalized, func(i, j int) bool {
		return strings.ToLower(strings.TrimSpace(normalized[i].SkillID)) <
			strings.ToLower(strings.TrimSpace(normalized[j].SkillID))
	})

	rows := make([]string, 0, len(normalized)+1)
	rows = append(rows, "Active skills:")
	for _, state := range normalized {
		if state.Missing {
			rows = append(rows, fmt.Sprintf("- %s [missing]", state.SkillID))
			continue
		}
		if state.Descriptor == nil {
			rows = append(rows, fmt.Sprintf("- %s [active]", state.SkillID))
			continue
		}
		description := strings.TrimSpace(state.Descriptor.Description)
		if description == "" {
			description = "-"
		}
		rows = append(rows, fmt.Sprintf("- %s [active] %s", state.Descriptor.ID, description))
	}
	return strings.Join(rows, "\n")
}
