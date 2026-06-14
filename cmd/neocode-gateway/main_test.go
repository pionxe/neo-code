package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMainHelpPathDoesNotExit(t *testing.T) {
	originalArgs := os.Args
	originalExit := exitGatewayProcess
	defer func() {
		os.Args = originalArgs
		exitGatewayProcess = originalExit
	}()

	var gotExitCode int
	exitGatewayProcess = func(code int) {
		gotExitCode = code
	}
	os.Args = []string{"neocode-gateway", "--help"}
	main()
	if gotExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", gotExitCode)
	}
}

func TestMainReturnsExitCodeOneOnCommandError(t *testing.T) {
	if os.Getenv("NEOCODE_GATEWAY_MAIN_HELPER") == "1" {
		os.Args = []string{"neocode-gateway", "--log-level", "trace"}
		main()
		return
	}

	command := exec.Command(os.Args[0], "-test.run=TestMainReturnsExitCodeOneOnCommandError")
	command.Env = append(os.Environ(), "NEOCODE_GATEWAY_MAIN_HELPER=1")
	var stderr bytes.Buffer
	command.Stderr = &stderr

	err := command.Run()
	if err == nil {
		t.Fatal("expected subprocess to exit with non-zero status")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), 1)
	}
	if !strings.Contains(stderr.String(), "neocode-gateway:") {
		t.Fatalf("stderr = %q, want contains %q", stderr.String(), "neocode-gateway:")
	}
}

func TestRunMainReturnsExitCoderValue(t *testing.T) {
	originalExecute := executeGatewayServer
	t.Cleanup(func() { executeGatewayServer = originalExecute })

	executeGatewayServer = func(context.Context, []string) error {
		return testGatewayExitError{message: "denied", code: 5}
	}

	var stderr strings.Builder
	exitCode := runMain(context.Background(), []string{"gateway"}, &stderr)
	if exitCode != 5 {
		t.Fatalf("exit code = %d, want 5", exitCode)
	}
	if !strings.Contains(stderr.String(), "neocode-gateway: denied") {
		t.Fatalf("stderr = %q, want gateway error", stderr.String())
	}
}

func TestRunMainReturnsZeroOnSuccess(t *testing.T) {
	originalExecute := executeGatewayServer
	t.Cleanup(func() { executeGatewayServer = originalExecute })

	var capturedArgs []string
	executeGatewayServer = func(_ context.Context, args []string) error {
		capturedArgs = append([]string(nil), args...)
		return nil
	}

	var stderr strings.Builder
	exitCode := runMain(context.Background(), []string{"--help"}, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if len(capturedArgs) != 1 || capturedArgs[0] != "--help" {
		t.Fatalf("captured args = %#v, want --help", capturedArgs)
	}
}

type testGatewayExitError struct {
	message string
	code    int
}

func (e testGatewayExitError) Error() string {
	return e.message
}

func (e testGatewayExitError) ExitCode() int {
	return e.code
}
