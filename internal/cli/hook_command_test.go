package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"neo-code/internal/config"
	agentruntime "neo-code/internal/runtime"
)

func TestRootCommandIncludesHookCommand(t *testing.T) {
	command := NewRootCommand()
	found := false
	for _, child := range command.Commands() {
		if child.Name() == "hook" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected root command to include hook subcommand")
	}
}

func TestHookCommandsHelp(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	cases := [][]string{
		{"hook", "lint", "--help"},
		{"hook", "dry-run", "--help"},
		{"hook", "trace", "--help"},
	}
	for _, args := range cases {
		command := NewRootCommand()
		command.SetArgs(args)
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("%v help failed: %v", args, err)
		}
	}
}

func TestReadHookTraceRecordsAndAggregate(t *testing.T) {
	tracePath := filepath.Join(t.TempDir(), "run-1.jsonl")
	records := []agentruntime.HookTraceRecord{
		{
			EventType:  "hook_started",
			Timestamp:  time.Unix(10, 0).UTC(),
			RunID:      "run-1",
			HookID:     "warn-bash",
			Point:      "before_tool_call",
			DurationMS: 0,
		},
		{
			EventType:  "hook_finished",
			Timestamp:  time.Unix(11, 0).UTC(),
			RunID:      "run-1",
			HookID:     "warn-bash",
			Point:      "before_tool_call",
			Status:     "pass",
			DurationMS: 12,
		},
		{
			EventType:  "hook_blocked",
			Timestamp:  time.Unix(12, 0).UTC(),
			RunID:      "run-1",
			HookID:     "repo-guard",
			Point:      "accept_gate",
			Status:     "block",
			DurationMS: 33,
		},
	}
	file, err := os.Create(tracePath)
	if err != nil {
		t.Fatalf("create trace file: %v", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("encode trace record: %v", err)
		}
	}

	loaded, err := readHookTraceRecords(tracePath)
	if err != nil {
		t.Fatalf("readHookTraceRecords() error = %v", err)
	}
	if len(loaded) != len(records) {
		t.Fatalf("records len = %d, want %d", len(loaded), len(records))
	}
	aggregates := aggregateHookTraceRecords(loaded)
	if len(aggregates) != 2 {
		t.Fatalf("aggregates len = %d, want 2", len(aggregates))
	}
	if aggregates[0].Count != 1 || aggregates[1].Count != 1 {
		t.Fatalf("aggregate counts = %#v, want terminal events only", aggregates)
	}
	if renderHookTraceHistogram(0) != "" {
		t.Fatal("expected empty histogram for zero duration")
	}
	if got := renderHookTraceHistogram(120); !strings.Contains(got, "#") {
		t.Fatalf("expected histogram bars, got %q", got)
	}
}

func TestReadHookTraceRecordsSkipsBlanksSortsAndReportsScannerErrors(t *testing.T) {
	tracePath := filepath.Join(t.TempDir(), "run.jsonl")
	content := strings.Join([]string{
		"",
		`{"event_type":"hook_finished","timestamp":"2026-01-01T00:00:02Z","run_id":"run-1","hook_id":"b","duration_ms":5}`,
		"   ",
		`{"event_type":"hook_finished","timestamp":"2026-01-01T00:00:01Z","run_id":"run-1","hook_id":"a","duration_ms":7}`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(trace) error = %v", err)
	}
	records, err := readHookTraceRecords(tracePath)
	if err != nil {
		t.Fatalf("readHookTraceRecords() error = %v", err)
	}
	if len(records) != 2 || records[0].HookID != "a" || records[1].HookID != "b" {
		t.Fatalf("records = %#v, want sorted non-blank records", records)
	}

	hugeLinePath := filepath.Join(t.TempDir(), "huge.jsonl")
	if err := os.WriteFile(hugeLinePath, []byte(strings.Repeat("x", 70*1024)), 0o644); err != nil {
		t.Fatalf("WriteFile(huge trace) error = %v", err)
	}
	if _, err := readHookTraceRecords(hugeLinePath); err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("expected scanner token-too-long error, got %v", err)
	}
}

