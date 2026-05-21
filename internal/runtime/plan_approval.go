package runtime

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	// ErrPlanApprovalCurrentPlanMissing 表示当前会话没有可批准的计划。
	ErrPlanApprovalCurrentPlanMissing = errors.New("runtime plan approval current plan missing")
	// ErrPlanApprovalPlanIDMismatch 表示客户端批准的计划 ID 已不是当前计划。
	ErrPlanApprovalPlanIDMismatch = errors.New("runtime plan approval plan id mismatch")
	// ErrPlanApprovalRevisionMismatch 表示客户端批准的 revision 已过期或非法。
	ErrPlanApprovalRevisionMismatch = errors.New("runtime plan approval revision mismatch")
	// ErrPlanApprovalStatusInvalid 表示当前计划状态不允许批准。
	ErrPlanApprovalStatusInvalid = errors.New("runtime plan approval status invalid")
)

// IsPlanApprovalInvalidError 判断错误是否属于可预期的计划审批业务拒绝。
func IsPlanApprovalInvalidError(err error) bool {
	return errors.Is(err, ErrPlanApprovalCurrentPlanMissing) ||
		errors.Is(err, ErrPlanApprovalPlanIDMismatch) ||
		errors.Is(err, ErrPlanApprovalRevisionMismatch) ||
		errors.Is(err, ErrPlanApprovalStatusInvalid)
}

// ApproveCurrentPlan 显式批准当前完整计划 revision，并安排下一轮做一次完整计划对齐。
func (s *Service) ApproveCurrentPlan(ctx context.Context, input ApproveCurrentPlanInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return errors.New("runtime: service is nil")
	}
	sessionID := strings.TrimSpace(input.SessionID)
	releaseLock := s.bindSessionLock(sessionID)
	defer releaseLock()

	session, err := s.LoadSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if err := approveCurrentPlan(&session, input.PlanID, input.Revision); err != nil {
		return err
	}
	session.UpdatedAt = time.Now()
	return s.sessionStore.UpdateSessionState(ctx, sessionStateInputFromSession(session))
}
