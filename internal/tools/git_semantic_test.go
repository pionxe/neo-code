package tools

import "testing"

func TestNormalizeGitSemanticClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "read only", raw: "read_only", want: BashIntentClassificationReadOnly},
		{name: "remote op upper", raw: "REMOTE_OP", want: BashIntentClassificationRemoteOp},
		{name: "destructive with spaces", raw: " destructive ", want: BashIntentClassificationDestructive},
		{name: "unknown fallback", raw: "custom", want: BashIntentClassificationUnknown},
		{name: "empty fallback", raw: "", want: BashIntentClassificationUnknown},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeGitSemanticClass(tt.raw); got != tt.want {
				t.Fatalf("NormalizeGitSemanticClass(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestBashGitResourceForClass(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "read only", raw: BashIntentClassificationReadOnly, want: BashGitResourceReadOnly},
		{name: "local mutation", raw: BashIntentClassificationLocalMutation, want: BashGitResourceLocalMutation},
		{name: "remote", raw: BashIntentClassificationRemoteOp, want: BashGitResourceRemoteOp},
		{name: "destructive", raw: BashIntentClassificationDestructive, want: BashGitResourceDestructive},
		{name: "unknown", raw: "something-else", want: BashGitResourceUnknown},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := BashGitResourceForClass(tt.raw); got != tt.want {
				t.Fatalf("BashGitResourceForClass(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
