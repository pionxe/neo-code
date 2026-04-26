package gateway

import (
	"strings"
	"testing"
)

func TestDecodePermissionResolutionInputAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("nil permission pointer", func(t *testing.T) {
		var input *PermissionResolutionInput
		_, err := decodePermissionResolutionInput(input)
		if err == nil || !strings.Contains(err.Error(), "is nil") {
			t.Fatalf("expected nil pointer error, got %v", err)
		}
	})

	t.Run("marshal error", func(t *testing.T) {
		payload := map[string]any{"bad": func() {}}
		_, err := decodePermissionResolutionInput(payload)
		if err == nil {
			t.Fatal("expected marshal error")
		}
	})

	t.Run("unmarshal error", func(t *testing.T) {
		_, err := decodePermissionResolutionInput([]byte("not-json-object"))
		if err == nil {
			t.Fatal("expected unmarshal error")
		}
	})
}

func TestValidateRequestFrameRunsInputPartsValidationForCompact(t *testing.T) {
	t.Parallel()

	err := ValidateFrame(MessageFrame{
		Type:      FrameTypeRequest,
		Action:    FrameActionCompact,
		SessionID: "sess-1",
		InputParts: []InputPart{{
			Type: InputPartTypeText,
			Text: "   ",
		}},
	})
	if err == nil {
		t.Fatal("expected input_parts validation error")
	}
	if err.Code != ErrorCodeInvalidMultimodalPayload.String() {
		t.Fatalf("error code = %q, want %q", err.Code, ErrorCodeInvalidMultimodalPayload.String())
	}
}

func TestValidateFrameCancelAndListSessions(t *testing.T) {
	t.Parallel()

	cancelErr := ValidateFrame(MessageFrame{
		Type:   FrameTypeRequest,
		Action: FrameActionCancel,
	})
	if cancelErr != nil {
		t.Fatalf("cancel request should be valid, got %v", cancelErr)
	}

	listErr := ValidateFrame(MessageFrame{
		Type:   FrameTypeRequest,
		Action: FrameActionListSessions,
	})
	if listErr != nil {
		t.Fatalf("list_sessions request should be valid, got %v", listErr)
	}

	listAvailableErr := ValidateFrame(MessageFrame{
		Type:   FrameTypeRequest,
		Action: FrameActionListAvailableSkills,
	})
	if listAvailableErr != nil {
		t.Fatalf("list_available_skills request should be valid, got %v", listAvailableErr)
	}

	listSessionErr := ValidateFrame(MessageFrame{
		Type:      FrameTypeRequest,
		Action:    FrameActionListSessionSkills,
		SessionID: "sess-1",
	})
	if listSessionErr != nil {
		t.Fatalf("list_session_skills request should be valid with session_id, got %v", listSessionErr)
	}

	executeErr := ValidateFrame(MessageFrame{
		Type:   FrameTypeRequest,
		Action: FrameActionExecuteSystemTool,
		Payload: map[string]any{
			"tool_name": "memo_list",
		},
	})
	if executeErr != nil {
		t.Fatalf("execute_system_tool request should be valid, got %v", executeErr)
	}

	bindErr := ValidateFrame(MessageFrame{
		Type:   FrameTypeRequest,
		Action: FrameActionBindStream,
	})
	if bindErr == nil {
		t.Fatal("bind_stream missing payload should be invalid")
	}
	if bindErr.Code != ErrorCodeMissingRequiredField.String() {
		t.Fatalf("error code = %q, want %q", bindErr.Code, ErrorCodeMissingRequiredField.String())
	}

	bindValidErr := ValidateFrame(MessageFrame{
		Type:   FrameTypeRequest,
		Action: FrameActionBindStream,
		Payload: map[string]string{
			"session_id": "sess-1",
		},
	})
	if bindValidErr != nil {
		t.Fatalf("bind_stream request should be valid, got %v", bindValidErr)
	}
}

func TestValidateResolvePermissionInvalidPayloadType(t *testing.T) {
	t.Parallel()

	err := ValidateFrame(MessageFrame{
		Type:    FrameTypeRequest,
		Action:  FrameActionResolvePermission,
		Payload: make(chan int),
	})
	if err == nil {
		t.Fatal("expected invalid resolve_permission payload error")
	}
	if err.Code != ErrorCodeInvalidAction.String() {
		t.Fatalf("error code = %q, want %q", err.Code, ErrorCodeInvalidAction.String())
	}
}
