package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// runtimeContractEventSourceFiles 定义 runtime 事件常量的源文件列表。
var runtimeContractEventSourceFiles = []string{
	"internal/runtime/events.go",
	"internal/runtime/events_subagent.go",
}

// TestRegisteredEventTypesSorted 验证 RegisteredEventTypes 返回排序后的列表。
func TestRegisteredEventTypesSorted(t *testing.T) {
	types := RegisteredEventTypes()
	if len(types) == 0 {
		t.Fatal("RegisteredEventTypes returned empty slice")
	}
	for i := 1; i < len(types); i++ {
		if types[i] < types[i-1] {
			t.Fatalf("RegisteredEventTypes not sorted: %q < %q at index %d", types[i], types[i-1], i)
		}
	}
}

// TestRequireConsumerKnownEvents 验证已知的 RequireConsumer=true 事件被正确注册。
func TestRequireConsumerKnownEvents(t *testing.T) {
	mustRequireConsumer := []EventType{
		EventUserMessage,
		EventToolStart,
		EventToolResult,
		EventPermissionRequested,
		EventCompactApplied,
		EventTokenUsage,
		EventHookStarted,
		EventCheckpointCreated,
		EventSubAgentStarted,
		EventRuntimeSnapshotUpdated,
		EventDecisionMade,
	}
	for _, eventType := range mustRequireConsumer {
		if !RequireConsumer(eventType) {
			t.Errorf("expected RequireConsumer(%q) = true, got false", eventType)
		}
	}
}

// TestRequireConsumerUnregistered 验证未注册事件返回 false（允许透传）。
func TestRequireConsumerUnregistered(t *testing.T) {
	if RequireConsumer("nonexistent_event") {
		t.Error("expected RequireConsumer for unregistered event to return false")
	}
}

// TestRequireConsumerPassthroughEvents 验证显式声明为透传安全的事件返回 false。
func TestRequireConsumerPassthroughEvents(t *testing.T) {
	passthroughEvents := []EventType{
		EventRunCanceled,
		EventSkillActivated,
		EventSkillDeactivated,
		EventSkillMissing,
		EventProgressEvaluated,
		EventTodoSummaryInjected,
	}
	for _, eventType := range passthroughEvents {
		if RequireConsumer(eventType) {
			t.Errorf("expected RequireConsumer(%q) = false for passthrough event, got true", eventType)
		}
	}
}

// TestIsRegisteredEventType 验证事件注册查询。
func TestIsRegisteredEventType(t *testing.T) {
	if !IsRegisteredEventType(EventUserMessage) {
		t.Error("expected EventUserMessage to be registered")
	}
	if IsRegisteredEventType("totally_unknown_event") {
		t.Error("expected unknown event to not be registered")
	}
}

