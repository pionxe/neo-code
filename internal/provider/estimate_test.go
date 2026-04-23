package provider

import (
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
