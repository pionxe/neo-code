package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/config"
	agentruntime "neo-code/internal/runtime"
)

type stubRunner struct {
	lastInput agentruntime.UserInput
	err       error
}

func (s *stubRunner) Run(ctx context.Context, input agentruntime.UserInput) error {
	s.lastInput = input
	return s.err
}

type stubCompactor struct {
	lastInput agentruntime.CompactInput
	err       error
}

func (s *stubCompactor) Compact(ctx context.Context, input agentruntime.CompactInput) (agentruntime.CompactResult, error) {
	s.lastInput = input
	return agentruntime.CompactResult{}, s.err
}

type stubProvider struct {
	selection config.ProviderSelection
	models    []config.ModelDescriptor
	err       error
}

func (s *stubProvider) SelectProvider(ctx context.Context, providerID string) (config.ProviderSelection, error) {
	return s.selection, s.err
}

func (s *stubProvider) SetCurrentModel(ctx context.Context, modelID string) (config.ProviderSelection, error) {
	return s.selection, s.err
}

func (s *stubProvider) ListModels(ctx context.Context) ([]config.ModelDescriptor, error) {
	return s.models, s.err
}

func TestListenForRuntimeEventCmd(t *testing.T) {
	ch := make(chan agentruntime.RuntimeEvent, 1)
	event := agentruntime.RuntimeEvent{Type: agentruntime.EventUserMessage}
	ch <- event

	msg := ListenForRuntimeEventCmd(
		ch,
		func(e agentruntime.RuntimeEvent) tea.Msg { return e },
		func() tea.Msg { return "closed" },
	)()
	got, ok := msg.(agentruntime.RuntimeEvent)
	if !ok || got.Type != agentruntime.EventUserMessage {
		t.Fatalf("expected runtime event msg, got %T %#v", msg, msg)
	}

	close(ch)
	msg = ListenForRuntimeEventCmd(
		ch,
		func(e agentruntime.RuntimeEvent) tea.Msg { return e },
		func() tea.Msg { return "closed" },
	)()
	if gotClosed, ok := msg.(string); !ok || gotClosed != "closed" {
		t.Fatalf("expected closed msg, got %T %#v", msg, msg)
	}
}

func TestRunAgentCmd(t *testing.T) {
	runner := &stubRunner{err: errors.New("boom")}
	input := agentruntime.UserInput{SessionID: "s1", Content: "hello", Workdir: "D:/"}
	msg := RunAgentCmd(runner, input, func(err error) tea.Msg { return err })()
	if runner.lastInput.SessionID != "s1" || runner.lastInput.Content != "hello" {
		t.Fatalf("unexpected runner input: %+v", runner.lastInput)
	}
	if err, ok := msg.(error); !ok || err == nil || err.Error() != "boom" {
		t.Fatalf("expected forwarded error message, got %T %#v", msg, msg)
	}
}

func TestRunCompactCmd(t *testing.T) {
	compactor := &stubCompactor{err: errors.New("compact failed")}
	input := agentruntime.CompactInput{SessionID: "s2"}
	msg := RunCompactCmd(compactor, input, func(err error) tea.Msg { return err })()
	if compactor.lastInput.SessionID != "s2" {
		t.Fatalf("unexpected compact input: %+v", compactor.lastInput)
	}
	if err, ok := msg.(error); !ok || err == nil || err.Error() != "compact failed" {
		t.Fatalf("expected forwarded compact error, got %T %#v", msg, msg)
	}
}

