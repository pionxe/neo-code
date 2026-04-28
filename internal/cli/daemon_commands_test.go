package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"neo-code/internal/gateway/adapters/urlscheme"
)

func TestDaemonServeSubcommandUsesFlags(t *testing.T) {
	originalRunner := runDaemonServeCommand
	t.Cleanup(func() { runDaemonServeCommand = originalRunner })

	var captured daemonServeCommandOptions
	runDaemonServeCommand = func(_ context.Context, options daemonServeCommandOptions) error {
		captured = options
		return nil
	}

	command := NewRootCommand()
	command.SetArgs([]string{
		"daemon", "serve",
		"--listen", " 127.0.0.1:19921 ",
		"--gateway-listen", " /tmp/gateway.sock ",
	})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if captured.ListenAddress != "127.0.0.1:19921" {
		t.Fatalf("listen address = %q, want %q", captured.ListenAddress, "127.0.0.1:19921")
	}
	if captured.GatewayListenAddress != "/tmp/gateway.sock" {
		t.Fatalf("gateway listen address = %q, want %q", captured.GatewayListenAddress, "/tmp/gateway.sock")
	}
}

func TestDaemonInstallDefaultRunnerUsesCurrentExecutable(t *testing.T) {
	originalRunner := runDaemonInstallCommand
	originalResolveExecutablePath := resolveExecutablePath
	originalInstall := installHTTPDaemon
	t.Cleanup(func() { runDaemonInstallCommand = originalRunner })
	t.Cleanup(func() { resolveExecutablePath = originalResolveExecutablePath })
	t.Cleanup(func() { installHTTPDaemon = originalInstall })

	runDaemonInstallCommand = defaultDaemonInstallCommandRunner
	resolveExecutablePath = func() (string, error) {
		return "/tmp/neocode", nil
	}
	var captured urlscheme.HTTPDaemonInstallOptions
	installHTTPDaemon = func(options urlscheme.HTTPDaemonInstallOptions) (urlscheme.HTTPDaemonInstallResult, error) {
		captured = options
		return urlscheme.HTTPDaemonInstallResult{
			ListenAddress: options.ListenAddress,
			AutostartMode: "test-mode",
		}, nil
	}

	command := NewRootCommand()
	command.SetOut(os.Stdout)
	command.SetArgs([]string{"daemon", "install", "--listen", "127.0.0.1:19921"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if captured.ExecutablePath != "/tmp/neocode" {
		t.Fatalf("executable path = %q, want %q", captured.ExecutablePath, "/tmp/neocode")
	}
	if captured.ListenAddress != "127.0.0.1:19921" {
		t.Fatalf("listen address = %q, want %q", captured.ListenAddress, "127.0.0.1:19921")
	}
}

func TestDaemonServeDoesNotExposeTokenFileFlag(t *testing.T) {
	command := NewRootCommand()
	command.SetArgs([]string{"daemon", "serve", "--token-file", "/tmp/auth.json"})
	err := command.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %v, want unknown flag", err)
	}
}

func TestDaemonSubcommandSkipsGlobalPreload(t *testing.T) {
	originalPreload := runGlobalPreload
	originalRunner := runDaemonStatusCommand
	t.Cleanup(func() { runGlobalPreload = originalPreload })
	t.Cleanup(func() { runDaemonStatusCommand = originalRunner })

	var called bool
	runGlobalPreload = func(context.Context) error {
		called = true
		return errors.New("should be skipped")
	}
	runDaemonStatusCommand = func(context.Context, daemonStatusCommandOptions) error { return nil }

	command := NewRootCommand()
	command.SetArgs([]string{"daemon", "status"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if called {
		t.Fatal("expected global preload to be skipped for daemon command")
	}
}

func TestDaemonSubcommandSkipsSilentUpdateCheck(t *testing.T) {
	originalSilentCheck := runSilentUpdateCheck
	originalRunner := runDaemonStatusCommand
	t.Cleanup(func() { runSilentUpdateCheck = originalSilentCheck })
	t.Cleanup(func() { runDaemonStatusCommand = originalRunner })

	var called bool
	runSilentUpdateCheck = func(context.Context) {
		called = true
	}
	runDaemonStatusCommand = func(context.Context, daemonStatusCommandOptions) error { return nil }

	command := NewRootCommand()
	command.SetArgs([]string{"daemon", "status"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if called {
		t.Fatal("expected silent update check to be skipped for daemon command")
	}
}
