//go:build windows

package urlscheme

import (
	"errors"
	"testing"
)

type fakeWindowsRegistryKey struct {
	values map[string]string
	closed bool
}

func (f *fakeWindowsRegistryKey) SetStringValue(name string, value string) error {
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[name] = value
	return nil
}

func (f *fakeWindowsRegistryKey) Close() error {
	f.closed = true
	return nil
}

func TestRegisterURLSchemeWindowsRejectsInvalidExecutablePath(t *testing.T) {
	called := false
	err := registerURLSchemeWindowsWithDeps("", windowsRegistryDeps{
		createKey: func(string) (windowsRegistryKey, error) {
			called = true
			return &fakeWindowsRegistryKey{}, nil
		},
	})
	if err == nil {
		t.Fatal("expected invalid executable path error")
	}
	if called {
		t.Fatal("registry writes should not run when executable path is invalid")
	}

	var dispatchErr *DispatchError
	if !errors.As(err, &dispatchErr) {
		t.Fatalf("error type = %T, want *DispatchError", err)
	}
	if dispatchErr.Code != ErrorCodeInternal {
		t.Fatalf("error code = %q, want %q", dispatchErr.Code, ErrorCodeInternal)
	}
}

func TestRegisterURLSchemeWindowsWritesExpectedRegistryValues(t *testing.T) {
	keys := map[string]*fakeWindowsRegistryKey{
		windowsURLSchemeRegistryPath:    {},
		windowsURLSchemeOpenCommandPath: {},
	}

	err := registerURLSchemeWindowsWithDeps(`C:\NeoCode\neocode.exe`, windowsRegistryDeps{
		createKey: func(path string) (windowsRegistryKey, error) {
			key, exists := keys[path]
			if !exists {
				return nil, errors.New("unexpected key path: " + path)
			}
			return key, nil
		},
	})
	if err != nil {
		t.Fatalf("registerURLSchemeWindowsWithDeps() error = %v", err)
	}

	rootKey := keys[windowsURLSchemeRegistryPath]
	if rootKey.values[""] != "URL:NeoCode Protocol" {
		t.Fatalf("root default value = %q, want %q", rootKey.values[""], "URL:NeoCode Protocol")
	}
	if rootKey.values["URL Protocol"] != "" {
		t.Fatalf("root URL Protocol = %q, want empty", rootKey.values["URL Protocol"])
	}
	if !rootKey.closed {
		t.Fatal("root key should be closed")
	}

	commandKey := keys[windowsURLSchemeOpenCommandPath]
	wantCommand := `"C:\NeoCode\neocode.exe" url-dispatch --url "%1"`
	if commandKey.values[""] != wantCommand {
		t.Fatalf("command value = %q, want %q", commandKey.values[""], wantCommand)
	}
	if !commandKey.closed {
		t.Fatal("command key should be closed")
	}
}
