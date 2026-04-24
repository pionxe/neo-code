package services

import (
	"path/filepath"
	"strings"

	"github.com/sahilm/fuzzy"

	tuiinfra "neo-code/internal/tui/infra"
)

// CollectWorkspaceFiles 收集工作区候选文件（由 infra 实际执行扫描）。
func CollectWorkspaceFiles(root string, limit int) ([]string, error) {
	return tuiinfra.CollectWorkspaceFiles(root, limit)
}

// SuggestFileMatches 基于 query 从候选集中返回优先级排序后的建议列表。
func SuggestFileMatches(query string, candidates []string, limit int) []string {
	if len(candidates) == 0 || limit <= 0 {
		return nil
	}

	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		end := min(len(candidates), limit)
		out := make([]string, 0, end)
		out = append(out, candidates[:end]...)
		return out
	}

	targets := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		targets = append(targets, strings.ToLower(candidate))
	}

	matches := fuzzy.Find(query, targets)
	if len(matches) == 0 {
		return nil
	}

	out := make([]string, 0, min(limit, len(matches)))
	for _, match := range matches {
		out = append(out, candidates[match.Index])
		if len(out) >= limit {
			break
		}
	}
	return out
}

// ResolveWorkspaceDirectory 将工作区路径解析为绝对路径，失败时返回空字符串。
func ResolveWorkspaceDirectory(workdir string) string {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return ""
	}
	if strings.ContainsRune(workdir, '\x00') {
		return ""
	}
	absolute, err := filepath.Abs(workdir)
	if err != nil {
		return ""
	}
	return absolute
}