func TestProviderCmds(t *testing.T) {
	svc := &stubProvider{
		selection: config.ProviderSelection{ProviderID: "openai", ModelID: "gpt-5.4"},
		models:    []config.ModelDescriptor{{ID: "gpt-5.4", Name: "GPT-5.4"}},
	}

	msg := SelectProviderCmd(svc, "openai", func(sel config.ProviderSelection, err error) tea.Msg { return sel })()
	if sel, ok := msg.(config.ProviderSelection); !ok || sel.ProviderID != "openai" {
		t.Fatalf("expected provider selection msg, got %T %#v", msg, msg)
	}

	msg = SelectModelCmd(svc, "gpt-5.4", func(sel config.ProviderSelection, err error) tea.Msg { return sel })()
	if sel, ok := msg.(config.ProviderSelection); !ok || sel.ModelID != "gpt-5.4" {
		t.Fatalf("expected model selection msg, got %T %#v", msg, msg)
	}

	msg = RefreshModelCatalogCmd(
		svc,
		"openai",
		func(providerID string, models []config.ModelDescriptor, err error) tea.Msg {
			return providerID + ":" + models[0].ID
		},
	)()
	if got, ok := msg.(string); !ok || got != "openai:gpt-5.4" {
		t.Fatalf("expected catalog refresh msg, got %T %#v", msg, msg)
	}

	if cmd := RefreshModelCatalogCmd(svc, "", func(providerID string, models []config.ModelDescriptor, err error) tea.Msg { return nil }); cmd != nil {
		t.Fatalf("expected nil cmd for empty provider id")
	}
}

func TestCommandCmds(t *testing.T) {
	localMsg := RunLocalCommandCmd(
		func(ctx context.Context) (string, error) { return "ok", nil },
		func(notice string, err error) tea.Msg { return notice },
	)()
	if got, ok := localMsg.(string); !ok || got != "ok" {
		t.Fatalf("expected local command notice msg, got %T %#v", localMsg, localMsg)
	}

	workspaceMsg := RunWorkspaceCommandCmd(
		func(ctx context.Context) (string, string, error) { return "git status", "clean", nil },
		func(command string, output string, err error) tea.Msg { return command + ":" + output },
	)()
	if got, ok := workspaceMsg.(string); !ok || got != "git status:clean" {
		t.Fatalf("expected workspace command msg, got %T %#v", workspaceMsg, workspaceMsg)
	}
}

func TestFileServices(t *testing.T) {
	matches := SuggestFileMatches("int", []string{
		"README.md",
		"internal/tui/update.go",
		"docs/internal-arch.md",
	}, 2)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d (%v)", len(matches), matches)
	}
	if matches[0] != "internal/tui/update.go" {
		t.Fatalf("expected prefix match first, got %v", matches)
	}

	root := t.TempDir()
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	files, err := CollectWorkspaceFiles(root, 10)
	if err != nil {
		t.Fatalf("CollectWorkspaceFiles() error = %v", err)
	}
	if len(files) == 0 || files[0] != "a.txt" {
		t.Fatalf("unexpected collected files: %v", files)
	}

	if resolved := ResolveWorkspaceDirectory(root); resolved == "" {
		t.Fatalf("expected resolved workspace directory")
	}
	if resolved := ResolveWorkspaceDirectory(""); resolved != "" {
		t.Fatalf("expected empty resolved path for blank input, got %q", resolved)
	}
}

func TestFileServiceEdgeCases(t *testing.T) {
	if got := SuggestFileMatches("x", nil, 2); got != nil {
		t.Fatalf("expected nil for nil candidates, got %v", got)
	}
	if got := SuggestFileMatches("x", []string{"a"}, 0); got != nil {
		t.Fatalf("expected nil for zero limit, got %v", got)
	}
	if got := SuggestFileMatches("", []string{"a", "b"}, 2); len(got) != 2 {
		t.Fatalf("expected empty query to match all as prefix, got %v", got)
	}
	if got := SuggestFileMatches("mid", []string{"abc_mid_def"}, 2); len(got) != 1 {
		t.Fatalf("expected contains match, got %v", got)
	}

	if resolved := ResolveWorkspaceDirectory("\x00"); resolved != "" {
		t.Fatalf("expected empty for NUL path, got %q", resolved)
	}
	if resolved := ResolveWorkspaceDirectory("   "); resolved != "" {
		t.Fatalf("expected empty for whitespace-only path, got %q", resolved)
	}
}

