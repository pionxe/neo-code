package config

import (
	"strings"
	"testing"

	"neo-code/internal/runtime/hooks"
)

func TestRuntimeHooksConfigApplyDefaultsAndValidate(t *testing.T) {
	t.Parallel()

	var hooksCfg RuntimeHooksConfig
	defaults := defaultRuntimeHooksConfig()
	hooksCfg.ApplyDefaults(defaults)

	if !hooksCfg.IsEnabled() {
		t.Fatal("expected hooks enabled by default")
	}
	if !hooksCfg.IsUserHooksEnabled() {
		t.Fatal("expected user hooks enabled by default")
	}
	if hooksCfg.DefaultTimeoutSec != DefaultRuntimeHookTimeoutSec {
		t.Fatalf("default timeout = %d, want %d", hooksCfg.DefaultTimeoutSec, DefaultRuntimeHookTimeoutSec)
	}
	if hooksCfg.DefaultFailurePolicy != runtimeHookFailurePolicyWarnOnly {
		t.Fatalf(
			"default failure policy = %q, want %q",
			hooksCfg.DefaultFailurePolicy,
			runtimeHookFailurePolicyWarnOnly,
		)
	}
	if err := hooksCfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRuntimeHooksConfigValidateUnsupportedFields(t *testing.T) {
	t.Parallel()

	base := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
	}

	tests := []RuntimeHookItemConfig{
		{
			ID:      "bad-scope",
			Point:   string(hooks.HookPointBeforeToolCall),
			Scope:   "repo",
			Kind:    runtimeHookKindBuiltIn,
			Mode:    runtimeHookModeSync,
			Handler: runtimeHookHandlerWarnOnToolCall,
		},
		{
			ID:      "bad-kind",
			Point:   string(hooks.HookPointBeforeToolCall),
			Scope:   runtimeHookScopeUser,
			Kind:    "command",
			Mode:    runtimeHookModeSync,
			Handler: runtimeHookHandlerWarnOnToolCall,
		},
		{
			ID:      "bad-mode",
			Point:   string(hooks.HookPointBeforeToolCall),
			Scope:   runtimeHookScopeUser,
			Kind:    runtimeHookKindBuiltIn,
			Mode:    "async",
			Handler: runtimeHookHandlerWarnOnToolCall,
		},
		{
			ID:      "bad-handler",
			Point:   string(hooks.HookPointBeforeToolCall),
			Scope:   runtimeHookScopeUser,
			Kind:    runtimeHookKindBuiltIn,
			Mode:    runtimeHookModeSync,
			Handler: "shell_exec",
		},
		{
			ID:      "bad-point",
			Point:   "unknown_point",
			Scope:   runtimeHookScopeUser,
			Kind:    runtimeHookKindBuiltIn,
			Mode:    runtimeHookModeSync,
			Handler: runtimeHookHandlerWarnOnToolCall,
		},
	}

	for _, item := range tests {
		cfg := base.Clone()
		cfg.Items = []RuntimeHookItemConfig{item}
		cfg.ApplyDefaults(defaultRuntimeHooksConfig())
		if err := cfg.Validate(); err == nil {
			t.Fatalf("expected validate error for item=%+v", item)
		}
	}
}

func TestRuntimeHooksConfigValidateRejectsExternalKindsWithP6LiteMessage(t *testing.T) {
	t.Parallel()

	base := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
	}
	externalKinds := []string{"prompt", "agent"}
	for _, kind := range externalKinds {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			cfg := base.Clone()
			cfg.Items = []RuntimeHookItemConfig{
				{
					ID:      "external-kind",
					Point:   string(hooks.HookPointBeforeToolCall),
					Scope:   runtimeHookScopeUser,
					Kind:    kind,
					Mode:    runtimeHookModeSync,
					Handler: runtimeHookHandlerWarnOnToolCall,
					Params:  map[string]any{"tool_name": "bash"},
				},
			}
			cfg.ApplyDefaults(defaultRuntimeHooksConfig())
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected external kind %q to be rejected", kind)
			}
			if !strings.Contains(err.Error(), "not supported in current stage") {
				t.Fatalf("error=%q, want contains not supported in current stage", err.Error())
			}
		})
	}
}

