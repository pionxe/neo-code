//go:build windows

package config

import (
	"errors"
	"fmt"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestIsWindowsRegistryNotExistWrapped(t *testing.T) {
	wrapped := fmt.Errorf("wrapped: %w", registry.ErrNotExist)
	if !isWindowsRegistryNotExist(wrapped) {
		t.Fatal("expected wrapped ErrNotExist to be treated as not-exist")
	}
}

func TestIsWindowsRegistryNotExistMismatch(t *testing.T) {
	if isWindowsRegistryNotExist(errors.New("other error")) {
		t.Fatal("expected non-ErrNotExist error to be false")
	}
}
