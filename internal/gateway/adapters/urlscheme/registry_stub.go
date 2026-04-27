//go:build !windows && !darwin && !linux

package urlscheme

import "fmt"

// RegisterURLScheme 在未适配平台上返回 not_supported，保持行为稳定并避免误导性成功。
func RegisterURLScheme(executablePath string) error {
	return newDispatchError(
		ErrorCodeNotSupported,
		fmt.Sprintf("url scheme registry is not supported on this platform: %s", executablePath),
	)
}
