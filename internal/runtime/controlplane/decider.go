package controlplane

import (
	"context"
	"errors"
	"strings"
)

// StopInput 汇总停止决议所需的信号（可多信号并存，由 DecideStopReason 按优先级表决）。
type StopInput struct {
	ContextCanceled bool
	RunError        error
	Success         bool
}

// DecideStopReason 按固定优先级返回唯一 StopReason：取消 > 错误 > 成功。
func DecideStopReason(in StopInput) (StopReason, string) {
	if in.ContextCanceled {
		return StopReasonCanceled, ""
	}
	if in.RunError != nil {
		if errors.Is(in.RunError, context.Canceled) {
			return StopReasonCanceled, ""
		}
		return StopReasonError, strings.TrimSpace(in.RunError.Error())
	}
	if in.Success {
		return StopReasonSuccess, ""
	}
	return StopReasonError, "runtime: stop reason undetermined"
}
