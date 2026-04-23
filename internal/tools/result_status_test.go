package tools

import (
	"math"
	"testing"
)

func TestToolResultMetadataMarksFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
		want     bool
	}{
		{name: "empty", metadata: nil, want: false},
		{name: "ok true bool", metadata: map[string]any{"ok": true}, want: false},
		{name: "ok false bool", metadata: map[string]any{"ok": false}, want: true},
		{name: "ok false string", metadata: map[string]any{"ok": "false"}, want: true},
		{name: "ok one number", metadata: map[string]any{"ok": 1}, want: false},
		{name: "ok zero number", metadata: map[string]any{"ok": 0}, want: true},
		{name: "ok invalid string with exit code", metadata: map[string]any{"ok": "unknown", "exit_code": 2}, want: true},
		{name: "exit code zero", metadata: map[string]any{"exit_code": 0}, want: false},
		{name: "exit code string non-zero", metadata: map[string]any{"exit_code": "3"}, want: true},
		{name: "exit code tiny positive float", metadata: map[string]any{"exit_code": 0.1}, want: true},
		{name: "exit code tiny negative float", metadata: map[string]any{"exit_code": -0.1}, want: true},
		{name: "exit code invalid string", metadata: map[string]any{"exit_code": "x"}, want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ToolResultMetadataMarksFailure(tt.metadata); got != tt.want {
				t.Fatalf("ToolResultMetadataMarksFailure(%v) = %v, want %v", tt.metadata, got, tt.want)
			}
		})
	}
}

func TestParseToolResultOKBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		raw          any
		wantOK       bool
		wantResolved bool
	}{
		{name: "bool true", raw: true, wantOK: true, wantResolved: true},
		{name: "string yes", raw: " YES ", wantOK: true, wantResolved: true},
		{name: "string no", raw: "n", wantOK: false, wantResolved: true},
		{name: "string unknown", raw: "maybe", wantResolved: false},
		{name: "int8", raw: int8(-1), wantOK: true, wantResolved: true},
		{name: "int", raw: int(1), wantOK: true, wantResolved: true},
		{name: "int16", raw: int16(2), wantOK: true, wantResolved: true},
		{name: "int32", raw: int32(3), wantOK: true, wantResolved: true},
		{name: "int64 zero", raw: int64(0), wantOK: false, wantResolved: true},
		{name: "uint", raw: uint(1), wantOK: true, wantResolved: true},
		{name: "uint8", raw: uint8(1), wantOK: true, wantResolved: true},
		{name: "uint16", raw: uint16(2), wantOK: true, wantResolved: true},
		{name: "uint32", raw: uint32(3), wantOK: true, wantResolved: true},
		{name: "uint64", raw: uint64(4), wantOK: true, wantResolved: true},
		{name: "float32 zero", raw: float32(0), wantOK: false, wantResolved: true},
		{name: "float64 non-zero", raw: 0.5, wantOK: true, wantResolved: true},
		{name: "unsupported", raw: map[string]any{"ok": true}, wantResolved: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotOK, gotResolved := parseToolResultOK(tt.raw)
			if gotResolved != tt.wantResolved {
				t.Fatalf("parseToolResultOK(%T) resolved = %v, want %v", tt.raw, gotResolved, tt.wantResolved)
			}
			if gotResolved && gotOK != tt.wantOK {
				t.Fatalf("parseToolResultOK(%T) ok = %v, want %v", tt.raw, gotOK, tt.wantOK)
			}
		})
	}
}

func TestParseToolResultExitCodeBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		raw          any
		wantCode     int
		wantResolved bool
	}{
		{name: "int8", raw: int8(-2), wantCode: -2, wantResolved: true},
		{name: "int", raw: int(6), wantCode: 6, wantResolved: true},
		{name: "int16", raw: int16(7), wantCode: 7, wantResolved: true},
		{name: "int32", raw: int32(8), wantCode: 8, wantResolved: true},
		{name: "int64", raw: int64(9), wantCode: 9, wantResolved: true},
		{name: "uint", raw: uint(10), wantCode: 10, wantResolved: true},
		{name: "uint8", raw: uint8(11), wantCode: 11, wantResolved: true},
		{name: "uint16", raw: uint16(12), wantCode: 12, wantResolved: true},
		{name: "uint32", raw: uint32(9), wantCode: 9, wantResolved: true},
		{name: "uint64", raw: uint64(13), wantCode: 13, wantResolved: true},
		{name: "float32 tiny positive", raw: float32(0.2), wantCode: 1, wantResolved: true},
		{name: "float64 tiny negative", raw: -0.2, wantCode: -1, wantResolved: true},
		{name: "string trimmed", raw: " 5 ", wantCode: 5, wantResolved: true},
		{name: "string empty", raw: " ", wantResolved: false},
		{name: "string invalid", raw: "x", wantResolved: false},
		{name: "nan", raw: math.NaN(), wantResolved: false},
		{name: "inf", raw: math.Inf(1), wantResolved: false},
		{name: "unsupported", raw: true, wantResolved: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotCode, gotResolved := parseToolResultExitCode(tt.raw)
			if gotResolved != tt.wantResolved {
				t.Fatalf("parseToolResultExitCode(%T) resolved = %v, want %v", tt.raw, gotResolved, tt.wantResolved)
			}
			if gotResolved && gotCode != tt.wantCode {
				t.Fatalf("parseToolResultExitCode(%T) code = %d, want %d", tt.raw, gotCode, tt.wantCode)
			}
		})
	}
}

func TestParseFloatExitCodeBranches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        float64
		wantCode     int
		wantResolved bool
	}{
		{name: "zero", value: 0, wantCode: 0, wantResolved: true},
		{name: "tiny positive", value: 0.1, wantCode: 1, wantResolved: true},
		{name: "tiny negative", value: -0.1, wantCode: -1, wantResolved: true},
		{name: "positive integer", value: 3.0, wantCode: 3, wantResolved: true},
		{name: "negative integer", value: -4.0, wantCode: -4, wantResolved: true},
		{name: "nan", value: math.NaN(), wantResolved: false},
		{name: "inf", value: math.Inf(-1), wantResolved: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotCode, gotResolved := parseFloatExitCode(tt.value)
			if gotResolved != tt.wantResolved {
				t.Fatalf("parseFloatExitCode(%v) resolved = %v, want %v", tt.value, gotResolved, tt.wantResolved)
			}
			if gotResolved && gotCode != tt.wantCode {
				t.Fatalf("parseFloatExitCode(%v) code = %d, want %d", tt.value, gotCode, tt.wantCode)
			}
		})
	}
}
