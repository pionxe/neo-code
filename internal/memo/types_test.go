package memo

import (
	"testing"
)

func TestValidTypes(t *testing.T) {
	types := ValidTypes()
	if len(types) != 4 {
		t.Fatalf("Expected 4 valid types, got %d", len(types))
	}
	expected := []Type{TypeUser, TypeFeedback, TypeProject, TypeReference}
	for i, typ := range expected {
		if types[i] != typ {
			t.Errorf("ValidTypes()[%d] = %q, want %q", i, types[i], typ)
		}
	}
}

func TestIsValidType(t *testing.T) {
	tests := []struct {
		input Type
		want  bool
	}{
		{TypeUser, true},
		{TypeFeedback, true},
		{TypeProject, true},
		{TypeReference, true},
		{Type("invalid"), false},
		{Type(""), false},
	}
	for _, tt := range tests {
		if got := IsValidType(tt.input); got != tt.want {
			t.Errorf("IsValidType(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseType(t *testing.T) {
	tests := []struct {
		input  string
		want   Type
		wantOK bool
	}{
		{"user", TypeUser, true},
		{"feedback", TypeFeedback, true},
		{"project", TypeProject, true},
		{"reference", TypeReference, true},
		{"invalid", Type(""), false},
		{"", Type(""), false},
		{"USER", Type(""), false},
	}
	for _, tt := range tests {
		got, ok := ParseType(tt.input)
		if ok != tt.wantOK {
			t.Errorf("ParseType(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
		}
		if ok && got != tt.want {
			t.Errorf("ParseType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSourceConstants(t *testing.T) {
	if SourceAutoExtract != "extractor_auto" {
		t.Errorf("SourceAutoExtract = %q, want %q", SourceAutoExtract, "extractor_auto")
	}
	if SourceUserManual != "user_manual" {
		t.Errorf("SourceUserManual = %q, want %q", SourceUserManual, "user_manual")
	}
	if SourceToolInitiated != "tool_initiated" {
		t.Errorf("SourceToolInitiated = %q, want %q", SourceToolInitiated, "tool_initiated")
	}
}

func TestEntryFields(t *testing.T) {
	e := Entry{
		ID:        "user_abc123",
		Type:      TypeUser,
		Title:     "偏好 tab 缩进",
		Content:   "用户偏好使用 tab 缩进...",
		Keywords:  []string{"tabs", "indentation"},
		Source:    SourceUserManual,
		TopicFile: "user_profile.md",
	}
	if e.Type != TypeUser {
		t.Errorf("Entry.Type = %q, want %q", e.Type, TypeUser)
	}
	if e.Source != SourceUserManual {
		t.Errorf("Entry.Source = %q, want %q", e.Source, SourceUserManual)
	}
	if len(e.Keywords) != 2 {
		t.Errorf("len(Entry.Keywords) = %d, want 2", len(e.Keywords))
	}
}

func TestEntryEvictionPriority(t *testing.T) {
	tests := []struct {
		name  string
		entry Entry
		want  int
	}{
		{
			name:  "reference auto extract",
			entry: Entry{Type: TypeReference, Source: SourceAutoExtract},
			want:  10,
		},
		{
			name:  "project auto extract",
			entry: Entry{Type: TypeProject, Source: SourceAutoExtract},
			want:  20,
		},
		{
			name:  "feedback auto extract",
			entry: Entry{Type: TypeFeedback, Source: SourceAutoExtract},
			want:  30,
		},
		{
			name:  "user auto extract",
			entry: Entry{Type: TypeUser, Source: SourceAutoExtract},
			want:  40,
		},
		{
			name:  "manual reference",
			entry: Entry{Type: TypeReference, Source: SourceUserManual},
			want:  60,
		},
		{
			name:  "tool initiated feedback",
			entry: Entry{Type: TypeFeedback, Source: SourceToolInitiated},
			want:  80,
		},
		{
			name:  "unknown type",
			entry: Entry{Type: Type("unknown"), Source: SourceAutoExtract},
			want:  0,
		},
	}
	for _, tt := range tests {
		if got := tt.entry.evictionPriority(); got != tt.want {
			t.Fatalf("%s: evictionPriority() = %d, want %d", tt.name, got, tt.want)
		}
	}
}
