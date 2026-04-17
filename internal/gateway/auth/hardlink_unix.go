//go:build !windows

package auth

import (
	"os"
	"syscall"
)

// isUnsafeCredentialHardLink 在 Unix 平台识别多硬链接文件，避免凭证被旁路引用。
func isUnsafeCredentialHardLink(fileInfo os.FileInfo) bool {
	if fileInfo == nil || fileInfo.IsDir() {
		return false
	}
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return false
	}
	return stat.Nlink > 1
}
