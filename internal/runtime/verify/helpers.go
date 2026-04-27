package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// verificationDeniedResult 统一生成路径越界等安全拒绝结果。
func verificationDeniedResult(name string, summary string, reason string, evidence map[string]any) VerificationResult {
	return VerificationResult{
		Name:       name,
		Status:     VerificationFail,
		Summary:    summary,
		Reason:     reason,
		ErrorClass: ErrorClassPermissionDenied,
		Evidence:   evidence,
	}
}

// collectArtifactTargets 汇总 required todo artifacts 与 task_state.key_artifacts。
func collectArtifactTargets(input FinalVerifyInput) []string {
	paths := make([]string, 0)
	seen := make(map[string]struct{})
	appendPath := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	for _, path := range input.TaskState.KeyArtifacts {
		appendPath(path)
	}
	for _, todo := range input.Todos {
		if !todo.Required {
			continue
		}
		for _, path := range todo.Artifacts {
			appendPath(path)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	return paths
}

// collectContentCheckRules 收敛 todo.content_checks 为 artifact -> tokens 规则映射。
func collectContentCheckRules(input FinalVerifyInput) (map[string][]string, error) {
	rules := make(map[string][]string)
	for _, todo := range input.Todos {
		if !todo.Required {
			continue
		}
		for _, check := range todo.ContentChecks {
			artifact := strings.TrimSpace(check.Artifact)
			if artifact == "" {
				return nil, fmt.Errorf("content check artifact is empty")
			}
			contains := compactStrings(check.Contains)
			if len(contains) == 0 {
				return nil, fmt.Errorf("content check for %q is empty", artifact)
			}
			rules[artifact] = append(rules[artifact], contains...)
		}
	}
	if len(rules) == 0 {
		return nil, nil
	}
	for artifact, tokens := range rules {
		rules[artifact] = compactStrings(tokens)
	}
	return rules, nil
}

// compactStrings 会去除空白与空字符串，返回紧凑切片。
func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if normalized := strings.TrimSpace(value); normalized != "" {
			out = append(out, normalized)
		}
	}
	return out
}

// resolvePathWithinWorkdir 将 verifier 输入路径解析为绝对路径，并确保路径仍位于 workdir 内部。
func resolvePathWithinWorkdir(workdir string, rawPath string) (string, error) {
	normalizedWorkdir := strings.TrimSpace(workdir)
	if normalizedWorkdir == "" {
		return "", fmt.Errorf("workdir is required")
	}
	workdirAbs, err := filepath.Abs(filepath.Clean(normalizedWorkdir))
	if err != nil {
		return "", fmt.Errorf("resolve workdir: %w", err)
	}

	normalizedPath := strings.TrimSpace(rawPath)
	if normalizedPath == "" {
		return "", fmt.Errorf("path is required")
	}
	resolvedPath := normalizedPath
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(workdirAbs, resolvedPath)
	}
	resolvedPath, err = filepath.Abs(filepath.Clean(resolvedPath))
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	if err := ensurePathWithinBase(workdirAbs, resolvedPath); err != nil {
		return "", fmt.Errorf("path %q is outside workdir", rawPath)
	}

	needsRealpathCheck, err := pathContainsSymlink(workdirAbs, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("inspect path symlinks: %w", err)
	}
	if !needsRealpathCheck {
		return resolvedPath, nil
	}

	workdirReal, err := filepath.EvalSymlinks(workdirAbs)
	if err != nil {
		workdirReal = workdirAbs
	}
	resolvedReal, err := filepath.EvalSymlinks(resolvedPath)
	switch {
	case err == nil:
		if err := ensurePathWithinBase(workdirReal, resolvedReal); err != nil {
			return "", fmt.Errorf("path %q resolves outside workdir", rawPath)
		}
	case os.IsNotExist(err):
	default:
		return "", fmt.Errorf("resolve symlink path: %w", err)
	}
	return resolvedPath, nil
}

// ensurePathWithinBase 校验目标路径仍位于基准目录内，避免路径穿越或边界越权。
func ensurePathWithinBase(base string, target string) error {
	normalizedBase := normalizeComparablePath(base)
	normalizedTarget := normalizeComparablePath(target)
	if normalizedBase == "" || normalizedTarget == "" {
		return fmt.Errorf("empty comparable path")
	}
	if normalizedTarget == normalizedBase {
		return nil
	}
	prefix := normalizedBase
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if !strings.HasPrefix(normalizedTarget, prefix) {
		return fmt.Errorf("outside base path")
	}
	return nil
}

// normalizeComparablePath 将路径规整为适合做“是否仍在基准目录内”判断的稳定形式。
func normalizeComparablePath(path string) string {
	normalized := filepath.Clean(strings.TrimSpace(path))
	if runtime.GOOS == "windows" {
		normalized = strings.TrimPrefix(normalized, `\\?\`)
		normalized = strings.ToLower(normalized)
	}
	return normalized
}

// pathContainsSymlink 判断从 base 到 target 的现有路径段里是否包含 symlink。
func pathContainsSymlink(base string, target string) (bool, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false, fmt.Errorf("check path relation: %w", err)
	}
	if rel == "." {
		info, err := os.Lstat(target)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return info.Mode()&os.ModeSymlink != 0, nil
	}

	current := base
	for _, segment := range strings.Split(rel, string(filepath.Separator)) {
		if segment == "" || segment == "." {
			continue
		}
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}