func TestAggregateAndFormatHookTraceEdgeCases(t *testing.T) {
	records := []agentruntime.HookTraceRecord{
		{EventType: "hook_started", HookID: "ignored", DurationMS: 100},
		{EventType: "hook_finished", HookID: " ", DurationMS: 100},
		{EventType: "hook_failed", HookID: "b", DurationMS: 5},
		{EventType: "hook_finished", HookID: "a", DurationMS: 12},
		{EventType: "hook_blocked", HookID: "a", DurationMS: 30},
	}
	aggregates := aggregateHookTraceRecords(records)
	if len(aggregates) != 2 {
		t.Fatalf("aggregates len = %d, want 2: %#v", len(aggregates), aggregates)
	}
	if aggregates[0].HookID != "a" || aggregates[0].Count != 2 || aggregates[0].DurationMS != 42 || aggregates[0].MaxDuration != 30 {
		t.Fatalf("aggregate[0] = %#v, want sorted aggregate for a", aggregates[0])
	}

	line := formatHookTraceRecord(agentruntime.HookTraceRecord{
		EventType:  "hook_failed",
		Timestamp:  time.Unix(1, 2).UTC(),
		HookID:     " warn ",
		Point:      " before_tool_call ",
		Status:     " failed ",
		Error:      " boom ",
		DurationMS: 3,
	})
	if !strings.Contains(line, "error=boom") || !strings.Contains(line, "duration_ms=3") || strings.Contains(line, " warn ") {
		t.Fatalf("unexpected formatted line: %q", line)
	}
	if got := renderHookTraceHistogram(1000); len(got) != 24 {
		t.Fatalf("histogram len = %d, want capped width 24", len(got))
	}
	if !isHookTraceTerminalRecord(agentruntime.HookTraceRecord{EventType: " hook_finished "}) {
		t.Fatal("expected trimmed hook_finished to be terminal")
	}
}

func TestHookTraceCommandReturnsExitCodeWhenTraceMissing(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	workdir := t.TempDir()
	command := NewRootCommand()
	command.SetArgs([]string{"--workdir", workdir, "hook", "trace", "--run-id", "missing-run"})
	err := command.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected missing trace error")
	}
	exitErr, ok := err.(ExitCoder)
	if !ok {
		t.Fatalf("error type = %T, want ExitCoder", err)
	}
	if exitErr.ExitCode() != hookExitLintFindings {
		t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), hookExitLintFindings)
	}
}

func TestHookTraceCommandReadsCorruptedTraceAsSystemError(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	homeDir := t.TempDir()
	workdir := t.TempDir()
	setHookTestHome(t, homeDir)

	tracePath, err := agentruntime.HookTracePath(filepath.Join(homeDir, ".neocode"), workdir, "run-bad")
	if err != nil {
		t.Fatalf("HookTracePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(tracePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(trace dir) error = %v", err)
	}
	if err := os.WriteFile(tracePath, []byte("{bad json}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(trace) error = %v", err)
	}

	command := NewRootCommand()
	command.SetArgs([]string{"--workdir", workdir, "hook", "trace", "--run-id", "run-bad"})
	err = command.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("expected corrupted trace error")
	}
	exitErr, ok := err.(ExitCoder)
	if !ok {
		t.Fatalf("error type = %T, want ExitCoder", err)
	}
	if exitErr.ExitCode() != hookExitSystemError {
		t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), hookExitSystemError)
	}
}

