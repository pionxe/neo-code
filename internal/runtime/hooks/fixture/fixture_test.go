package fixture

import (
	"strings"
	"testing"
)

func TestParseBytesYAMLAndJSON(t *testing.T) {
	yamlFixture := []byte(`
payload_version: "1"
point: before_tool_call
run_id: run-1
session_id: session-1
metadata:
  tool_name: bash
  tool_call_id: call-1
`)
	parsed, err := ParseBytes(yamlFixture, "fixture.yaml")
	if err != nil {
		t.Fatalf("ParseBytes(yaml) error = %v", err)
	}
	if string(parsed.Point) != "before_tool_call" {
		t.Fatalf("point = %q, want before_tool_call", parsed.Point)
	}

	jsonFixture := []byte(`{"payload_version":"1","point":"before_tool_call","metadata":{"tool_name":"bash","tool_call_id":"call-1"}}`)
	if _, err := ParseBytes(jsonFixture, "fixture.json"); err != nil {
		t.Fatalf("ParseBytes(json) error = %v", err)
	}
}

func TestParseBytesRejectsUnknownFieldsAndSchemaMismatch(t *testing.T) {
	unknownField := []byte(`
payload_version: "1"
point: before_tool_call
metadata:
  tool_name: bash
extra_field: value
`)
	if _, err := ParseBytes(unknownField, "fixture.yaml"); err == nil || !strings.Contains(err.Error(), "unknown top-level field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}

	badVersion := []byte(`
payload_version: "9"
point: before_tool_call
metadata:
  tool_name: bash
`)
	if _, err := ParseBytes(badVersion, "fixture.yaml"); err == nil || !strings.Contains(err.Error(), "payload_version") {
		t.Fatalf("expected payload_version error, got %v", err)
	}

	badMetadata := []byte(`
payload_version: "1"
point: before_tool_call
metadata:
  phase: plan
`)
	if _, err := ParseBytes(badMetadata, "fixture.yaml"); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected metadata schema error, got %v", err)
	}
}
