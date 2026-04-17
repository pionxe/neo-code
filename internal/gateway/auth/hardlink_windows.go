//go:build windows

package auth

import "os"

// isUnsafeCredentialHardLink 在 Windows 平台暂不做硬链接计数判断，保持与软链接拦截策略兼容。
func isUnsafeCredentialHardLink(_ os.FileInfo) bool {
	return false
}