func TestHookTraceCommandPrintsReplayAndSummary(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	homeDir := t.TempDir()
	workdir := t.TempDir()
	setHookTestHome(t, homeDir)

	tracePath, err := agentruntime.HookTracePath(filepath.Join(homeDir, ".neocode"), workdir, "run-1")
	if err != nil {
		t.Fatalf("HookTracePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(tracePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(trace dir) error = %v", err)
	}
	records := []agentruntime.HookTraceRecord{
		{
			EventType: "hook_started",
			Timestamp: time.Unix(10, 0).UTC(),
			RunID:     "run-1",
			HookID:    "warn-bash",
			Point:     "before_tool_call",
		},
		{
			EventType:  "hook_finished",
			Timestamp:  time.Unix(11, 0).UTC(),
			RunID:      "run-1",
			HookID:     "warn-bash",
			Point:      "before_tool_call",
			Status:     "pass",
			Message:    "ok",
			DurationMS: 18,
		},
		{
			EventType:  "hook_blocked",
			Timestamp:  time.Unix(12, 0).UTC(),
			RunID:      "run-1",
			HookID:     "repo-guard",
			Point:      "accept_gate",
			Status:     "block",
			Message:    "manual review required",
			DurationMS: 33,
		},
	}
	file, err := os.Create(tracePath)
	if err != nil {
		t.Fatalf("create trace file: %v", err)
	}
	for _, record := range records {
		if err := json.NewEncoder(file).Encode(record); err != nil {
			file.Close()
			t.Fatalf("encode trace record: %v", err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(trace file) error = %v", err)
	}

	command := NewRootCommand()
	buffer := &bytes.Buffer{}
	command.SetOut(buffer)
	command.SetErr(buffer)
	command.SetArgs([]string{"--workdir", workdir, "hook", "trace", "--run-id", "run-1"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext(trace) error = %v", err)
	}
	output := buffer.String()
	if !strings.Contains(output, "hook_finished") || !strings.Contains(output, "hook_blocked") {
		t.Fatalf("expected replay output, got %q", output)
	}
	if !strings.Contains(output, "summary:") || !strings.Contains(output, "warn-bash count=1") {
		t.Fatalf("expected summary output, got %q", output)
	}
}

func TestHookLintCommandDetectsExpectedScenarios(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	cases := []struct {
		name       string
		path       string
		content    string
		wantSubstr string
		wantHint   string
	}{
		{
			name: "unsupported point",
			path: "hooks.yaml",
			content: `
hooks:
  items:
    - id: bad-point
      point: impossible_point
      scope: repo
      kind: builtin
      mode: sync
      handler: add_context_note
      params:
        note: bad
`,
			wantSubstr: `error: point "impossible_point" is not supported`,
			wantHint:   "supported hook point",
		},
		{
			name: "user disallowed point",
			path: "config.yaml",
			content: `
runtime:
  hooks:
    items:
      - id: no-user-permission
        point: before_permission_decision
        scope: user
        kind: builtin
        mode: sync
        handler: add_context_note
        params:
          note: bad
`,
			wantSubstr: `error: point "before_permission_decision" does not allow user hooks`,
			wantHint:   "supported hook point",
		},
		{
			name: "warn_on_tool_call missing match",
			path: "config.yaml",
			content: `
runtime:
  hooks:
    items:
      - id: missing-match
        point: before_tool_call
        scope: user
        kind: builtin
        mode: sync
        handler: warn_on_tool_call
        params:
          message: warn
`,
			wantSubstr: `error: handler "warn_on_tool_call" requires match`,
			wantHint:   "match section",
		},
		{
			name: "matcher unknown field",
			path: "config.yaml",
			content: `
runtime:
  hooks:
    items:
      - id: matcher-unknown
        point: before_tool_call
        scope: user
        kind: builtin
        mode: sync
        handler: warn_on_tool_call
        match:
          tool_name: bash
          unknown_field: bash
`,
			wantSubstr: `error: match: match contains unknown field`,
			wantHint:   "tool_name_regex",
		},
		{
			name: "matcher invalid regex",
			path: "config.yaml",
			content: `
runtime:
  hooks:
    items:
      - id: matcher-regex
        point: before_tool_call
        scope: user
        kind: builtin
        mode: sync
        handler: warn_on_tool_call
        match:
          tool_name_regex: "["
`,
			wantSubstr: `error: match: matcher field "tool_name_regex" has invalid regex`,
			wantHint:   "regular expression",
		},
		{
			name: "http non loopback",
			path: "config.yaml",
			content: `
runtime:
  hooks:
    items:
      - id: http-remote
        point: before_tool_call
        scope: user
        kind: http
        mode: observe
        params:
          url: https://example.com/hook
`,
			wantSubstr: `error: kind http params.url host "example.com" is not allowed`,
			wantHint:   "loopback",
		},
		{
			name: "http fail_closed",
			path: "config.yaml",
			content: `
runtime:
  hooks:
    items:
      - id: http-fail-closed
        point: before_tool_call
        scope: user
        kind: http
        mode: observe
        failure_policy: fail_closed
        params:
          url: http://127.0.0.1:8080/hook
`,
			wantSubstr: `error: failure_policy "fail_closed" is not supported for kind http observe`,
			wantHint:   "warn_only or fail_open",
		},
		{
			name: "command params invalid",
			path: "hooks.yaml",
			content: `
hooks:
  items:
    - id: command-invalid
      point: before_tool_call
      scope: repo
      kind: command
      mode: sync
      params:
        command: echo hello
`,
			wantSubstr: `error: string params.command requires params.shell=true`,
			wantHint:   "argv array or string command",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, tc.path)
			if err := os.WriteFile(target, []byte(strings.TrimSpace(tc.content)+"\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(%s) error = %v", target, err)
			}

			command := NewRootCommand()
			buffer := &bytes.Buffer{}
			command.SetOut(buffer)
			command.SetErr(buffer)
			command.SetArgs([]string{"hook", "lint", target})
			err := command.ExecuteContext(context.Background())
			if err == nil {
				t.Fatal("expected lint findings")
			}
			exitErr, ok := err.(ExitCoder)
			if !ok {
				t.Fatalf("error type = %T, want ExitCoder", err)
			}
			if exitErr.ExitCode() != hookExitLintFindings {
				t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), hookExitLintFindings)
			}
			output := buffer.String()
			if !strings.Contains(output, tc.wantSubstr) {
				t.Fatalf("expected output to contain %q, got %q", tc.wantSubstr, output)
			}
			if !strings.Contains(output, "(hint: ") || !strings.Contains(output, tc.wantHint) {
				t.Fatalf("expected output hint to contain %q, got %q", tc.wantHint, output)
			}
			if !strings.Contains(output, target+":") {
				t.Fatalf("expected output to include file path, got %q", output)
			}
		})
	}
}

