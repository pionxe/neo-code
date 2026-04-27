//go:build !windows && !darwin && !linux

package urlscheme

import (
	"errors"
	"testing"
)

func TestRegisterURLSchemeNotSupportedOnUnsupportedPlatforms(t *testing.T) {
	err := RegisterURLScheme("/tmp/neocode")
	if err == nil {
		t.Fatal("expected not supported error")
	}

	var dispatchErr *DispatchError
	if !errors.As(err, &dispatchErr) {
		t.Fatalf("error type = %T, want *DispatchError", err)
	}
	if dispatchErr.Code != ErrorCodeNotSupported {
		t.Fatalf("error code = %q, want %q", dispatchErr.Code, ErrorCodeNotSupported)
	}
}
