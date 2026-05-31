package provider

import (
	"strings"
	"testing"

	providertypes "neo-code/internal/provider/types"
)

func TestEstimateSerializedPayloadTokens(t *testing.T) {
	t.Parallel()

	tokens, err := EstimateSerializedPayloadTokens(map[string]any{
		"model": "x",
		"input": "hello",
	})
	if err != nil {
		t.Fatalf("EstimateSerializedPayloadTokens() error = %v", err)
	}
	if tokens <= 0 {
		t.Fatalf("EstimateSerializedPayloadTokens() = %d, want > 0", tokens)
	}
}

func TestEstimateSerializedPayloadTokensMarshalError(t *testing.T) {
	t.Parallel()

	if _, err := EstimateSerializedPayloadTokens(make(chan int)); err == nil {
		t.Fatal("EstimateSerializedPayloadTokens() expected marshal error, got nil")
	}
}

func TestEstimateTextTokens(t *testing.T) {
	t.Parallel()

	if got := EstimateTextTokens(""); got != 0 {
		t.Fatalf("EstimateTextTokens(\"\") = %d, want 0", got)
	}
	if got := EstimateTextTokens("1234"); got != 2 {
		t.Fatalf("EstimateTextTokens(\"1234\") = %d, want 2", got)
	}
}

func TestRequestContainsImagePart(t *testing.T) {
	t.Parallel()

	textOnly := providertypes.GenerateRequest{
		Messages: []providertypes.Message{{
			Role:  providertypes.RoleUser,
			Parts: []providertypes.ContentPart{providertypes.NewTextPart("hello")},
		}},
	}
	if RequestContainsImagePart(textOnly) {
		t.Fatal("expected text-only request to report no images")
	}
	withImage := textOnly
	withImage.Messages[0].Parts = append(withImage.Messages[0].Parts, providertypes.NewSessionAssetImagePart("asset-1", "image/png"))
	if !RequestContainsImagePart(withImage) {
		t.Fatal("expected image request to report images")
	}
}

func TestEstimateProjectedInputTokensDoesNotCountBase64Transport(t *testing.T) {
	t.Parallel()

	tokens, err := EstimateProjectedInputTokens(providertypes.GenerateRequest{
		SystemPrompt: "You are concise.",
		Messages: []providertypes.Message{{
			Role: providertypes.RoleUser,
			Parts: []providertypes.ContentPart{
				providertypes.NewTextPart("describe this"),
				providertypes.NewSessionAssetImagePart("asset-1", "image/png"),
			},
		}},
		Tools: []providertypes.ToolSpec{{
			Name:        "filesystem_read_file",
			Description: "Read a file",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		}},
	}, "gpt-4.1")
	if err != nil {
		t.Fatalf("EstimateProjectedInputTokens() error = %v", err)
	}
	if tokens <= DefaultImageInputTokenEstimate {
		t.Fatalf("expected text and tool schema to add tokens, got %d", tokens)
	}
	if tokens > 10_000 {
		t.Fatalf("projected estimate counted transport-sized payload, got %d", tokens)
	}

	oneMiBDataURLTokens := EstimateTextTokens(strings.Repeat("x", int(EstimateDataURLTransportBytes(1024*1024, "image/png"))))
	if tokens >= oneMiBDataURLTokens {
		t.Fatalf("projected estimate = %d, want below data URL transport estimate %d", tokens, oneMiBDataURLTokens)
	}
}

func TestEstimateProjectedInputTokensValidatesPartsAndModel(t *testing.T) {
	t.Parallel()

	if _, err := EstimateProjectedInputTokens(providertypes.GenerateRequest{}, " "); err == nil {
		t.Fatal("expected empty model error")
	}
	_, err := EstimateProjectedInputTokens(providertypes.GenerateRequest{
		Messages: []providertypes.Message{{
			Role:  providertypes.RoleUser,
			Parts: []providertypes.ContentPart{{Kind: "invalid"}},
		}},
	}, "gpt")
	if err == nil {
		t.Fatal("expected invalid parts error")
	}
}

func TestBuildGenerateRequestSignature(t *testing.T) {
	t.Parallel()

	reqA := providertypes.GenerateRequest{
		Model: "gpt",
		Messages: []providertypes.Message{
			{
				Role:  providertypes.RoleUser,
				Parts: []providertypes.ContentPart{providertypes.NewTextPart("hello")},
			},
		},
	}
	reqB := reqA
	reqC := reqA
	reqC.Model = "gpt-2"

	sigA := BuildGenerateRequestSignature(reqA)
	sigB := BuildGenerateRequestSignature(reqB)
	sigC := BuildGenerateRequestSignature(reqC)
	if sigA == "" {
		t.Fatal("BuildGenerateRequestSignature(reqA) returned empty signature")
	}
	if sigA != sigB {
		t.Fatalf("same request should have same signature: %q != %q", sigA, sigB)
	}
	if sigA == sigC {
		t.Fatalf("different requests should have different signatures: %q == %q", sigA, sigC)
	}
}
