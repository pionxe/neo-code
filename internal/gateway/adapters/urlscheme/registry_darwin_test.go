//go:build darwin

package urlscheme

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterURLSchemeDarwinWritesBundleAndRefreshesLaunchServices(t *testing.T) {
	homeDir := t.TempDir()
	var commandName string
	var commandArgs []string

	err := registerURLSchemeDarwinWithDeps("/Applications/NeoCode.app/Contents/MacOS/neocode", darwinRegisterDeps{
		userHomeDir: func() (string, error) { return homeDir, nil },
		mkdirAll:    os.MkdirAll,
		writeFile:   os.WriteFile,
		chmod:       os.Chmod,
		runCommand: func(name string, args ...string) error {
			commandName = name
			commandArgs = append([]string{}, args...)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("registerURLSchemeDarwinWithDeps() error = %v", err)
	}

	bundleRoot := filepath.Join(homeDir, "Applications", darwinURLHandlerBundleName)
	infoPlistPath := filepath.Join(bundleRoot, "Contents", "Info.plist")
	launcherPath := filepath.Join(bundleRoot, "Contents", "MacOS", darwinURLHandlerExecutableName)

	infoPlistContent, readInfoErr := os.ReadFile(infoPlistPath)
	if readInfoErr != nil {
		t.Fatalf("read Info.plist: %v", readInfoErr)
	}
	infoText := string(infoPlistContent)
	if !strings.Contains(infoText, "<key>CFBundleExecutable</key>") ||
		!strings.Contains(infoText, darwinURLHandlerExecutableName) {
		t.Fatalf("Info.plist should declare executable %q", darwinURLHandlerExecutableName)
	}
	if !strings.Contains(infoText, "<string>neocode</string>") {
		t.Fatal("Info.plist should include neocode scheme")
	}

	launcherContent, readLauncherErr := os.ReadFile(launcherPath)
	if readLauncherErr != nil {
		t.Fatalf("read launcher script: %v", readLauncherErr)
	}
	launcherText := string(launcherContent)
	if !strings.Contains(launcherText, "-url") || !strings.Contains(launcherText, "--url") {
		t.Fatal("launcher script should support -url and --url parameters")
	}
	if !strings.Contains(launcherText, "neocode://") {
		t.Fatal("launcher script should parse positional neocode:// arguments")
	}
	if !strings.Contains(launcherText, `url-dispatch --url "$url"`) {
		t.Fatal("launcher script should forward to url-dispatch")
	}

	launcherInfo, statErr := os.Stat(launcherPath)
	if statErr != nil {
		t.Fatalf("stat launcher script: %v", statErr)
	}
	if launcherInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("launcher script mode = %o, want executable", launcherInfo.Mode().Perm())
	}

	if commandName != darwinLSRegisterPath {
		t.Fatalf("command name = %q, want %q", commandName, darwinLSRegisterPath)
	}
	if len(commandArgs) != 2 || commandArgs[0] != "-f" || commandArgs[1] != bundleRoot {
		t.Fatalf("command args = %#v, want [-f %q]", commandArgs, bundleRoot)
	}
}

func TestRegisterURLSchemeDarwinMapsMissingLSRegisterToNotSupported(t *testing.T) {
	err := registerURLSchemeDarwinWithDeps("/Applications/NeoCode.app/Contents/MacOS/neocode", darwinRegisterDeps{
		userHomeDir: func() (string, error) { return t.TempDir(), nil },
		mkdirAll:    os.MkdirAll,
		writeFile:   os.WriteFile,
		chmod:       os.Chmod,
		runCommand: func(string, ...string) error {
			return exec.ErrNotFound
		},
	})
	if err == nil {
		t.Fatal("expected lsregister command error")
	}

	var dispatchErr *DispatchError
	if !errors.As(err, &dispatchErr) {
		t.Fatalf("error type = %T, want *DispatchError", err)
	}
	if dispatchErr.Code != ErrorCodeNotSupported {
		t.Fatalf("error code = %q, want %q", dispatchErr.Code, ErrorCodeNotSupported)
	}
}
