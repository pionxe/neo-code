//go:build linux

package urlscheme

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	linuxDesktopFilename      = "neocode-url-handler.desktop"
	linuxMimeHandlerScheme    = "x-scheme-handler/neocode"
	linuxDesktopEntryTemplate = `[Desktop Entry]
Name=NeoCode URL Handler
Type=Application
NoDisplay=true
Exec="%s" url-dispatch --url "%%u"
Terminal=false
MimeType=x-scheme-handler/neocode;
Categories=Utility;
`
)

type linuxRegisterDeps struct {
	userHomeDir func() (string, error)
	mkdirAll    func(path string, perm fs.FileMode) error
	writeFile   func(name string, data []byte, perm fs.FileMode) error
	runCommand  func(name string, args ...string) error
}

// RegisterURLScheme 在 Linux 用户目录写入 desktop entry 并通过 xdg-mime 注册 neocode:// 协议处理器。
func RegisterURLScheme(executablePath string) error {
	return registerURLSchemeLinuxWithDeps(executablePath, linuxRegisterDeps{
		userHomeDir: os.UserHomeDir,
		mkdirAll:    os.MkdirAll,
		writeFile:   os.WriteFile,
		runCommand: func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		},
	})
}

// registerURLSchemeLinuxWithDeps 负责创建 desktop entry 并刷新 MIME handler 映射，保证点击链接可唤醒 neocode。
func registerURLSchemeLinuxWithDeps(executablePath string, deps linuxRegisterDeps) error {
	normalizedExecutable, normalizeErr := normalizeURLSchemeExecutablePath(executablePath)
	if normalizeErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("invalid executable path: %v", normalizeErr))
	}
	if deps.userHomeDir == nil || deps.mkdirAll == nil || deps.writeFile == nil || deps.runCommand == nil {
		return newDispatchError(ErrorCodeInternal, "linux url scheme dependencies are incomplete")
	}

	homeDir, homeErr := deps.userHomeDir()
	if homeErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("resolve home directory: %v", homeErr))
	}
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return newDispatchError(ErrorCodeInternal, "resolve home directory: empty path")
	}

	desktopDir := filepath.Join(homeDir, ".local", "share", "applications")
	desktopPath := filepath.Join(desktopDir, linuxDesktopFilename)

	if mkdirErr := deps.mkdirAll(desktopDir, 0o755); mkdirErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("prepare linux applications directory: %v", mkdirErr))
	}
	if writeErr := deps.writeFile(desktopPath, []byte(buildLinuxDesktopEntry(normalizedExecutable)), 0o644); writeErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("write linux desktop entry: %v", writeErr))
	}

	if registerErr := deps.runCommand("xdg-mime", "default", linuxDesktopFilename, linuxMimeHandlerScheme); registerErr != nil {
		return mapURLSchemeCommandError("xdg-mime", registerErr)
	}
	return nil
}

// buildLinuxDesktopEntry 生成 xdg 桌面入口定义，固定使用 %u 接收单个 URL 入参。
func buildLinuxDesktopEntry(executablePath string) string {
	escapedExecutable := strings.ReplaceAll(executablePath, `"`, `\"`)
	return fmt.Sprintf(linuxDesktopEntryTemplate, escapedExecutable)
}
