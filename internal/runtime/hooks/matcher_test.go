package hooks

import (
	"regexp"
	"testing"
)

func TestHasHookMatcherConfig(t *testing.T) {
	t.Parallel()

	if HasHookMatcherConfig(nil) {
		t.Fatal("nil matcher config should be false")
	}
	if HasHookMatcherConfig(map[string]any{}) {
		t.Fatal("empty matcher config should be false")
	}
	if !HasHookMatcherConfig(map[string]any{"tool_name": "bash"}) {
		t.Fatal("tool_name matcher should be true")
	}
	if !HasHookMatcherConfig(map[string]any{"tool_name_regex": []any{"^bash$"}}) {
		t.Fatal("tool_name_regex matcher should be true")
	}
	if !HasHookMatcherConfig(map[string]any{"arguments_contains": []string{"rm -rf"}}) {
		t.Fatal("arguments_contains matcher should be true")
	}
	if HasHookMatcherConfig(map[string]any{"tool_name": "  "}) {
		t.Fatal("whitespace-only tool_name should be false")
	}
}

func TestCompileHookMatcherAndMatch(t *testing.T) {
	t.Parallel()

	matcher, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{
		"tool_name":          []any{"bash", "filesystem"},
		"tool_name_regex":    []string{`^(bash|shell)$`},
		"arguments_contains": []string{"rm -rf"},
	})
	if err != nil {
		t.Fatalf("CompileHookMatcher() error = %v", err)
	}
	if matcher == nil {
		t.Fatal("expected matcher to be compiled")
	}

	if !matcher.Match(HookContext{
		Metadata: map[string]any{
			"tool_name":              "bash",
			"tool_arguments_preview": "sudo rm -rf /tmp/test",
		},
	}) {
		t.Fatal("expected matcher to pass for matching metadata")
	}
	if matcher.Match(HookContext{
		Metadata: map[string]any{
			"tool_name":              "bash",
			"tool_arguments_preview": "echo hello",
		},
	}) {
		t.Fatal("expected matcher to fail when arguments_contains not matched")
	}
	if matcher.Match(HookContext{
		Metadata: map[string]any{
			"tool_name":              "filesystem",
			"tool_arguments_preview": "rm -rf /tmp",
		},
	}) {
		t.Fatal("expected matcher to fail when tool_name_regex not matched")
	}
}

func TestCompileHookMatcherValidation(t *testing.T) {
	t.Parallel()

	if _, err := CompileHookMatcher(HookPointSessionStart, map[string]any{
		"tool_name": "bash",
	}); err == nil {
		t.Fatal("expected session_start tool_name matcher to be rejected")
	}

	if _, err := CompileHookMatcher(HookPointAfterToolResult, map[string]any{
		"arguments_contains": []string{"rm -rf"},
	}); err == nil {
		t.Fatal("expected after_tool_result arguments_contains to be rejected")
	}

	if _, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{
		"tool_name_regex": "(",
	}); err == nil {
		t.Fatal("expected invalid regex to fail")
	}

	longRegex := make([]byte, MaxHookMatcherRegexLength+1)
	for i := range longRegex {
		longRegex[i] = 'a'
	}
	if _, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{
		"tool_name_regex": string(longRegex),
	}); err == nil {
		t.Fatal("expected overlong regex to fail")
	}

	if _, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{
		"tool_names": "bash",
	}); err == nil {
		t.Fatal("expected unknown matcher field to be rejected")
	}

	if _, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{
		"unknown": "value",
	}); err == nil {
		t.Fatal("expected completely unknown matcher field to be rejected")
	}
	if _, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{
		"tool_name":  "bash",
		"tool_names": []any{"filesystem"},
	}); err == nil {
		t.Fatal("expected mixed matcher fields with typo to be rejected")
	}

	if _, err := CompileHookMatcher(HookPointBeforeToolCall, nil); err != nil {
		t.Fatal("nil raw should succeed with nil matcher")
	}
	if _, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{}); err != nil {
		t.Fatal("empty raw should succeed with nil matcher")
	}
}

