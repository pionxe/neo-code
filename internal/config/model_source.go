package config

import "strings"

const (
	ModelSourceDiscover = "discover"
	ModelSourceManual   = "manual"
)

// NormalizeModelSource 规范化模型来源枚举，未知值返回空字符串供上层做校验与回退。
func NormalizeModelSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ModelSourceDiscover:
		return ModelSourceDiscover
	case ModelSourceManual:
		return ModelSourceManual
	default:
		return ""
	}
}
