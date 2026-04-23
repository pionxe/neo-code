package context

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"neo-code/internal/repository"
)

// collectSystemState 汇总运行时上下文，并通过 repository summary 获取 git 摘要。
func collectSystemState(ctx context.Context, metadata Metadata, summaryProvider repositorySummaryFunc) (SystemState, error) {
	state := SystemState{
		Workdir:  strings.TrimSpace(metadata.Workdir),
		Shell:    strings.TrimSpace(metadata.Shell),
		Provider: strings.TrimSpace(metadata.Provider),
		Model:    strings.TrimSpace(metadata.Model),
	}

	if err := ctx.Err(); err != nil {
		return state, err
	}
	if summaryProvider == nil || state.Workdir == "" {
		return state, nil
	}

	summary, err := summaryProvider(ctx, state.Workdir)
	if err != nil {
		if isContextError(err) {
			return state, err
		}
		return state, nil
	}

	state.Git = toGitState(summary)
	return state, nil
}

// toGitState 将 repository 层的结构化摘要映射为 context 当前使用的最小 git 状态。
func toGitState(summary repository.Summary) GitState {
	if !summary.InGitRepo {
		return GitState{}
	}
	return GitState{
		Available: true,
		Branch:    strings.TrimSpace(summary.Branch),
		Dirty:     summary.Dirty,
		Ahead:     summary.Ahead,
		Behind:    summary.Behind,
	}
}

func renderSystemStateSection(state SystemState) promptSection {
	lines := []string{
		fmt.Sprintf("- workdir: `%s`", promptValue(state.Workdir)),
		fmt.Sprintf("- shell: `%s`", promptValue(state.Shell)),
		fmt.Sprintf("- provider: `%s`", promptValue(state.Provider)),
		fmt.Sprintf("- model: `%s`", promptValue(state.Model)),
	}

	if state.Git.Available {
		dirty := "clean"
		if state.Git.Dirty {
			dirty = "dirty"
		}
		lines = append(lines, fmt.Sprintf(
			"- git: branch=`%s`, dirty=`%s`, ahead=`%d`, behind=`%d`",
			promptValue(state.Git.Branch),
			dirty,
			state.Git.Ahead,
			state.Git.Behind,
		))
	} else {
		lines = append(lines, "- git: unavailable")
	}

	return promptSection{
		Title:   "System State",
		Content: strings.Join(lines, "\n"),
	}
}

func promptValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
