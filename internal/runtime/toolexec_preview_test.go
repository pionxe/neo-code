package runtime

import (
	"strings"
	"testing"
)

func TestBuildToolArgumentsPreviewMaskJSONSensitiveFields(t *testing.T) {
	t.Parallel()

	raw := `{"api_key":"sk-123","password":"p@ss","nested":{"secret":"abc"},"safe":"ok"}`
	preview := buildToolArgumentsPreview(raw)
	if strings.Contains(preview, "sk-123") {
		t.Fatalf("preview leaked api_key: %q", preview)
	}
	if strings.Contains(preview, "p@ss") {
		t.Fatalf("preview leaked password: %q", preview)
	}
	if strings.Contains(preview, `"secret":"abc"`) {
		t.Fatalf("preview leaked nested secret: %q", preview)
	}
	if !strings.Contains(preview, `"api_key":"***"`) {
		t.Fatalf("preview should mask api_key: %q", preview)
	}
	if !strings.Contains(preview, `"password":"***"`) {
		t.Fatalf("preview should mask password: %q", preview)
	}
	if !strings.Contains(preview, `"secret":"***"`) {
		t.Fatalf("preview should mask nested secret: %q", preview)
	}
	if !strings.Contains(preview, `"safe":"ok"`) {
		t.Fatalf("preview should keep non-sensitive keys: %q", preview)
	}
}

func TestBuildToolArgumentsPreviewMaskNonJSONFallback(t *testing.T) {
	t.Parallel()

	preview := buildToolArgumentsPreview(`token=abc password:xyz arg=ok`)
	if strings.Contains(preview, "abc") || strings.Contains(preview, "xyz") {
		t.Fatalf("preview leaked fallback credentials: %q", preview)
	}
	if !strings.Contains(preview, "token=***") {
		t.Fatalf("preview should mask token in fallback mode: %q", preview)
	}
	if !strings.Contains(preview, "password=***") {
		t.Fatalf("preview should mask password in fallback mode: %q", preview)
	}
}

func TestBuildToolArgumentsPreviewTruncate(t *testing.T) {
	t.Parallel()

	raw := strings.Repeat("a", hookToolArgumentsPreviewMaxChars+20)
	preview := buildToolArgumentsPreview(raw)
	if len([]rune(preview)) != hookToolArgumentsPreviewMaxChars {
		t.Fatalf("preview length=%d, want %d", len([]rune(preview)), hookToolArgumentsPreviewMaxChars)
	}
}

func TestIsSensitiveHookToolArgumentKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want bool
	}{
		{key: "api_key", want: true},
		{key: "accessKey", want: true},
		{key: "authorization", want: true},
		{key: "auth_token", want: true},
		{key: "password", want: true},
		{key: "author", want: false},
		{key: "tool_name", want: false},
	}
	for _, tc := range cases {
		if got := isSensitiveHookToolArgumentKey(tc.key); got != tc.want {
			t.Fatalf("isSensitiveHookToolArgumentKey(%q)=%v, want %v", tc.key, got, tc.want)
		}
	}
}