func TestRuntimeHooksConfigValidateAllowsCommand(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "accept-command",
				Point:         string(hooks.HookPointAcceptGate),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindCommand,
				Mode:          runtimeHookModeSync,
				TimeoutSec:    2,
				FailurePolicy: runtimeHookFailurePolicyWarnOnly,
				Params:        map[string]any{"command": []any{"echo", "ok"}},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRuntimeHooksConfigValidateCommandShellMode(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "cmd-shell",
				Point:         string(hooks.HookPointAcceptGate),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindCommand,
				Mode:          runtimeHookModeSync,
				TimeoutSec:    2,
				FailurePolicy: runtimeHookFailurePolicyWarnOnly,
				Params:        map[string]any{"command": "echo ok", "shell": true},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRuntimeHooksConfigValidateCommandStringWithoutShellRejected(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "cmd-no-shell",
				Point:         string(hooks.HookPointAcceptGate),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindCommand,
				Mode:          runtimeHookModeSync,
				TimeoutSec:    2,
				FailurePolicy: runtimeHookFailurePolicyWarnOnly,
				Params:        map[string]any{"command": "echo ok"},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for string command without shell=true")
	}
}

func TestRuntimeHooksConfigValidateCommandArgvMode(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "cmd-argv",
				Point:         string(hooks.HookPointAcceptGate),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindCommand,
				Mode:          runtimeHookModeSync,
				TimeoutSec:    2,
				FailurePolicy: runtimeHookFailurePolicyWarnOnly,
				Params:        map[string]any{"command": []string{"echo", "hello"}},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRuntimeHooksConfigValidateAllowsHTTPObserve(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:    "observe-http",
				Point: string(hooks.HookPointBeforeToolCall),
				Scope: runtimeHookScopeUser,
				Kind:  runtimeHookKindHTTP,
				Params: map[string]any{
					"url": "http://127.0.0.1:19090/hook",
				},
			},
		},
	}
	cfg.ApplyDefaults(defaultRuntimeHooksConfig())
	if cfg.Items[0].Mode != runtimeHookModeObserve {
		t.Fatalf("mode=%q, want observe default", cfg.Items[0].Mode)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected http observe to pass, got err=%v", err)
	}
}

func TestRuntimeHooksConfigValidateRejectsInvalidHTTPObserveConfig(t *testing.T) {
	t.Parallel()

	base := RuntimeHookItemConfig{
		ID:    "observe-http",
		Point: string(hooks.HookPointBeforeToolCall),
		Scope: runtimeHookScopeUser,
		Kind:  runtimeHookKindHTTP,
		Mode:  runtimeHookModeObserve,
		Params: map[string]any{
			"url": "http://127.0.0.1:19090/hook",
		},
	}
	tests := []struct {
		name string
		edit func(*RuntimeHookItemConfig)
	}{
		{
			name: "sync mode not allowed",
			edit: func(item *RuntimeHookItemConfig) {
				item.Mode = runtimeHookModeSync
			},
		},
		{
			name: "fail closed not allowed",
			edit: func(item *RuntimeHookItemConfig) {
				item.FailurePolicy = runtimeHookFailurePolicyFailClose
			},
		},
		{
			name: "handler must be empty",
			edit: func(item *RuntimeHookItemConfig) {
				item.Handler = runtimeHookHandlerAddContextNote
			},
		},
		{
			name: "missing url",
			edit: func(item *RuntimeHookItemConfig) {
				item.Params = map[string]any{}
			},
		},
		{
			name: "bad scheme",
			edit: func(item *RuntimeHookItemConfig) {
				item.Params = map[string]any{"url": "file:///tmp/hook"}
			},
		},
		{
			name: "bad method",
			edit: func(item *RuntimeHookItemConfig) {
				item.Params = map[string]any{"url": "http://127.0.0.1:19090/hook", "method": "TRACE"}
			},
		},
		{
			name: "remote host not allowed",
			edit: func(item *RuntimeHookItemConfig) {
				item.Params = map[string]any{"url": "https://example.com/hook"}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			item := base.Clone()
			tc.edit(&item)
			cfg := RuntimeHooksConfig{
				Enabled:              boolPtr(true),
				UserHooksEnabled:     boolPtr(true),
				DefaultTimeoutSec:    2,
				DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
				Items:                []RuntimeHookItemConfig{item},
			}
			cfg.ApplyDefaults(defaultRuntimeHooksConfig())
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected invalid http observe config to fail: %+v", item)
			}
		})
	}
}

