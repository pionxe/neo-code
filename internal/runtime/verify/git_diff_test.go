package verify

import (
	"context"
	"errors"
	"testing"

	"neo-code/internal/config"
)

func verifyConfigForGitDiffTests() config.VerificationConfig {
	cfg := config.StaticDefaults().Runtime.Verification
	cfg.ExecutionPolicy = config.VerificationExecutionPolicyConfig{
		Mode:             "non_interactive",
		DefaultTimeout:   1,
		DefaultOutputCap: 1,
	}
	return cfg
}

func TestGitDiffVerifier(t *testing.T) {
	t.Parallel()

	t.Run("empty output soft blocks", func(t *testing.T) {
		t.Parallel()
		cfg := verifyConfigForGitDiffTests()
		executor := &stubCommandExecutor{result: CommandExecutionResult{ExitCode: 0, Stdout: ""}}
		result, err := (GitDiffVerifier{Executor: executor}).VerifyFinal(context.Background(), FinalVerifyInput{VerificationConfig: cfg})
		if err != nil {
			t.Fatalf("VerifyFinal() error = %v", err)
		}
		if result.Status != VerificationSoftBlock {
			t.Fatalf("status = %q, want soft_block", result.Status)
		}
	})

	t.Run("changed files pass", func(t *testing.T) {
		t.Parallel()
		cfg := verifyConfigForGitDiffTests()
		executor := &stubCommandExecutor{result: CommandExecutionResult{ExitCode: 0, Stdout: "M main.go\n?? new.txt\n"}}
		result, err := (GitDiffVerifier{Executor: executor}).VerifyFinal(context.Background(), FinalVerifyInput{
			Workdir:            "/workspace",
			VerificationConfig: cfg,
		})
		if err != nil {
			t.Fatalf("VerifyFinal() error = %v", err)
		}
		if result.Status != VerificationPass {
			t.Fatalf("status = %q, want pass", result.Status)
		}
		if len(executor.requests) != 1 || executor.requests[0].Argv[1] != "status" {
			t.Fatalf("unexpected argv: %+v", executor.requests)
		}
	})

	t.Run("staged only pass", func(t *testing.T) {
		t.Parallel()
		cfg := verifyConfigForGitDiffTests()
		executor := &stubCommandExecutor{result: CommandExecutionResult{ExitCode: 0, Stdout: "A  staged.txt\n"}}
		result, err := (GitDiffVerifier{Executor: executor}).VerifyFinal(context.Background(), FinalVerifyInput{VerificationConfig: cfg})
		if err != nil {
			t.Fatalf("VerifyFinal() error = %v", err)
		}
		if result.Status != VerificationPass {
			t.Fatalf("status = %q, want pass", result.Status)
		}
	})

	t.Run("unstaged only pass", func(t *testing.T) {
		t.Parallel()
		cfg := verifyConfigForGitDiffTests()
		executor := &stubCommandExecutor{result: CommandExecutionResult{ExitCode: 0, Stdout: " M unstaged.go\n"}}
		result, err := (GitDiffVerifier{Executor: executor}).VerifyFinal(context.Background(), FinalVerifyInput{VerificationConfig: cfg})
		if err != nil {
			t.Fatalf("VerifyFinal() error = %v", err)
		}
		if result.Status != VerificationPass {
			t.Fatalf("status = %q, want pass", result.Status)
		}
	})

	t.Run("untracked only pass", func(t *testing.T) {
		t.Parallel()
		cfg := verifyConfigForGitDiffTests()
		executor := &stubCommandExecutor{result: CommandExecutionResult{ExitCode: 0, Stdout: "?? untracked.txt\n"}}
		result, err := (GitDiffVerifier{Executor: executor}).VerifyFinal(context.Background(), FinalVerifyInput{VerificationConfig: cfg})
		if err != nil {
			t.Fatalf("VerifyFinal() error = %v", err)
		}
		if result.Status != VerificationPass {
			t.Fatalf("status = %q, want pass", result.Status)
		}
	})

	t.Run("ignored only pass", func(t *testing.T) {
		t.Parallel()
		cfg := verifyConfigForGitDiffTests()
		executor := &stubCommandExecutor{result: CommandExecutionResult{ExitCode: 0, Stdout: "!! ignored.log\n"}}
		result, err := (GitDiffVerifier{Executor: executor}).VerifyFinal(context.Background(), FinalVerifyInput{VerificationConfig: cfg})
		if err != nil {
			t.Fatalf("VerifyFinal() error = %v", err)
		}
		if result.Status != VerificationPass {
			t.Fatalf("status = %q, want pass", result.Status)
		}
	})

	t.Run("execution error fails", func(t *testing.T) {
		t.Parallel()
		cfg := verifyConfigForGitDiffTests()
		executor := &stubCommandExecutor{err: errors.New("timeout")}
		result, err := (GitDiffVerifier{Executor: executor}).VerifyFinal(context.Background(), FinalVerifyInput{VerificationConfig: cfg})
		if err != nil {
			t.Fatalf("VerifyFinal() error = %v", err)
		}
		if result.Status != VerificationFail || result.ErrorClass != ErrorClassTimeout {
			t.Fatalf("unexpected result: %+v", result)
		}
	})
}
