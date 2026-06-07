package gateway

import (
	"errors"
	"strings"
)

var (
	// ErrRuntimeAccessDenied 表示运行时拒绝当前主体访问目标资源。
	ErrRuntimeAccessDenied = errors.New("runtime access denied")
	// ErrRuntimeResourceNotFound 表示运行时未找到目标资源。
	ErrRuntimeResourceNotFound = errors.New("runtime resource not found")
	// ErrRuntimeUnavailable 表示运行时或其可选下游能力不可用。
	ErrRuntimeUnavailable = errors.New("runtime unavailable")
	// ErrRuntimeInvalidAction 表示运行时拒绝了语义非法或已过期的动作。
	ErrRuntimeInvalidAction = errors.New("runtime invalid action")
	// ErrRuntimeMaxTurnExceeded 表示运行时达到 runtime.max_turns 后受控停止。
	ErrRuntimeMaxTurnExceeded = errors.New("runtime max turn exceeded")
)

// RuntimeMaxTurnExceededError 携带 runtime 原始 max_turns 停止说明，供 Gateway 对外展示。
type RuntimeMaxTurnExceededError struct {
	Detail string
}

// Error 返回可展示的 max_turns 停止说明。
func (e RuntimeMaxTurnExceededError) Error() string {
	detail := strings.TrimSpace(e.Detail)
	if detail != "" {
		return detail
	}
	return ErrRuntimeMaxTurnExceeded.Error()
}

// Unwrap 保留稳定哨兵错误，便于 errors.Is 做语义判断。
func (e RuntimeMaxTurnExceededError) Unwrap() error {
	return ErrRuntimeMaxTurnExceeded
}

// NewRuntimeMaxTurnExceededError 创建带细节的 max_turns 受控停止错误。
func NewRuntimeMaxTurnExceededError(detail string) error {
	return RuntimeMaxTurnExceededError{Detail: detail}
}

// RuntimeMaxTurnExceededDetail 提取 max_turns 受控停止错误中的展示文本。
func RuntimeMaxTurnExceededDetail(err error) string {
	var target RuntimeMaxTurnExceededError
	if errors.As(err, &target) {
		return target.Error()
	}
	if errors.Is(err, ErrRuntimeMaxTurnExceeded) {
		return ErrRuntimeMaxTurnExceeded.Error()
	}
	return ""
}
