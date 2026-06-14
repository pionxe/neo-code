package runtime

import (
	"context"
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
