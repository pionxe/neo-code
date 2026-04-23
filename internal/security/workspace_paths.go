package security

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveWorkspacePath 按 workspace sandbox 的既有语义解析并校验工作区路径。
func ResolveWorkspacePath(root string, target string) (string, string, error) {
	trimmedRoot := strings.TrimSpace(root)
	if trimmedRoot == "" {
		return "", "", errors.New("security: workspace root is empty")
	}

	absoluteRoot, err := filepath.Abs(trimmedRoot)
	if err != nil {
		return "", "", fmt.Errorf("security: resolve workspace root: %w", err)
	}

	canonicalRoot, _, err := resolveCanonicalWorkspaceRoot(cleanedPathKey(absoluteRoot))
	if err != nil {
		return "", "", err
	}

	absoluteTarget, err := absoluteWorkspaceTarget(canonicalRoot, target)
	if err != nil {
		return "", "", err
	}
	if !isWithinWorkspace(canonicalRoot, absoluteTarget) {
		return "", "", fmt.Errorf("security: path %q escapes workspace root", target)
	}

	if _, err := ensureNoSymlinkEscape(canonicalRoot, absoluteTarget, target); err != nil {
		return "", "", err
	}
	return canonicalRoot, absoluteTarget, nil
}
