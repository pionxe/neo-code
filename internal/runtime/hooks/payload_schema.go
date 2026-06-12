package hooks

import (
	"encoding/json"
	"slices"
	"strings"
)

// PayloadVersion 定义对外公开的 hook payload 协议版本号。
const PayloadVersion = "1"

// PayloadStability 描述字段在对外契约中的稳定性等级。
type PayloadStability string

const (
	// PayloadStabilityStable 表示字段已进入稳定契约。
	PayloadStabilityStable PayloadStability = "stable"
	// PayloadStabilityExperimental 表示字段仍可能继续演进。
	PayloadStabilityExperimental PayloadStability = "experimental"
	// PayloadStabilityDeprecated 表示字段已进入弃用阶段。
	PayloadStabilityDeprecated PayloadStability = "deprecated"
)

// PayloadFieldSchema 描述单个 payload 字段的公开契约。
type PayloadFieldSchema struct {
	Name           string           `json:"name"`
	JSONType       string           `json:"json_type"`
	Stability      PayloadStability `json:"stability"`
	Description    string           `json:"description,omitempty"`
	Properties     []PayloadFieldSchema
	ItemType       string
	ItemProperties []PayloadFieldSchema
}

// PointPayloadSchema 描述单个 hook 点位的公开 payload 契约。
type PointPayloadSchema struct {
	Point          HookPoint            `json:"point"`
	PayloadVersion string               `json:"payload_version"`
	TopLevel       []PayloadFieldSchema `json:"top_level"`
	Metadata       []PayloadFieldSchema `json:"metadata"`
}

var payloadTopLevelFields = []PayloadFieldSchema{
	payloadStringField("payload_version", PayloadStabilityStable, "公开 payload 协议版本号。"),
	payloadStringField("hook_id", PayloadStabilityStable, "触发当前 payload 的 hook 标识。"),
	payloadStringField("point", PayloadStabilityStable, "当前 payload 对应的 hook 点位。"),
	payloadStringField("run_id", PayloadStabilityStable, "本次运行的 run 标识。"),
	payloadStringField("session_id", PayloadStabilityStable, "当前会话标识。"),
	payloadStringField("scope", PayloadStabilityStable, "hook 的 scope；HTTP observe 作为传输附加字段透传。"),
	payloadStringField("kind", PayloadStabilityStable, "hook 的 kind；HTTP observe 作为传输附加字段透传。"),
	payloadStringField("mode", PayloadStabilityStable, "hook 的 mode；HTTP observe 作为传输附加字段透传。"),
	payloadStringField("triggered_at", PayloadStabilityStable, "HTTP observe payload 发出时的 UTC 时间戳。"),
}

