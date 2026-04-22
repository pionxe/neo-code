package controlplane

import (
	"context"
	"errors"
	"strings"
)

// StopInput 汇总最终 stop 决议所需的信号。
type StopInput struct {
	UserInterrupted bool
	FatalError      error
	Completed       bool
}

// DecideStopReason 按固定优先级返回唯一的最终 stop 原因。
func DecideStopReason(in StopInput) (StopReason, string) {
	if in.UserInterrupted {
		return StopReasonUserInterrupt, ""
	}
	if in.FatalError != nil {
		if errors.Is(in.FatalError, context.Canceled) {
			return StopReasonUserInterrupt, ""
		}
		return StopReasonFatalError, strings.TrimSpace(in.FatalError.Error())
	}
	if in.Completed {
		return StopReasonCompleted, ""
	}
	return StopReasonFatalError, "runtime: stop reason undetermined"
}
