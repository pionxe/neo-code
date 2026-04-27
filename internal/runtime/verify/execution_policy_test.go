package verify

import (
	"context"
	"errors"
	"testing"

	"neo-code/internal/config"
)

func defaultExecutionPolicy() config.VerificationExecutionPolicyConfig {
	return config.StaticDefaults().Runtime.Verification.ExecutionPolicy
}

func assertExecutionDenied(t *testing.T, argv []string) {
	t.Helper()

	_, err := PolicyCommandExecutor{}.Execute(context.Background(), CommandExecutionRequest{
		Argv:   argv,
		Policy: defaultExecutionPolicy(),
	})
	if err == nil {
		t.Fatal("expected command to be denied")
	}
	if !errors.Is(err, ErrVerificationExecutionDenied) {
		t.Fatalf("error = %v, want ErrVerificationExecutionDenied", err)
	}
}

func TestPolicyCommandExecutorAllowsWhitelistedCommand(t *testing.T) {
	t.Parallel()

	result, err := PolicyCommandExecutor{}.Execute(context.Background(), CommandExecutionRequest{
		Argv:   []string{"go", "version"},
		Policy: defaultExecutionPolicy(),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", result.ExitCode)
	}
}

func TestPolicyCommandExecutorRejectsDeniedCommand(t *testing.T) {
	t.Parallel()

	assertExecutionDenied(t, []string{"rm", "-rf", "."})
}

func TestPolicyCommandExecutorRejectsGitWriteSubcommand(t *testing.T) {
	t.Parallel()

	assertExecutionDenied(t, []string{"git", "checkout", "."})
}

func TestPolicyCommandExecutorRejectsNonWhitelistedCommand(t *testing.T) {
	t.Parallel()

	assertExecutionDenied(t, []string{"echo", "$HOME"})
}