var pointPayloadMetadata = map[HookPoint][]PayloadFieldSchema{
	HookPointAcceptGate: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("workdir", PayloadStabilityStable, "本次运行的工作目录。"),
		payloadBoolField("workspace_changed", PayloadStabilityStable, "当前 run 是否已产生 workspace 写入。"),
		payloadBoolField("assistant_text_empty", PayloadStabilityStable, "assistant 文本输出是否为空。"),
		payloadObjectField(
			"todo_summary",
			PayloadStabilityExperimental,
			"当前 todo 摘要，字段仍可能继续演进。",
			payloadIntegerField("total", PayloadStabilityStable, "todo 总数。"),
			payloadIntegerField("required_total", PayloadStabilityStable, "required todo 总数。"),
			payloadIntegerField("required_completed", PayloadStabilityStable, "已完成的 required todo 数量。"),
			payloadIntegerField("required_failed", PayloadStabilityStable, "已失败的 required todo 数量。"),
			payloadIntegerField("required_open", PayloadStabilityStable, "尚未终态的 required todo 数量。"),
		),
		payloadArrayOfObjectsField(
			"recent_tool_summary",
			PayloadStabilityExperimental,
			"最近一批工具调用摘要，结构仍可能继续演进。",
			payloadStringField("name", PayloadStabilityStable, "工具名称。"),
			payloadBoolField("is_error", PayloadStabilityStable, "该工具结果是否为错误。"),
		),
	},
	HookPointAfterToolFailure: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("tool_call_id", PayloadStabilityStable, "当前工具调用标识。"),
		payloadStringField("tool_name", PayloadStabilityStable, "当前工具名称。"),
		payloadStringField("tool_arguments_preview", PayloadStabilityExperimental, "脱敏并截断后的工具参数预览。"),
		payloadBoolField("is_error", PayloadStabilityStable, "工具结果是否为错误。"),
		payloadStringField("error_class", PayloadStabilityStable, "工具错误分类。"),
		payloadStringField("execution_error", PayloadStabilityStable, "工具执行层错误文本。"),
		payloadStringField("result_content_preview", PayloadStabilityExperimental, "工具结果内容预览。"),
		payloadStringField("workdir", PayloadStabilityStable, "本次运行的工作目录。"),
	},
	HookPointAfterToolResult: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("tool_call_id", PayloadStabilityStable, "当前工具调用标识。"),
		payloadStringField("tool_name", PayloadStabilityStable, "当前工具名称。"),
		payloadBoolField("is_error", PayloadStabilityStable, "工具结果是否为错误。"),
		payloadStringField("error_class", PayloadStabilityStable, "工具错误分类。"),
		payloadStringField("result_content_preview", PayloadStabilityExperimental, "工具结果内容预览。"),
		payloadBoolField("result_metadata_present", PayloadStabilityStable, "工具结果 metadata 是否非空。"),
		payloadStringField("execution_error", PayloadStabilityStable, "工具执行层错误文本。"),
		payloadStringField("workdir", PayloadStabilityStable, "本次运行的工作目录。"),
	},
	HookPointBeforeCompletionDecision: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
	},
	HookPointBeforePermissionDecision: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("tool_call_id", PayloadStabilityStable, "当前工具调用标识。"),
		payloadStringField("tool_name", PayloadStabilityStable, "当前工具名称。"),
		payloadStringField("decision", PayloadStabilityStable, "权限决策结果。"),
		payloadStringField("reason", PayloadStabilityStable, "权限决策原因。"),
		payloadStringField("rule_id", PayloadStabilityStable, "命中的权限规则标识。"),
		payloadStringField("workdir", PayloadStabilityStable, "本次运行的工作目录。"),
	},
	HookPointBeforeToolCall: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("tool_call_id", PayloadStabilityStable, "当前工具调用标识。"),
		payloadStringField("tool_name", PayloadStabilityStable, "当前工具名称。"),
		payloadStringField("tool_arguments_preview", PayloadStabilityExperimental, "脱敏并截断后的工具参数预览。"),
		payloadStringField("workdir", PayloadStabilityStable, "本次运行的工作目录。"),
	},
	HookPointPostCompact: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("workdir", PayloadStabilityStable, "compact 使用的工作目录。"),
		payloadStringField("trigger_mode", PayloadStabilityStable, "触发 compact 的模式。"),
		payloadBoolField("applied", PayloadStabilityStable, "compact 结果是否已应用。"),
	},
	HookPointPreCompact: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("workdir", PayloadStabilityStable, "compact 使用的工作目录。"),
		payloadStringField("trigger_mode", PayloadStabilityStable, "触发 compact 的模式。"),
	},
	HookPointSessionEnd: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("stop_reason", PayloadStabilityStable, "当前运行的停止原因。"),
		payloadStringField("detail", PayloadStabilityStable, "停止原因的补充说明。"),
	},
	HookPointSessionStart: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("workdir", PayloadStabilityStable, "本次运行的工作目录。"),
	},
	HookPointSubAgentStart: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("task_id", PayloadStabilityStable, "子代理任务标识。"),
		payloadStringField("role", PayloadStabilityStable, "子代理角色。"),
		payloadStringField("workspace", PayloadStabilityStable, "子代理工作目录。"),
		payloadStringField("tool_name", PayloadStabilityStable, "触发子代理的工具名。"),
		payloadStringField("trigger", PayloadStabilityStable, "触发子代理的来源。"),
		payloadStringField("workdir", PayloadStabilityStable, "子代理运行工作目录。"),
	},
	HookPointSubAgentStop: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("task_id", PayloadStabilityStable, "子代理任务标识。"),
		payloadStringField("role", PayloadStabilityStable, "子代理角色。"),
		payloadStringField("state", PayloadStabilityStable, "子代理结束时状态。"),
		payloadStringField("stop_reason", PayloadStabilityStable, "子代理停止原因。"),
		payloadIntegerField("step_count", PayloadStabilityStable, "子代理累计步数。"),
		payloadStringField("error", PayloadStabilityStable, "子代理错误信息。"),
	},
	HookPointUserPromptSubmit: {
		payloadStringField("run_id", PayloadStabilityStable, "当前 run 标识的 metadata 冗余副本。"),
		payloadStringField("session_id", PayloadStabilityStable, "当前 session 标识的 metadata 冗余副本。"),
		payloadStringField("workdir", PayloadStabilityStable, "本次运行的工作目录。"),
	},
}