func TestHookLintCommandSkipsMissingDefaultFiles(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	homeDir := t.TempDir()
	workdir := t.TempDir()
	setHookTestHome(t, homeDir)

	command := NewRootCommand()
	buffer := &bytes.Buffer{}
	command.SetOut(buffer)
	command.SetErr(buffer)
	command.SetArgs([]string{"--workdir", workdir, "hook", "lint"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext(lint default) error = %v", err)
	}
	if !strings.Contains(buffer.String(), "hook lint passed") {
		t.Fatalf("expected success message, got %q", buffer.String())
	}
}

func TestHookLintTargetsRejectsDirectoriesAndMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	if _, err := lintHookTargets([]string{dir}); err == nil || !strings.Contains(err.Error(), "target is a directory") {
		t.Fatalf("expected directory target error, got %v", err)
	}

	path := filepath.Join(t.TempDir(), "hooks.yaml")
	if err := os.WriteFile(path, []byte("hooks: ["), 0o644); err != nil {
		t.Fatalf("WriteFile(bad yaml) error = %v", err)
	}
	if _, err := lintHookTargets([]string{path}); err == nil || !strings.Contains(err.Error(), "parse hook yaml") {
		t.Fatalf("expected parse hook yaml error, got %v", err)
	}
}

func TestHookLintHelpersCoverMissingMappingsAndDefaultHint(t *testing.T) {
	var emptyRoot yaml.Node
	if lines := collectHookItemLines(&emptyRoot, "repo"); lines != nil {
		t.Fatalf("empty root lines = %#v, want nil", lines)
	}
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Value: "not-a-map"}
	if got := findMappingValue(scalar, "hooks"); got != nil {
		t.Fatalf("findMappingValue(non-map) = %#v, want nil", got)
	}
	if got := hookLintHint("something else"); got != "fix the hook item so it matches current runtime hook schema" {
		t.Fatalf("hookLintHint(default) = %q", got)
	}
}

func TestResolveHookLintTargetsExplicitAndWorkspaceErrors(t *testing.T) {
	target := filepath.Join(t.TempDir(), "hooks.yaml")
	targets, err := resolveHookLintTargets("ignored", []string{target})
	if err != nil {
		t.Fatalf("resolveHookLintTargets(explicit) error = %v", err)
	}
	if len(targets) != 1 || targets[0] != target {
		t.Fatalf("targets = %#v, want explicit absolute path", targets)
	}

	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := resolveHookLintTargets(missing, nil); err == nil {
		t.Fatal("expected missing workspace error")
	}
}

