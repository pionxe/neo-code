package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"neo-code/internal/config"
	runtimehooks "neo-code/internal/runtime/hooks"
)

func TestEvaluateWorkspaceTrustFallbackModes(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workspace := filepath.Join(homeDir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	storePath := resolveTrustedWorkspacesPath()
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		t.Fatalf("mkdir trust store dir: %v", err)
	}

	assertUntrustedWithInvalid := func(label string) {
		t.Helper()
		decision := evaluateWorkspaceTrust(workspace)
		if decision.Trusted {
			t.Fatalf("%s: expected untrusted", label)
		}
		if strings.TrimSpace(decision.InvalidReason) == "" {
			t.Fatalf("%s: expected invalid reason", label)
		}
	}

	assertUntrustedWithInvalid("missing")
	if err := os.WriteFile(storePath, []byte(" \n\t "), 0o644); err != nil {
		t.Fatalf("write empty store: %v", err)
	}
	assertUntrustedWithInvalid("empty")

	if err := os.WriteFile(storePath, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write malformed store: %v", err)
	}
	assertUntrustedWithInvalid("malformed")

	if err := os.WriteFile(storePath, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("write invalid shape store: %v", err)
	}
	assertUntrustedWithInvalid("shape mismatch")

	store := trustedWorkspaceStore{Version: repoHooksTrustStoreVersion, Workspaces: []string{workspace}}
	encoded, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("marshal trust store: %v", err)
	}
	if err := os.WriteFile(storePath, encoded, 0o644); err != nil {
		t.Fatalf("write trusted store: %v", err)
	}
	decision := evaluateWorkspaceTrust(workspace)
	if !decision.Trusted || strings.TrimSpace(decision.InvalidReason) != "" {
		t.Fatalf("trusted decision = %+v, want trusted and no invalid reason", decision)
	}
}

func TestLoadRepoHookItemsRejectsDuplicateIDWithinRepoSource(t *testing.T) {
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, ".neocode", "hooks.yaml")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	content := `
hooks:
  items:
    - id: same
      point: before_tool_call
      scope: repo
      kind: builtin
      mode: sync
      handler: add_context_note
      params:
        note: first
    - id: same
      point: after_tool_result
      scope: repo
      kind: builtin
      mode: sync
      handler: add_context_note
      params:
        note: second
`
	if err := os.WriteFile(hooksPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write hooks file: %v", err)
	}
	_, err := loadRepoHookItems(hooksPath, config.StaticDefaults().Runtime.Hooks)
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("loadRepoHookItems() error = %v, want duplicate id error", err)
	}
}

func TestConfigureRuntimeHooksFromConfigComposesInternalUserRepoOrder(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, ".neocode", "hooks.yaml")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	repoHooks := `
hooks:
  items:
    - id: shared-id
      point: before_tool_call
      scope: repo
      kind: builtin
      mode: sync
      handler: add_context_note
      params:
        note: repo-note
`
	if err := os.WriteFile(hooksPath, []byte(repoHooks), 0o644); err != nil {
		t.Fatalf("write repo hooks file: %v", err)
	}

	storePath := resolveTrustedWorkspacesPath()
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		t.Fatalf("mkdir trust store dir: %v", err)
	}
	store := trustedWorkspaceStore{Version: repoHooksTrustStoreVersion, Workspaces: []string{workspace}}
	rawStore, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("marshal trust store: %v", err)
	}
	if err := os.WriteFile(storePath, rawStore, 0o644); err != nil {
		t.Fatalf("write trust store: %v", err)
	}

	cfg := *config.StaticDefaults()
	cfg.Workdir = workspace
	cfg.Runtime.Hooks.Items = []config.RuntimeHookItemConfig{
		{
			ID:      "shared-id",
			Enabled: runtimeBoolPtr(true),
			Point:   "before_tool_call",
			Scope:   "user",
			Kind:    "builtin",
			Mode:    "sync",
			Handler: "add_context_note",
			Params:  map[string]any{"note": "user-note"},
		},
	}
	cfg.Runtime.Hooks.ApplyDefaults(config.StaticDefaults().Runtime.Hooks)

	base := &countingHookExecutor{
		output: runtimehooks.RunOutput{
			Results: []runtimehooks.HookResult{
				{
					HookID:  "base-id",
					Scope:   runtimehooks.HookScopeInternal,
					Source:  runtimehooks.HookSourceInternal,
					Status:  runtimehooks.HookResultPass,
					Message: "base-note",
				},
			},
		},
	}
	service := &Service{
		hookExecutor: base,
		events:       make(chan RuntimeEvent, 64),
	}

	if err := configureRuntimeHooksFromConfig(service, cfg); err != nil {
		t.Fatalf("configureRuntimeHooksFromConfig() error = %v", err)
	}
	if service.hookExecutor == nil {
		t.Fatal("expected composed hook executor")
	}

	out := service.hookExecutor.Run(context.Background(), runtimehooks.HookPointBeforeToolCall, runtimehooks.HookContext{
		Metadata: map[string]any{
			"tool_name":      "bash",
			"tool_arguments": "secret-value",
			"workdir":        workspace,
		},
	})
	if len(out.Results) != 3 {
		t.Fatalf("results len = %d, want 3 (%+v)", len(out.Results), out.Results)
	}
	if out.Results[0].Source != runtimehooks.HookSourceInternal {
		t.Fatalf("result[0].source = %q, want internal", out.Results[0].Source)
	}
	if out.Results[1].Source != runtimehooks.HookSourceUser || out.Results[1].Message != "user-note" {
		t.Fatalf("result[1] = %+v, want user source + user-note", out.Results[1])
	}
	if out.Results[2].Source != runtimehooks.HookSourceRepo || out.Results[2].Message != "repo-note" {
		t.Fatalf("result[2] = %+v, want repo source + repo-note", out.Results[2])
	}
}

