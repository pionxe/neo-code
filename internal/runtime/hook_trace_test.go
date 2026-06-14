package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHookTracePath(t *testing.T) {
	path, err := HookTracePath(t.TempDir(), t.TempDir(), "run-1")
	if err != nil {
		t.Fatalf("HookTracePath() error = %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("hook-traces", "run-1.jsonl")) {
		t.Fatalf("unexpected trace path: %s", path)
	}
}

func TestHookTracePathRejectsEmptyInputs(t *testing.T) {
	cases := []struct {
		name      string
		baseDir   string
		workspace string
		runID     string
		want      string
	}{
		{name: "baseDir", workspace: t.TempDir(), runID: "run-1", want: "baseDir is empty"},
		{name: "workspace", baseDir: t.TempDir(), runID: "run-1", want: "workspace is empty"},
		{name: "runID", baseDir: t.TempDir(), workspace: t.TempDir(), want: "run_id is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := HookTracePath(tc.baseDir, tc.workspace, tc.runID)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("HookTracePath() error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestHookTracePathEscapesRunID(t *testing.T) {
	path, err := HookTracePath(t.TempDir(), t.TempDir(), "../../../../.ssh/foo")
	if err != nil {
		t.Fatalf("HookTracePath() error = %v", err)
	}
	if strings.Contains(path, "..") {
		t.Fatalf("expected escaped trace path, got %s", path)
	}
	if strings.Contains(path, string(filepath.Separator)+".ssh"+string(filepath.Separator)) {
		t.Fatalf("expected run_id to stay inside hook-traces dir, got %s", path)
	}
	if filepath.Base(path) != "~2e~2e~2f~2e~2e~2f~2e~2e~2f~2e~2e~2f~2essh~2ffoo.jsonl" {
		t.Fatalf("unexpected escaped file name: %s", filepath.Base(path))
	}
}

func TestHookTraceRunIDEscapingCoversUnsafeBytes(t *testing.T) {
	if got := escapeHookTraceRunID(" run:你好/A_B-9 "); !strings.Contains(got, "~3a") || !strings.Contains(got, "~2f") {
		t.Fatalf("escapeHookTraceRunID() = %q, want escaped colon and slash", got)
	}
	if got := escapeHookTraceRunID(" "); got != "" {
		t.Fatalf("escapeHookTraceRunID(blank) = %q, want empty", got)
	}
	for _, value := range []byte{'a', 'Z', '7', '-', '_'} {
		if !isHookTraceSafeRunIDByte(value) {
			t.Fatalf("byte %q should be safe", value)
		}
	}
	if isHookTraceSafeRunIDByte('.') {
		t.Fatal("dot should not be a safe run_id byte")
	}
}

func TestHookTraceRecorderWritesHookEvents(t *testing.T) {
	baseDir := t.TempDir()
	workspace := t.TempDir()
	recorder := NewHookTraceRecorder(baseDir, workspace)
	t.Cleanup(func() {
		if err := recorder.Close(); err != nil {
			t.Fatalf("recorder.Close() error = %v", err)
		}
	})

	recorder.RecordRuntimeEvent(context.Background(), RuntimeEvent{
		Type:      EventHookFinished,
		RunID:     "run-1",
		SessionID: "session-1",
		Turn:      2,
		Phase:     "execute",
		Timestamp: time.Unix(123, 0).UTC(),
		Payload: HookEventPayload{
			HookID:     "warn-bash",
			Point:      "before_tool_call",
			Source:     "user",
			Kind:       "function",
			Mode:       "sync",
			Status:     "pass",
			Message:    "ok",
			StartedAt:  time.Unix(122, 0).UTC(),
			DurationMS: 12,
		},
	})

	tracePath, err := HookTracePath(baseDir, workspace, "run-1")
	if err != nil {
		t.Fatalf("HookTracePath() error = %v", err)
	}
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	if !strings.Contains(string(data), `"event_type":"hook_finished"`) {
		t.Fatalf("trace file missing hook_finished record: %s", string(data))
	}
	if !strings.Contains(string(data), `"hook_id":"warn-bash"`) {
		t.Fatalf("trace file missing hook id: %s", string(data))
	}
}

func TestHookTraceRecorderWritesBlockedPayloadAndReusesWriter(t *testing.T) {
	baseDir := t.TempDir()
	workspace := t.TempDir()
	recorder := NewHookTraceRecorder(" "+baseDir+" ", " "+workspace+" ")
	t.Cleanup(func() {
		if err := recorder.Close(); err != nil {
			t.Fatalf("recorder.Close() error = %v", err)
		}
	})

	event := RuntimeEvent{
		Type:      EventHookBlocked,
		RunID:     "run-block",
		SessionID: "session-1",
		Timestamp: time.Unix(200, 0).UTC(),
		Payload: HookBlockedPayload{
			HookID: "guard",
			Point:  "accept_gate",
			Source: "repo",
			Reason: "manual review",
		},
	}
	recorder.RecordRuntimeEvent(context.Background(), event)
	recorder.RecordRuntimeEvent(context.Background(), event)
	if len(recorder.writers) != 1 {
		t.Fatalf("writer count = %d, want 1", len(recorder.writers))
	}

	tracePath, err := HookTracePath(baseDir, workspace, "run-block")
	if err != nil {
		t.Fatalf("HookTracePath() error = %v", err)
	}
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	lines := strings.Count(strings.TrimSpace(string(data)), "\n") + 1
	if lines != 2 {
		t.Fatalf("trace lines = %d, want 2: %s", lines, string(data))
	}
	if !strings.Contains(string(data), `"status":"block"`) || !strings.Contains(string(data), `"message":"manual review"`) {
		t.Fatalf("trace file missing blocked payload fields: %s", string(data))
	}
}

func TestHookTraceRecorderIgnoresUnsupportedEventsAndInvalidRecords(t *testing.T) {
	baseDir := t.TempDir()
	workspace := t.TempDir()
	recorder := NewHookTraceRecorder(baseDir, workspace)

	recorder.RecordRuntimeEvent(context.Background(), RuntimeEvent{Type: EventAgentChunk, RunID: "run-1"})
	recorder.RecordRuntimeEvent(context.Background(), RuntimeEvent{
		Type:      EventHookFinished,
		Timestamp: time.Unix(1, 0),
		Payload:   HookEventPayload{HookID: "missing-run"},
	})
	recorder.RecordRuntimeEvent(context.Background(), RuntimeEvent{
		Type:    EventHookFinished,
		RunID:   "run-1",
		Payload: map[string]any{"hook_id": "unsupported"},
	})
	if err := recorder.Close(); err != nil {
		t.Fatalf("recorder.Close() error = %v", err)
	}

	projectRoot := filepath.Join(baseDir, "projects")
	entries, err := os.ReadDir(projectRoot)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir(project root) error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no trace directories, got %d", len(entries))
	}
}

func TestHookTraceRecorderCloseHandlesNilAndInjectedWriters(t *testing.T) {
	if err := (*HookTraceRecorder)(nil).Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}

	recorder := NewHookTraceRecorder(t.TempDir(), t.TempDir())
	recorder.writers["nil"] = nil
	if err := recorder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(recorder.writers) != 0 {
		t.Fatalf("writers len = %d, want 0", len(recorder.writers))
	}
}

func TestHookTraceRecorderSkipsWriterErrors(t *testing.T) {
	recorder := NewHookTraceRecorder("", t.TempDir())
	recorder.RecordRuntimeEvent(context.Background(), RuntimeEvent{
		Type:      EventHookFinished,
		RunID:     "run-1",
		Timestamp: time.Unix(1, 0).UTC(),
		Payload:   HookEventPayload{HookID: "warn-bash"},
	})
	if len(recorder.writers) != 0 {
		t.Fatalf("writer count = %d, want 0 after invalid baseDir", len(recorder.writers))
	}
}

func TestBuildHookTraceRecordPayloadVariants(t *testing.T) {
	startedAt := time.Unix(50, 0).UTC()
	record, ok := buildHookTraceRecord(RuntimeEvent{
		Type:      EventHookFailed,
		RunID:     " run-1 ",
		SessionID: " session-1 ",
		Turn:      3,
		Phase:     " hooks ",
		Timestamp: time.Unix(60, 0),
		Payload: HookEventPayload{
			HookID:     " hook ",
			Point:      " before_tool_call ",
			Source:     " user ",
			Kind:       " builtin ",
			Mode:       " sync ",
			Status:     " failed ",
			Message:    " nope ",
			Error:      " boom ",
			StartedAt:  startedAt,
			DurationMS: 22,
		},
	})
	if !ok {
		t.Fatal("buildHookTraceRecord() ok = false")
	}
	if record.RunID != "run-1" || record.HookID != "hook" || record.Error != "boom" || record.StartedAt != startedAt {
		encoded, _ := json.Marshal(record)
		t.Fatalf("unexpected record: %s", encoded)
	}

	if _, ok := buildHookTraceRecord(RuntimeEvent{Type: EventHookFinished, RunID: "run-1"}); ok {
		t.Fatal("expected unsupported payload to be rejected")
	}
	if _, ok := buildHookTraceRecord(RuntimeEvent{Type: EventHookFinished, Payload: HookEventPayload{HookID: "x"}}); ok {
		t.Fatal("expected missing run_id to be rejected")
	}
}
