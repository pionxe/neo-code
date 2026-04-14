package context

import (
	"strings"
	"testing"

	providertypes "neo-code/internal/provider/types"
)

func TestProjectToolMessagesForModelSkipsMessagesThatCannotBeProjected(t *testing.T) {
	t.Parallel()

	messages := []providertypes.Message{
		{Role: providertypes.RoleUser, Content: "user"},
		{
			Role:         providertypes.RoleTool,
			ToolCallID:   "call-1",
			Content:      "tool output",
			ToolMetadata: nil,
		},
		{
			Role:         providertypes.RoleTool,
			ToolCallID:   "call-2",
			Content:      "   ",
			ToolMetadata: map[string]string{"tool_name": "bash"},
		},
		{
			Role:         providertypes.RoleTool,
			ToolCallID:   "call-3",
			Content:      microCompactClearedMessage,
			ToolMetadata: map[string]string{"tool_name": "bash"},
		},
		{
			Role:         providertypes.RoleTool,
			ToolCallID:   "call-4",
			Content:      "result",
			ToolMetadata: map[string]string{"tool_name": "filesystem_read_file", "path": "README.md"},
		},
	}

	projected := ProjectToolMessagesForModel(cloneContextMessages(messages))
	if projected[0].Content != "user" {
		t.Fatalf("non-tool message should remain unchanged, got %+v", projected[0])
	}
	if projected[1].Content != "tool output" || projected[1].ToolMetadata != nil {
		t.Fatalf("tool without metadata-free projection should remain unchanged, got %+v", projected[1])
	}
	if projected[2].Content != "   " || projected[2].ToolMetadata == nil {
		t.Fatalf("empty tool content should not be projected, got %+v", projected[2])
	}
	if projected[3].Content != microCompactClearedMessage || projected[3].ToolMetadata == nil {
		t.Fatalf("cleared tool content should not be projected, got %+v", projected[3])
	}
	if !strings.Contains(projected[4].Content, "tool result") || projected[4].ToolMetadata != nil {
		t.Fatalf("valid tool message should be projected, got %+v", projected[4])
	}
}

func TestBuildRecentMessagesForModelBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		messages []providertypes.Message
		limit    int
	}{
		{
			name:     "empty messages",
			messages: nil,
			limit:    10,
		},
		{
			name:     "non-positive limit",
			messages: []providertypes.Message{{Role: providertypes.RoleUser, Content: "x"}},
			limit:    0,
		},
		{
			name: "no keepable anchor",
			messages: []providertypes.Message{
				{Role: providertypes.RoleTool, ToolCallID: "orphan", Content: "orphan"},
			},
			limit: 10,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := BuildRecentMessagesForModel(tt.messages, tt.limit); got != nil {
				t.Fatalf("expected nil, got %+v", got)
			}
		})
	}
}

func TestBuildRecentMessagesForModelKeepsOnlyRecentValidAnchors(t *testing.T) {
	t.Parallel()

	original := []providertypes.Message{
		{Role: providertypes.RoleUser, Content: "old-user"},
		{
			Role: providertypes.RoleAssistant,
			ToolCalls: []providertypes.ToolCall{
				{ID: "call-1", Name: "filesystem_read_file", Arguments: `{"path":"README.md"}`},
			},
		},
		{
			Role:         providertypes.RoleTool,
			ToolCallID:   "call-1",
			Content:      "README body",
			ToolMetadata: map[string]string{"tool_name": "filesystem_read_file", "path": "README.md"},
		},
		{Role: providertypes.RoleUser, Content: "latest-user"},
	}

	recent := BuildRecentMessagesForModel(original, 2)
	if len(recent) != 3 {
		t.Fatalf("len(recent) = %d, want 3", len(recent))
	}
	if recent[0].Role != providertypes.RoleAssistant || len(recent[0].ToolCalls) != 1 {
		t.Fatalf("expected valid tool span to remain, got %+v", recent[0])
	}
	if recent[1].Role != providertypes.RoleTool || !strings.Contains(recent[1].Content, "tool result") {
		t.Fatalf("expected tool message to be projected, got %+v", recent[1])
	}
	if strings.Contains(recent[1].Content, "content:\nREADME body") {
		t.Fatalf("expected tool payload to be minimized, got %+v", recent[1])
	}
	if !strings.Contains(recent[1].Content, "content_excerpt:") {
		t.Fatalf("expected tool payload excerpt marker, got %+v", recent[1])
	}
	if recent[2].Content != "latest-user" {
		t.Fatalf("expected latest user anchor to remain, got %+v", recent[2])
	}

	recent[1].Content = "changed"
	if original[2].Content != "README body" {
		t.Fatalf("expected original messages to remain unchanged, got %+v", original[2])
	}
}