func TestParseRunContextPayload(t *testing.T) {
	if _, ok := ParseRunContextPayload(42); ok {
		t.Fatalf("expected unknown type to fail")
	}
	if _, ok := ParseRunContextPayload((*RuntimeRunContextPayload)(nil)); ok {
		t.Fatalf("expected nil pointer to fail")
	}

	out, ok := ParseRunContextPayload(RuntimeRunContextPayload{Provider: "openai", Model: "gpt-4"})
	if !ok || out.Provider != "openai" || out.Model != "gpt-4" {
		t.Fatalf("expected struct parse, got %+v ok=%v", out, ok)
	}

	ptr := &RuntimeRunContextPayload{Provider: "anthropic", Model: "claude"}
	out, ok = ParseRunContextPayload(ptr)
	if !ok || out.Provider != "anthropic" {
		t.Fatalf("expected pointer parse, got %+v ok=%v", out, ok)
	}

	m := map[string]any{"Provider": "openai", "Model": "gpt-5", "Workdir": "/tmp", "Mode": "agent"}
	out, ok = ParseRunContextPayload(m)
	if !ok || out.Provider != "openai" || out.Model != "gpt-5" || out.Workdir != "/tmp" || out.Mode != "agent" {
		t.Fatalf("expected map parse, got %+v ok=%v", out, ok)
	}

	empty, ok := ParseRunContextPayload(map[string]any{})
	if ok {
		t.Fatalf("expected all-empty fields to fail, got %+v", empty)
	}
}

func TestParseToolStatusPayload(t *testing.T) {
	if _, ok := ParseToolStatusPayload(42); ok {
		t.Fatalf("expected unknown type to fail")
	}
	if _, ok := ParseToolStatusPayload((*RuntimeToolStatusPayload)(nil)); ok {
		t.Fatalf("expected nil pointer to fail")
	}

	out, ok := ParseToolStatusPayload(RuntimeToolStatusPayload{ToolCallID: "tc1", ToolName: "bash", Status: "succeeded"})
	if !ok || out.ToolCallID != "tc1" || out.ToolName != "bash" {
		t.Fatalf("expected struct parse, got %+v ok=%v", out, ok)
	}

	ptr := &RuntimeToolStatusPayload{ToolCallID: "tc2", ToolName: "read", Status: "running"}
	out, ok = ParseToolStatusPayload(ptr)
	if !ok || out.ToolCallID != "tc2" {
		t.Fatalf("expected pointer parse, got %+v ok=%v", out, ok)
	}

	m := map[string]any{"ToolCallID": "tc3", "ToolName": "write", "Status": "planned", "Message": "msg", "DurationMS": int64(100)}
	out, ok = ParseToolStatusPayload(m)
	if !ok || out.ToolCallID != "tc3" || out.ToolName != "write" || out.Status != "planned" || out.DurationMS != 100 {
		t.Fatalf("expected map parse, got %+v ok=%v", out, ok)
	}

	empty, ok := ParseToolStatusPayload(map[string]any{"Status": "running"})
	if ok {
		t.Fatalf("expected empty ToolCallID+ToolName to fail, got %+v", empty)
	}
}

func TestParseUsagePayload(t *testing.T) {
	if _, ok := ParseUsagePayload(42); ok {
		t.Fatalf("expected unknown type to fail")
	}
	if _, ok := ParseUsagePayload((*RuntimeUsagePayload)(nil)); ok {
		t.Fatalf("expected nil pointer to fail")
	}

	out, ok := ParseUsagePayload(RuntimeUsagePayload{Delta: RuntimeUsageSnapshot{InputTokens: 10}})
	if !ok || out.Delta.InputTokens != 10 {
		t.Fatalf("expected struct parse, got %+v ok=%v", out, ok)
	}

	m := map[string]any{
		"Delta":   map[string]any{"InputTokens": 5, "OutputTokens": 3, "TotalTokens": 8},
		"Run":     RuntimeUsageSnapshot{InputTokens: 100},
		"Session": &RuntimeUsageSnapshot{InputTokens: 200},
	}
	out, ok = ParseUsagePayload(m)
	if !ok || out.Delta.InputTokens != 5 || out.Run.InputTokens != 100 || out.Session.InputTokens != 200 {
		t.Fatalf("expected map parse, got %+v ok=%v", out, ok)
	}

	empty, ok := ParseUsagePayload(map[string]any{})
	if ok {
		t.Fatalf("expected all-empty to fail, got %+v", empty)
	}
}

