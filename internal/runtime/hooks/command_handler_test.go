package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildCommandPayload(t *testing.T) {
	t.Parallel()
	payload := BuildCommandPayload("my-hook", HookPointBeforeToolCall, map[string]any{
		"tool_name": "bash",
		"workdir":   "/tmp",
	})
	if payload.PayloadVersion != CommandHookPayloadVersion {
		t.Fatalf("payload_version = %q, want %q", payload.PayloadVersion, CommandHookPayloadVersion)
	}
	if payload.HookID != "my-hook" {
		t.Fatalf("hook_id = %q, want %q", payload.HookID, "my-hook")
	}
	if payload.Point != string(HookPointBeforeToolCall) {
		t.Fatalf("point = %q, want %q", payload.Point, HookPointBeforeToolCall)
	}
	if payload.Metadata["tool_name"] != "bash" {
		t.Fatalf("metadata[tool_name] = %v, want %q", payload.Metadata["tool_name"], "bash")
	}
}

func TestBuildCommandPayloadEmptyMetadata(t *testing.T) {
	t.Parallel()
	payload := BuildCommandPayload("hook", HookPointSessionStart, nil)
	if payload.Metadata != nil {
		t.Fatalf("metadata should be nil for empty input, got %v", payload.Metadata)
	}
}

