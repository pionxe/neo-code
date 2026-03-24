package tools

import (
	"fmt"
	"strings"
	"sync"

	"go-llm-demo/internal/server/domain"
)

var (
	securityCheckerMu sync.RWMutex
	securityChecker   domain.SecurityChecker

	securityAskApprovalMu sync.Mutex
	securityAskApprovals  = map[string]int{}
)

// SetSecurityChecker 设置工具执行前使用的安全检查器。
// 传入 nil 表示关闭安全检查（默认行为）。
func SetSecurityChecker(checker domain.SecurityChecker) {
	securityCheckerMu.Lock()
	securityChecker = checker
	securityCheckerMu.Unlock()
}

func getSecurityChecker() domain.SecurityChecker {
	securityCheckerMu.RLock()
	checker := securityChecker
	securityCheckerMu.RUnlock()
	return checker
}

// ApproveSecurityAsk 为指定的安全询问发放一次性放行许可。
// 该许可会在下一次匹配到同一 (toolType, target) 时被消费。
func ApproveSecurityAsk(toolType, target string) {
	key := securityApprovalKey(toolType, target)
	if key == "" {
		return
	}
	securityAskApprovalMu.Lock()
	securityAskApprovals[key]++
	securityAskApprovalMu.Unlock()
}

func consumeSecurityAskApproval(toolType, target string) bool {
	key := securityApprovalKey(toolType, target)
	if key == "" {
		return false
	}
	securityAskApprovalMu.Lock()
	defer securityAskApprovalMu.Unlock()
	count := securityAskApprovals[key]
	if count <= 0 {
		return false
	}
	if count == 1 {
		delete(securityAskApprovals, key)
		return true
	}
	securityAskApprovals[key] = count - 1
	return true
}

func securityApprovalKey(toolType, target string) string {
	normalizedType := strings.ToLower(strings.TrimSpace(toolType))
	normalizedTarget := strings.TrimSpace(target)
	if normalizedType == "" || normalizedTarget == "" {
		return ""
	}
	return normalizedType + "\x00" + normalizedTarget
}

func guardToolExecution(toolType, target, toolName string) *ToolResult {
	checker := getSecurityChecker()
	if checker == nil {
		return nil
	}

	action := checker.Check(toolType, target)
	metadata := map[string]interface{}{
		"securityToolType": toolType,
		"securityTarget":   target,
		"securityAction":   string(action),
	}

	switch action {
	case domain.ActionAllow:
		return nil
	case domain.ActionDeny:
		return &ToolResult{
			ToolName: toolName,
			Success:  false,
			Error:    fmt.Sprintf("安全策略拒绝执行 %s: %s", toolType, target),
			Metadata: metadata,
		}
	case domain.ActionAsk:
		if consumeSecurityAskApproval(toolType, target) {
			return nil
		}
		return &ToolResult{
			ToolName: toolName,
			Success:  false,
			Error:    fmt.Sprintf("命中安全策略，执行 %s 前需要用户确认: %s", toolType, target),
			Metadata: metadata,
		}
	default:
		metadata["securityAction"] = string(domain.ActionAsk)
		if consumeSecurityAskApproval(toolType, target) {
			return nil
		}
		return &ToolResult{
			ToolName: toolName,
			Success:  false,
			Error:    fmt.Sprintf("安全策略返回未知动作(%s)，已按需确认处理: %s", action, target),
			Metadata: metadata,
		}
	}
}