func TestResolveHookWorkspaceUsesCurrentDirectory(t *testing.T) {
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir(temp) error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalCwd); err != nil {
			t.Fatalf("restore cwd error = %v", err)
		}
	})
	resolved, err := resolveHookWorkspace("")
	if err != nil {
		t.Fatalf("resolveHookWorkspace(empty) error = %v", err)
	}
	if resolved != tempDir {
		t.Fatalf("resolved = %q, want %q", resolved, tempDir)
	}
}

func TestHookDryRunCommandPassesBuiltInHook(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	homeDir := t.TempDir()
	setHookTestHome(t, homeDir)
	workdir := t.TempDir()

	writeUserHookConfig(t, homeDir, []config.RuntimeHookItemConfig{
		{
			ID:      "warn-bash",
			Enabled: boolPtrForHookTest(true),
			Point:   "before_tool_call",
			Scope:   "user",
			Kind:    "builtin",
			Mode:    "sync",
			Handler: "warn_on_tool_call",
			Match: map[string]any{
				"tool_name": "bash",
			},
			Params: map[string]any{
				"message": "bash warned",
			},
		},
	})

	fixturePath := writeHookFixture(t, "fixture.yaml", `
payload_version: "1"
point: before_tool_call
run_id: run-1
session_id: session-1
metadata:
  tool_name: bash
  tool_call_id: call-1
  tool_arguments_preview: echo hello
  workdir: `+workdir+`
`)

	command := NewRootCommand()
	buffer := &bytes.Buffer{}
	command.SetOut(buffer)
	command.SetErr(buffer)
	command.SetArgs([]string{"--workdir", workdir, "hook", "dry-run", "--hook", "warn-bash", "--fixture", fixturePath})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext(dry-run) error = %v", err)
	}
	output := buffer.String()
	if !strings.Contains(output, "status: pass") {
		t.Fatalf("expected pass output, got %q", output)
	}
	if !strings.Contains(output, "message: bash warned") {
		t.Fatalf("expected hook message in output, got %q", output)
	}
}

func TestHookDryRunCommandSupportsBuiltinHandlersAndExitCodes(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	homeDir := t.TempDir()
	setHookTestHome(t, homeDir)
	workdir := t.TempDir()
	requiredFile := filepath.Join(workdir, "README.md")
	if err := os.WriteFile(requiredFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile(required file) error = %v", err)
	}

	writeUserHookConfig(t, homeDir, []config.RuntimeHookItemConfig{
		{
			ID:      "require-readme",
			Enabled: boolPtrForHookTest(true),
			Point:   "before_tool_call",
			Scope:   "user",
			Kind:    "builtin",
			Mode:    "sync",
			Handler: "require_file_exists",
			Params: map[string]any{
				"path": "README.md",
			},
		},
		{
			ID:      "note-context",
			Enabled: boolPtrForHookTest(true),
			Point:   "session_start",
			Scope:   "user",
			Kind:    "builtin",
			Mode:    "sync",
			Handler: "add_context_note",
			Params: map[string]any{
				"note": "remember this context",
			},
		},
		{
			ID:      "block-tool",
			Enabled: boolPtrForHookTest(true),
			Point:   "before_tool_call",
			Scope:   "user",
			Kind:    "command",
			Mode:    "sync",
			Params:  commandHookExitParamsForTest(1),
		},
		{
			ID:      "fail-tool",
			Enabled: boolPtrForHookTest(true),
			Point:   "before_tool_call",
			Scope:   "user",
			Kind:    "command",
			Mode:    "sync",
			Params:  commandHookExitParamsForTest(9),
		},
	})

	beforeToolFixture := writeHookFixture(t, "before-tool.yaml", `
payload_version: "1"
point: before_tool_call
run_id: run-1
session_id: session-1
metadata:
  tool_name: bash
  tool_call_id: call-1
  tool_arguments_preview: echo hello
  workdir: `+workdir+`
`)
	sessionStartFixture := writeHookFixture(t, "session-start.json", `{"payload_version":"1","point":"session_start","run_id":"run-2","session_id":"session-2","metadata":{"run_id":"run-2","session_id":"session-2","workdir":"`+filepath.ToSlash(workdir)+`"}}`)

	t.Run("require_file_exists passes", func(t *testing.T) {
		output, err := runHookCommand(t, workdir, []string{"hook", "dry-run", "--hook", "require-readme", "--fixture", beforeToolFixture})
		if err != nil {
			t.Fatalf("runHookCommand(require-readme) error = %v", err)
		}
		if !strings.Contains(output, "status: pass") {
			t.Fatalf("expected pass output, got %q", output)
		}
	})

	t.Run("add_context_note passes with json fixture", func(t *testing.T) {
		output, err := runHookCommand(t, workdir, []string{"hook", "dry-run", "--hook", "note-context", "--fixture", sessionStartFixture})
		if err != nil {
			t.Fatalf("runHookCommand(note-context) error = %v", err)
		}
		if !strings.Contains(output, "message: remember this context") {
			t.Fatalf("expected note output, got %q", output)
		}
	})

	t.Run("command block exits with 3", func(t *testing.T) {
		output, err := runHookCommand(t, workdir, []string{"hook", "dry-run", "--hook", "block-tool", "--fixture", beforeToolFixture})
		if err == nil {
			t.Fatal("expected block exit error")
		}
		exitErr, ok := err.(ExitCoder)
		if !ok {
			t.Fatalf("error type = %T, want ExitCoder", err)
		}
		if exitErr.ExitCode() != hookExitHookBlocked {
			t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), hookExitHookBlocked)
		}
		if !strings.Contains(output, "status: block") || !strings.Contains(output, "block: true") {
			t.Fatalf("expected block output, got %q", output)
		}
	})

	t.Run("command failure exits with 4", func(t *testing.T) {
		output, err := runHookCommand(t, workdir, []string{"hook", "dry-run", "--hook", "fail-tool", "--fixture", beforeToolFixture})
		if err == nil {
			t.Fatal("expected failure exit error")
		}
		exitErr, ok := err.(ExitCoder)
		if !ok {
			t.Fatalf("error type = %T, want ExitCoder", err)
		}
		if exitErr.ExitCode() != hookExitHookFailed {
			t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), hookExitHookFailed)
		}
		if !strings.Contains(output, "status: failed") {
			t.Fatalf("expected failed output, got %q", output)
		}
	})
}