func TestBuildRepoHookExecutorUntrustedSkipsAndEmitsEvent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, ".neocode", "hooks.yaml")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	content := `
hooks:
  items:
    - id: repo-hook
      point: before_tool_call
      scope: repo
      kind: builtin
      mode: sync
      handler: add_context_note
      params:
        note: repo-note
`
	if err := os.WriteFile(hooksPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}

	// trust store 存在但不包含当前 workspace，命中 untrusted 分支。
	storePath := resolveTrustedWorkspacesPath()
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		t.Fatalf("mkdir trust store dir: %v", err)
	}
	otherWorkspace := filepath.Join(homeDir, "other")
	if err := os.MkdirAll(otherWorkspace, 0o755); err != nil {
		t.Fatalf("mkdir other workspace: %v", err)
	}
	rawStore, err := json.Marshal(trustedWorkspaceStore{
		Version:    repoHooksTrustStoreVersion,
		Workspaces: []string{otherWorkspace},
	})
	if err != nil {
		t.Fatalf("marshal trust store: %v", err)
	}
	if err := os.WriteFile(storePath, rawStore, 0o644); err != nil {
		t.Fatalf("write trust store: %v", err)
	}

	cfg := *config.StaticDefaults()
	cfg.Workdir = workspace
	service := &Service{events: make(chan RuntimeEvent, 64)}
	exec, err := buildRepoHookExecutor(service, cfg, config.StaticDefaults().Runtime.Hooks)
	if err != nil {
		t.Fatalf("buildRepoHookExecutor() error = %v", err)
	}
	if exec != nil {
		t.Fatal("expected nil repo executor for untrusted workspace")
	}

	events := collectRuntimeEvents(service.Events())
	if !containsRuntimeEventType(events, EventRepoHooksDiscovered) {
		t.Fatalf("expected %s event", EventRepoHooksDiscovered)
	}
	if !containsRuntimeEventType(events, EventRepoHooksSkippedUntrusted) {
		t.Fatalf("expected %s event", EventRepoHooksSkippedUntrusted)
	}
}

func TestBuildRepoHookExecutorMissingTrustStoreEmitsInvalidEvent(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, ".neocode", "hooks.yaml")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}
	content := `
hooks:
  items:
    - id: repo-hook
      point: before_tool_call
      scope: repo
      kind: builtin
      mode: sync
      handler: add_context_note
      params:
        note: repo-note
`
	if err := os.WriteFile(hooksPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write hooks: %v", err)
	}

	cfg := *config.StaticDefaults()
	cfg.Workdir = workspace
	service := &Service{events: make(chan RuntimeEvent, 64)}
	exec, err := buildRepoHookExecutor(service, cfg, config.StaticDefaults().Runtime.Hooks)
	if err != nil {
		t.Fatalf("buildRepoHookExecutor() error = %v", err)
	}
	if exec != nil {
		t.Fatal("expected nil repo executor when trust store is missing")
	}

	events := collectRuntimeEvents(service.Events())
	if !containsRuntimeEventType(events, EventRepoHooksTrustStoreInvalid) {
		t.Fatalf("expected %s event", EventRepoHooksTrustStoreInvalid)
	}
	if !containsRuntimeEventType(events, EventRepoHooksSkippedUntrusted) {
		t.Fatalf("expected %s event", EventRepoHooksSkippedUntrusted)
	}
}

func containsRuntimeEventType(events []RuntimeEvent, target EventType) bool {
	for _, event := range events {
		if event.Type == target {
			return true
		}
	}
	return false
}