func TestParseSessionContextSnapshot(t *testing.T) {
	if _, ok := ParseSessionContextSnapshot(42); ok {
		t.Fatalf("expected unknown type to fail")
	}
	if _, ok := ParseSessionContextSnapshot((*RuntimeSessionContextSnapshot)(nil)); ok {
		t.Fatalf("expected nil pointer to fail")
	}

	out, ok := ParseSessionContextSnapshot(RuntimeSessionContextSnapshot{SessionID: "s1", Provider: "openai"})
	if !ok || out.SessionID != "s1" {
		t.Fatalf("expected struct parse, got %+v ok=%v", out, ok)
	}

	ptr := &RuntimeSessionContextSnapshot{SessionID: "s2", Model: "gpt-4"}
	out, ok = ParseSessionContextSnapshot(ptr)
	if !ok || out.SessionID != "s2" {
		t.Fatalf("expected pointer parse, got %+v ok=%v", out, ok)
	}

	m := map[string]any{"SessionID": "s3", "Provider": "anthropic", "Model": "claude", "Workdir": "/tmp", "Mode": "agent"}
	out, ok = ParseSessionContextSnapshot(m)
	if !ok || out.SessionID != "s3" || out.Provider != "anthropic" {
		t.Fatalf("expected map parse, got %+v ok=%v", out, ok)
	}

	empty, ok := ParseSessionContextSnapshot(map[string]any{"Mode": "agent"})
	if ok {
		t.Fatalf("expected empty SessionID+Provider+Workdir to fail, got %+v", empty)
	}
}

func TestParseRunSnapshot(t *testing.T) {
	if _, ok := ParseRunSnapshot(42); ok {
		t.Fatalf("expected unknown type to fail")
	}
	if _, ok := ParseRunSnapshot((*RuntimeRunSnapshot)(nil)); ok {
		t.Fatalf("expected nil pointer to fail")
	}

	out, ok := ParseRunSnapshot(RuntimeRunSnapshot{RunID: "r1", SessionID: "s1"})
	if !ok || out.RunID != "r1" {
		t.Fatalf("expected struct parse, got %+v ok=%v", out, ok)
	}

	m := map[string]any{
		"RunID":     "r2",
		"SessionID": "s2",
		"Context":   map[string]any{"RunID": "cr1", "SessionID": "cs1", "Provider": "openai", "Model": "gpt-4", "Workdir": "/tmp", "Mode": "agent"},
		"ToolStates": []any{
			map[string]any{"ToolCallID": "tc1", "ToolName": "bash", "Status": "succeeded", "Message": "ok", "DurationMS": int64(50)},
		},
		"Usage":        map[string]any{"InputTokens": 10, "OutputTokens": 20, "TotalTokens": 30},
		"SessionUsage": RuntimeUsageSnapshot{InputTokens: 100},
	}
	out, ok = ParseRunSnapshot(m)
	if !ok || out.RunID != "r2" || out.SessionID != "s2" {
		t.Fatalf("expected map parse, got %+v ok=%v", out, ok)
	}
	if out.Context.Provider != "openai" || out.Context.Model != "gpt-4" {
		t.Fatalf("expected context parsed, got %+v", out.Context)
	}
	if len(out.ToolStates) != 1 || out.ToolStates[0].ToolCallID != "tc1" {
		t.Fatalf("expected tool states parsed, got %v", out.ToolStates)
	}
	if out.Usage.InputTokens != 10 || out.SessionUsage.InputTokens != 100 {
		t.Fatalf("expected usage parsed, got %+v %+v", out.Usage, out.SessionUsage)
	}

	empty, ok := ParseRunSnapshot(map[string]any{})
	if ok {
		t.Fatalf("expected empty RunID+SessionID to fail, got %+v", empty)
	}
}

