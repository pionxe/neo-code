package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// writeWebCommandTestFile 写入 web 命令测试所需的最小文件内容，避免各测试重复拼装目录。
func writeWebCommandTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// chdirForWebCommandTest 切换当前工作目录，并在测试结束后恢复。
func chdirForWebCommandTest(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
}

// stubResolveExecutablePath 替换可执行文件路径解析，便于覆盖发布包布局分支。
func stubResolveExecutablePath(t *testing.T, fn func() (string, error)) {
	t.Helper()
	original := resolveExecutablePath
	resolveExecutablePath = fn
	t.Cleanup(func() {
		resolveExecutablePath = original
	})
}

// stubWebCommandHooks 替换 web 命令测试中的可注入执行点，并在结束后恢复。
func stubWebCommandHooks(
	t *testing.T,
	startGateway func(context.Context, gatewayCommandOptions, string, fs.FS, func(string)) error,
	build func(string, *log.Logger) error,
	lookPath func(string) (string, error),
) {
	t.Helper()
	originalStart := webCommandStartGatewayServer
	originalBuild := webCommandBuildFrontend
	originalLookPath := webCommandLookPath
	if startGateway != nil {
		webCommandStartGatewayServer = startGateway
	}
	if build != nil {
		webCommandBuildFrontend = build
	}
	if lookPath != nil {
		webCommandLookPath = lookPath
	}
	t.Cleanup(func() {
		webCommandStartGatewayServer = originalStart
		webCommandBuildFrontend = originalBuild
		webCommandLookPath = originalLookPath
	})
}

// stubWebCommandEmbeddedAssets 替换内嵌前端资源探测逻辑，便于覆盖发布版回退分支。
func stubWebCommandEmbeddedAssets(t *testing.T, assets fs.FS, available bool) {
	t.Helper()
	original := webCommandEmbeddedAssets
	webCommandEmbeddedAssets = func() (fs.FS, bool) {
		return assets, available
	}
	t.Cleanup(func() {
		webCommandEmbeddedAssets = original
	})
}

func TestFindWebSourceDirUsesCurrentWorkdir(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)
	stubResolveExecutablePath(t, func() (string, error) {
		return "", errors.New("skip executable lookup")
	})

	writeWebCommandTestFile(t, filepath.Join(tempDir, "web", "package.json"), "{}")

	got := findWebSourceDir()
	want := filepath.Join(tempDir, "web")
	if got != want {
		t.Fatalf("findWebSourceDir() = %q, want %q", got, want)
	}
}

func TestFindWebSourceDirFallsBackToExecutableDir(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)

	releaseDir := filepath.Join(tempDir, "release")
	writeWebCommandTestFile(t, filepath.Join(releaseDir, "web", "package.json"), "{}")
	stubResolveExecutablePath(t, func() (string, error) {
		return filepath.Join(releaseDir, "neocode.exe"), nil
	})

	got := findWebSourceDir()
	want := filepath.Join(releaseDir, "web")
	if got != want {
		t.Fatalf("findWebSourceDir() = %q, want %q", got, want)
	}
}

func TestResolveWebStaticDirFallsBackToExecutableDir(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)

	releaseDir := filepath.Join(tempDir, "release")
	writeWebCommandTestFile(t, filepath.Join(releaseDir, "web", "dist", "index.html"), "<html></html>")
	stubResolveExecutablePath(t, func() (string, error) {
		return filepath.Join(releaseDir, "neocode.exe"), nil
	})

	got, err := resolveWebStaticDir("")
	if err != nil {
		t.Fatalf("resolveWebStaticDir returned error: %v", err)
	}
	want := filepath.Join(releaseDir, "web", "dist")
	if got != want {
		t.Fatalf("resolveWebStaticDir() = %q, want %q", got, want)
	}
}

