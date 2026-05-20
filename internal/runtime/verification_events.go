package runtime

import (
	"context"
	"strings"

	"neo-code/internal/runtime/acceptgate"
	"neo-code/internal/runtime/controlplane"
)

// emitVerificationLifecycleEvents 发出 verification 开始、分阶段与结束事件，补齐可观测闭环。
func (s *Service) emitVerificationLifecycleEvents(
	ctx context.Context,
	state *runState,
	completionState controlplane.CompletionState,
	report acceptgate.Report,
) {
	if s == nil || state == nil {
		return
	}
	completionPassed := completionState.CompletionBlockedReason == controlplane.CompletionBlockedReasonNone
	s.emitRunScopedOptional(EventVerificationStarted, state, VerificationStartedPayload{
		CompletionPassed:        completionPassed,
		CompletionBlockedReason: string(completionState.CompletionBlockedReason),
	})

	for _, result := range report.Results {
		stageStatus := "pass"
		if !result.Passed {
			stageStatus = "fail"
		}
		s.emitRunScopedOptional(EventVerificationStageFinished, state, VerificationStageFinishedPayload{
			Name:       strings.TrimSpace(result.Name),
			Status:     stageStatus,
			Summary:    strings.TrimSpace(result.Reason),
			Reason:     strings.TrimSpace(result.Reason),
			ErrorClass: classifyVerificationStageErrorClass(result),
		})
	}

	errorClass := ""
	if report.Outcome != acceptgate.OutcomeAccepted {
		errorClass = "unknown"
	}
	s.emitRunScopedOptional(EventVerificationFinished, state, VerificationFinishedPayload{
		AcceptanceStatus: string(report.Outcome),
		StopReason:       report.StopReason,
		ErrorClass:       errorClass,
	})
}

// classifyVerificationStageErrorClass 将 acceptgate 单项结果映射为 verifier 兼容错误分类。
func classifyVerificationStageErrorClass(result acceptgate.CheckResult) string {
	if result.Passed {
		return ""
	}
	reason := strings.ToLower(strings.TrimSpace(result.Reason))
	switch {
	case strings.Contains(reason, "permission"):
		return "permission_denied"
	case strings.Contains(reason, "timeout"):
		return "timeout"
	case strings.Contains(reason, "not found"):
		return "command_not_found"
	default:
		return "unknown"
	}
}