func TestBuildRecentMessagesForModelRespectsAbsoluteMessageBudget(t *testing.T) {
	t.Parallel()

	longSpan := []providertypes.Message{
		{
			Role: providertypes.RoleAssistant,
			ToolCalls: []providertypes.ToolCall{
				{ID: "call-1", Name: "bash", Arguments: `{}`},
				{ID: "call-2", Name: "bash", Arguments: `{}`},
				{ID: "call-3", Name: "bash", Arguments: `{}`},
				{ID: "call-4", Name: "bash", Arguments: `{}`},
				{ID: "call-5", Name: "bash", Arguments: `{}`},
			},
		},
		{Role: providertypes.RoleTool, ToolCallID: "call-1", Content: "one", ToolMetadata: map[string]string{"tool_name": "bash"}},
		{Role: providertypes.RoleTool, ToolCallID: "call-2", Content: "two", ToolMetadata: map[string]string{"tool_name": "bash"}},
		{Role: providertypes.RoleTool, ToolCallID: "call-3", Content: "three", ToolMetadata: map[string]string{"tool_name": "bash"}},
		{Role: providertypes.RoleTool, ToolCallID: "call-4", Content: "four", ToolMetadata: map[string]string{"tool_name": "bash"}},
		{Role: providertypes.RoleTool, ToolCallID: "call-5", Content: "five", ToolMetadata: map[string]string{"tool_name": "bash"}},
	}

	messages := []providertypes.Message{
		{Role: providertypes.RoleUser, Content: "old-user"},
	}
	messages = append(messages, longSpan...)
	messages = append(messages, providertypes.Message{Role: providertypes.RoleUser, Content: "latest-user"})

	recent := BuildRecentMessagesForModel(messages, 3)
	if len(recent) != 2 {
		t.Fatalf("len(recent) = %d, want 2", len(recent))
	}
	if recent[0].Content != "old-user" || recent[1].Content != "latest-user" {
		t.Fatalf("expected oversized tool span to be skipped, got %+v", recent)
	}
}

func TestSanitizeProjectedToolContent(t *testing.T) {
	t.Parallel()

	rawBody := strings.Repeat("甲", recentWindowToolContentCharLimit+10)
	projected := "tool result\nstatus: ok\n\ncontent:\n" + rawBody
	sanitized := sanitizeProjectedToolContent(projected)
	if !strings.Contains(sanitized, "content_excerpt:") {
		t.Fatalf("expected excerpt marker, got %q", sanitized)
	}
	if strings.Contains(sanitized, "content:\n") {
		t.Fatalf("expected original content marker removed, got %q", sanitized)
	}
	if !strings.Contains(sanitized, "[content truncated for memo extraction]") {
		t.Fatalf("expected truncation marker, got %q", sanitized)
	}
}

func TestMatchedToolCallSpanRejectsInvalidAssistantStates(t *testing.T) {
	t.Parallel()

	messages := []providertypes.Message{
		{Role: providertypes.RoleUser, Content: "user"},
		{
			Role: providertypes.RoleAssistant,
			ToolCalls: []providertypes.ToolCall{
				{ID: " ", Name: "bash", Arguments: `{}`},
			},
		},
		{Role: providertypes.RoleTool, ToolCallID: "call-1", Content: "tool output"},
	}

	if span := matchedToolCallSpan(messages, -1); span != nil {
		t.Fatalf("expected nil span for invalid negative index, got %+v", span)
	}
	if span := matchedToolCallSpan(messages, len(messages)); span != nil {
		t.Fatalf("expected nil span for invalid upper index, got %+v", span)
	}
	if span := matchedToolCallSpan(messages, 0); span != nil {
		t.Fatalf("expected nil span for non-assistant message, got %+v", span)
	}
	if span := matchedToolCallSpan(messages, 1); span != nil {
		t.Fatalf("expected nil span for blank tool call id, got %+v", span)
	}
}

