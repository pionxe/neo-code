//go:build linux

package urlscheme

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterURLSchemeLinuxWritesDesktopEntryAndRegistersMimeHandler(t *testing.T) {
	homeDir := t.TempDir()
	var commandName string
	var commandArgs []string

	err := registerURLSchemeLinuxWithDeps("/usr/local/bin/neocode", linuxRegisterDeps{
		userHomeDir: func() (string, error) { return homeDir, nil },
		mkdirAll:    os.MkdirAll,
		writeFile:   os.WriteFile,
		runCommand: func(name string, args ...string) error {
			commandName = name
			commandArgs = append([]string{}, args...)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("registerURLSchemeLinuxWithDeps() error = %v", err)
	}

	desktopPath := filepath.Join(homeDir, ".local", "share", "applications", linuxDesktopFilename)
	content, readErr := os.ReadFile(desktopPath)
	if readErr != nil {
		t.Fatalf("read desktop entry: %v", readErr)
	}
	text := string(content)
	if !strings.Contains(text, `Exec="/usr/local/bin/neocode" url-dispatch --url "%u"`) {
		t.Fatal("desktop entry should contain %u placeholder in Exec")
	}
	if !strings.Contains(text, "MimeType=x-scheme-handler/neocode;") {
		t.Fatal("desktop entry should register neocode mime handler")
	}

	if commandName != "xdg-mime" {
		t.Fatalf("command name = %q, want %q", commandName, "xdg-mime")
	}
	wantArgs := []string{"default", linuxDesktopFilename, linuxMimeHandlerScheme}
	if len(commandArgs) != len(wantArgs) {
		t.Fatalf("command args = %#v, want %#v", commandArgs, wantArgs)
	}
	for index := range wantArgs {
		if commandArgs[index] != wantArgs[index] {
			t.Fatalf("command args[%d] = %q, want %q", index, commandArgs[index], wantArgs[index])
		}
	}
}

func TestRegisterURLSchemeLinuxMapsMissingXdgMimeToNotSupported(t *testing.T) {
	err := registerURLSchemeLinuxWithDeps("/usr/local/bin/neocode", linuxRegisterDeps{
		userHomeDir: func() (string, error) { return t.TempDir(), nil },
		mkdirAll:    os.MkdirAll,
		writeFile:   os.WriteFile,
		runCommand: func(string, ...string) error {
			return exec.ErrNotFound
		},
	})
	if err == nil {
		t.Fatal("expected xdg-mime command error")
	}

	var dispatchErr *DispatchError
	if !errors.As(err, &dispatchErr) {
		t.Fatalf("error type = %T, want *DispatchError", err)
	}
	if dispatchErr.Code != ErrorCodeNotSupported {
		t.Fatalf("error code = %q, want %q", dispatchErr.Code, ErrorCodeNotSupported)
	}
}
