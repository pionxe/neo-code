package tools

import (
	"math"
	"strconv"
	"strings"
)

// ToolResultMetadataMarksFailure 判断工具 metadata 是否显式标记了失败。
// 兼容 ok=false / ok="false" / ok=0 以及 exit_code!=0 等常见序列化形态。
func ToolResultMetadataMarksFailure(metadata map[string]any) bool {
	if len(metadata) == 0 {
		return false
	}
	if raw, exists := metadata["ok"]; exists {
		if ok, resolved := parseToolResultOK(raw); resolved {
			return !ok
		}
	}
	if rawExitCode, exists := metadata["exit_code"]; exists {
		if exitCode, resolved := parseToolResultExitCode(rawExitCode); resolved {
			return exitCode != 0
		}
	}
	return false
}

// parseToolResultOK 解析工具元数据里的 ok 字段，兼容 bool/数字/字符串。
func parseToolResultOK(raw any) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case string:
		trimmed := strings.ToLower(strings.TrimSpace(value))
		switch trimmed {
		case "true", "1", "yes", "y":
			return true, true
		case "false", "0", "no", "n":
			return false, true
		default:
			return false, false
		}
	case int:
		return value != 0, true
	case int8:
		return value != 0, true
	case int16:
		return value != 0, true
	case int32:
		return value != 0, true
	case int64:
		return value != 0, true
	case uint:
		return value != 0, true
	case uint8:
		return value != 0, true
	case uint16:
		return value != 0, true
	case uint32:
		return value != 0, true
	case uint64:
		return value != 0, true
	case float32:
		return value != 0, true
	case float64:
		return value != 0, true
	default:
		return false, false
	}
}

// parseToolResultExitCode 解析工具元数据里的 exit_code 字段，兼容数字和字符串。
func parseToolResultExitCode(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case uint:
		return int(value), true
	case uint8:
		return int(value), true
	case uint16:
		return int(value), true
	case uint32:
		return int(value), true
	case uint64:
		return int(value), true
	case float32:
		return parseFloatExitCode(float64(value))
	case float64:
		return parseFloatExitCode(value)
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

// parseFloatExitCode 将浮点退出码折叠为稳定整数，避免 0<|x|<1 被截断为 0。
func parseFloatExitCode(value float64) (int, bool) {
	if value == 0 {
		return 0, true
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	parsed := int(value)
	if parsed == 0 {
		if value > 0 {
			return 1, true
		}
		return -1, true
	}
	return parsed, true
}
