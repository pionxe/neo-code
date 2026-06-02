package filesystem

import (
	"testing"
)

func TestFilesystemToolMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		toolName    string
		description string
		schema      map[string]any
	}{
		{
			name:        "delete file",
			toolName:    NewDelete("/workspace").Name(),
			description: NewDelete("/workspace").Description(),
			schema:      NewDelete("/workspace").Schema(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.toolName == "" {
				t.Fatal("tool name should not be empty")
			}
			if tt.description == "" {
				t.Fatal("description should not be empty")
			}
			if got, _ := tt.schema["type"].(string); got != "object" {
				t.Fatalf("schema type = %q, want object", got)
			}
			required, ok := tt.schema["required"].([]string)
			if !ok || len(required) == 0 {
				t.Fatalf("required schema fields missing: %#v", tt.schema["required"])
			}
		})
	}
}
