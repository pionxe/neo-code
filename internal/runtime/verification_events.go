package runtime

import (
	"context"
	"strings"

	"neo-code/internal/runtime/acceptgate"
	"neo-code/internal/runtime/controlplane"
	runtimeverify "neo-code/internal/runtime/verify"
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
		stageStatus := runtimeverify.VerificationPass
		if !result.Passed {
			stageStatus = runtimeverify.VerificationFail
		}
		s.emitRunScopedOptional(EventVerificationStageFinished, state, VerificationStageFinishedPayload{
			Name:       strings.TrimSpace(result.Name),
			Status:     stageStatus,
			Summary:    strings.TrimSpace(result.Reason),
			Reason:     strings.TrimSpace(result.Reason),
			ErrorClass: classifyVerificationStageErrorClass(result),
		})
	}

	errorClass := runtimeverify.ErrorClass("")
	if report.Outcome != acceptgate.OutcomeAccepted {
		errorClass = runtimeverify.ErrorClassUnknown
	}
	s.emitRunScopedOptional(EventVerificationFinished, state, VerificationFinishedPayload{
		AcceptanceStatus: string(report.Outcome),
		StopReason:       report.StopReason,
		ErrorClass:       errorClass,
	})
}

// classifyVerificationStageErrorClass 将 acceptgate 单项结果映射为 verifier 兼容错误分类。
func classifyVerificationStageErrorClass(result acceptgate.CheckResult) runtimeverify.ErrorClass {
	if result.Passed {
		return ""
	}
	reason := strings.ToLower(strings.TrimSpace(result.Reason))
	switch {
	case strings.Contains(reason, "permission"):
		return runtimeverify.ErrorClassPermissionDenied
	case strings.Contains(reason, "timeout"):
		return runtimeverify.ErrorClassTimeout
	case strings.Contains(reason, "not found"):
		return runtimeverify.ErrorClassCommandNotFound
	default:
		return runtimeverify.ErrorClassUnknown
	}
}