func TestRuntimeHooksConfigValidateRejectsDisallowedUserPoint(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "deny-pre-compact",
				Point:         string(hooks.HookPointPreCompact),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindBuiltIn,
				Mode:          runtimeHookModeSync,
				Handler:       runtimeHookHandlerAddContextNote,
				TimeoutSec:    2,
				FailurePolicy: runtimeHookFailurePolicyWarnOnly,
				Params:        map[string]any{"note": "test"},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected user-disallowed point to fail validation")
	}
}

func TestRuntimeHooksConfigItemDefaultsAndClone(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    3,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:      "warn-bash",
				Point:   string(hooks.HookPointBeforeToolCall),
				Handler: runtimeHookHandlerWarnOnToolCall,
				Match: map[string]any{
					"tool_name": "bash",
				},
				Params: map[string]any{
					"tags": []any{"warn", "tool"},
				},
			},
		},
	}
	cfg.ApplyDefaults(defaultRuntimeHooksConfig())

	item := cfg.Items[0]
	if !item.IsEnabled() {
		t.Fatal("expected hook item enabled by default")
	}
	if item.Scope != runtimeHookScopeUser {
		t.Fatalf("scope=%q, want %q", item.Scope, runtimeHookScopeUser)
	}
	if item.Kind != runtimeHookKindBuiltIn {
		t.Fatalf("kind=%q, want %q", item.Kind, runtimeHookKindBuiltIn)
	}
	if item.Mode != runtimeHookModeSync {
		t.Fatalf("mode=%q, want %q", item.Mode, runtimeHookModeSync)
	}
	if item.TimeoutSec != cfg.DefaultTimeoutSec {
		t.Fatalf("timeout=%d, want %d", item.TimeoutSec, cfg.DefaultTimeoutSec)
	}
	if item.FailurePolicy != runtimeHookFailurePolicyWarnOnly {
		t.Fatalf("failure policy=%q, want %q", item.FailurePolicy, runtimeHookFailurePolicyWarnOnly)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	cloned := cfg.Clone()
	cloned.Items[0].Params["tool_name"] = "filesystem"
	tags, _ := cloned.Items[0].Params["tags"].([]any)
	tags[0] = "changed"
	cloned.Items[0].Params["tags"] = tags

	if cfg.Items[0].Params["tool_name"] == "filesystem" {
		t.Fatal("expected params map to be deep-copied")
	}
	originalTags, _ := cfg.Items[0].Params["tags"].([]any)
	if len(originalTags) > 0 && originalTags[0] == "changed" {
		t.Fatal("expected params slice to be deep-copied")
	}
}

func TestRuntimeHooksConfigValidateItemFailurePolicy(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "require-readme",
				Point:         string(hooks.HookPointBeforeCompletionDecision),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindBuiltIn,
				Mode:          runtimeHookModeSync,
				Handler:       runtimeHookHandlerRequireFileExists,
				TimeoutSec:    2,
				FailurePolicy: "invalid_policy",
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid item failure_policy to be rejected")
	}
}

func TestRuntimeHooksConfigValidateWarnOnToolCallRequiresTarget(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "warn-missing-target",
				Point:         string(hooks.HookPointBeforeToolCall),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindBuiltIn,
				Mode:          runtimeHookModeSync,
				Handler:       runtimeHookHandlerWarnOnToolCall,
				TimeoutSec:    2,
				FailurePolicy: runtimeHookFailurePolicyWarnOnly,
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected warn_on_tool_call without target to fail validation")
	}
}

