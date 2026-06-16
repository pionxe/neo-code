package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestCommandExitErrorNilAndDefaultBranches(t *testing.T) {
	var nilErr *commandExitError
	if nilErr.Error() != "" {
		t.Fatalf("nil Error() = %q, want empty", nilErr.Error())
	}
	if nilErr.Unwrap() != nil {
		t.Fatalf("nil Unwrap() = %#v, want nil", nilErr.Unwrap())
	}
	if nilErr.ExitCode() != 1 {
		t.Fatalf("nil ExitCode() = %d, want 1", nilErr.ExitCode())
	}

	empty := &commandExitError{}
	if empty.Error() != "" {
		t.Fatalf("empty Error() = %q, want empty", empty.Error())
	}
	if empty.ExitCode() != 1 {
		t.Fatalf("empty ExitCode() = %d, want 1", empty.ExitCode())
	}
}

func TestCommandExitErrorWrapsCause(t *testing.T) {
	cause := errors.New("cause")
	err := &commandExitError{code: 9, err: cause}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is() did not match wrapped cause")
	}
	if !strings.Contains(err.Error(), "cause") {
		t.Fatalf("Error() = %q, want cause", err.Error())
	}
	if err.ExitCode() != 9 {
		t.Fatalf("ExitCode() = %d, want 9", err.ExitCode())
	}
}
