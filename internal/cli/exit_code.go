package cli

import "fmt"

// ExitCoder 描述 CLI 错误可携带的进程退出码。
type ExitCoder interface {
	error
	ExitCode() int
}

type commandExitError struct {
	code int
	err  error
}

// Error 返回底层错误文案，供主入口统一打印。
func (e *commandExitError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

// Unwrap 暴露底层错误，便于测试和 errors.Is/As 复用。
func (e *commandExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// ExitCode 返回命令预期的进程退出码。
func (e *commandExitError) ExitCode() int {
	if e == nil || e.code <= 0 {
		return 1
	}
	return e.code
}

// newCommandExitError 构造带退出码的 CLI 错误。
func newCommandExitError(code int, format string, args ...any) error {
	return &commandExitError{
		code: code,
		err:  fmt.Errorf(format, args...),
	}
}
