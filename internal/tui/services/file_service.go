package services

import (
	"path/filepath"
	"sort"
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

	pathTargets := make([]string, 0, len(candidates))
	baseTargets := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		pathTargets = append(pathTargets, strings.ToLower(candidate))
		baseTargets = append(baseTargets, strings.ToLower(filepath.Base(candidate)))
	}

	pathMatches := fuzzy.Find(query, pathTargets)
	baseMatches := fuzzy.Find(query, baseTargets)
	if len(pathMatches) == 0 && len(baseMatches) == 0 {
		return nil
	}

	const filenameMatchBoost = 1000

	scores := make(map[int]int, len(pathMatches)+len(baseMatches))
	for _, match := range pathMatches {
		scores[match.Index] = max(scores[match.Index], match.Score)
	}
	for _, match := range baseMatches {
		scores[match.Index] = max(scores[match.Index], match.Score+filenameMatchBoost)
	}

	type rankedMatch struct {
		index int
		score int
	}
	ranked := make([]rankedMatch, 0, len(scores))
	for index, score := range scores {
		ranked = append(ranked, rankedMatch{index: index, score: score})
	}
	sort.SliceStable(ranked, func(i int, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		leftBase := filepath.Base(candidates[ranked[i].index])
		rightBase := filepath.Base(candidates[ranked[j].index])
		if len(leftBase) != len(rightBase) {
			return len(leftBase) < len(rightBase)
		}
		return candidates[ranked[i].index] < candidates[ranked[j].index]
	})

	out := make([]string, 0, min(limit, len(ranked)))
	for _, match := range ranked {
		out = append(out, candidates[match.index])
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
