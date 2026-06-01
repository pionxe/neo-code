package hooks

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// MaxHookMatcherRegexLength 限制 tool_name_regex 单条表达式长度，避免超长输入拖慢匹配。
	MaxHookMatcherRegexLength = 256
)

const (
	hookMatcherFieldToolName          = "tool_name"
	hookMatcherFieldToolNameRegex     = "tool_name_regex"
	hookMatcherFieldArgumentsContains = "arguments_contains"
	hookMatcherMetadataToolName       = "tool_name"
	hookMatcherMetadataArguments      = "tool_arguments_preview"
)

// HookMatcher 描述编译后的 hook 匹配器。
type HookMatcher struct {
	ToolNames         []string
	ToolNameRegex     []*regexp.Regexp
	ArgumentsContains []string
}

// HasHookMatcherConfig 判断 matcher 配置是否包含至少一个非空维度。
func HasHookMatcherConfig(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	names := readHookMatcherStringValues(raw, hookMatcherFieldToolName)
	if len(names) > 0 {
		return true
	}
	regexes := readHookMatcherStringValues(raw, hookMatcherFieldToolNameRegex)
	if len(regexes) > 0 {
		return true
	}
	contains := readHookMatcherStringValues(raw, hookMatcherFieldArgumentsContains)
	return len(contains) > 0
}

// ValidateHookMatcher 校验 matcher 配置在指定点位上是否合法。
func ValidateHookMatcher(point HookPoint, raw map[string]any) error {
	_, err := CompileHookMatcher(point, raw)
	return err
}

// CompileHookMatcher 将 matcher 原始配置编译为可执行结构，并在点位能力上做 fail-fast 校验。
func CompileHookMatcher(point HookPoint, raw map[string]any) (*HookMatcher, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if err := validateHookMatcherFields(raw); err != nil {
		return nil, err
	}
	if !HasHookMatcherConfig(raw) {
		return nil, fmt.Errorf("match contains no recognized matcher fields (expected: tool_name, tool_name_regex, arguments_contains)")
	}
	capability, ok := HookPointCapabilities(point)
	if !ok {
		return nil, fmt.Errorf("point %q is not supported", point)
	}

	namesRaw := readHookMatcherStringValues(raw, hookMatcherFieldToolName)
	regexRaw := readHookMatcherStringValues(raw, hookMatcherFieldToolNameRegex)
	containsRaw := readHookMatcherStringValues(raw, hookMatcherFieldArgumentsContains)

	if len(namesRaw) > 0 && !capability.Matcher.ToolName {
		return nil, fmt.Errorf("point %q does not support matcher field %q", point, hookMatcherFieldToolName)
	}
	if len(regexRaw) > 0 && !capability.Matcher.ToolNameRegex {
		return nil, fmt.Errorf("point %q does not support matcher field %q", point, hookMatcherFieldToolNameRegex)
	}
	if len(containsRaw) > 0 && !capability.Matcher.ArgumentsContains {
		return nil, fmt.Errorf("point %q does not support matcher field %q", point, hookMatcherFieldArgumentsContains)
	}

	matcher := &HookMatcher{
		ToolNames:         normalizeHookMatcherValues(namesRaw),
		ArgumentsContains: normalizeHookMatcherValues(containsRaw),
	}
	for _, expression := range regexRaw {
		trimmed := strings.TrimSpace(expression)
		if trimmed == "" {
			continue
		}
		if len(trimmed) > MaxHookMatcherRegexLength {
			return nil, fmt.Errorf(
				"matcher field %q expression length exceeds %d",
				hookMatcherFieldToolNameRegex,
				MaxHookMatcherRegexLength,
			)
		}
		compiled, err := regexp.Compile(trimmed)
		if err != nil {
			return nil, fmt.Errorf("matcher field %q has invalid regex %q: %w", hookMatcherFieldToolNameRegex, trimmed, err)
		}
		matcher.ToolNameRegex = append(matcher.ToolNameRegex, compiled)
	}
	if matcher.IsEmpty() {
		return nil, fmt.Errorf("match must include at least one non-empty matcher field")
	}
	return matcher, nil
}

// validateHookMatcherFields 校验 matcher 配置中不存在未支持字段，避免拼写错误被静默忽略。
func validateHookMatcherFields(raw map[string]any) error {
	if len(raw) == 0 {
		return nil
	}
	for key := range raw {
		normalized := strings.ToLower(strings.TrimSpace(key))
		switch normalized {
		case hookMatcherFieldToolName, hookMatcherFieldToolNameRegex, hookMatcherFieldArgumentsContains:
			continue
		default:
			return fmt.Errorf(
				"match contains unknown field %q (allowed: tool_name, tool_name_regex, arguments_contains)",
				key,
			)
		}
	}
	return nil
}

// IsEmpty 判断 matcher 是否包含可执行维度。
func (m *HookMatcher) IsEmpty() bool {
	if m == nil {
		return true
	}
	return len(m.ToolNames) == 0 && len(m.ToolNameRegex) == 0 && len(m.ArgumentsContains) == 0
}

// Match 根据 HookContext 执行 matcher 判定；字段间为 AND，同字段多值为 OR。
func (m *HookMatcher) Match(input HookContext) bool {
	if m == nil || m.IsEmpty() {
		return true
	}
	toolName := strings.TrimSpace(readHookMatcherMetadataString(input.Metadata, hookMatcherMetadataToolName))
	if len(m.ToolNames) > 0 {
		if toolName == "" || !containsEqualFold(m.ToolNames, toolName) {
			return false
		}
	}
	if len(m.ToolNameRegex) > 0 {
		if toolName == "" {
			return false
		}
		matched := false
		for _, compiled := range m.ToolNameRegex {
			if compiled.MatchString(toolName) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(m.ArgumentsContains) > 0 {
		argumentsPreview := strings.ToLower(strings.TrimSpace(readHookMatcherMetadataString(
			input.Metadata,
			hookMatcherMetadataArguments,
		)))
		if argumentsPreview == "" {
			return false
		}
		matched := false
		for _, fragment := range m.ArgumentsContains {
			if strings.Contains(argumentsPreview, fragment) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// readHookMatcherStringValues 读取 matcher 字段中的字符串集合，兼容 string / []string / []any。
func readHookMatcherStringValues(raw map[string]any, key string) []string {
	if len(raw) == 0 {
		return nil
	}
	value, ok := raw[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if strings.TrimSpace(item) == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if item == nil {
				continue
			}
			text := strings.TrimSpace(fmt.Sprintf("%v", item))
			if text == "" {
				continue
			}
			out = append(out, text)
		}
		return out
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", typed))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

// normalizeHookMatcherValues 将 matcher 词条规范为小写并剔除空值。
func normalizeHookMatcherValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.ToLower(strings.TrimSpace(value))
		if text == "" {
			continue
		}
		normalized = append(normalized, text)
	}
	return normalized
}

// containsEqualFold 判断字符串列表是否包含目标值（忽略大小写）。
func containsEqualFold(values []string, target string) bool {
	normalizedTarget := strings.ToLower(strings.TrimSpace(target))
	if normalizedTarget == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), normalizedTarget) {
			return true
		}
	}
	return false
}

// readHookMatcherMetadataString 从 metadata 中读取字符串，兼容大小写键和非字符串值。
func readHookMatcherMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if normalizedKey == "" {
		return ""
	}
	if value, ok := metadata[normalizedKey]; ok && value != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	for currentKey, value := range metadata {
		if !strings.EqualFold(strings.TrimSpace(currentKey), normalizedKey) || value == nil {
			continue
		}
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	return ""
}
