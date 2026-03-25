package tools

import (
	"errors"
	"strings"
	"testing"

	"go-llm-demo/internal/server/domain"
)

type mockSecurityChecker struct {
	action domain.Action
}

func (m mockSecurityChecker) Check(_ string, _ string) domain.Action {
	return m.action
}

func TestBashTool_Run_DeniedBySecurity(t *testing.T) {
	SetSecurityChecker(mockSecurityChecker{action: domain.ActionDeny})
	defer SetSecurityChecker(nil)

	result := (&BashTool{}).Run(map[string]interface{}{
		"command": "echo hello",
	})

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Success {
		t.Fatal("expected bash execution to be denied")
	}
	if !strings.Contains(result.Error, "Security policy denied execution") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Metadata["securityAction"] != string(domain.ActionDeny) {
		t.Fatalf("unexpected security action: %#v", result.Metadata["securityAction"])
	}
}

func TestBashTool_Run_AskBySecurity(t *testing.T) {
	SetSecurityChecker(mockSecurityChecker{action: domain.ActionAsk})
	defer SetSecurityChecker(nil)

	result := (&BashTool{}).Run(map[string]interface{}{
		"command": "go build ./...",
	})

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Success {
		t.Fatal("expected bash execution to require confirmation")
	}
	if !strings.Contains(result.Error, "requires user confirmation") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Metadata["securityAction"] != string(domain.ActionAsk) {
		t.Fatalf("unexpected security action: %#v", result.Metadata["securityAction"])
	}
}

func TestPreferredShellCommandFallsBackToCmdOnWindows(t *testing.T) {
	lookup := func(name string) (string, error) {
		if name == "cmd.exe" {
			return name, nil
		}
		return "", errors.New("not found")
	}

	shell, args := preferredShellCommand("windows", "echo hello", lookup, "powershell", []string{"-Command", "echo hello"})
	if shell != "cmd.exe" {
		t.Fatalf("expected cmd.exe fallback, got %q", shell)
	}
	if len(args) != 2 || args[0] != "/C" || args[1] != "echo hello" {
		t.Fatalf("unexpected cmd.exe args: %#v", args)
	}
}

func TestPreferredShellCommandFallsBackToShWithoutBash(t *testing.T) {
	lookup := func(name string) (string, error) {
		if name == "sh" {
			return name, nil
		}
		return "", errors.New("not found")
	}

	shell, args := preferredShellCommand("linux", "echo hello", lookup, "bash", []string{"-lc", "echo hello"})
	if shell != "sh" {
		t.Fatalf("expected sh fallback, got %q", shell)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "echo hello" {
		t.Fatalf("unexpected sh args: %#v", args)
	}
}
