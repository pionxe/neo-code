package state

import (
	"testing"
	"time"

	"neo-code/internal/tuiv2/gateway"
)

// 本文件补齐 state 包旧路径（全量 Reduce）的分支覆盖，
// 使包覆盖率满足 issue #23 验收的 100% 硬目标。

type stringerPayload struct{}

func (stringerPayload) String() string { return "via-stringer" }

func TestPayloadStringBranches(t *testing.T) {
	if got := payloadString(map[string]any{"k": stringerPayload{}}, "k"); got != "via-stringer" {
		t.Fatalf("Stringer branch = %q", got)
	}
	if got := payloadString(map[string]any{"k": nil}, "k"); got != "" {
		t.Fatalf("nil branch = %q", got)
	}
	if got := payloadString(map[string]any{"k": 42}, "k"); got != "42" {
		t.Fatalf("fmt.Sprint branch = %q", got)
	}
	if got := payloadString(map[string]any{"k": "v"}, "missing", "k"); got != "v" {
		t.Fatalf("multi-key fallback = %q", got)
	}
	if got := payloadString(nil, "k"); got != "" {
		t.Fatalf("nil payload = %q", got)
	}
}

func TestPayloadIntBranches(t *testing.T) {
	if got := payloadInt(map[string]any{"k": int64(7)}, 0, "k"); got != 7 {
		t.Fatalf("int64 branch = %d", got)
	}
	if got := payloadInt(map[string]any{"k": 1.9}, 0, "k"); got != 1 {
		t.Fatalf("float64 branch = %d", got)
	}
	if got := payloadInt(map[string]any{}, 5, "k"); got != 5 {
		t.Fatalf("fallback branch = %d", got)
	}
}

func TestPayloadStringSliceBranches(t *testing.T) {
	if got := payloadStringSlice(map[string]any{"opts": []string{"a"}}, "opts"); len(got) != 1 || got[0] != "a" {
		t.Fatalf("[]string branch = %v", got)
	}
	if got := payloadStringSlice(map[string]any{"opts": []any{1, true}}, "opts"); len(got) != 2 || got[0] != "1" || got[1] != "true" {
		t.Fatalf("[]any branch = %v", got)
	}
	if got := payloadStringSlice(map[string]any{"opts": 3}, "opts"); got != nil {
		t.Fatalf("non-slice branch = %v", got)
	}
	if got := payloadStringSlice(nil, "opts"); got != nil {
		t.Fatalf("nil payload branch = %v", got)
	}
}

func TestDeleteSessionAndUpsertSession(t *testing.T) {
	sessions := []gateway.SessionSummary{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	kept := deleteSession(sessions, "b")
	if len(kept) != 2 || kept[0].ID != "a" || kept[1].ID != "c" {
		t.Fatalf("deleteSession = %v", kept)
	}
	// upsert：更新命中。
	updated := upsertSession(sessions, gateway.SessionSummary{ID: "b", Title: "B2"})
	if len(updated) != 3 || updated[1].Title != "B2" {
		t.Fatalf("upsert update = %v", updated)
	}
	// upsert：追加。
	appended := upsertSession(sessions, gateway.SessionSummary{ID: "d", Title: "D"})
	if len(appended) != 4 || appended[3].ID != "d" {
		t.Fatalf("upsert append = %v", appended)
	}
	// upsert：空 ID no-op（返回拷贝）。
	noop := upsertSession(sessions, gateway.SessionSummary{})
	if len(noop) != 3 {
		t.Fatalf("upsert empty id = %v", noop)
	}
}

func TestReduceAgentChunkNilMetadataOnNewEntry(t *testing.T) {
	// 新建消息条目路径：streamEntry 构造的 Metadata 非 nil；本用例锁
	// open entry 的 role 注入与 done 置位。
	next := Reduce(NewViewState(), event(gateway.EventAgentChunk, map[string]any{"text": "hi"}))
	if next.Stream[0].Metadata["role"] != "assistant" || next.Stream[0].Metadata["done"] != false {
		t.Fatalf("metadata = %v", next.Stream[0].Metadata)
	}
}

func TestReduceAgentMessageEndNonMessageTail(t *testing.T) {
	// 末条目非 message：end 只更新 tokens，不标记。
	next := Reduce(NewViewState(), event(gateway.EventToolStart, map[string]any{"tool": "t"}))
	next = Reduce(next, event(gateway.EventAgentMessageEnd, map[string]any{"total": 3}))
	if next.Stream[len(next.Stream)-1].Type != "tool_start" {
		t.Fatalf("tail type = %s", next.Stream[len(next.Stream)-1].Type)
	}
	if next.Runtime.Tokens.Total != 3 {
		t.Fatalf("tokens = %+v", next.Runtime.Tokens)
	}
}

func TestStreamEntryDoneNonBoolMetadata(t *testing.T) {
	if streamEntryDone(StreamEntry{Metadata: map[string]any{"done": "yes"}}) {
		t.Fatal("non-bool done should not count as done")
	}
	if streamEntryDone(StreamEntry{}) {
		t.Fatal("nil metadata should not count as done")
	}
}

func TestEventTimeZeroFallsBackToNow(t *testing.T) {
	if eventTime(time.Time{}).IsZero() {
		t.Fatal("zero time should fall back to now")
	}
}

func TestReduceNilInputCreatesState(t *testing.T) {
	// nil 输入契约：返回新建空状态（Reduce 第一分支）。
	next := Reduce(nil, event(gateway.EventRunStarted, nil))
	if next == nil || next.Runtime.Phase != RuntimePhaseRunning {
		t.Fatalf("nil input reduce = %+v", next)
	}
}

func TestReduceAgentChunkOpenEntryNilMetadata(t *testing.T) {
	// open entry 且 Metadata 为 nil 的防御分支（S1 惰性建表）。
	next := NewViewState()
	next.Stream = []StreamEntry{{ID: "m", Type: "message", Content: "base"}}
	next = Reduce(next, event(gateway.EventAgentChunk, map[string]any{"text": "!"}))
	if next.Stream[0].Content != "base!" {
		t.Fatalf("content = %q", next.Stream[0].Content)
	}
	if next.Stream[0].Metadata["role"] != "assistant" {
		t.Fatalf("metadata = %v", next.Stream[0].Metadata)
	}
}

func TestReduceAgentMessageEndOpenEntryNilMetadata(t *testing.T) {
	// end 标记对 nil Metadata open entry 的惰性建表分支。
	next := NewViewState()
	next.Stream = []StreamEntry{{ID: "m", Type: "message", Content: "x"}}
	next = Reduce(next, event(gateway.EventAgentMessageEnd, nil))
	if next.Stream[0].Metadata["done"] != true {
		t.Fatalf("metadata = %v", next.Stream[0].Metadata)
	}
}

func TestClonePayloadEmptyAndEventTimeNonZero(t *testing.T) {
	clone := clonePayload(map[string]any{})
	if clone == nil || len(clone) != 0 {
		t.Fatalf("empty payload clone = %v", clone)
	}
	at := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	if !eventTime(at).Equal(at) {
		t.Fatal("non-zero At should pass through")
	}
}
