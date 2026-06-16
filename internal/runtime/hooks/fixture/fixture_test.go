package fixture

import (
	"os"
	"path/filepath"
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

func TestParseFileReadsFixtureAndWrapsReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.yaml")
	if err := os.WriteFile(path, []byte(`
payload_version: "1"
point: before_tool_call
run_id: run-file
session_id: session-file
metadata:
  tool_name: bash
  tool_call_id: call-1
`), 0o644); err != nil {
		t.Fatalf("WriteFile(fixture) error = %v", err)
	}

	parsed, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if parsed.Context.RunID != "run-file" || parsed.Context.SessionID != "session-file" {
		t.Fatalf("context = %#v, want run/session from fixture", parsed.Context)
	}
	if _, ok := parsed.Payload["metadata"]; !ok {
		t.Fatalf("payload = %#v, want metadata preserved", parsed.Payload)
	}

	if _, err := ParseFile(filepath.Join(dir, "missing.yaml")); err == nil || !strings.Contains(err.Error(), "read fixture") {
		t.Fatalf("expected wrapped read error, got %v", err)
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

func TestParseBytesRejectsMalformedAndUnsupportedFixtures(t *testing.T) {
	cases := []struct {
		name       string
		sourcePath string
		content    string
		want       string
	}{
		{
			name:       "empty",
			sourcePath: "fixture.yaml",
			content:    " \n\t ",
			want:       "fixture is empty",
		},
		{
			name:       "bad json",
			sourcePath: "fixture.json",
			content:    `{"payload_version":`,
			want:       "parse fixture json",
		},
		{
			name:       "bad yaml",
			sourcePath: "fixture.yaml",
			content:    "payload_version: [",
			want:       "parse fixture yaml",
		},
		{
			name:       "unsupported point",
			sourcePath: "fixture.yaml",
			content: `
payload_version: "1"
point: unsupported_point
`,
			want: "not supported",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tc.content), tc.sourcePath)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ParseBytes() error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestParseBytesDefaultsToYAMLAndInitializesMetadata(t *testing.T) {
	parsed, err := ParseBytes([]byte(`
payload_version: "1"
point: session_start
run_id: " run-1 "
session_id: " session-1 "
`), "fixture")
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	if string(parsed.Point) != "session_start" {
		t.Fatalf("point = %q, want session_start", parsed.Point)
	}
	if parsed.Context.RunID != "run-1" || parsed.Context.SessionID != "session-1" {
		t.Fatalf("context = %#v, want trimmed run/session", parsed.Context)
	}
	if parsed.Context.Metadata == nil {
		t.Fatal("metadata map is nil")
	}
}