func TestMatchedToolCallSpanRequiresInjectableResponsesAndSkipsDuplicates(t *testing.T) {
	t.Parallel()

	messages := []providertypes.Message{
		{
			Role: providertypes.RoleAssistant,
			ToolCalls: []providertypes.ToolCall{
				{ID: "call-1", Name: "bash", Arguments: `{}`},
				{ID: "call-2", Name: "filesystem_read_file", Arguments: `{}`},
			},
		},
		{Role: providertypes.RoleTool, ToolCallID: "call-1", Content: "", ToolMetadata: map[string]string{"tool_name": "bash"}},
		{Role: providertypes.RoleTool, ToolCallID: "call-1", Content: "first result", ToolMetadata: map[string]string{"tool_name": "bash"}},
		{Role: providertypes.RoleTool, ToolCallID: "call-1", Content: "duplicate result", ToolMetadata: map[string]string{"tool_name": "bash"}},
		{Role: providertypes.RoleTool, ToolCallID: "ignored", Content: "ignored result", ToolMetadata: map[string]string{"tool_name": "bash"}},
		{Role: providertypes.RoleTool, ToolCallID: "call-2", Content: "second result", ToolMetadata: map[string]string{"tool_name": "filesystem_read_file"}},
		{Role: providertypes.RoleUser, Content: "after"},
	}

	span := matchedToolCallSpan(messages, 0)
	if len(span) != 3 {
		t.Fatalf("len(span) = %d, want 3 (%+v)", len(span), span)
	}
	if span[0] != 0 || span[1] != 2 || span[2] != 5 {
		t.Fatalf("unexpected span indexes %+v", span)
	}
}

func TestMatchedToolCallSpanRejectsResponsesWithoutProjectionMetadata(t *testing.T) {
	t.Parallel()

	messages := []providertypes.Message{
		{
			Role: providertypes.RoleAssistant,
			ToolCalls: []providertypes.ToolCall{
				{ID: "call-1", Name: "bash", Arguments: `{}`},
			},
		},
		{Role: providertypes.RoleTool, ToolCallID: "call-1", Content: "raw result"},
	}

	if span := matchedToolCallSpan(messages, 0); span != nil {
		t.Fatalf("expected nil span when tool metadata is missing, got %+v", span)
	}
}

func TestIsInjectableToolMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message providertypes.Message
		want    bool
	}{
		{
			name:    "non-tool",
			message: providertypes.Message{Role: providertypes.RoleUser, Content: "user"},
			want:    false,
		},
		{
			name:    "empty",
			message: providertypes.Message{Role: providertypes.RoleTool, Content: "   "},
			want:    false,
		},
		{
			name:    "cleared",
			message: providertypes.Message{Role: providertypes.RoleTool, Content: microCompactClearedMessage},
			want:    false,
		},
		{
			name:    "valid",
			message: providertypes.Message{Role: providertypes.RoleTool, Content: "ok"},
			want:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isInjectableToolMessage(tt.message); got != tt.want {
				t.Fatalf("isInjectableToolMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeProjectedToolContentFallsBackForRawPayload(t *testing.T) {
	t.Parallel()

	raw := strings.Repeat("x", recentWindowToolContentCharLimit+10)
	sanitized := sanitizeProjectedToolContent(raw)
	if !strings.Contains(sanitized, "content_excerpt:") {
		t.Fatalf("expected raw payload to be excerpted, got %q", sanitized)
	}
	if !strings.Contains(sanitized, "[content truncated for memo extraction]") {
		t.Fatalf("expected truncation marker, got %q", sanitized)
	}
}