func TestHookDryRunCommandSkipsMissingDefaultUserConfigWhenRepoHookExists(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	homeDir := t.TempDir()
	setHookTestHome(t, homeDir)
	workdir := t.TempDir()

	repoHooksPath := filepath.Join(workdir, ".neocode", "hooks.yaml")
	if err := os.MkdirAll(filepath.Dir(repoHooksPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(repo hooks dir) error = %v", err)
	}
	repoHooks := `
hooks:
  items:
    - id: repo-only
      point: before_tool_call
      handler: warn_on_tool_call
      match:
        tool_name: bash
      params:
        message: from repo only
`
	if err := os.WriteFile(repoHooksPath, []byte(strings.TrimSpace(repoHooks)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(repo hooks) error = %v", err)
	}

	fixturePath := writeHookFixture(t, "fixture.yaml", `
payload_version: "1"
point: before_tool_call
metadata:
  tool_name: bash
  tool_call_id: call-1
  tool_arguments_preview: echo hello
  workdir: `+workdir+`
`)

	output, err := runHookCommand(t, workdir, []string{"hook", "dry-run", "--repo", "--hook", "repo-only", "--fixture", fixturePath})
	if err != nil {
		t.Fatalf("repo-only dry-run error = %v", err)
	}
	if !strings.Contains(output, "message: from repo only") {
		t.Fatalf("expected repo-only output, got %q", output)
	}
}

func TestHookLintCommandUsesRepoDefaults(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	repoHooksPath := filepath.Join(t.TempDir(), "hooks.yaml")
	repoHooks := `
hooks:
  items:
    - id: repo-defaults
      point: before_tool_call
      handler: warn_on_tool_call
      match:
        tool_name: bash
`
	if err := os.WriteFile(repoHooksPath, []byte(strings.TrimSpace(repoHooks)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(repo hooks) error = %v", err)
	}

	output, err := runHookCommand(t, t.TempDir(), []string{"hook", "lint", repoHooksPath})
	if err != nil {
		t.Fatalf("repo lint error = %v, output = %q", err, output)
	}
	if !strings.Contains(output, "hook lint passed") {
		t.Fatalf("expected lint pass output, got %q", output)
	}
}

func TestHookDryRunCommandRejectsInvalidFixtureAndPointMismatch(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	homeDir := t.TempDir()
	setHookTestHome(t, homeDir)
	workdir := t.TempDir()

	writeUserHookConfig(t, homeDir, []config.RuntimeHookItemConfig{
		{
			ID:      "warn-bash",
			Enabled: boolPtrForHookTest(true),
			Point:   "before_tool_call",
			Scope:   "user",
			Kind:    "builtin",
			Mode:    "sync",
			Handler: "warn_on_tool_call",
			Match: map[string]any{
				"tool_name": "bash",
			},
		},
	})

	cases := []struct {
		name       string
		content    string
		fileName   string
		wantSubstr string
	}{
		{
			name:     "bad payload version",
			fileName: "bad-version.yaml",
			content: `
payload_version: "9"
point: before_tool_call
metadata:
  tool_name: bash
  tool_call_id: call-1
`,
			wantSubstr: "payload_version",
		},
		{
			name:     "unknown metadata field",
			fileName: "bad-metadata.yaml",
			content: `
payload_version: "1"
point: before_tool_call
metadata:
  unknown_field: value
`,
			wantSubstr: "unknown field",
		},
		{
			name:     "point mismatch",
			fileName: "point-mismatch.yaml",
			content: `
payload_version: "1"
point: session_start
metadata:
  run_id: run-1
  session_id: session-1
  workdir: ` + workdir + `
`,
			wantSubstr: "does not match hook",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixturePath := writeHookFixture(t, tc.fileName, tc.content)
			_, err := runHookCommand(t, workdir, []string{"hook", "dry-run", "--hook", "warn-bash", "--fixture", fixturePath})
			if err == nil {
				t.Fatal("expected dry-run system error")
			}
			exitErr, ok := err.(ExitCoder)
			if !ok {
				t.Fatalf("error type = %T, want ExitCoder", err)
			}
			if exitErr.ExitCode() != hookExitSystemError {
				t.Fatalf("exit code = %d, want %d", exitErr.ExitCode(), hookExitSystemError)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("expected error to contain %q, got %v", tc.wantSubstr, err)
			}
		})
	}
}

func TestHookDryRunPrefersUserHookUnlessRepoRequested(t *testing.T) {
	runGlobalPreload = func(context.Context) error { return nil }
	t.Cleanup(func() { runGlobalPreload = defaultGlobalPreload })

	homeDir := t.TempDir()
	setHookTestHome(t, homeDir)
	workdir := t.TempDir()
	trustStorePath := filepath.Join(homeDir, ".neocode", "trusted-workspaces.json")
	if err := os.MkdirAll(filepath.Dir(trustStorePath), 0o755); err != nil {
		t.Fatalf("MkdirAll(trust store dir) error = %v", err)
	}
	trustStore := `{"version":1,"workspaces":["` + strings.ReplaceAll(filepath.Clean(workdir), `\`, `\\`) + `"]}`
	if err := os.WriteFile(trustStorePath, []byte(trustStore), 0o644); err != nil {
		t.Fatalf("WriteFile(trust store) error = %v", err)
	}

	writeUserHookConfig(t, homeDir, []config.RuntimeHookItemConfig{
		{
			ID:      "same-id",
			Enabled: boolPtrForHookTest(true),
			Point:   "before_tool_call",
			Scope:   "user",
			Kind:    "builtin",
			Mode:    "sync",
			Handler: "warn_on_tool_call",
			Match: map[string]any{
				"tool_name": "bash",
			},
			Params: map[string]any{
				"message": "from user",
			},
		},
	})
	repoHooksPath := filepath.Join(workdir, ".neocode", "hooks.yaml")
	if err := os.MkdirAll(filepath.Dir(repoHooksPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(repo hooks dir) error = %v", err)
	}
	repoHooks := `
hooks:
  items:
    - id: same-id
      point: before_tool_call
      scope: repo
      kind: builtin
      mode: sync
      handler: warn_on_tool_call
      match:
        tool_name: bash
      params:
        message: from repo
`
	if err := os.WriteFile(repoHooksPath, []byte(strings.TrimSpace(repoHooks)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(repo hooks) error = %v", err)
	}

	fixturePath := writeHookFixture(t, "fixture.yaml", `
payload_version: "1"
point: before_tool_call
metadata:
  tool_name: bash
  tool_call_id: call-1
  tool_arguments_preview: echo hello
  workdir: `+workdir+`
`)

	output, err := runHookCommand(t, workdir, []string{"hook", "dry-run", "--hook", "same-id", "--fixture", fixturePath})
	if err != nil {
		t.Fatalf("user-preferred dry-run error = %v", err)
	}
	if !strings.Contains(output, "message: from user") {
		t.Fatalf("expected user hook output, got %q", output)
	}

	output, err = runHookCommand(t, workdir, []string{"hook", "dry-run", "--repo", "--hook", "same-id", "--fixture", fixturePath})
	if err != nil {
		t.Fatalf("repo dry-run error = %v", err)
	}
	if !strings.Contains(output, "message: from repo") {
		t.Fatalf("expected repo hook output, got %q", output)
	}

	output, err = runHookCommand(t, workdir, []string{"hook", "dry-run", repoHooksPath, "--hook", "same-id", "--fixture", fixturePath})
	if err != nil {
		t.Fatalf("explicit-path dry-run error = %v", err)
	}
	if !strings.Contains(output, "message: from repo") {
		t.Fatalf("expected explicit repo path output, got %q", output)
	}
}

func TestResolveHookCandidateReportsMissingHooks(t *testing.T) {
	homeDir := t.TempDir()
	setHookTestHome(t, homeDir)
	workdir := t.TempDir()

	if _, err := resolveHookCandidate(workdir, "missing", false, ""); err == nil || !strings.Contains(err.Error(), `hook "missing" not found`) {
		t.Fatalf("expected missing hook error, got %v", err)
	}
	if _, err := resolveHookCandidate(workdir, "missing", true, ""); err == nil || !strings.Contains(err.Error(), `repo hook "missing" not found`) {
		t.Fatalf("expected missing repo hook error, got %v", err)
	}
}

func TestBuildHookSpecForCandidateDefaultsWorkdirToConfigDir(t *testing.T) {
	dir := t.TempDir()
	candidate := hookCandidate{
		Path:  filepath.Join(dir, "config.yaml"),
		Scope: "user",
		Item: config.RuntimeHookItemConfig{
			ID:      "note",
			Enabled: boolPtrForHookTest(true),
			Point:   "session_start",
			Scope:   "user",
			Kind:    "builtin",
			Mode:    "sync",
			Handler: "add_context_note",
			Params: map[string]any{
				"note": "hello",
			},
		},
	}
	spec, err := buildHookSpecForCandidate(candidate, "")
	if err != nil {
		t.Fatalf("buildHookSpecForCandidate() error = %v", err)
	}
	if string(spec.Point) != "session_start" || spec.ID != "note" {
		t.Fatalf("spec = %#v, want compiled note hook", spec)
	}
}

func runHookCommand(t *testing.T, workdir string, args []string) (string, error) {
	t.Helper()
	command := NewRootCommand()
	buffer := &bytes.Buffer{}
	command.SetOut(buffer)
	command.SetErr(buffer)
	command.SetArgs(append([]string{"--workdir", workdir}, args...))
	err := command.ExecuteContext(context.Background())
	return buffer.String(), err
}

func writeHookFixture(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
	return path
}

func writeUserHookConfig(t *testing.T, homeDir string, items []config.RuntimeHookItemConfig) {
	t.Helper()
	cfgLoader := config.NewLoader(filepath.Join(homeDir, ".neocode"), config.StaticDefaults())
	cfg := cfgLoader.DefaultConfig()
	cfg.Runtime.Hooks.Items = items
	if err := cfgLoader.Save(context.Background(), &cfg); err != nil {
		t.Fatalf("Save(config) error = %v", err)
	}
}

func setHookTestHome(t *testing.T, homeDir string) {
	t.Helper()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
}

func boolPtrForHookTest(value bool) *bool {
	return &value
}

func commandHookExitParamsForTest(code int) map[string]any {
	return map[string]any{
		"command": "exit " + strconv.Itoa(code),
		"shell":   true,
	}
}