func TestParseCommandResponsePass(t *testing.T) {
	t.Parallel()
	resp, err := ParseCommandResponse([]byte(`{"status":"pass","message":"ok"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "pass" {
		t.Fatalf("status = %q, want %q", resp.Status, "pass")
	}
	if resp.Message != "ok" {
		t.Fatalf("message = %q, want %q", resp.Message, "ok")
	}
}

func TestParseCommandResponseBlock(t *testing.T) {
	t.Parallel()
	resp, err := ParseCommandResponse([]byte(`{"status":"block","message":"denied"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "block" {
		t.Fatalf("status = %q, want %q", resp.Status, "block")
	}
}

func TestParseCommandResponseFailed(t *testing.T) {
	t.Parallel()
	resp, err := ParseCommandResponse([]byte(`{"status":"failed","message":"broken"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "failed" {
		t.Fatalf("status = %q, want %q", resp.Status, "failed")
	}
}

func TestParseCommandResponseWithAnnotations(t *testing.T) {
	t.Parallel()
	resp, err := ParseCommandResponse([]byte(`{"status":"pass","annotations":["note1","note2"]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Annotations) != 2 || resp.Annotations[0] != "note1" {
		t.Fatalf("annotations = %v, want [note1 note2]", resp.Annotations)
	}
}

func TestParseCommandResponseWithUpdateInput(t *testing.T) {
	t.Parallel()
	resp, err := ParseCommandResponse([]byte(`{"status":"pass","update_input":{"text":"rewritten"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.UpdateInput) == 0 {
		t.Fatal("update_input should not be empty")
	}
	var update struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(resp.UpdateInput, &update); err != nil {
		t.Fatalf("unmarshal update_input: %v", err)
	}
	if update.Text != "rewritten" {
		t.Fatalf("update_input.text = %q, want %q", update.Text, "rewritten")
	}
}

func TestParseCommandResponseInvalidStatus(t *testing.T) {
	t.Parallel()
	_, err := ParseCommandResponse([]byte(`{"status":"unknown"}`))
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestParseCommandResponseInvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseCommandResponse([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for non-JSON input")
	}
}

func TestParseCommandResponseEmptyStdout(t *testing.T) {
	t.Parallel()
	_, err := ParseCommandResponse([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestRunCommandHookArgvMode(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("argv mode test uses echo which is a shell builtin on Windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := CommandHookSpec{
		HookID:  "test-argv",
		Point:   HookPointBeforeToolCall,
		Command: []string{"echo", `{"status":"pass","message":"hello from argv"}`},
		Shell:   false,
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
	}
	if result.Message != "hello from argv" {
		t.Fatalf("message = %q, want %q", result.Message, "hello from argv")
	}
}

func TestRunCommandHookArgvModeWindows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := CommandHookSpec{
		HookID:  "test-argv-win",
		Point:   HookPointBeforeToolCall,
		Command: []string{"powershell", "-Command", "Write-Output '{\"status\":\"pass\",\"message\":\"hello from argv\"}'"},
		Shell:   false,
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
	}
	if result.Message != "hello from argv" {
		t.Fatalf("message = %q, want %q", result.Message, "hello from argv")
	}
}

func TestRunCommandHookShellMode(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell mode test uses sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := CommandHookSpec{
		HookID:  "test-shell",
		Point:   HookPointBeforeToolCall,
		Command: []string{`echo '{"status":"pass","message":"from shell"}'`},
		Shell:   true,
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
	}
	if result.Message != "from shell" {
		t.Fatalf("message = %q, want %q", result.Message, "from shell")
	}
}

func TestRunCommandHookExitCodeNonZeroEmptyStdout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "test-exit3",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "exit 3"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "test-exit3",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "exit 3"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultFailed {
		t.Fatalf("status = %q, want %q", result.Status, HookResultFailed)
	}
}

func TestRunCommandHookExitCodeBlock(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "test-exit1",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output 'blocked'; exit 1"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "test-exit1",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "echo blocked; exit 1"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultBlock {
		t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultBlock, result.Message)
	}
	if result.Message != "blocked" {
		t.Fatalf("message = %q, want %q", result.Message, "blocked")
	}
}

func TestRunCommandHookTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "test-timeout",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Start-Sleep -Seconds 10"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "test-timeout",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "sleep 10"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultFailed {
		t.Fatalf("status = %q, want %q", result.Status, HookResultFailed)
	}
}

func TestRunCommandHookEnvIsolation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	if runtime.GOOS == "windows" {
		script := filepath.Join(tmpDir, "check_env.ps1")
		if err := os.WriteFile(script, []byte(`$env:NEOCODE_HOOK_HOOK_ID; $env:NEOCODE_HOOK_POINT; $env:NEOCODE_HOOK_PAYLOAD_VERSION; if ($env:PATH) { "HAS_PATH=1" }; '{"status":"pass"}'`), 0o755); err != nil {
			t.Fatalf("write script: %v", err)
		}
		spec := CommandHookSpec{
			HookID:  "env-test",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-ExecutionPolicy", "Bypass", "-File", script},
		}
		result := RunCommandHook(ctx, spec, HookContext{})
		if result.Status != HookResultPass {
			t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
		}
		if !strings.Contains(result.Message, "env-test") {
			t.Fatalf("expected NEOCODE_HOOK_HOOK_ID in output, got: %s", result.Message)
		}
		if strings.Contains(result.Message, "HAS_PATH=1") {
			t.Fatal("PATH should not be inherited in isolated env")
		}
	} else {
		script := filepath.Join(tmpDir, "check_env.sh")
		if err := os.WriteFile(script, []byte("#!/bin/sh\nenv | grep NEOCODE_HOOK_ | sort\nif [ -n \"$PATH\" ]; then echo \"HAS_PATH=1\"; fi\necho '{\"status\":\"pass\"}'\n"), 0o755); err != nil {
			t.Fatalf("write script: %v", err)
		}
		spec := CommandHookSpec{
			HookID:  "env-test",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", script},
		}
		result := RunCommandHook(ctx, spec, HookContext{})
		if result.Status != HookResultPass {
			t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
		}
		if !strings.Contains(result.Message, "NEOCODE_HOOK_HOOK_ID=env-test") {
			t.Fatalf("expected NEOCODE_HOOK_HOOK_ID in output, got: %s", result.Message)
		}
		if !strings.Contains(result.Message, "NEOCODE_HOOK_POINT=before_tool_call") {
			t.Fatalf("expected NEOCODE_HOOK_POINT in output, got: %s", result.Message)
		}
		if strings.Contains(result.Message, "HAS_PATH=1") {
			t.Fatal("PATH should not be inherited in isolated env")
		}
	}
}

func TestRunCommandHookBackwardCompatPlainText(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "compat",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output 'just a message'"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "compat",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "echo just a message; exit 0"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q", result.Status, HookResultPass)
	}
	if result.Message != "just a message" {
		t.Fatalf("message = %q, want %q", result.Message, "just a message")
	}
}

func TestRunCommandHookAnnotationsPopulated(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "annotated",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output '{\"status\":\"pass\",\"annotations\":[\"a1\",\"a2\"]}'"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "annotated",
			Point:   HookPointBeforeToolCall,
			Command: []string{"echo", `{"status":"pass","annotations":["a1","a2"]}`},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q", result.Status, HookResultPass)
	}
	if len(result.Metadata.Annotations) != 2 {
		t.Fatalf("annotations count = %d, want 2; annotations: %v", len(result.Metadata.Annotations), result.Metadata.Annotations)
	}
}

func TestRunCommandHookWorkdir(t *testing.T) {
	t.Parallel()
	tmpDir, err := os.MkdirTemp("", "hook-workdir-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "workdir-test",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output (Get-Location).Path; exit 0"},
			Workdir: tmpDir,
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "workdir-test",
			Point:   HookPointBeforeToolCall,
			Command: []string{"pwd"},
			Workdir: tmpDir,
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
	}
	if !strings.Contains(strings.ToLower(result.Message), strings.ToLower(filepath.Base(tmpDir))) {
		t.Fatalf("expected workdir in output, got: %s", result.Message)
	}
}

func TestRunCommandHookStdinPayload(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "stdin-test",
			Point:   HookPointUserPromptSubmit,
			Command: []string{"powershell", "-Command", "$input"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "stdin-test",
			Point:   HookPointUserPromptSubmit,
			Command: []string{"cat"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{Metadata: map[string]any{"workdir": "/tmp"}})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q", result.Status, HookResultPass)
	}
	if !strings.Contains(result.Message, CommandHookPayloadVersion) {
		t.Fatalf("stdin payload should contain payload_version, got: %s", result.Message)
	}
}