// TestRuntimeEventContractConsistency 扫描 runtime 事件常量并与 contractRegistry 求差集。
// 若存在 RequireConsumer=true 的事件未在 contractRegistry 中注册，测试失败。
func TestRuntimeEventContractConsistency(t *testing.T) {
	runtimeEventValues := collectRuntimeEventConstants(t)
	if len(runtimeEventValues) == 0 {
		t.Fatal("no runtime Event* constants found in events.go / events_subagent.go")
	}

	registeredTypes := make(map[EventType]struct{}, len(contractRegistry))
	for eventType := range contractRegistry {
		registeredTypes[eventType] = struct{}{}
	}

	var violations []string
	for _, eventValue := range runtimeEventValues {
		eventType := EventType(eventValue)
		if _, registered := registeredTypes[eventType]; registered {
			continue
		}
		// 未注册到 contractRegistry → 默认 RequireConsumer=false（透传安全）
		// 这是可接受的，仅记录日志
		t.Logf("runtime event %q not registered in contractRegistry (passthrough allowed)", eventValue)
	}

	// 反向检查：contractRegistry 中 RequireConsumer=true 的事件是否都在 runtime 中定义
	// 这确保了 TUI 侧不会声明一个 runtime 根本不产生的事件需要消费者
	runtimeEventSet := make(map[string]struct{}, len(runtimeEventValues))
	for _, v := range runtimeEventValues {
		runtimeEventSet[v] = struct{}{}
	}
	for eventType, entry := range contractRegistry {
		if !entry.RequireConsumer {
			continue
		}
		// 跳过 TUI 侧特有的 bridge 事件（如 run_context, tool_status, usage）
		if isTUIBridgeEvent(eventType) {
			continue
		}
		if _, exists := runtimeEventSet[string(eventType)]; !exists {
			violations = append(violations, string(eventType))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf(
			"contractRegistry events with RequireConsumer=true not found in runtime events.go:\n  %s\n\n"+
				"These events are declared as requiring consumers but runtime does not produce them.\n"+
				"Fix: remove from contractRegistry or add the event to runtime events.go.",
			strings.Join(violations, "\n  "),
		)
	}
}

// TestGatewayDecodeBranchConsistency 扫描 gateway_stream_client.go 的 decode 分支，
// 验证所有 decode 分支中处理的事件类型都在 contractRegistry 中注册。
func TestGatewayDecodeBranchConsistency(t *testing.T) {
	decodedConstNames := collectGatewayDecodeConstNames(t)
	if len(decodedConstNames) == 0 {
		t.Fatal("no decode branches found in restoreRuntimePayload")
	}

	// 构建 contractRegistry 值到 EventType 的反向映射
	valueToEventType := make(map[string]EventType, len(contractRegistry))
	for eventType := range contractRegistry {
		valueToEventType[string(eventType)] = eventType
	}

	// 构建 contractRegistry 中所有已注册的事件值集合
	registeredValues := make(map[string]struct{}, len(contractRegistry))
	for eventType := range contractRegistry {
		registeredValues[string(eventType)] = struct{}{}
	}

	// TUI bridge 事件值
	bridgeValues := map[string]struct{}{
		"run_context":   {},
		"tool_status":   {},
		"usage":         {},
	}

	for _, constName := range decodedConstNames {
		// 如果是字符串值（如 "user_message"），直接检查
		if _, registered := registeredValues[constName]; registered {
			continue
		}
		// 如果是 bridge 事件，跳过
		if _, isBridge := bridgeValues[constName]; isBridge {
			continue
		}
		// 如果是常量名（如 "EventUserMessage"），尝试解析
		if resolvedValue, ok := resolveConstNameToValue(constName); ok {
			if _, registered := registeredValues[resolvedValue]; registered {
				continue
			}
		}
		t.Errorf(
			"gateway decode branch handles %q but it is not registered in contractRegistry; "+
				"add it to contractRegistry with RequireConsumer=true",
			constName,
		)
	}
}

// TestRequireConsumerMustHaveDecodeBranch 验证 contractRegistry 中 RequireConsumer=true 的事件
// 必须在 gateway_stream_client.go 中有对应的 decode 分支。
// 这是 CI 防漏的关键测试：新增 RequireConsumer=true 事件但忘记添加 decode 分支时，此测试失败。
func TestRequireConsumerMustHaveDecodeBranch(t *testing.T) {
	decodedValues := collectGatewayDecodeConstNames(t)
	decodedSet := make(map[string]struct{}, len(decodedValues))
	for _, v := range decodedValues {
		decodedSet[v] = struct{}{}
	}

	// bridge 事件值
	bridgeValues := map[string]struct{}{
		RuntimeEventRunContext: {},
		RuntimeEventToolStatus: {},
		RuntimeEventUsage:      {},
	}

	var violations []string
	for eventType, entry := range contractRegistry {
		if !entry.RequireConsumer {
			continue
		}
		value := string(eventType)
		if _, decoded := decodedSet[value]; decoded {
			continue
		}
		if _, isBridge := bridgeValues[value]; isBridge {
			continue
		}
		violations = append(violations, value)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf(
			"contractRegistry events with RequireConsumer=true missing gateway decode branch:\n  %s\n\n"+
				"Fix: add a decode branch in restoreRuntimePayload (gateway_stream_client.go), or "+
				"set RequireConsumer=false in contractRegistry if passthrough is acceptable.",
			strings.Join(violations, "\n  "),
		)
	}
}

// isTUIBridgeEvent 判断事件是否为 TUI 侧特有的 bridge 事件（非 runtime 产生）。
func isTUIBridgeEvent(eventType EventType) bool {
	bridgeEvents := map[EventType]struct{}{
		EventType(RuntimeEventRunContext):  {},
		EventType(RuntimeEventToolStatus):  {},
		EventType(RuntimeEventUsage):       {},
		EventRunCanceled:                   {},
	}
	_, ok := bridgeEvents[eventType]
	return ok
}

// resolveConstNameToValue 尝试将常量名（如 "EventUserMessage"）解析为字符串值（如 "user_message"）。
// 使用 contractRegistry 的键作为已知值映射。
func resolveConstNameToValue(constName string) (string, bool) {
	// 从 gateway_stream_client.go 中的 EventType(RuntimeEventXxx) 模式
	// 这些是 bridge 事件，值在 runtime_bridge.go 中定义
	bridgeConstMap := map[string]string{
		"RuntimeEventRunContext":  RuntimeEventRunContext,
		"RuntimeEventToolStatus": RuntimeEventToolStatus,
		"RuntimeEventUsage":      RuntimeEventUsage,
	}
	if value, ok := bridgeConstMap[constName]; ok {
		return value, true
	}
	return "", false
}

// collectRuntimeEventConstants 从 runtime 事件源文件中提取所有 Event* 常量值。
func collectRuntimeEventConstants(t *testing.T) []string {
	t.Helper()

	projectRoot := findProjectRoot(t)
	var allValues []string
	for _, relPath := range runtimeContractEventSourceFiles {
		filePath := filepath.Join(projectRoot, filepath.FromSlash(relPath))
		allValues = append(allValues, extractEventConstValues(t, filePath)...)
	}
	return allValues
}

// extractEventConstValues 使用 AST 解析提取文件中 EventType 类型的常量值。
func extractEventConstValues(t *testing.T, filePath string) []string {
	t.Helper()

	src, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", filePath, err)
	}

	var values []string
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// 检查类型是否为 EventType
			if valueSpec.Type == nil {
				continue
			}
			typeIdent, ok := valueSpec.Type.(*ast.Ident)
			if !ok || typeIdent.Name != "EventType" {
				continue
			}
			for i, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, "Event") {
					continue
				}
				if i < len(valueSpec.Values) {
					if basicLit, ok := valueSpec.Values[i].(*ast.BasicLit); ok {
						// 去掉引号
						value := strings.Trim(basicLit.Value, "\"")
						values = append(values, value)
					}
				}
			}
		}
	}
	return values
}