func TestFindNPMBinaryMissingMessage(t *testing.T) {
	stubWebCommandHooks(t, nil, nil, func(string) (string, error) {
		return "", errors.New("not found")
	})

	_, err := findNPMBinary()
	if err == nil {
		t.Fatal("findNPMBinary() error = nil, want error")
	}
	message := err.Error()
	if !strings.Contains(message, "Node.js and npm") {
		t.Fatalf("findNPMBinary() error = %q, want Node.js/npm guidance", message)
	}
	if !strings.Contains(message, "`neocode web`") {
		t.Fatalf("findNPMBinary() error = %q, want neocode web guidance", message)
	}
}

func TestRunWebCommandBuildsFrontendWhenDistMissing(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)
	writeWebCommandTestFile(t, filepath.Join(tempDir, "web", "package.json"), "{}")

	buildCalled := false
	var capturedStaticDir string
	sentinelErr := errors.New("stop after start")
	stubWebCommandHooks(
		t,
		func(_ context.Context, _ gatewayCommandOptions, staticDir string, _ fs.FS, _ func(string)) error {
			capturedStaticDir = staticDir
			return sentinelErr
		},
		func(webDir string, _ *log.Logger) error {
			buildCalled = true
			writeWebCommandTestFile(t, filepath.Join(webDir, "dist", "index.html"), "<html></html>")
			return nil
		},
		nil,
	)

	err := runWebCommand(context.Background(), webCommandOptions{
		HTTPAddress: "127.0.0.1:8080",
		LogLevel:    "info",
		OpenBrowser: false,
		Workdir:     tempDir,
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("runWebCommand() error = %v, want sentinel error %v", err, sentinelErr)
	}
	if !buildCalled {
		t.Fatal("runWebCommand() did not invoke frontend build when dist was missing")
	}
	wantStaticDir := filepath.Join(tempDir, "web", "dist")
	if capturedStaticDir != wantStaticDir {
		t.Fatalf("startGatewayServer staticDir = %q, want %q", capturedStaticDir, wantStaticDir)
	}
}

func TestRunWebCommandUsesEmbeddedAssetsWhenDistMissing(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)
	writeWebCommandTestFile(t, filepath.Join(tempDir, "web", "package.json"), "{}")

	buildCalled := false
	var capturedStaticDir string
	var capturedStaticFS fs.FS
	sentinelErr := errors.New("stop after start")
	stubWebCommandEmbeddedAssets(t, fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	}, true)
	stubWebCommandHooks(
		t,
		func(_ context.Context, _ gatewayCommandOptions, staticDir string, staticFS fs.FS, _ func(string)) error {
			capturedStaticDir = staticDir
			capturedStaticFS = staticFS
			return sentinelErr
		},
		func(_ string, _ *log.Logger) error {
			buildCalled = true
			return nil
		},
		nil,
	)

	err := runWebCommand(context.Background(), webCommandOptions{
		HTTPAddress: "127.0.0.1:8080",
		LogLevel:    "info",
		OpenBrowser: false,
		Workdir:     tempDir,
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("runWebCommand() error = %v, want sentinel error %v", err, sentinelErr)
	}
	if buildCalled {
		t.Fatal("runWebCommand() unexpectedly invoked frontend build when embedded assets were available")
	}
	if capturedStaticDir != "" {
		t.Fatalf("startGatewayServer staticDir = %q, want empty string", capturedStaticDir)
	}
	if capturedStaticFS == nil {
		t.Fatal("startGatewayServer staticFS = nil, want embedded assets FS")
	}
}