func TestParseUsageSnapshot(t *testing.T) {
	out, ok := ParseUsageSnapshot(RuntimeUsageSnapshot{InputTokens: 42})
	if !ok || out.InputTokens != 42 {
		t.Fatalf("expected struct parse, got %+v ok=%v", out, ok)
	}

	ptr := &RuntimeUsageSnapshot{OutputTokens: 99}
	out, ok = ParseUsageSnapshot(ptr)
	if !ok || out.OutputTokens != 99 {
		t.Fatalf("expected pointer parse, got %+v ok=%v", out, ok)
	}

	m := map[string]any{"InputTokens": 10, "OutputTokens": 20, "TotalTokens": 30}
	out, ok = ParseUsageSnapshot(m)
	if !ok || out.InputTokens != 10 || out.TotalTokens != 30 {
		t.Fatalf("expected map parse, got %+v ok=%v", out, ok)
	}

	if _, ok := ParseUsageSnapshot(map[string]any{}); ok {
		t.Fatalf("expected empty to fail")
	}
	if _, ok := ParseUsageSnapshot(42); ok {
		t.Fatalf("expected unknown type to fail")
	}
}

func TestMapFunctions(t *testing.T) {
	ctx := MapRunContextPayload("r1", "s1", RuntimeRunContextPayload{Provider: "openai", Model: "gpt-4", Workdir: "/tmp", Mode: "agent"})
	if ctx.RunID != "r1" || ctx.SessionID != "s1" || ctx.Provider != "openai" {
		t.Fatalf("unexpected context: %+v", ctx)
	}

	snap := RuntimeSessionContextSnapshot{SessionID: "s1", Provider: "anthropic", Model: "claude", Workdir: "/home", Mode: "plan"}
	ctx = MapSessionContextSnapshot(snap)
	if ctx.SessionID != "s1" || ctx.Provider != "anthropic" {
		t.Fatalf("unexpected session context: %+v", ctx)
	}

	tool := MapToolStatusPayload(RuntimeToolStatusPayload{ToolCallID: "tc1", ToolName: "bash", Status: "succeeded", Message: "done", DurationMS: 100})
	if tool.ToolCallID != "tc1" || tool.ToolName != "bash" {
		t.Fatalf("unexpected tool: %+v", tool)
	}

	usage := MapUsagePayload(RuntimeUsagePayload{
		Run:     RuntimeUsageSnapshot{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
		Session: RuntimeUsageSnapshot{InputTokens: 100, OutputTokens: 200, TotalTokens: 300},
	})
	if usage.RunInputTokens != 10 || usage.SessionInputTokens != 100 {
		t.Fatalf("unexpected usage: %+v", usage)
	}

	current := TokenUsageVM{RunInputTokens: 5, SessionInputTokens: 50}
	updated := MapUsageSnapshot(RuntimeUsageSnapshot{InputTokens: 999, OutputTokens: 888, TotalTokens: 777}, current)
	if updated.SessionInputTokens != 999 || updated.RunInputTokens != 5 {
		t.Fatalf("expected session updated but run preserved, got %+v", updated)
	}
}

func TestMapRunSnapshotDetailed(t *testing.T) {
	snap := RuntimeRunSnapshot{
		RunID:     "r1",
		SessionID: "s1",
		Context:   RuntimeRunContextSnapshot{Provider: "openai", Model: "gpt-4", Workdir: "/tmp", Mode: "agent"},
		ToolStates: []RuntimeToolStateSnapshot{
			{ToolCallID: "tc1", ToolName: "bash", Status: "succeeded", Message: "ok", DurationMS: 50},
		},
		Usage:        RuntimeUsageSnapshot{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
		SessionUsage: RuntimeUsageSnapshot{InputTokens: 100, OutputTokens: 200, TotalTokens: 300},
	}
	ctx, tools, usage := MapRunSnapshot(snap)
	if ctx.RunID != "r1" || ctx.Provider != "openai" {
		t.Fatalf("unexpected context: %+v", ctx)
	}
	if len(tools) != 1 || tools[0].ToolCallID != "tc1" {
		t.Fatalf("unexpected tools: %+v", tools)
	}
	if usage.RunInputTokens != 10 || usage.SessionInputTokens != 100 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestMapToolLifecycleStatus(t *testing.T) {
	for _, tc := range []struct {
		input    string
		expected string
	}{
		{"planned", "planned"},
		{"running", "running"},
		{"succeeded", "succeeded"},
		{"failed", "failed"},
		{"", "running"},
		{"unknown", "running"},
		{"  SUCCEEDED  ", "succeeded"},
	} {
		payload := RuntimeToolStatusPayload{ToolCallID: "tc1", ToolName: "bash", Status: tc.input}
		result := MapToolStatusPayload(payload)
		if string(result.Status) != tc.expected {
			t.Fatalf("status %q -> expected %q, got %q", tc.input, tc.expected, result.Status)
		}
	}
}

func TestMergeToolStates(t *testing.T) {
	existing := []ToolStateVM{{ToolCallID: "tc1", ToolName: "bash", Status: "running"}}
	incoming := ToolStateVM{ToolCallID: "tc1", ToolName: "bash", Status: "succeeded"}
	merged := MergeToolStates(existing, incoming, 10)
	if len(merged) != 1 || merged[0].Status != "succeeded" {
		t.Fatalf("expected update, got %+v", merged)
	}

	// Append new tool.
	incoming2 := ToolStateVM{ToolCallID: "tc2", ToolName: "read", Status: "planned"}
	merged = MergeToolStates(merged, incoming2, 10)
	if len(merged) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(merged))
	}

	// Limit enforcement.
	merged = MergeToolStates(merged, ToolStateVM{ToolCallID: "tc3", ToolName: "write", Status: "running"}, 2)
	if len(merged) != 2 {
		t.Fatalf("expected limit enforcement, got %d", len(merged))
	}

	// Default limit.
	merged = MergeToolStates(nil, incoming, 0)
	if len(merged) != 1 {
		t.Fatalf("expected default limit to work, got %d", len(merged))
	}
}

func TestReadMapHelpers(t *testing.T) {
	m := map[string]any{
		"IntVal":   42,
		"Int64Val": int64(99),
		"FloatVal": float64(3.14),
		"StrInt":   "123",
		"StrBad":   "not-a-number",
		"TimeVal":  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if got := readMapInt(m, "IntVal"); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if got := readMapInt(m, "Int64Val"); got != 99 {
		t.Fatalf("expected 99, got %d", got)
	}
	if got := readMapInt(m, "FloatVal"); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
	if got := readMapInt(m, "StrInt"); got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
	if got := readMapInt(m, "StrBad"); got != 0 {
		t.Fatalf("expected 0 for bad string, got %d", got)
	}
	if got := readMapInt(m, "Missing"); got != 0 {
		t.Fatalf("expected 0 for missing key, got %d", got)
	}

	if got := readMapInt64(m, "IntVal"); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if got := readMapInt64(m, "Int64Val"); got != 99 {
		t.Fatalf("expected 99, got %d", got)
	}
	if got := readMapInt64(m, "StrInt"); got != 123 {
		t.Fatalf("expected 123, got %d", got)
	}
	if got := readMapInt64(m, "StrBad"); got != 0 {
		t.Fatalf("expected 0 for bad string, got %d", got)
	}

	if got := readMapTime(m, "TimeVal"); got.Year() != 2026 {
		t.Fatalf("expected 2026, got %v", got)
	}
	if got := readMapTime(m, "Missing"); !got.IsZero() {
		t.Fatalf("expected zero time for missing key, got %v", got)
	}
	if got := readMapTime(m, "IntVal"); !got.IsZero() {
		t.Fatalf("expected zero time for non-time type, got %v", got)
	}
}

func TestParseToolStatesFromAny(t *testing.T) {
	states := []RuntimeToolStateSnapshot{
		{ToolCallID: "tc1", ToolName: "bash", Status: "succeeded"},
	}
	got := parseToolStatesFromAny(states)
	if len(got) != 1 || got[0].ToolCallID != "tc1" {
		t.Fatalf("expected slice parse, got %v", got)
	}

	anySlice := []any{
		map[string]any{"ToolCallID": "tc2", "ToolName": "read", "Status": "running"},
		42,
	}
	got = parseToolStatesFromAny(anySlice)
	if len(got) != 1 || got[0].ToolCallID != "tc2" {
		t.Fatalf("expected []any parse with invalid items skipped, got %v", got)
	}

	if got := parseToolStatesFromAny(42); got != nil {
		t.Fatalf("expected nil for unknown type, got %v", got)
	}
}

func TestParseToolStateFromAny(t *testing.T) {
	if _, ok := parseToolStateFromAny(42); ok {
		t.Fatalf("expected unknown type to fail")
	}
	if _, ok := parseToolStateFromAny((*RuntimeToolStateSnapshot)(nil)); ok {
		t.Fatalf("expected nil pointer to fail")
	}

	snap := RuntimeToolStateSnapshot{ToolCallID: "tc1", ToolName: "bash", Status: "succeeded"}
	out, ok := parseToolStateFromAny(snap)
	if !ok || out.ToolCallID != "tc1" {
		t.Fatalf("expected struct parse, got %+v ok=%v", out, ok)
	}

	ptr := &RuntimeToolStateSnapshot{ToolCallID: "tc2", ToolName: "read"}
	out, ok = parseToolStateFromAny(ptr)
	if !ok || out.ToolCallID != "tc2" {
		t.Fatalf("expected pointer parse, got %+v ok=%v", out, ok)
	}

	m := map[string]any{"ToolCallID": "tc3", "ToolName": "write", "Status": "planned", "Message": "msg", "DurationMS": int64(75)}
	out, ok = parseToolStateFromAny(m)
	if !ok || out.ToolCallID != "tc3" || out.Status != "planned" || out.DurationMS != 75 {
		t.Fatalf("expected map parse, got %+v ok=%v", out, ok)
	}
}

func TestParseRunContextSnapshotFromAny(t *testing.T) {
	if got := parseRunContextSnapshotFromAny(42); got != (RuntimeRunContextSnapshot{}) {
		t.Fatalf("expected zero for unknown type, got %+v", got)
	}
	if got := parseRunContextSnapshotFromAny((*RuntimeRunContextSnapshot)(nil)); got != (RuntimeRunContextSnapshot{}) {
		t.Fatalf("expected zero for nil pointer, got %+v", got)
	}

	snap := RuntimeRunContextSnapshot{RunID: "r1", SessionID: "s1", Provider: "openai"}
	got := parseRunContextSnapshotFromAny(snap)
	if got.RunID != "r1" {
		t.Fatalf("expected struct parse, got %+v", got)
	}

	ptr := &RuntimeRunContextSnapshot{RunID: "r2"}
	got = parseRunContextSnapshotFromAny(ptr)
	if got.RunID != "r2" {
		t.Fatalf("expected pointer parse, got %+v", got)
	}

	m := map[string]any{"RunID": "r3", "SessionID": "s3", "Provider": "anthropic", "Model": "claude", "Workdir": "/tmp", "Mode": "agent"}
	got = parseRunContextSnapshotFromAny(m)
	if got.RunID != "r3" || got.Provider != "anthropic" {
		t.Fatalf("expected map parse, got %+v", got)
	}
}

func TestReadUsageFromAny(t *testing.T) {
	if got := readUsageFromAny(42); got != (RuntimeUsageSnapshot{}) {
		t.Fatalf("expected zero for unknown type, got %+v", got)
	}
	if got := readUsageFromAny((*RuntimeUsageSnapshot)(nil)); got != (RuntimeUsageSnapshot{}) {
		t.Fatalf("expected zero for nil pointer, got %+v", got)
	}

	snap := RuntimeUsageSnapshot{InputTokens: 42}
	got := readUsageFromAny(snap)
	if got.InputTokens != 42 {
		t.Fatalf("expected struct parse, got %+v", got)
	}

	ptr := &RuntimeUsageSnapshot{OutputTokens: 99}
	got = readUsageFromAny(ptr)
	if got.OutputTokens != 99 {
		t.Fatalf("expected pointer parse, got %+v", got)
	}

	m := map[string]any{"InputTokens": 10, "OutputTokens": 20, "TotalTokens": 30}
	got = readUsageFromAny(m)
	if got.InputTokens != 10 || got.TotalTokens != 30 {
		t.Fatalf("expected map parse, got %+v", got)
	}
}