func TestRuntimeHooksConfigValidateWarnOnToolCallAllowsMatchWithoutLegacyTargets(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "warn-with-match",
				Point:         string(hooks.HookPointBeforeToolCall),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindBuiltIn,
				Mode:          runtimeHookModeSync,
				Handler:       runtimeHookHandlerWarnOnToolCall,
				TimeoutSec:    2,
				FailurePolicy: runtimeHookFailurePolicyWarnOnly,
				Match: map[string]any{
					"tool_name": "bash",
				},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRuntimeHooksConfigValidateRejectsUnsupportedMatcherDimensionForPoint(t *testing.T) {
	t.Parallel()

	cfg := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		Items: []RuntimeHookItemConfig{
			{
				ID:            "session-start-match",
				Point:         string(hooks.HookPointSessionStart),
				Scope:         runtimeHookScopeUser,
				Kind:          runtimeHookKindBuiltIn,
				Mode:          runtimeHookModeSync,
				Handler:       runtimeHookHandlerAddContextNote,
				TimeoutSec:    2,
				FailurePolicy: runtimeHookFailurePolicyWarnOnly,
				Params:        map[string]any{"note": "observe"},
				Match: map[string]any{
					"tool_name": "bash",
				},
			},
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsupported matcher dimension to fail validation")
	}
}

func TestRuntimeHooksConfigEdgeBranches(t *testing.T) {
	t.Parallel()

	t.Run("apply defaults fallback when defaults pointers are nil", func(t *testing.T) {
		t.Parallel()
		cfg := RuntimeHooksConfig{}
		cfg.ApplyDefaults(RuntimeHooksConfig{
			DefaultTimeoutSec:    5,
			DefaultFailurePolicy: runtimeHookFailurePolicyFailOpen,
		})
		if cfg.Enabled == nil || !*cfg.Enabled {
			t.Fatal("expected enabled fallback to true")
		}
		if cfg.UserHooksEnabled == nil || !*cfg.UserHooksEnabled {
			t.Fatal("expected user_hooks_enabled fallback to true")
		}
	})

	t.Run("validate root errors and duplicate id", func(t *testing.T) {
		t.Parallel()
		cfg := RuntimeHooksConfig{
			DefaultTimeoutSec:    0,
			DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
		}
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected timeout validation error")
		}

		cfg = RuntimeHooksConfig{
			DefaultTimeoutSec:    2,
			DefaultFailurePolicy: "bad",
		}
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected default failure policy validation error")
		}

		cfg = RuntimeHooksConfig{
			DefaultTimeoutSec:    2,
			DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
			Items: []RuntimeHookItemConfig{
				{ID: "dup", Point: string(hooks.HookPointBeforeToolCall), Scope: runtimeHookScopeUser, Kind: runtimeHookKindBuiltIn, Mode: runtimeHookModeSync, Handler: runtimeHookHandlerWarnOnToolCall, TimeoutSec: 1, Params: map[string]any{"tool_name": "bash"}},
				{ID: " DUP ", Point: string(hooks.HookPointBeforeToolCall), Scope: runtimeHookScopeUser, Kind: runtimeHookKindBuiltIn, Mode: runtimeHookModeSync, Handler: runtimeHookHandlerWarnOnToolCall, TimeoutSec: 1, Params: map[string]any{"tool_name": "bash"}},
			},
		}
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected duplicate id error")
		}
	})

	t.Run("item validate missing id and timeout", func(t *testing.T) {
		t.Parallel()
		if err := (RuntimeHookItemConfig{}).Validate(runtimeHookFailurePolicyWarnOnly); err == nil {
			t.Fatal("expected missing id error")
		}
		item := RuntimeHookItemConfig{
			ID:      "x",
			Point:   string(hooks.HookPointBeforeToolCall),
			Scope:   runtimeHookScopeUser,
			Kind:    runtimeHookKindBuiltIn,
			Mode:    runtimeHookModeSync,
			Handler: runtimeHookHandlerAddContextNote,
			Params:  map[string]any{"note": "ok"},
		}
		if err := item.Validate(runtimeHookFailurePolicyWarnOnly); err == nil {
			t.Fatal("expected timeout error")
		}
	})

	t.Run("helper functions", func(t *testing.T) {
		t.Parallel()
		if !(RuntimeHooksConfig{Enabled: boolPtr(true)}).IsEnabled() {
			t.Fatal("expected enabled true")
		}
		if (RuntimeHooksConfig{Enabled: boolPtr(false)}).IsEnabled() {
			t.Fatal("expected enabled false")
		}
		if !(RuntimeHooksConfig{}).IsEnabled() {
			t.Fatal("expected enabled default true when nil")
		}
		if !(RuntimeHooksConfig{UserHooksEnabled: boolPtr(true)}).IsUserHooksEnabled() {
			t.Fatal("expected user hooks enabled true")
		}
		if (RuntimeHooksConfig{UserHooksEnabled: boolPtr(false)}).IsUserHooksEnabled() {
			t.Fatal("expected user hooks enabled false")
		}
		if !(RuntimeHooksConfig{}).IsUserHooksEnabled() {
			t.Fatal("expected user hooks default true when nil")
		}
		if (RuntimeHookItemConfig{Enabled: boolPtr(false)}).IsEnabled() {
			t.Fatal("expected item disabled false")
		}
		if !(RuntimeHookItemConfig{}).IsEnabled() {
			t.Fatal("expected item default enabled when nil")
		}
		if err := validateRuntimeHookFailurePolicy(runtimeHookFailurePolicyFailClose); err != nil {
			t.Fatalf("expected fail_close valid, got %v", err)
		}
		if err := validateRuntimeHookFailurePolicy("bad"); err == nil {
			t.Fatal("expected invalid policy error")
		}

		original := map[string]any{
			"a": []any{"x", map[string]any{"b": "c"}},
		}
		cloned, ok := cloneRuntimeHookParamValue(original).(map[string]any)
		if !ok {
			t.Fatal("expected cloned map")
		}
		clonedSlice := cloned["a"].([]any)
		nested := clonedSlice[1].(map[string]any)
		nested["b"] = "changed"
		origNested := original["a"].([]any)[1].(map[string]any)
		if origNested["b"] == "changed" {
			t.Fatal("expected deep clone for nested map in slice")
		}

		matchCfg := RuntimeHookItemConfig{
			Match: map[string]any{
				"tool_name_regex": []any{`^bash$`},
			},
		}
		clonedCfg := matchCfg.Clone()
		clonedRegexes := clonedCfg.Match["tool_name_regex"].([]any)
		clonedRegexes[0] = "^filesystem$"
		clonedCfg.Match["tool_name_regex"] = clonedRegexes
		originalRegexes := matchCfg.Match["tool_name_regex"].([]any)
		if originalRegexes[0] == "^filesystem$" {
			t.Fatal("expected match field to be deep-cloned")
		}
	})
}