func TestRunWebCommandSkipBuildStillUsesEmbeddedAssets(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)

	buildCalled := false
	var capturedStaticFS fs.FS
	sentinelErr := errors.New("stop after start")
	stubWebCommandEmbeddedAssets(t, fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html></html>")},
	}, true)
	stubWebCommandHooks(
		t,
		func(_ context.Context, _ gatewayCommandOptions, _ string, staticFS fs.FS, _ func(string)) error {
			capturedStaticFS = staticFS
			return sentinelErr
		},
		func(_ string, _ *log.Logger) error {
			buildCalled = true
			return nil
		},
		nil,
	)

	err := runWebCommand(context.Background(), webCommandOptions{
		HTTPAddress: "127.0.0.1:8080",
		LogLevel:    "info",
		OpenBrowser: false,
		SkipBuild:   true,
		Workdir:     tempDir,
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("runWebCommand() error = %v, want sentinel error %v", err, sentinelErr)
	}
	if buildCalled {
		t.Fatal("runWebCommand() unexpectedly invoked frontend build when --skip-build used with embedded assets")
	}
	if capturedStaticFS == nil {
		t.Fatal("startGatewayServer staticFS = nil, want embedded assets FS")
	}
}

func TestValidateStaticDirAndResolveOverride(t *testing.T) {
	tempDir := t.TempDir()
	staticDir := filepath.Join(tempDir, "dist")
	if _, err := validateStaticDir(staticDir); err == nil {
		t.Fatal("validateStaticDir() error = nil, want missing index.html error")
	}

	writeWebCommandTestFile(t, filepath.Join(staticDir, "index.html"), "<html></html>")
	got, err := resolveWebStaticDir(staticDir)
	if err != nil {
		t.Fatalf("resolveWebStaticDir() error = %v", err)
	}
	if got != staticDir {
		t.Fatalf("resolveWebStaticDir() = %q, want %q", got, staticDir)
	}

	dirIndex := filepath.Join(tempDir, "bad-dist", "index.html")
	if err := os.MkdirAll(dirIndex, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dirIndex, err)
	}
	if _, err := validateStaticDir(filepath.Dir(dirIndex)); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Fatalf("validateStaticDir() error = %v, want directory error", err)
	}
}

func TestIsStaleFrontendBuildBranches(t *testing.T) {
	tempDir := t.TempDir()
	webDir := filepath.Join(tempDir, "web")
	srcDir := filepath.Join(webDir, "src")
	distIndex := filepath.Join(webDir, "dist", "index.html")

	if !isStaleFrontendBuild(webDir) {
		t.Fatal("isStaleFrontendBuild() = false, want true when dist is missing")
	}

	writeWebCommandTestFile(t, distIndex, "<html></html>")
	writeWebCommandTestFile(t, filepath.Join(webDir, "package.json"), "{}")
	writeWebCommandTestFile(t, filepath.Join(webDir, "vite.config.ts"), "export default {}")
	writeWebCommandTestFile(t, filepath.Join(webDir, "tsconfig.json"), "{}")
	writeWebCommandTestFile(t, filepath.Join(srcDir, "main.ts"), "console.log('ok')")

	distTime := time.Now()
	if err := os.Chtimes(distIndex, distTime, distTime); err != nil {
		t.Fatalf("chtimes dist: %v", err)
	}
	olderTime := distTime.Add(-time.Minute)
	for _, path := range []string{
		filepath.Join(webDir, "package.json"),
		filepath.Join(webDir, "vite.config.ts"),
		filepath.Join(webDir, "tsconfig.json"),
		filepath.Join(srcDir, "main.ts"),
	} {
		if err := os.Chtimes(path, olderTime, olderTime); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}

	if isStaleFrontendBuild(webDir) {
		t.Fatal("isStaleFrontendBuild() = true, want false when dist is newest")
	}

	newerTime := distTime.Add(time.Minute)
	packageJSON := filepath.Join(webDir, "package.json")
	if err := os.Chtimes(packageJSON, newerTime, newerTime); err != nil {
		t.Fatalf("chtimes package.json: %v", err)
	}
	if !isStaleFrontendBuild(webDir) {
		t.Fatal("isStaleFrontendBuild() = false, want true when package.json is newer")
	}

	if err := os.Chtimes(packageJSON, olderTime, olderTime); err != nil {
		t.Fatalf("restore package.json time: %v", err)
	}
	srcFile := filepath.Join(srcDir, "main.ts")
	if err := os.Chtimes(srcFile, newerTime, newerTime); err != nil {
		t.Fatalf("chtimes src: %v", err)
	}
	if !isStaleFrontendBuild(webDir) {
		t.Fatal("isStaleFrontendBuild() = false, want true when src file is newer")
	}
}

