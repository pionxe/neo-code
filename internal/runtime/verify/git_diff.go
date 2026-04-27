package verify

import (
	"context"
	"strings"
)

const gitDiffVerifierName = "git_diff"

// GitDiffVerifier 校验工作区是否存在真实交付证据。
type GitDiffVerifier struct {
	Executor CommandExecutor
}

// Name 返回 verifier 名称。
func (v GitDiffVerifier) Name() string {
	return gitDiffVerifierName
}

// VerifyFinal 执行 git status 检查，确保 edit/fix/refactor 任务有真实改动。
func (v GitDiffVerifier) VerifyFinal(ctx context.Context, input FinalVerifyInput) (VerificationResult, error) {
	executor := v.Executor
	if executor == nil {
		executor = PolicyCommandExecutor{}
	}
	cfg := input.VerificationConfig.Verifiers[gitDiffVerifierName]
	argv := compactStrings(cfg.Command)
	if len(argv) == 0 {
		argv = []string{"git", "status", "--porcelain", "--untracked-files=normal"}
	}

	result, err := executor.Execute(ctx, CommandExecutionRequest{
		Argv:          argv,
		Workdir:       input.Workdir,
		TimeoutSec:    cfg.TimeoutSec,
		OutputCapByte: cfg.OutputCapBytes,
		Policy:        input.VerificationConfig.ExecutionPolicy,
	})
	if err != nil {
		return VerificationResult{
			Name:       gitDiffVerifierName,
			Status:     VerificationFail,
			Summary:    err.Error(),
			Reason:     "git status command execution failed",
			ErrorClass: classifyCommandExecutionError(err),
			Evidence:   commandEvidence(argv, result),
		}, nil
	}
	if result.ExitCode != 0 {
		return VerificationResult{
			Name:       gitDiffVerifierName,
			Status:     VerificationFail,
			Summary:    "git status command returned non-zero",
			Reason:     "git status command failed",
			ErrorClass: ErrorClassUnknown,
			Evidence:   commandEvidence(argv, result),
		}, nil
	}

	lines := nonEmptyLines(result.Stdout)
	if len(lines) == 0 {
		return VerificationResult{
			Name:     gitDiffVerifierName,
			Status:   VerificationSoftBlock,
			Summary:  "git status is empty",
			Reason:   "no changed files detected",
			Evidence: commandEvidence(argv, result),
		}, nil
	}
	evidence := commandEvidence(argv, result)
	evidence["changed_files"] = lines
	evidence["changed_files_count"] = len(lines)
	return VerificationResult{
		Name:     gitDiffVerifierName,
		Status:   VerificationPass,
		Summary:  "git status contains changed files",
		Reason:   "workspace change detected",
		Evidence: evidence,
	}, nil
}

// nonEmptyLines 返回文本中的非空行列表。
func nonEmptyLines(text string) []string {
	raw := strings.Split(text, "\n")
	lines := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		lines = append(lines, item)
	}
	return lines
}
