//go:build windows

package urlscheme

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	windowsURLSchemeRegistryPath    = `Software\Classes\neocode`
	windowsURLSchemeOpenCommandPath = `Software\Classes\neocode\shell\open\command`
)

type windowsRegistryKey interface {
	SetStringValue(name string, value string) error
	Close() error
}

type windowsRegistryDeps struct {
	createKey func(path string) (windowsRegistryKey, error)
}

type windowsRegistryKeyAdapter struct {
	key registry.Key
}

func (a windowsRegistryKeyAdapter) SetStringValue(name string, value string) error {
	return a.key.SetStringValue(name, value)
}

func (a windowsRegistryKeyAdapter) Close() error {
	return a.key.Close()
}

// RegisterURLScheme 在 Windows 上将 neocode:// 绑定到当前可执行文件的 url-dispatch 子命令。
func RegisterURLScheme(executablePath string) error {
	return registerURLSchemeWindowsWithDeps(executablePath, windowsRegistryDeps{
		createKey: createWindowsRegistryKey,
	})
}

// registerURLSchemeWindowsWithDeps 负责写入协议注册表键，供系统将 neocode:// 唤醒转发到 neocode url-dispatch。
func registerURLSchemeWindowsWithDeps(executablePath string, deps windowsRegistryDeps) error {
	normalizedExecutable, normalizeErr := normalizeURLSchemeExecutablePath(executablePath)
	if normalizeErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("invalid executable path: %v", normalizeErr))
	}
	if deps.createKey == nil {
		return newDispatchError(ErrorCodeInternal, "windows registry writer is unavailable")
	}

	registryKey, keyErr := deps.createKey(windowsURLSchemeRegistryPath)
	if keyErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("open windows url scheme key: %v", keyErr))
	}
	defer func() {
		_ = registryKey.Close()
	}()

	if setErr := registryKey.SetStringValue("", "URL:NeoCode Protocol"); setErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("write windows url scheme default value: %v", setErr))
	}
	if setErr := registryKey.SetStringValue("URL Protocol", ""); setErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("write windows url protocol flag: %v", setErr))
	}

	commandKey, commandKeyErr := deps.createKey(windowsURLSchemeOpenCommandPath)
	if commandKeyErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("open windows url command key: %v", commandKeyErr))
	}
	defer func() {
		_ = commandKey.Close()
	}()

	command := fmt.Sprintf(`"%s" url-dispatch --url "%%1"`, normalizedExecutable)
	if setErr := commandKey.SetStringValue("", command); setErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("write windows url command: %v", setErr))
	}
	return nil
}

// createWindowsRegistryKey 打开（不存在则创建）当前用户范围的协议注册表键，避免管理员权限依赖。
func createWindowsRegistryKey(path string) (windowsRegistryKey, error) {
	openedKey, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return nil, err
	}
	return windowsRegistryKeyAdapter{key: openedKey}, nil
}