func TestBuildFrontendAndReadGatewayToken(t *testing.T) {
	tempDir := t.TempDir()
	webDir := filepath.Join(tempDir, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatalf("mkdir webdir: %v", err)
	}

	npmPath := filepath.Join(tempDir, "npm")
	script := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"if [ \"$1\" = \"install\" ]; then",
		"  exit 0",
		"fi",
		"if [ \"$1\" = \"run\" ] && [ \"$2\" = \"build\" ]; then",
		"  mkdir -p \"$PWD/dist\"",
		"  printf '<html></html>' > \"$PWD/dist/index.html\"",
		"  exit 0",
		"fi",
		"exit 1",
	}, "\n")
	if err := os.WriteFile(npmPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write npm stub: %v", err)
	}
	stubWebCommandHooks(t, nil, nil, func(string) (string, error) {
		return npmPath, nil
	})

	logger := log.New(&bytes.Buffer{}, "", 0)
	if err := buildFrontend(webDir, logger); err != nil {
		t.Fatalf("buildFrontend() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(webDir, "dist", "index.html")); err != nil {
		t.Fatalf("built dist/index.html missing: %v", err)
	}

	homeDir := filepath.Join(tempDir, "home")
	authDir := filepath.Join(homeDir, ".neocode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	authData, err := json.Marshal(map[string]string{"token": "  secret-token  "})
	if err != nil {
		t.Fatalf("marshal auth data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), authData, 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	originalHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	if got := readGatewayToken(); got != "secret-token" {
		t.Fatalf("readGatewayToken() = %q, want %q", got, "secret-token")
	}
}

func TestWaitForGatewayAndOpenBrowserAndResolveListenAddress(t *testing.T) {
	tempDir := t.TempDir()
	homeDir := filepath.Join(tempDir, "home")
	authDir := filepath.Join(homeDir, ".neocode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	authData, err := json.Marshal(map[string]string{"token": "token-123"})
	if err != nil {
		t.Fatalf("marshal auth data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), authData, 0o644); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	originalHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}
	openLog := filepath.Join(tempDir, "opened-url.txt")
	scriptPath := filepath.Join(binDir, "xdg-open")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > \"" + openLog + "\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write xdg-open stub: %v", err)
	}
	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := log.New(&bytes.Buffer{}, "", 0)
	waitForGatewayAndOpenBrowser(context.Background(), strings.TrimPrefix(server.URL, "http://"), logger)

	var data []byte
	var readErr error
	for i := 0; i < 20; i++ {
		data, readErr = os.ReadFile(openLog)
		if readErr == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if readErr != nil {
		t.Fatalf("read open log: %v", readErr)
	}
	if got := string(data); got != server.URL+"/?token=token-123" {
		t.Fatalf("opened url = %q, want %q", got, server.URL+"/?token=token-123")
	}

	if got := resolveWebListenAddress("bad-address"); got != "bad-address" {
		t.Fatalf("resolveWebListenAddress() = %q, want original invalid address", got)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	occupied := listener.Addr().String()
	resolved := resolveWebListenAddress(occupied)
	if resolved == occupied {
		t.Fatalf("resolveWebListenAddress() = %q, want fallback port", resolved)
	}
}

func TestResolveWebStaticDirCurrentWorkdirAndReadGatewayTokenInvalid(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)
	writeWebCommandTestFile(t, filepath.Join(tempDir, "web", "dist", "index.html"), "<html></html>")
	stubResolveExecutablePath(t, func() (string, error) {
		return "", errors.New("skip executable lookup")
	})

	got, err := resolveWebStaticDir("")
	if err != nil {
		t.Fatalf("resolveWebStaticDir() error = %v", err)
	}
	if got != filepath.Join(tempDir, "web", "dist") {
		t.Fatalf("resolveWebStaticDir() = %q, want cwd web/dist", got)
	}

	homeDir := filepath.Join(tempDir, "home")
	authDir := filepath.Join(homeDir, ".neocode")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write invalid auth.json: %v", err)
	}
	originalHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("set HOME: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("HOME", originalHome)
	})
	if got := readGatewayToken(); got != "" {
		t.Fatalf("readGatewayToken() = %q, want empty on invalid json", got)
	}
}

func TestRunWebCommandFallbackAndSkipBuildErrors(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)
	stubResolveExecutablePath(t, func() (string, error) {
		return "", errors.New("skip executable lookup")
	})
	stubWebCommandEmbeddedAssets(t, nil, false)

	err := runWebCommand(context.Background(), webCommandOptions{
		HTTPAddress: "127.0.0.1:8080",
		LogLevel:    "info",
		SkipBuild:   true,
		OpenBrowser: false,
		Workdir:     tempDir,
	})
	if err == nil || !strings.Contains(err.Error(), "--skip-build is set") {
		t.Fatalf("runWebCommand() error = %v, want skip-build missing assets error", err)
	}

	writeWebCommandTestFile(t, filepath.Join(tempDir, "web", "package.json"), "{}")
	err = runWebCommand(context.Background(), webCommandOptions{
		HTTPAddress: "127.0.0.1:8080",
		LogLevel:    "info",
		OpenBrowser: false,
		Workdir:     tempDir,
	})
	if err == nil || !strings.Contains(err.Error(), "frontend build failed on this machine") {
		t.Fatalf("runWebCommand() error = %v, want build failure error", err)
	}
}

func TestRunWebCommandRebuildsStaleDistAndDefaultsWorkdir(t *testing.T) {
	tempDir := t.TempDir()
	chdirForWebCommandTest(t, tempDir)
	webDir := filepath.Join(tempDir, "web")
	writeWebCommandTestFile(t, filepath.Join(webDir, "package.json"), "{}")
	writeWebCommandTestFile(t, filepath.Join(webDir, "vite.config.ts"), "export default {}")
	writeWebCommandTestFile(t, filepath.Join(webDir, "tsconfig.json"), "{}")
	writeWebCommandTestFile(t, filepath.Join(webDir, "src", "main.ts"), "console.log('stale')")
	writeWebCommandTestFile(t, filepath.Join(webDir, "dist", "index.html"), "<html></html>")

	distIndex := filepath.Join(webDir, "dist", "index.html")
	oldTime := time.Now().Add(-time.Hour)
	newTime := time.Now()
	if err := os.Chtimes(distIndex, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes dist: %v", err)
	}
	for _, path := range []string{
		filepath.Join(webDir, "package.json"),
		filepath.Join(webDir, "vite.config.ts"),
		filepath.Join(webDir, "tsconfig.json"),
		filepath.Join(webDir, "src", "main.ts"),
	} {
		if err := os.Chtimes(path, newTime, newTime); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}

	buildCalled := false
	var captured gatewayCommandOptions
	sentinelErr := errors.New("stop after start")
	stubWebCommandHooks(
		t,
		func(_ context.Context, options gatewayCommandOptions, _ string, _ fs.FS, _ func(string)) error {
			captured = options
			return sentinelErr
		},
		func(webDir string, _ *log.Logger) error {
			buildCalled = true
			writeWebCommandTestFile(t, filepath.Join(webDir, "dist", "index.html"), "<html></html>")
			return nil
		},
		nil,
	)

	err := runWebCommand(context.Background(), webCommandOptions{
		HTTPAddress: "127.0.0.1:8080",
		LogLevel:    "info",
		OpenBrowser: false,
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("runWebCommand() error = %v, want sentinel error", err)
	}
	if !buildCalled {
		t.Fatal("runWebCommand() did not rebuild stale frontend dist")
	}
	if captured.Workdir != tempDir {
		t.Fatalf("gateway workdir = %q, want cwd %q", captured.Workdir, tempDir)
	}
}
