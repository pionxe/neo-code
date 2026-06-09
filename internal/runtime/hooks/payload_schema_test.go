package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPayloadSchemaTopLevelStableFields(t *testing.T) {
	t.Parallel()

	want := []string{
		"hook_id",
		"kind",
		"mode",
		"payload_version",
		"point",
		"run_id",
		"scope",
		"session_id",
		"triggered_at",
	}
	for _, point := range ListHookPoints() {
		point := point
		t.Run(string(point), func(t *testing.T) {
			t.Parallel()
			got := fieldNamesByStability(PayloadSchema(point).TopLevel, PayloadStabilityStable)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("top-level stable fields = %v, want %v", got, want)
			}
		})
	}
}

func TestPayloadSchemaMetadataStableFields(t *testing.T) {
	t.Parallel()

	want := map[HookPoint][]string{
		HookPointAcceptGate:               {"assistant_text_empty", "run_id", "session_id", "workdir", "workspace_changed"},
		HookPointAfterToolFailure:         {"error_class", "execution_error", "is_error", "run_id", "session_id", "tool_call_id", "tool_name", "workdir"},
		HookPointAfterToolResult:          {"error_class", "execution_error", "is_error", "result_metadata_present", "run_id", "session_id", "tool_call_id", "tool_name", "workdir"},
		HookPointBeforeCompletionDecision: {"run_id", "session_id"},
		HookPointBeforePermissionDecision: {"decision", "reason", "rule_id", "run_id", "session_id", "tool_call_id", "tool_name", "workdir"},
		HookPointBeforeToolCall:           {"run_id", "session_id", "tool_call_id", "tool_name", "workdir"},
		HookPointPostCompact:              {"applied", "run_id", "session_id", "trigger_mode", "workdir"},
		HookPointPreCompact:               {"run_id", "session_id", "trigger_mode", "workdir"},
		HookPointSessionEnd:               {"detail", "run_id", "session_id", "stop_reason"},
		HookPointSessionStart:             {"run_id", "session_id", "workdir"},
		HookPointSubAgentStart:            {"role", "run_id", "session_id", "task_id", "tool_name", "trigger", "workdir", "workspace"},
		HookPointSubAgentStop:             {"error", "role", "run_id", "session_id", "state", "step_count", "stop_reason", "task_id"},
		HookPointUserPromptSubmit:         {"run_id", "session_id", "workdir"},
	}
	for point, expected := range want {
		point := point
		expected := expected
		t.Run(string(point), func(t *testing.T) {
			t.Parallel()
			got := fieldNamesByStability(PayloadSchema(point).Metadata, PayloadStabilityStable)
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("stable metadata fields = %v, want %v", got, expected)
			}
		})
	}
}

func TestPayloadSchemaDropsUnproducedAllowlistFields(t *testing.T) {
	t.Parallel()

	blocked := map[string]struct{}{
		"point":             {},
		"completion_passed": {},
		"has_tool_calls":    {},
		"assistant_role":    {},
	}
	for _, schema := range ListPayloadSchemas() {
		for _, field := range schema.Metadata {
			if _, exists := blocked[field.Name]; exists {
				t.Fatalf("payload schema for %q should not expose unproduced field %q", schema.Point, field.Name)
			}
		}
	}
}

func TestPayloadSchemaNestedTypes(t *testing.T) {
	t.Parallel()

	schema := PayloadSchema(HookPointAcceptGate)
	if len(schema.Metadata) == 0 {
		t.Fatal("accept_gate metadata should not be empty")
	}
	var todoSummary PayloadFieldSchema
	var recentToolSummary PayloadFieldSchema
	for _, field := range schema.Metadata {
		switch field.Name {
		case "todo_summary":
			todoSummary = field
		case "recent_tool_summary":
			recentToolSummary = field
		}
	}
	if todoSummary.JSONType != "object" || len(todoSummary.Properties) == 0 {
		t.Fatalf("todo_summary schema = %+v, want object with properties", todoSummary)
	}
	if recentToolSummary.JSONType != "array" || recentToolSummary.ItemType != "object" || len(recentToolSummary.ItemProperties) == 0 {
		t.Fatalf("recent_tool_summary schema = %+v, want array of objects", recentToolSummary)
	}
}

func TestMarshalPayloadJSONSchemaMatchesGeneratedFile(t *testing.T) {
	t.Parallel()

	got, err := MarshalPayloadJSONSchema()
	if err != nil {
		t.Fatalf("MarshalPayloadJSONSchema() error = %v", err)
	}
	targetPath := filepath.Join("..", "..", "..", "docs", "reference", fmt.Sprintf("hook-payload.v%s.json", PayloadVersion))
	want, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read generated schema file: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("generated schema mismatch with %s; run `go generate ./internal/runtime/hooks`", targetPath)
	}
}