// collectGatewayDecodeConstNames 从 gateway_stream_client.go 的 restoreRuntimePayload 中提取解码的事件类型值。
// 对于常量引用（如 EventUserMessage），通过解析同包 const 声明解析为字符串值。
func collectGatewayDecodeConstNames(t *testing.T) []string {
	t.Helper()

	projectRoot := findProjectRoot(t)

	// 从 runtime_contract.go 构建常量名→值映射
	constNameToValue := buildEventTypeConstMap(t, filepath.Join(projectRoot, "internal", "tui", "services", "runtime_contract.go"))
	// 从 runtime_bridge.go 构建 bridge 常量名→值映射
	bridgeConstMap := buildBridgeConstMap(t, filepath.Join(projectRoot, "internal", "tui", "services", "runtime_bridge.go"))
	for k, v := range bridgeConstMap {
		constNameToValue[k] = v
	}

	filePath := filepath.Join(projectRoot, "internal", "tui", "services", "gateway_stream_client.go")

	src, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", filePath, err)
	}

	var eventValues []string
	ast.Inspect(file, func(n ast.Node) bool {
		caseClause, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expr := range caseClause.List {
			switch v := expr.(type) {
			case *ast.Ident:
				// case EventXxx:
				if strings.HasPrefix(v.Name, "Event") || strings.HasPrefix(v.Name, "RuntimeEvent") {
					if value, ok := constNameToValue[v.Name]; ok {
						eventValues = append(eventValues, value)
					} else {
						// 无法解析的常量名，保留原始名称用于诊断
						eventValues = append(eventValues, v.Name)
					}
				}
			case *ast.CallExpr:
				// case EventType("xxx") 或 case EventType(RuntimeEventXxx):
				if funIdent, ok := v.Fun.(*ast.Ident); ok && funIdent.Name == "EventType" {
					if len(v.Args) > 0 {
						switch arg := v.Args[0].(type) {
						case *ast.BasicLit:
							// case EventType("xxx"):
							value := strings.Trim(arg.Value, "\"")
							eventValues = append(eventValues, value)
						case *ast.Ident:
							// case EventType(RuntimeEventXxx):
							if value, ok := constNameToValue[arg.Name]; ok {
								eventValues = append(eventValues, value)
							} else {
								eventValues = append(eventValues, arg.Name)
							}
						}
					}
				}
			}
		}
		return true
	})
	return eventValues
}

// buildEventTypeConstMap 从 runtime_contract.go 中提取 EventType 常量名→值映射。
func buildEventTypeConstMap(t *testing.T, filePath string) map[string]string {
	t.Helper()
	return extractConstStringMap(t, filePath, "EventType")
}

// buildBridgeConstMap 从 runtime_bridge.go 中提取常量名→值映射（包括无类型常量）。
func buildBridgeConstMap(t *testing.T, filePath string) map[string]string {
	t.Helper()

	src, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", filePath, err)
	}

	result := make(map[string]string)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// 只提取以 RuntimeEvent 开头的常量
			for i, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, "RuntimeEvent") {
					continue
				}
				if i < len(valueSpec.Values) {
					if basicLit, ok := valueSpec.Values[i].(*ast.BasicLit); ok {
						value := strings.Trim(basicLit.Value, "\"")
						result[name.Name] = value
					}
				}
			}
		}
	}
	return result
}

// extractConstStringMap 从指定文件中提取指定类型的 const 字符串映射。
func extractConstStringMap(t *testing.T, filePath string, typeName string) map[string]string {
	t.Helper()

	src, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", filePath, err)
	}

	result := make(map[string]string)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if valueSpec.Type == nil {
				continue
			}
			typeIdent, ok := valueSpec.Type.(*ast.Ident)
			if !ok || typeIdent.Name != typeName {
				continue
			}
			for i, name := range valueSpec.Names {
				if i < len(valueSpec.Values) {
					if basicLit, ok := valueSpec.Values[i].(*ast.BasicLit); ok {
						value := strings.Trim(basicLit.Value, "\"")
						result[name.Name] = value
					}
				}
			}
		}
	}
	return result
}

// findProjectRoot 向上查找 go.mod 所在目录。
func findProjectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (go.mod not found)")
		}
		dir = parent
	}
}