func TestRuntimeHTTPObserveValidationHelpers(t *testing.T) {
	t.Parallel()

	t.Run("allow localhost variants", func(t *testing.T) {
		t.Parallel()
		for _, rawURL := range []string{
			"http://localhost:19090/hook",
			"http://[::1]:19090/hook",
		} {
			item := RuntimeHookItemConfig{
				ID:    "observe-http",
				Point: string(hooks.HookPointBeforeToolCall),
				Scope: runtimeHookScopeUser,
				Kind:  runtimeHookKindHTTP,
				Mode:  runtimeHookModeObserve,
				Params: map[string]any{
					"url":     rawURL,
					"method":  "PATCH",
					"headers": map[string]any{"X-Test": 7},
				},
			}
			if err := validateRuntimeHTTPObserveItem(item, runtimeHookFailurePolicyWarnOnly); err != nil {
				t.Fatalf("validateRuntimeHTTPObserveItem(%q) error = %v", rawURL, err)
			}
		}
	})

	t.Run("reject malformed headers and urls", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			item RuntimeHookItemConfig
		}{
			{
				name: "invalid absolute url",
				item: RuntimeHookItemConfig{
					ID:    "observe-http",
					Point: string(hooks.HookPointBeforeToolCall),
					Scope: runtimeHookScopeUser,
					Kind:  runtimeHookKindHTTP,
					Mode:  runtimeHookModeObserve,
					Params: map[string]any{
						"url": "://bad",
					},
				},
			},
			{
				name: "headers must be map",
				item: RuntimeHookItemConfig{
					ID:    "observe-http",
					Point: string(hooks.HookPointBeforeToolCall),
					Scope: runtimeHookScopeUser,
					Kind:  runtimeHookKindHTTP,
					Mode:  runtimeHookModeObserve,
					Params: map[string]any{
						"url":     "http://127.0.0.1:19090/hook",
						"headers": "bad",
					},
				},
			},
			{
				name: "empty header name",
				item: RuntimeHookItemConfig{
					ID:    "observe-http",
					Point: string(hooks.HookPointBeforeToolCall),
					Scope: runtimeHookScopeUser,
					Kind:  runtimeHookKindHTTP,
					Mode:  runtimeHookModeObserve,
					Params: map[string]any{
						"url":     "http://127.0.0.1:19090/hook",
						"headers": map[string]any{" ": "x"},
					},
				},
			},
			{
				name: "empty header value",
				item: RuntimeHookItemConfig{
					ID:    "observe-http",
					Point: string(hooks.HookPointBeforeToolCall),
					Scope: runtimeHookScopeUser,
					Kind:  runtimeHookKindHTTP,
					Mode:  runtimeHookModeObserve,
					Params: map[string]any{
						"url":     "http://127.0.0.1:19090/hook",
						"headers": map[string]any{"X-Test": "   "},
					},
				},
			},
		}
		for _, tc := range tests {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if err := validateRuntimeHTTPObserveItem(tc.item, runtimeHookFailurePolicyWarnOnly); err == nil {
					t.Fatalf("expected validation error for %+v", tc.item.Params)
				}
			})
		}
	})

	t.Run("helper functions", func(t *testing.T) {
		t.Parallel()
		if !isRuntimeHookHTTPObserveLoopbackHost("localhost") {
			t.Fatal("localhost should be treated as loopback")
		}
		if !isRuntimeHookHTTPObserveLoopbackHost("::1") {
			t.Fatal("::1 should be treated as loopback")
		}
		if isRuntimeHookHTTPObserveLoopbackHost("") {
			t.Fatal("empty host should not be loopback")
		}
		if isRuntimeHookHTTPObserveLoopbackHost("example.com") {
			t.Fatal("remote host should not be loopback")
		}
		if got := readRuntimeHookParamString(nil, "x"); got != "" {
			t.Fatalf("readRuntimeHookParamString(nil) = %q", got)
		}
		if got := readRuntimeHookParamString(map[string]any{"x": 123}, "x"); got != "123" {
			t.Fatalf("readRuntimeHookParamString(non-string) = %q", got)
		}
		if !hooks.IsUserAllowed(hooks.HookPointBeforeToolCall) {
			t.Fatal("before_tool_call should allow user hooks")
		}
		for _, point := range []hooks.HookPoint{
			hooks.HookPointBeforePermissionDecision,
			hooks.HookPointPreCompact,
			hooks.HookPointSubAgentStart,
		} {
			if hooks.IsUserAllowed(point) {
				t.Fatalf("%s should be rejected for user hooks", point)
			}
		}
	})
}