// PayloadSchema 返回指定点位的公开 payload 契约定义。
func PayloadSchema(point HookPoint) PointPayloadSchema {
	metadata, ok := pointPayloadMetadata[point]
	if !ok {
		return PointPayloadSchema{}
	}
	return PointPayloadSchema{
		Point:          point,
		PayloadVersion: PayloadVersion,
		TopLevel:       clonePayloadFieldSchemas(payloadTopLevelFields),
		Metadata:       clonePayloadFieldSchemas(metadata),
	}
}

// ListPayloadSchemas 返回所有已注册点位的 payload 契约，顺序与 ListHookPoints 一致。
func ListPayloadSchemas() []PointPayloadSchema {
	points := ListHookPoints()
	schemas := make([]PointPayloadSchema, 0, len(points))
	for _, point := range points {
		schema := PayloadSchema(point)
		if schema.Point == "" {
			continue
		}
		schemas = append(schemas, schema)
	}
	return schemas
}

// BuildPayloadJSONSchema 构建对外公开的 hook payload JSON Schema。
func BuildPayloadJSONSchema() map[string]any {
	oneOf := make([]any, 0, len(pointPayloadMetadata))
	for _, schema := range ListPayloadSchemas() {
		oneOf = append(oneOf, buildPointPayloadJSONSchema(schema))
	}
	return map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         "https://neocode.dev/schemas/hook-payload.v1.json",
		"title":       "NeoCode Hook Payload v1",
		"description": "Public hook payload contract for command stdin and shared HTTP observe fields.",
		"type":        "object",
		"oneOf":       oneOf,
	}
}

// MarshalPayloadJSONSchema 以稳定格式序列化 hook payload JSON Schema。
func MarshalPayloadJSONSchema() ([]byte, error) {
	encoded, err := json.MarshalIndent(BuildPayloadJSONSchema(), "", "  ")
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')
	return encoded, nil
}

// payloadStringField 构造 string 类型的字段契约定义。
func payloadStringField(name string, stability PayloadStability, description string) PayloadFieldSchema {
	return PayloadFieldSchema{
		Name:        name,
		JSONType:    "string",
		Stability:   stability,
		Description: description,
	}
}

// payloadBoolField 构造 boolean 类型的字段契约定义。
func payloadBoolField(name string, stability PayloadStability, description string) PayloadFieldSchema {
	return PayloadFieldSchema{
		Name:        name,
		JSONType:    "boolean",
		Stability:   stability,
		Description: description,
	}
}

// payloadIntegerField 构造 integer 类型的字段契约定义。
func payloadIntegerField(name string, stability PayloadStability, description string) PayloadFieldSchema {
	return PayloadFieldSchema{
		Name:        name,
		JSONType:    "integer",
		Stability:   stability,
		Description: description,
	}
}

// payloadObjectField 构造 object 类型的字段契约定义。
func payloadObjectField(
	name string,
	stability PayloadStability,
	description string,
	properties ...PayloadFieldSchema,
) PayloadFieldSchema {
	return PayloadFieldSchema{
		Name:        name,
		JSONType:    "object",
		Stability:   stability,
		Description: description,
		Properties:  clonePayloadFieldSchemas(properties),
	}
}