func TestValidateHookMatcher(t *testing.T) {
	t.Parallel()

	if err := ValidateHookMatcher(HookPointBeforeToolCall, map[string]any{
		"tool_name": "bash",
	}); err != nil {
		t.Fatalf("ValidateHookMatcher() error = %v", err)
	}
	if err := ValidateHookMatcher(HookPointSessionStart, map[string]any{
		"tool_name": "bash",
	}); err == nil {
		t.Fatal("expected session_start matcher to fail validation")
	}
}

func TestIsEmpty(t *testing.T) {
	t.Parallel()

	var nilMatcher *HookMatcher
	if !nilMatcher.IsEmpty() {
		t.Fatal("nil matcher should be empty")
	}
	if !(&HookMatcher{}).IsEmpty() {
		t.Fatal("zero-value matcher should be empty")
	}
	if (&HookMatcher{ToolNames: []string{"bash"}}).IsEmpty() {
		t.Fatal("matcher with tool_name should not be empty")
	}
}

func TestMatchNilAndEmpty(t *testing.T) {
	t.Parallel()

	var nilMatcher *HookMatcher
	if !nilMatcher.Match(HookContext{}) {
		t.Fatal("nil matcher should match everything")
	}
	empty := &HookMatcher{}
	if !empty.Match(HookContext{}) {
		t.Fatal("empty matcher should match everything")
	}
}

func TestMatchSingleDimension(t *testing.T) {
	t.Parallel()

	t.Run("tool_name only", func(t *testing.T) {
		t.Parallel()
		m := &HookMatcher{ToolNames: []string{"bash", "filesystem"}}
		if !m.Match(HookContext{Metadata: map[string]any{"tool_name": "bash"}}) {
			t.Fatal("expected match for bash")
		}
		if m.Match(HookContext{Metadata: map[string]any{"tool_name": "python"}}) {
			t.Fatal("expected no match for python")
		}
		if m.Match(HookContext{Metadata: map[string]any{}}) {
			t.Fatal("expected no match when tool_name metadata missing")
		}
	})

	t.Run("tool_name_regex only", func(t *testing.T) {
		t.Parallel()
		compiled := regexp.MustCompile(`^(bash|shell)$`)
		m := &HookMatcher{ToolNameRegex: []*regexp.Regexp{compiled}}
		if !m.Match(HookContext{Metadata: map[string]any{"tool_name": "bash"}}) {
			t.Fatal("expected regex match for bash")
		}
		if m.Match(HookContext{Metadata: map[string]any{"tool_name": "python"}}) {
			t.Fatal("expected regex no match for python")
		}
		if m.Match(HookContext{Metadata: map[string]any{}}) {
			t.Fatal("expected no match when tool_name missing for regex")
		}
	})

	t.Run("arguments_contains only", func(t *testing.T) {
		t.Parallel()
		m := &HookMatcher{ArgumentsContains: []string{"rm -rf", "sudo"}}
		if !m.Match(HookContext{Metadata: map[string]any{"tool_arguments_preview": "sudo rm -rf /tmp"}}) {
			t.Fatal("expected arguments_contains match")
		}
		if m.Match(HookContext{Metadata: map[string]any{"tool_arguments_preview": "echo hello"}}) {
			t.Fatal("expected arguments_contains no match")
		}
		if m.Match(HookContext{Metadata: map[string]any{}}) {
			t.Fatal("expected no match when arguments_preview missing")
		}
	})
}

func TestReadHookMatcherStringValues(t *testing.T) {
	t.Parallel()

	if got := readHookMatcherStringValues(nil, "x"); len(got) != 0 {
		t.Fatal("nil raw should return nil")
	}
	if got := readHookMatcherStringValues(map[string]any{}, "x"); len(got) != 0 {
		t.Fatal("empty raw should return nil")
	}
	if got := readHookMatcherStringValues(map[string]any{"x": nil}, "x"); len(got) != 0 {
		t.Fatal("nil value should return nil")
	}
	if got := readHookMatcherStringValues(map[string]any{"x": "  "}, "x"); len(got) != 0 {
		t.Fatal("whitespace-only string should return nil")
	}
	if got := readHookMatcherStringValues(map[string]any{"x": 42}, "x"); len(got) != 1 || got[0] != "42" {
		t.Fatalf("int value should be converted to string, got %v", got)
	}
	if got := readHookMatcherStringValues(map[string]any{"x": []any{" a ", nil, 123}}, "x"); len(got) != 2 || got[0] != "a" || got[1] != "123" {
		t.Fatalf("[]any with mixed values, got %v", got)
	}
	if got := readHookMatcherStringValues(map[string]any{"x": "hello"}, "y"); len(got) != 0 {
		t.Fatal("missing key should return nil")
	}
}