// TestHookPointSingleSourceConsistency 验证 config 侧与 runtime hooks 包的点位定义一致。
// 新增 hook point 时只需修改 runtime hooks 包，config 侧自动接受。
func TestHookPointSingleSourceConsistency(t *testing.T) {
	t.Parallel()

	// 所有 runtime hooks 包导出的点位都应被 config 接受。
	allPoints := hooks.ListHookPoints()
	if len(allPoints) == 0 {
		t.Fatal("expected at least one hook point from runtime hooks package")
	}

	base := RuntimeHooksConfig{
		Enabled:              boolPtr(true),
		UserHooksEnabled:     boolPtr(true),
		DefaultTimeoutSec:    2,
		DefaultFailurePolicy: runtimeHookFailurePolicyWarnOnly,
	}

	for _, point := range allPoints {
		point := point
		t.Run(string(point), func(t *testing.T) {
			t.Parallel()
			if !hooks.IsUserAllowed(point) {
				// 跳过不允许 user 的点位，它们在 config 校验中会被拒绝。
				return
			}
			cfg := base.Clone()
			cfg.Items = []RuntimeHookItemConfig{
				{
					ID:            "test-" + string(point),
					Point:         string(point),
					Scope:         runtimeHookScopeUser,
					Kind:          runtimeHookKindBuiltIn,
					Mode:          runtimeHookModeSync,
					Handler:       runtimeHookHandlerAddContextNote,
					TimeoutSec:    2,
					FailurePolicy: runtimeHookFailurePolicyWarnOnly,
					Params:        map[string]any{"note": "consistency check"},
				},
			}
			if err := cfg.Validate(); err != nil {
				t.Fatalf("config rejected point %q: %v", point, err)
			}
		})
	}

	// 验证 accept_gate 在 runtime hooks 包中存在且允许 user。
	acceptGateCap, ok := hooks.HookPointCapabilities(hooks.HookPointAcceptGate)
	if !ok {
		t.Fatal("accept_gate not found in runtime hooks capabilities")
	}
	if !acceptGateCap.UserAllowed {
		t.Fatal("accept_gate should allow user hooks")
	}
}