// payloadArrayOfObjectsField 构造对象数组类型的字段契约定义。
func payloadArrayOfObjectsField(
	name string,
	stability PayloadStability,
	description string,
	itemProperties ...PayloadFieldSchema,
) PayloadFieldSchema {
	return PayloadFieldSchema{
		Name:           name,
		JSONType:       "array",
		Stability:      stability,
		Description:    description,
		ItemType:       "object",
		ItemProperties: clonePayloadFieldSchemas(itemProperties),
	}
}

// clonePayloadFieldSchemas 深拷贝字段定义，避免调用方修改共享 schema。
func clonePayloadFieldSchemas(fields []PayloadFieldSchema) []PayloadFieldSchema {
	if len(fields) == 0 {
		return nil
	}
	cloned := make([]PayloadFieldSchema, len(fields))
	copy(cloned, fields)
	for index := range cloned {
		cloned[index].Properties = clonePayloadFieldSchemas(fields[index].Properties)
		cloned[index].ItemProperties = clonePayloadFieldSchemas(fields[index].ItemProperties)
	}
	return cloned
}

// sanitizePayloadMetadata 按点位 schema 过滤 metadata，并复制允许暴露的值。
func sanitizePayloadMetadata(point HookPoint, metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	schema := PayloadSchema(point)
	if schema.Point == "" || len(schema.Metadata) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(schema.Metadata))
	for _, field := range schema.Metadata {
		allowed[strings.ToLower(strings.TrimSpace(field.Name))] = struct{}{}
	}
	sanitized := make(map[string]any, len(schema.Metadata))
	for key, value := range metadata {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if _, ok := allowed[normalizedKey]; !ok {
			continue
		}
		sanitized[normalizedKey] = cloneMetadataValue(value)
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

// buildPointPayloadJSONSchema 将单点位契约转换为 JSON Schema 分支。
func buildPointPayloadJSONSchema(schema PointPayloadSchema) map[string]any {
	properties := make(map[string]any, len(schema.TopLevel)+1)
	required := []string{"payload_version", "hook_id", "point"}
	for _, field := range schema.TopLevel {
		propertySchema := buildPayloadFieldJSONSchema(field)
		switch field.Name {
		case "payload_version":
			propertySchema["const"] = schema.PayloadVersion
		case "point":
			propertySchema["const"] = string(schema.Point)
		}
		properties[field.Name] = propertySchema
	}
	properties["metadata"] = map[string]any{
		"type":                 "object",
		"description":          "Point-specific hook metadata fields.",
		"additionalProperties": false,
		"properties":           buildPayloadPropertySchemas(schema.Metadata),
	}
	return map[string]any{
		"title":                string(schema.Point),
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}

// buildPayloadFieldJSONSchema 将字段契约转换为 JSON Schema 节点。
func buildPayloadFieldJSONSchema(field PayloadFieldSchema) map[string]any {
	schema := map[string]any{
		"type":        field.JSONType,
		"x-stability": string(field.Stability),
	}
	if description := strings.TrimSpace(field.Description); description != "" {
		schema["description"] = description
	}
	switch field.JSONType {
	case "array":
		items := map[string]any{}
		if field.ItemType != "" {
			items["type"] = field.ItemType
		}
		if len(field.ItemProperties) > 0 {
			items["properties"] = buildPayloadPropertySchemas(field.ItemProperties)
			items["additionalProperties"] = false
		}
		if len(items) > 0 {
			schema["items"] = items
		}
	case "object":
		schema["additionalProperties"] = false
		if len(field.Properties) > 0 {
			schema["properties"] = buildPayloadPropertySchemas(field.Properties)
		}
	}
	return schema
}

// buildPayloadPropertySchemas 批量构造字段名到 JSON Schema 的映射。
func buildPayloadPropertySchemas(fields []PayloadFieldSchema) map[string]any {
	if len(fields) == 0 {
		return map[string]any{}
	}
	properties := make(map[string]any, len(fields))
	for _, field := range fields {
		properties[field.Name] = buildPayloadFieldJSONSchema(field)
	}
	return properties
}

// fieldNamesByStability 收集指定稳定性等级的字段名，供测试回归校验使用。
func fieldNamesByStability(fields []PayloadFieldSchema, stability PayloadStability) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.Stability != stability {
			continue
		}
		names = append(names, field.Name)
	}
	slices.Sort(names)
	return names
}
