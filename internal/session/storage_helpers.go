package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ensurePathWithinBase 校验目标路径位于指定基目录内，避免路径越界。
func ensurePathWithinBase(baseDir string, target string) error {
	baseResolved, err := resolvePathForContainment(baseDir)
	if err != nil {
		return fmt.Errorf("resolve base dir %q: %w", baseDir, err)
	}
	targetResolved, err := resolvePathForContainment(target)
	if err != nil {
		return fmt.Errorf("resolve target path %q: %w", target, err)
	}
	rel, err := filepath.Rel(baseResolved, targetResolved)
	if err != nil {
		return fmt.Errorf("compute relative path %q -> %q: %w", baseResolved, targetResolved, err)
	}
	if rel == "." {
		return nil
	}
	if !filepath.IsLocal(rel) {
		return fmt.Errorf("target path %q escapes base dir %q", targetResolved, baseResolved)
	}
	return nil
}

// resolvePathForContainment 将路径归一化为绝对路径并解析软链接，确保包含性校验基于真实路径。
func resolvePathForContainment(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return resolved, nil
	}
	if errors.Is(err, os.ErrPermission) {
		return "", fmt.Errorf("eval symlinks: %w", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("eval symlinks: %w", err)
	}
	parent := filepath.Dir(absPath)
	resolvedParent, parentErr := filepath.EvalSymlinks(parent)
	if parentErr == nil {
		return filepath.Join(resolvedParent, filepath.Base(absPath)), nil
	}
	if errors.Is(parentErr, os.ErrPermission) {
		return "", fmt.Errorf("eval parent symlinks: %w", parentErr)
	}
	return "", fmt.Errorf("eval parent symlinks: %w", parentErr)
}

// createTempFile 在目标目录中创建唯一临时文件。
func createTempFile(dir string, pattern string, op string) (*os.File, string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, "", fmt.Errorf("session: %s: %w", op, err)
	}
	if err := ensurePathWithinBase(dir, file.Name()); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, "", fmt.Errorf("session: %s: %w", op, err)
	}
	return file, file.Name(), nil
}

// replaceFileWithTemp 使用原子重命名替换目标文件。
func replaceFileWithTemp(tempPath string, target string, label string) error {
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("session: replace %s: %w", label, err)
	}
	if err := os.Rename(tempPath, target); err != nil {
		return fmt.Errorf("session: commit %s: %w", label, err)
	}
	return nil
}
