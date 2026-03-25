package tools

import (
	"fmt"
	"log"
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
	key, err := securityApprovalKey(toolType, target)
	if err != nil {
		log.Printf("warning: failed to record security ask approval: %v", err)
		return
	}
	securityAskApprovalMu.Lock()
	securityAskApprovals[key]++
	securityAskApprovalMu.Unlock()
}

func consumeSecurityAskApproval(toolType, target string) bool {
	key, err := securityApprovalKey(toolType, target)
	if err != nil {
		log.Printf("warning: failed to consume security ask approval: %v", err)
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

func securityApprovalKey(toolType, target string) (string, error) {
	normalizedType := strings.ToLower(strings.TrimSpace(toolType))
	normalizedTarget := strings.TrimSpace(target)
	if normalizedType == "" || normalizedTarget == "" {
		return "", fmt.Errorf("invalid security approval context: toolType=%q target=%q", toolType, target)
	}
	return normalizedType + "\x00" + normalizedTarget, nil
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
			Error:    fmt.Sprintf("Security policy denied execution of %s: %s", toolType, target),
			Metadata: metadata,
		}
	case domain.ActionAsk:
		if consumeSecurityAskApproval(toolType, target) {
			return nil
		}
		return &ToolResult{
			ToolName: toolName,
			Success:  false,
			Error:    fmt.Sprintf("Execution of %s on %s requires user confirmation (Action: Ask).", toolType, target),
			Metadata: metadata,
		}
	default:
		return nil
	}
}
