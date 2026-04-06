package tools

import "strings"

const (
	// DefaultOutputLimitBytes is the default max size of tool output content.
	DefaultOutputLimitBytes = 64 * 1024
	truncatedSuffix         = "\n...[truncated]"
)

// ApplyOutputLimit truncates tool output content and adds a truncated metadata flag.
func ApplyOutputLimit(result ToolResult, limit int) ToolResult {
	if limit <= 0 {
		return result
	}
	if len(result.Content) <= limit {
		return result
	}

	result.Content = result.Content[:limit] + truncatedSuffix
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	if existing, ok := result.Metadata["truncated"].(bool); !ok || !existing {
		result.Metadata["truncated"] = true
	}
	return result
}

// FormatError builds a consistent error payload for tool failures.
func FormatError(toolName string, reason string, details string) string {
	toolName = strings.TrimSpace(toolName)
	reason = strings.TrimSpace(reason)
	details = strings.TrimSpace(details)

	lines := []string{"tool error"}
	if toolName != "" {
		lines = append(lines, "tool: "+toolName)
	}
	if reason != "" {
		lines = append(lines, "reason: "+reason)
	}
	if details != "" {
		lines = append(lines, "details: "+details)
	}

	return strings.Join(lines, "\n")
}

// NormalizeErrorReason strips the tool name prefix from an error message, if present.
func NormalizeErrorReason(toolName string, err error) string {
	if err == nil {
		return ""
	}
	reason := strings.TrimSpace(err.Error())
	if toolName == "" {
		return reason
	}

	prefix := strings.ToLower(strings.TrimSpace(toolName)) + ":"
	lower := strings.ToLower(reason)
	if strings.HasPrefix(lower, prefix) {
		return strings.TrimSpace(reason[len(prefix):])
	}
	return reason
}

// NewErrorResult returns a standardized error ToolResult.
func NewErrorResult(toolName string, reason string, details string, metadata map[string]any) ToolResult {
	return ToolResult{
		Name:     toolName,
		Content:  FormatError(toolName, reason, details),
		IsError:  true,
		Metadata: metadata,
	}
}
