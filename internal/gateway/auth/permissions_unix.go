//go:build !windows

package auth

import (
	"fmt"
	"os"
)

// applyAuthDirPermission 在非 Windows 平台收紧凭证目录权限为 0700。
func applyAuthDirPermission(dir string) error {
	if err := os.Chmod(dir, authDirPerm); err != nil {
		return fmt.Errorf("gateway auth: set auth dir permission: %w", err)
	}
	return nil
}

// applyAuthFilePermission 在非 Windows 平台收紧凭证文件权限为 0600。
func applyAuthFilePermission(path string) error {
	if err := os.Chmod(path, authFilePerm); err != nil {
		return fmt.Errorf("gateway auth: set auth file permission: %w", err)
	}
	return nil
}
