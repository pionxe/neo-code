package controlplane

// StopReason 表示一次 Run 的最终硬停止原因。
type StopReason string

const (
	// StopReasonFatalError 表示出现不可恢复错误。
	StopReasonFatalError StopReason = "STOP_FATAL_ERROR"
	// StopReasonCompleted 表示运行满足完成条件。
	StopReasonCompleted StopReason = "STOP_COMPLETED"
	// StopReasonUserInterrupt 表示运行被用户或上层上下文中断。
	StopReasonUserInterrupt StopReason = "STOP_USER_INTERRUPT"
)
