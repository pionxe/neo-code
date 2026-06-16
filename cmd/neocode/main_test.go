package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type testExitError struct {
	message string
	code    int
}

func (e testExitError) Error() string {
	return e.message
}

func (e testExitError) ExitCode() int {
	return e.code
}

func TestRunMainReturnsExitCodeFromCLI(t *testing.T) {
	originalExecute := executeCLI
	originalConsume := consumeCLIUpdateNotice
	t.Cleanup(func() {
		executeCLI = originalExecute
		consumeCLIUpdateNotice = originalConsume
	})

	executeCLI = func(context.Context) error {
		return testExitError{message: "boom", code: 7}
	}
	consumeCLIUpdateNotice = func() string { return "" }

	var stdout strings.Builder
	var stderr strings.Builder
	exitCode := runMain(context.Background(), &stdout, &stderr)
	if exitCode != 7 {
		t.Fatalf("exitCode = %d, want 7", exitCode)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "neocode: boom") {
		t.Fatalf("stderr = %q, want error output", stderr.String())
	}
}

func TestMainInvokesExitProcessWithRunMainResult(t *testing.T) {
	originalExecute := executeCLI
	originalConsume := consumeCLIUpdateNotice
	originalExit := exitProcess
	t.Cleanup(func() {
		executeCLI = originalExecute
		consumeCLIUpdateNotice = originalConsume
		exitProcess = originalExit
	})

	executeCLI = func(context.Context) error {
		return testExitError{message: "boom", code: 6}
	}
	consumeCLIUpdateNotice = func() string { return "" }
	var gotExitCode int
	exitProcess = func(code int) {
		gotExitCode = code
	}

	main()
	if gotExitCode != 6 {
		t.Fatalf("exit code = %d, want 6", gotExitCode)
	}
}

func TestRunMainPrintsUpdateNoticeOnSuccess(t *testing.T) {
	originalExecute := executeCLI
	originalConsume := consumeCLIUpdateNotice
	t.Cleanup(func() {
		executeCLI = originalExecute
		consumeCLIUpdateNotice = originalConsume
	})

	executeCLI = func(context.Context) error { return nil }
	consumeCLIUpdateNotice = func() string { return "update available" }

	var stdout strings.Builder
	var stderr strings.Builder
	exitCode := runMain(context.Background(), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout.String(), "update available") {
		t.Fatalf("stdout = %q, want update notice", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunMainFallsBackToExitCodeOne(t *testing.T) {
	originalExecute := executeCLI
	originalConsume := consumeCLIUpdateNotice
	t.Cleanup(func() {
		executeCLI = originalExecute
		consumeCLIUpdateNotice = originalConsume
	})

	executeCLI = func(context.Context) error { return errors.New("plain failure") }
	consumeCLIUpdateNotice = func() string { return "" }

	var stderr strings.Builder
	exitCode := runMain(context.Background(), &strings.Builder{}, &stderr)
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "plain failure") {
		t.Fatalf("stderr = %q, want plain failure", stderr.String())
	}
}
