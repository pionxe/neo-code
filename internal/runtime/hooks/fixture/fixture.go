package fixture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	runtimehooks "neo-code/internal/runtime/hooks"
)

// Parsed 描述 dry-run fixture 归一化后的结果。
type Parsed struct {
	Point   runtimehooks.HookPoint
	Context runtimehooks.HookContext
	Payload map[string]any
}

type rawFixture struct {
	PayloadVersion string         `json:"payload_version" yaml:"payload_version"`
	Point          string         `json:"point" yaml:"point"`
	RunID          string         `json:"run_id" yaml:"run_id"`
	SessionID      string         `json:"session_id" yaml:"session_id"`
	Metadata       map[string]any `json:"metadata" yaml:"metadata"`
}

// ParseFile 读取并校验 YAML/JSON fixture 文件。
func ParseFile(path string) (Parsed, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Parsed{}, fmt.Errorf("read fixture: %w", err)
	}
	return ParseBytes(raw, path)
}

// ParseBytes 解析并校验 fixture 内容，扩展名仅用于错误提示和格式推断。
func ParseBytes(raw []byte, sourcePath string) (Parsed, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Parsed{}, fmt.Errorf("fixture is empty")
	}
	var payload map[string]any
	var decoded rawFixture
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(sourcePath)))
	switch ext {
	case ".json":
		if err := json.Unmarshal(raw, &payload); err != nil {
			return Parsed{}, fmt.Errorf("parse fixture json: %w", err)
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return Parsed{}, fmt.Errorf("parse fixture json: %w", err)
		}
	default:
		decoder := yaml.NewDecoder(bytes.NewReader(raw))
		decoder.KnownFields(false)
		if err := decoder.Decode(&payload); err != nil {
			return Parsed{}, fmt.Errorf("parse fixture yaml: %w", err)
		}
		decoder = yaml.NewDecoder(bytes.NewReader(raw))
		decoder.KnownFields(false)
		if err := decoder.Decode(&decoded); err != nil {
			return Parsed{}, fmt.Errorf("parse fixture yaml: %w", err)
		}
	}
	point := runtimehooks.HookPoint(strings.TrimSpace(decoded.Point))
	schema := runtimehooks.PayloadSchema(point)
	if schema.Point == "" {
		return Parsed{}, fmt.Errorf("fixture point %q is not supported", decoded.Point)
	}
	if strings.TrimSpace(decoded.PayloadVersion) != runtimehooks.PayloadVersion {
		return Parsed{}, fmt.Errorf(
			"fixture payload_version %q does not match %q",
			decoded.PayloadVersion,
			runtimehooks.PayloadVersion,
		)
	}
	allowedTopLevel := make(map[string]struct{}, len(schema.TopLevel)+1)
	for _, field := range schema.TopLevel {
		allowedTopLevel[strings.ToLower(strings.TrimSpace(field.Name))] = struct{}{}
	}
	allowedTopLevel["metadata"] = struct{}{}
	for key := range payload {
		if _, ok := allowedTopLevel[strings.ToLower(strings.TrimSpace(key))]; !ok {
			return Parsed{}, fmt.Errorf("fixture contains unknown top-level field %q", key)
		}
	}
	if decoded.Metadata == nil {
		decoded.Metadata = map[string]any{}
	}
	allowedMetadata := make(map[string]struct{}, len(schema.Metadata))
	for _, field := range schema.Metadata {
		allowedMetadata[strings.ToLower(strings.TrimSpace(field.Name))] = struct{}{}
	}
	for key := range decoded.Metadata {
		if _, ok := allowedMetadata[strings.ToLower(strings.TrimSpace(key))]; !ok {
			return Parsed{}, fmt.Errorf("fixture metadata contains unknown field %q for point %q", key, point)
		}
	}
	return Parsed{
		Point: point,
		Context: runtimehooks.HookContext{
			RunID:     strings.TrimSpace(decoded.RunID),
			SessionID: strings.TrimSpace(decoded.SessionID),
			Metadata:  decoded.Metadata,
		},
		Payload: payload,
	}, nil
}
