//go:build darwin

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
	darwinURLHandlerBundleName       = "NeoCodeURLHandler.app"
	darwinURLHandlerExecutableName   = "NeoCodeURLHandler"
	darwinURLHandlerBundleIdentifier = "io.neocode.urlhandler"
	darwinLSRegisterPath             = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

type darwinRegisterDeps struct {
	userHomeDir func() (string, error)
	mkdirAll    func(path string, perm fs.FileMode) error
	writeFile   func(name string, data []byte, perm fs.FileMode) error
	chmod       func(name string, mode fs.FileMode) error
	runCommand  func(name string, args ...string) error
}

// RegisterURLScheme 在 macOS 用户目录下创建 URL Handler bundle，并注册 neocode:// 协议关联。
func RegisterURLScheme(executablePath string) error {
	return registerURLSchemeDarwinWithDeps(executablePath, darwinRegisterDeps{
		userHomeDir: os.UserHomeDir,
		mkdirAll:    os.MkdirAll,
		writeFile:   os.WriteFile,
		chmod:       os.Chmod,
		runCommand: func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		},
	})
}

// registerURLSchemeDarwinWithDeps 负责生成并刷新 URL Handler bundle，使浏览器点击 neocode:// 后可转发到 neocode。
func registerURLSchemeDarwinWithDeps(executablePath string, deps darwinRegisterDeps) error {
	normalizedExecutable, normalizeErr := normalizeURLSchemeExecutablePath(executablePath)
	if normalizeErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("invalid executable path: %v", normalizeErr))
	}
	if deps.userHomeDir == nil || deps.mkdirAll == nil || deps.writeFile == nil || deps.chmod == nil || deps.runCommand == nil {
		return newDispatchError(ErrorCodeInternal, "darwin url scheme dependencies are incomplete")
	}

	homeDir, homeErr := deps.userHomeDir()
	if homeErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("resolve home directory: %v", homeErr))
	}
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return newDispatchError(ErrorCodeInternal, "resolve home directory: empty path")
	}

	bundleRoot := filepath.Join(homeDir, "Applications", darwinURLHandlerBundleName)
	contentsDir := filepath.Join(bundleRoot, "Contents")
	macosDir := filepath.Join(contentsDir, "MacOS")
	infoPlistPath := filepath.Join(contentsDir, "Info.plist")
	launcherPath := filepath.Join(macosDir, darwinURLHandlerExecutableName)

	if mkdirErr := deps.mkdirAll(macosDir, 0o755); mkdirErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("prepare darwin bundle directory: %v", mkdirErr))
	}
	if writeErr := deps.writeFile(infoPlistPath, []byte(buildDarwinInfoPlist()), 0o644); writeErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("write darwin Info.plist: %v", writeErr))
	}
	if writeErr := deps.writeFile(launcherPath, []byte(buildDarwinLauncherScript(normalizedExecutable)), 0o755); writeErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("write darwin launcher script: %v", writeErr))
	}
	if chmodErr := deps.chmod(launcherPath, 0o755); chmodErr != nil {
		return newDispatchError(ErrorCodeInternal, fmt.Sprintf("chmod darwin launcher script: %v", chmodErr))
	}
	if refreshErr := deps.runCommand(darwinLSRegisterPath, "-f", bundleRoot); refreshErr != nil {
		return mapURLSchemeCommandError("lsregister", refreshErr)
	}
	return nil
}

// buildDarwinInfoPlist 生成最小可用的 Info.plist，声明 neocode:// 协议由该 bundle 接管。
func buildDarwinInfoPlist() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>NeoCode URL Handler</string>
	<key>CFBundleIdentifier</key>
	<string>` + darwinURLHandlerBundleIdentifier + `</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>CFBundleShortVersionString</key>
	<string>1.0</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleExecutable</key>
	<string>` + darwinURLHandlerExecutableName + `</string>
	<key>CFBundleURLTypes</key>
	<array>
		<dict>
			<key>CFBundleURLName</key>
			<string>NeoCode URL Scheme</string>
			<key>CFBundleURLSchemes</key>
			<array>
				<string>neocode</string>
			</array>
		</dict>
	</array>
</dict>
</plist>
`
}

// buildDarwinLauncherScript 生成 bundle 可执行脚本，兼容解析 -url/--url 与位置参数 URL。
func buildDarwinLauncherScript(executablePath string) string {
	return `#!/bin/sh
set -eu

NEOCODE_BIN="` + escapeDoubleQuotedShellLiteral(executablePath) + `"
url=""
expect_url_arg="0"

for arg in "$@"; do
	if [ "$expect_url_arg" = "1" ]; then
		url="$arg"
		break
	fi
	case "$arg" in
		-url|--url)
			expect_url_arg="1"
			;;
		neocode://*)
			url="$arg"
			break
			;;
	esac
done

if [ -z "$url" ]; then
	exit 0
fi

exec "$NEOCODE_BIN" url-dispatch --url "$url"
`
}

// escapeDoubleQuotedShellLiteral 转义双引号 shell 字符串中的反斜杠与引号，避免路径截断。
func escapeDoubleQuotedShellLiteral(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(escaped, `"`, `\"`)
}
