package verify

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"neo-code/internal/config"
)

func TestPolicyCommandExecutorExecuteBranches(t *testing.T) {
	t.Parallel()

	t.Run("empty command denied", func(t *testing.T) {
		t.Parallel()
		_, err := PolicyCommandExecutor{}.Execute(context.Background(), CommandExecutionRequest{Argv: nil})
		if err == nil || !errors.Is(err, ErrVerificationExecutionDenied) {
			t.Fatalf("expected ErrVerificationExecutionDenied, got %v", err)
		}
	})

	t.Run("invalid workdir returns execution error", func(t *testing.T) {
		t.Parallel()
		policy := config.StaticDefaults().Runtime.Verification.ExecutionPolicy
		result, err := PolicyCommandExecutor{}.Execute(context.Background(), CommandExecutionRequest{
			Argv:    []string{"go", "version"},
			Workdir: "/path/not/exist",
			Policy:  policy,
		})
		if err == nil || !errors.Is(err, ErrVerificationExecutionError) {
			t.Fatalf("expected ErrVerificationExecutionError, got result=%+v err=%v", result, err)
		}
	})

	t.Run("command timeout", func(t *testing.T) {
		t.Parallel()
		timeoutCommand := []string{"sh", "-c", "sleep 2"}
		allowedCommand := "sh"
		if runtime.GOOS == "windows" {
			timeoutCommand = []string{"powershell", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 2"}
			allowedCommand = "powershell"
		}
		policy := config.VerificationExecutionPolicyConfig{
			Mode:             "non_interactive",
			AllowedCommands:  []string{allowedCommand},
			DefaultTimeout:   1,
			DefaultOutputCap: 1024,
		}
		result, err := PolicyCommandExecutor{}.Execute(context.Background(), CommandExecutionRequest{
			Argv:   timeoutCommand,
			Policy: policy,
		})
		if err == nil || !errors.Is(err, ErrVerificationExecutionError) {
			t.Fatalf("expected timeout execution error, got result=%+v err=%v", result, err)
		}
		if !result.TimedOut {
			t.Fatalf("result.TimedOut = false, want true")
		}
	})

	t.Run("non-zero exit code", func(t *testing.T) {
		t.Parallel()
		failCommand := []string{"sh", "-c", "exit 7"}
		allowedCommand := "sh"
		if runtime.GOOS == "windows" {
			failCommand = []string{"go", "tool", "definitely-not-a-real-tool"}
			allowedCommand = "go"
		}
		policy := config.VerificationExecutionPolicyConfig{
			Mode:             "non_interactive",
			AllowedCommands:  []string{allowedCommand},
			DefaultTimeout:   5,
			DefaultOutputCap: 1024,
		}
		result, err := PolicyCommandExecutor{}.Execute(context.Background(), CommandExecutionRequest{
			Argv:   failCommand,
			Policy: policy,
		})
		if err != nil {
			t.Fatalf("expected nil err for exit status, got %v", err)
		}
		if result.ExitCode == 0 {
			t.Fatalf("exit code = %d, want non-zero", result.ExitCode)
		}
	})
}

func TestExecutionPolicyHelpers(t *testing.T) {
	t.Parallel()

	t.Run("isCommandAllowed branches", func(t *testing.T) {
		t.Parallel()
		policy := config.VerificationExecutionPolicyConfig{
			Mode:            "non_interactive",
			AllowedCommands: []string{"go", "git"},
			DeniedCommands:  []string{"rm"},
		}
		if ok, _ := isCommandAllowed([]string{"go", "test", "./..."}, policy); !ok {
			t.Fatalf("go should be allowed")
		}
		if ok, reason := isCommandAllowed([]string{"rm", "-rf", "."}, policy); ok || reason == "" {
			t.Fatalf("rm should be denied")
		}
		if ok, _ := isCommandAllowed([]string{"python", "-V"}, policy); ok {
			t.Fatalf("python should not pass allow list")
		}
		if ok, _ := isCommandAllowed([]string{"git"}, policy); ok {
			t.Fatalf("git without subcommand should be denied")
		}
		if ok, _ := isCommandAllowed([]string{"git", "checkout", "."}, policy); ok {
			t.Fatalf("git write subcommand should be denied")
		}
		if ok, _ := isCommandAllowed([]string{"git", "diff", "--output=../leak.txt"}, policy); ok {
			t.Fatalf("git output flag should be denied")
		}
		if ok, _ := isCommandAllowed([]string{"git", "show", "-o", "../leak.txt"}, policy); ok {
			t.Fatalf("git short output flag should be denied")
		}
		if ok, _ := isCommandAllowed([]string{"git", "diff", "-c", "core.pager=cat"}, policy); ok {
			t.Fatalf("git -c override should be denied")
		}
	})

	t.Run("basic parser helpers", func(t *testing.T) {
		t.Parallel()
		if commandHead(nil) != "" {
			t.Fatalf("commandHead blank should be empty")
		}
		if commandHead([]string{"Go", "Test"}) != "go" {
			t.Fatalf("commandHead lower-case mismatch")
		}
		if gitSubcommand([]string{"git", "status"}) != "status" {
			t.Fatalf("gitSubcommand mismatch")
		}
		if gitSubcommand([]string{"go", "test"}) != "" {
			t.Fatalf("gitSubcommand should be empty for non-git")
		}
		if denied, _ := hasDangerousGitArguments([]string{"git", "diff", "--output=tmp.txt"}); !denied {
			t.Fatalf("expected dangerous git output argument to be denied")
		}
		if denied, _ := hasDangerousGitArguments([]string{"git", "status"}); denied {
			t.Fatalf("git status should not be denied by dangerous argument checker")
		}
		set := normalizedCommandSet([]string{" Go  ", "", "GIT"})
		if _, ok := set["go"]; !ok {
			t.Fatalf("normalized set should include go")
		}
		if _, ok := set["git"]; !ok {
			t.Fatalf("normalized set should include git")
		}
		spacedSet := normalizedCommandSet([]string{"go test ./...", " git status "})
		if _, ok := spacedSet["go"]; !ok {
			t.Fatalf("normalized set should parse command head from spaced go command")
		}
		if _, ok := spacedSet["git"]; !ok {
			t.Fatalf("normalized set should parse command head from spaced git command")
		}
		if _, ok := spacedSet["go test ./..."]; ok {
			t.Fatalf("normalized set should not retain entire shell-like command as key")
		}
		if got := firstPositive(-1, 0, 9, 10); got != 9 {
			t.Fatalf("firstPositive() = %d, want 9", got)
		}
		if got := firstPositive(-1, 0); got != 0 {
			t.Fatalf("firstPositive() = %d, want 0", got)
		}
	})

	t.Run("cappedBuffer branches", func(t *testing.T) {
		t.Parallel()
		buffer := newCappedBuffer(3)
		if _, err := buffer.Write([]byte("abc")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if _, err := buffer.Write([]byte("def")); err != nil {
			t.Fatalf("Write() overflow error = %v", err)
		}
		if !buffer.Truncated() {
			t.Fatalf("buffer should be truncated")
		}
		if buffer.String() != "abc" {
			t.Fatalf("buffer content = %q, want %q", buffer.String(), "abc")
		}

		var nilBuffer *cappedBuffer
		if _, err := nilBuffer.Write([]byte("x")); err != nil {
			t.Fatalf("nil buffer Write() error = %v", err)
		}
		if nilBuffer.String() != "" {
			t.Fatalf("nil buffer String() should be empty")
		}
		if nilBuffer.Truncated() {
			t.Fatalf("nil buffer Truncated() should be false")
		}
	})
}
