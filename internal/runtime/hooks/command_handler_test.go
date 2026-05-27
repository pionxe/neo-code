package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildCommandPayload(t *testing.T) {
	t.Parallel()
	payload := BuildCommandPayload("my-hook", HookPointBeforeToolCall, HookContext{
		RunID:     "run-123",
		SessionID: "sess-456",
		Metadata: map[string]any{
			"tool_name": "bash",
			"workdir":   "/tmp",
		},
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
	if payload.RunID != "run-123" {
		t.Fatalf("run_id = %q, want %q", payload.RunID, "run-123")
	}
	if payload.SessionID != "sess-456" {
		t.Fatalf("session_id = %q, want %q", payload.SessionID, "sess-456")
	}
	if payload.Metadata["tool_name"] != "bash" {
		t.Fatalf("metadata[tool_name] = %v, want %q", payload.Metadata["tool_name"], "bash")
	}
}

func TestBuildCommandPayloadEmptyMetadata(t *testing.T) {
	t.Parallel()
	payload := BuildCommandPayload("hook", HookPointSessionStart, HookContext{})
	if payload.Metadata != nil {
		t.Fatalf("metadata should be nil for empty input, got %v", payload.Metadata)
	}
	if payload.RunID != "" {
		t.Fatalf("run_id should be empty, got %q", payload.RunID)
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
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "env-test",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "$env:NEOCODE_HOOK_HOOK_ID; $env:NEOCODE_HOOK_POINT; $env:NEOCODE_HOOK_PAYLOAD_VERSION; '{\"status\":\"pass\"}'"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "env-test",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "echo $NEOCODE_HOOK_HOOK_ID; echo $NEOCODE_HOOK_POINT; echo $NEOCODE_HOOK_PAYLOAD_VERSION; echo '{\"status\":\"pass\"}'"},
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
	}
	if !strings.Contains(result.Message, "env-test") {
		t.Fatalf("expected NEOCODE_HOOK_HOOK_ID in output, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "before_tool_call") {
		t.Fatalf("expected NEOCODE_HOOK_POINT in output, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, CommandHookPayloadVersion) {
		t.Fatalf("expected NEOCODE_HOOK_PAYLOAD_VERSION in output, got: %s", result.Message)
	}
}

func TestBuildCommandEnvContainsHookVars(t *testing.T) {
	t.Parallel()
	spec := CommandHookSpec{HookID: "id-123", Point: HookPointSessionEnd}
	env := buildCommandEnv(spec)
	envMap := make(map[string]bool)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = true
		}
	}
	if !envMap["NEOCODE_HOOK_HOOK_ID"] {
		t.Fatal("missing NEOCODE_HOOK_HOOK_ID")
	}
	if !envMap["NEOCODE_HOOK_POINT"] {
		t.Fatal("missing NEOCODE_HOOK_POINT")
	}
	if !envMap["NEOCODE_HOOK_PAYLOAD_VERSION"] {
		t.Fatal("missing NEOCODE_HOOK_PAYLOAD_VERSION")
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

func TestBuildCommandPayloadRunSessionID(t *testing.T) {
	t.Parallel()
	payload := BuildCommandPayload("my-hook", HookPointBeforeToolCall, HookContext{
		RunID:     "run-abc",
		SessionID: "sess-xyz",
	})
	if payload.RunID != "run-abc" {
		t.Fatalf("run_id = %q, want %q", payload.RunID, "run-abc")
	}
	if payload.SessionID != "sess-xyz" {
		t.Fatalf("session_id = %q, want %q", payload.SessionID, "sess-xyz")
	}
}

func TestBuildCommandPayloadEmptyRunSessionID(t *testing.T) {
	t.Parallel()
	payload := BuildCommandPayload("hook", HookPointSessionStart, HookContext{})
	if payload.RunID != "" {
		t.Fatalf("run_id should be empty, got %q", payload.RunID)
	}
	if payload.SessionID != "" {
		t.Fatalf("session_id should be empty, got %q", payload.SessionID)
	}
}

func TestRunCommandHookExitCodePrecedenceOverJSON(t *testing.T) {
	// Security: non-zero exit code must override JSON status.
	// A malicious script claiming "pass" while exiting 1 should result in block, not pass.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "precedence-test",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output '{\"status\":\"pass\",\"message\":\"claiming pass\"}'; exit 1"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "precedence-test",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "echo '{\"status\":\"pass\",\"message\":\"claiming pass\"}'; exit 1"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultBlock {
		t.Fatalf("status = %q, want %q (exit code must take precedence over JSON status)", result.Status, HookResultBlock)
	}
	// message should still be extracted from JSON stdout
	if result.Message != "claiming pass" {
		t.Fatalf("message = %q, want %q (should extract message from JSON even when exit code wins)", result.Message, "claiming pass")
	}
}

func TestRunCommandHookExitCodeThreeWithJSONMessage(t *testing.T) {
	// exit code 3 + JSON with message → failed status, message from JSON
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "exit3-json",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output '{\"status\":\"pass\",\"message\":\"from json\"}'; exit 3"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "exit3-json",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "echo '{\"status\":\"pass\",\"message\":\"from json\"}'; exit 3"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultFailed {
		t.Fatalf("status = %q, want %q", result.Status, HookResultFailed)
	}
	if result.Message != "from json" {
		t.Fatalf("message = %q, want %q", result.Message, "from json")
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
	result := RunCommandHook(ctx, spec, HookContext{
		RunID:     "run-789",
		SessionID: "sess-012",
		Metadata:  map[string]any{"workdir": "/tmp"},
	})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q", result.Status, HookResultPass)
	}
	if !strings.Contains(result.Message, CommandHookPayloadVersion) {
		t.Fatalf("stdin payload should contain payload_version, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "run-789") {
		t.Fatalf("stdin payload should contain run_id, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "sess-012") {
		t.Fatalf("stdin payload should contain session_id, got: %s", result.Message)
	}
}

func TestRunCommandHookShellModeWindows(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := CommandHookSpec{
		HookID:  "test-shell-win",
		Point:   HookPointBeforeToolCall,
		Command: []string{`Write-Output '{"status":"pass","message":"from powershell shell"}'`},
		Shell:   true,
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
	}
	if result.Message != "from powershell shell" {
		t.Fatalf("message = %q, want %q", result.Message, "from powershell shell")
	}
}

func TestRunCommandHookEnvIsolationNoLeak(t *testing.T) {
	// Verify that host env vars like PATH, HOME, USER are NOT leaked to the subprocess.
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("PATH leaks at system level on Windows; see buildCommandEnv")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := CommandHookSpec{
		HookID:  "env-no-leak",
		Point:   HookPointBeforeToolCall,
		Command: []string{"env"},
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q; message: %s", result.Status, HookResultPass, result.Message)
	}
	for _, leaked := range []string{"PATH=", "HOME=", "USER="} {
		if strings.Contains(result.Message, leaked) {
			t.Fatalf("host env var %q should not be leaked to subprocess, got: %s", leaked, result.Message)
		}
	}
}

func TestParseCommandParamsAllBranches(t *testing.T) {
	t.Parallel()

	t.Run("string with shell=true", func(t *testing.T) {
		t.Parallel()
		argv, shell, err := ParseCommandParams(map[string]any{"command": "echo hi", "shell": true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !shell {
			t.Fatal("expected shell=true")
		}
		if len(argv) != 1 || argv[0] != "echo hi" {
			t.Fatalf("argv = %v, want [echo hi]", argv)
		}
	})

	t.Run("string with whitespace shell=true", func(t *testing.T) {
		t.Parallel()
		argv, shell, err := ParseCommandParams(map[string]any{"command": "  echo hi  ", "shell": true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !shell || argv[0] != "echo hi" {
			t.Fatalf("argv = %v, shell = %v", argv, shell)
		}
	})

	t.Run("[]string valid", func(t *testing.T) {
		t.Parallel()
		argv, shell, err := ParseCommandParams(map[string]any{"command": []string{"echo", "hello"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if shell {
			t.Fatal("expected shell=false for array")
		}
		if len(argv) != 2 || argv[0] != "echo" || argv[1] != "hello" {
			t.Fatalf("argv = %v", argv)
		}
	})

	t.Run("[]string empty", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseCommandParams(map[string]any{"command": []string{}})
		if err == nil {
			t.Fatal("expected error for empty []string")
		}
	})

	t.Run("[]string with empty element", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseCommandParams(map[string]any{"command": []string{"echo", "  ", "ok"}})
		if err == nil {
			t.Fatal("expected error for empty element in []string")
		}
	})

	t.Run("[]any with empty element after Sprintf", func(t *testing.T) {
		t.Parallel()
		// nil element => fmt.Sprintf("%v", nil) => "<nil>" which is non-empty
		// but empty string element => fmt.Sprintf("%v", "") => "" which is empty
		_, _, err := ParseCommandParams(map[string]any{"command": []any{"echo", ""}})
		if err == nil {
			t.Fatal("expected error for empty element in []any")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseCommandParams(map[string]any{"command": 123})
		if err == nil {
			t.Fatal("expected error for unsupported type")
		}
	})

	t.Run("nil command value", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseCommandParams(map[string]any{"command": nil})
		if err == nil {
			t.Fatal("expected error for nil command")
		}
	})

	t.Run("shell=false on string", func(t *testing.T) {
		t.Parallel()
		_, _, err := ParseCommandParams(map[string]any{"command": "echo ok", "shell": false})
		if err == nil {
			t.Fatal("expected error for string without shell=true")
		}
	})
}

func TestRunCommandHookStdoutTooLarge(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Generate output slightly above the 1MiB limit
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "stdout-toolarge",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output ('x' * 1048577)"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "stdout-toolarge",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "printf '%1048577s' ''"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultFailed {
		t.Fatalf("status = %q, want %q", result.Status, HookResultFailed)
	}
	if !strings.Contains(result.Message, "byte limit") {
		t.Fatalf("message should mention byte limit, got: %s", result.Message)
	}
}

func TestRunCommandHookStdinPayloadWithMetadata(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "stdin-meta",
			Point:   HookPointUserPromptSubmit,
			Command: []string{"powershell", "-Command", "$input"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "stdin-meta",
			Point:   HookPointUserPromptSubmit,
			Command: []string{"cat"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{
		RunID:     "run-meta",
		SessionID: "sess-meta",
		Metadata:  map[string]any{"tool_name": "bash", "workdir": "/tmp"},
	})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q", result.Status, HookResultPass)
	}
	if !strings.Contains(result.Message, `"tool_name"`) {
		t.Fatalf("stdin should contain tool_name metadata, got: %s", result.Message)
	}
}

func TestRunCommandHookExitCodeTwoBlocks(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "exit2",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "exit 2"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "exit2",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "exit 2"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultBlock {
		t.Fatalf("status = %q, want %q", result.Status, HookResultBlock)
	}
	if result.Error == "" {
		t.Fatal("expected Error to be set for exit code 2 block")
	}
}

func TestRunCommandHookExitCodeZeroEmptyStdout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "exit0-empty",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", ""},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "exit0-empty",
			Point:   HookPointBeforeToolCall,
			Command: []string{"true"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q", result.Status, HookResultPass)
	}
}

func TestRunCommandHookNonExistentBinary(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	spec := CommandHookSpec{
		HookID:  "no-such-binary",
		Point:   HookPointBeforeToolCall,
		Command: []string{"nonexistent_binary_xyz_12345"},
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultFailed {
		t.Fatalf("status = %q, want %q", result.Status, HookResultFailed)
	}
	if result.Error == "" {
		t.Fatal("expected Error to be set for nonexistent binary")
	}
}

func TestRunCommandHookBlockWithMessage(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "block-msg",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output '{\"status\":\"block\",\"message\":\"not allowed\"}'"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "block-msg",
			Point:   HookPointBeforeToolCall,
			Command: []string{"echo", `{"status":"block","message":"not allowed"}`},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultBlock {
		t.Fatalf("status = %q, want %q", result.Status, HookResultBlock)
	}
	if result.Message != "not allowed" {
		t.Fatalf("message = %q, want %q", result.Message, "not allowed")
	}
}

func TestRunCommandHookFailedStatusWithDefaultMessage(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "failed-default",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output '{\"status\":\"failed\"}'"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "failed-default",
			Point:   HookPointBeforeToolCall,
			Command: []string{"echo", `{"status":"failed"}`},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultFailed {
		t.Fatalf("status = %q, want %q", result.Status, HookResultFailed)
	}
	if result.Message != "hook returned failed status" {
		t.Fatalf("message = %q, want default failed message", result.Message)
	}
	if result.Error != "hook returned failed status" {
		t.Fatalf("error = %q, want default failed message", result.Error)
	}
}

func TestRunCommandHookFailedStatusWithCustomMessage(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "failed-custom",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Output '{\"status\":\"failed\",\"message\":\"custom error\"}'"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "failed-custom",
			Point:   HookPointBeforeToolCall,
			Command: []string{"echo", `{"status":"failed","message":"custom error"}`},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultFailed {
		t.Fatalf("status = %q, want %q", result.Status, HookResultFailed)
	}
	if result.Message != "custom error" {
		t.Fatalf("message = %q, want %q", result.Message, "custom error")
	}
	if result.Error != "custom error" {
		t.Fatalf("error = %q, want %q", result.Error, "custom error")
	}
}

func TestRunCommandHookPassWithAnnotationsAndUpdateInput(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	payload := `{"status":"pass","message":"ok","annotations":["a1","a2"],"update_input":{"text":"rewritten"}}`
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "full-output",
			Point:   HookPointUserPromptSubmit,
			Command: []string{"powershell", "-Command", fmt.Sprintf("Write-Output '%s'", payload)},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "full-output",
			Point:   HookPointUserPromptSubmit,
			Command: []string{"echo", payload},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultPass {
		t.Fatalf("status = %q, want %q", result.Status, HookResultPass)
	}
	if result.Message != "ok" {
		t.Fatalf("message = %q, want %q", result.Message, "ok")
	}
	if len(result.Metadata.Annotations) != 2 || result.Metadata.Annotations[0] != "a1" {
		t.Fatalf("annotations = %v, want [a1 a2]", result.Metadata.Annotations)
	}
	if len(result.Metadata.UpdateInput) == 0 {
		t.Fatal("expected UpdateInput to be populated")
	}
}

func TestRunCommandHookExitCodeThreeWithStderr(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var spec CommandHookSpec
	if runtime.GOOS == "windows" {
		spec = CommandHookSpec{
			HookID:  "exit3-stderr",
			Point:   HookPointBeforeToolCall,
			Command: []string{"powershell", "-Command", "Write-Error 'bad thing'; exit 3"},
		}
	} else {
		spec = CommandHookSpec{
			HookID:  "exit3-stderr",
			Point:   HookPointBeforeToolCall,
			Command: []string{"sh", "-c", "echo bad thing >&2; exit 3"},
		}
	}
	result := RunCommandHook(ctx, spec, HookContext{})
	if result.Status != HookResultFailed {
		t.Fatalf("status = %q, want %q", result.Status, HookResultFailed)
	}
}

func TestBuildCommandEnvContainsNEOCODEVars(t *testing.T) {
	t.Parallel()
	spec := CommandHookSpec{HookID: "id-env", Point: HookPointSessionStart}
	env := buildCommandEnv(spec)
	envMap := make(map[string]bool)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = true
		}
	}
	for _, key := range []string{"NEOCODE_HOOK_HOOK_ID", "NEOCODE_HOOK_POINT", "NEOCODE_HOOK_PAYLOAD_VERSION"} {
		if !envMap[key] {
			t.Fatalf("missing %s in env", key)
		}
	}
	if runtime.GOOS == "windows" {
		for _, key := range []string{"SystemRoot", "SystemDrive", "USERPROFILE"} {
			if os.Getenv(key) != "" && !envMap[key] {
				t.Fatalf("missing Windows env var %s", key)
			}
		}
	}
}

func TestValidateCommandParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		params  map[string]any
		wantErr bool
	}{
		{"nil params", nil, true},
		{"empty params", map[string]any{}, true},
		{"missing command", map[string]any{"other": "val"}, true},
		{"empty string command", map[string]any{"command": ""}, true},
		{"string without shell", map[string]any{"command": "echo ok"}, true},
		{"string with shell", map[string]any{"command": "echo ok", "shell": true}, false},
		{"empty array", map[string]any{"command": []any{}}, true},
		{"valid array", map[string]any{"command": []any{"echo", "ok"}}, false},
		{"array with empty element", map[string]any{"command": []any{"echo", ""}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCommandParams(tc.params)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateCommandParams() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