func TestNormalizeHookMatcherValues(t *testing.T) {
	t.Parallel()

	if got := normalizeHookMatcherValues(nil); len(got) != 0 {
		t.Fatal("nil values should return nil")
	}
	if got := normalizeHookMatcherValues([]string{}); len(got) != 0 {
		t.Fatal("empty values should return nil")
	}
	if got := normalizeHookMatcherValues([]string{"  ", "\t"}); len(got) != 0 {
		t.Fatal("whitespace-only values should return empty")
	}
	if got := normalizeHookMatcherValues([]string{" BASH ", "", " Filesystem "}); len(got) != 2 || got[0] != "bash" || got[1] != "filesystem" {
		t.Fatalf("mixed values should be normalized, got %v", got)
	}
}

func TestContainsEqualFold(t *testing.T) {
	t.Parallel()

	if containsEqualFold(nil, "bash") {
		t.Fatal("nil values should not match")
	}
	if containsEqualFold([]string{"bash"}, "") {
		t.Fatal("empty target should not match")
	}
	if containsEqualFold([]string{"bash"}, "  ") {
		t.Fatal("whitespace-only target should not match")
	}
	if !containsEqualFold([]string{"BASH", "FILESYSTEM"}, "  bash  ") {
		t.Fatal("case-insensitive match should work")
	}
	if containsEqualFold([]string{"bash"}, "python") {
		t.Fatal("non-matching should return false")
	}
}

func TestReadHookMatcherMetadataString(t *testing.T) {
	t.Parallel()

	if got := readHookMatcherMetadataString(nil, "x"); got != "" {
		t.Fatal("nil metadata should return empty")
	}
	if got := readHookMatcherMetadataString(map[string]any{}, "x"); got != "" {
		t.Fatal("empty metadata should return empty")
	}
	if got := readHookMatcherMetadataString(map[string]any{"x": nil}, "x"); got != "" {
		t.Fatal("nil value should return empty")
	}
	if got := readHookMatcherMetadataString(map[string]any{"x": 123}, "x"); got != "123" {
		t.Fatalf("non-string value should be converted, got %q", got)
	}
	if got := readHookMatcherMetadataString(map[string]any{"x": "hello"}, "  "); got != "" {
		t.Fatal("empty key should return empty")
	}
	if got := readHookMatcherMetadataString(map[string]any{"x": "hello"}, ""); got != "" {
		t.Fatal("empty string key should return empty")
	}
	if got := readHookMatcherMetadataString(map[string]any{"TOOL_NAME": "bash"}, "tool_name"); got != "bash" {
		t.Fatalf("case-insensitive key lookup failed, got %q", got)
	}
	if got := readHookMatcherMetadataString(map[string]any{"y": "hello"}, "x"); got != "" {
		t.Fatal("missing key should return empty")
	}
}

func TestCompileHookMatcherRegexWhitespaceSkipped(t *testing.T) {
	t.Parallel()

	matcher, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{
		"tool_name":       "bash",
		"tool_name_regex": []string{"  ", "\t"},
	})
	if err != nil {
		t.Fatalf("CompileHookMatcher() error = %v", err)
	}
	if matcher == nil {
		t.Fatal("expected matcher compiled even when regex values are whitespace-only")
	}
	if len(matcher.ToolNameRegex) != 0 {
		t.Fatalf("expected empty tool_name_regex slice, got %d entries", len(matcher.ToolNameRegex))
	}
}

func TestCompileHookMatcherRegexOnly(t *testing.T) {
	t.Parallel()

	matcher, err := CompileHookMatcher(HookPointBeforeToolCall, map[string]any{
		"tool_name_regex": `^bash`,
	})
	if err != nil {
		t.Fatalf("CompileHookMatcher() error = %v", err)
	}
	if matcher == nil {
		t.Fatal("expected matcher compiled for regex-only config")
	}
	if !matcher.Match(HookContext{Metadata: map[string]any{"tool_name": "bash-script"}}) {
		t.Fatal("expected regex to match")
	}
}
