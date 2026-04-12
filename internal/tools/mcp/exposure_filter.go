package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ExposureFilterReason 描述 MCP tool 暴露过滤命中的原因。
type ExposureFilterReason string

const (
	ExposureFilterReasonOffline       ExposureFilterReason = "offline"
	ExposureFilterReasonPolicyDeny    ExposureFilterReason = "policy_deny"
	ExposureFilterReasonAgentMismatch ExposureFilterReason = "agent_mismatch"
	ExposureFilterReasonAllowlistMiss ExposureFilterReason = "allowlist_miss"
	ExposureFilterReasonFilterError   ExposureFilterReason = "filter_error"
)

// ExposureFilterInput 描述一次 MCP specs 暴露过滤所依赖的上下文。
type ExposureFilterInput struct {
	SessionID string
	Agent     string
	Query     string
}

// ExposureDecision 记录单个 MCP tool 是否暴露以及命中的过滤原因。
type ExposureDecision struct {
	ServerID     string
	ToolName     string
	ToolFullName string
	Allowed      bool
	Reason       ExposureFilterReason
}

// ExposureFilter 定义 ListAvailableSpecs 阶段的 MCP 暴露过滤接口。
type ExposureFilter interface {
	Filter(ctx context.Context, snapshots []ServerSnapshot, input ExposureFilterInput) ([]ServerSnapshot, []ExposureDecision, error)
}

// ExposureFilterConfig 描述 MCP tool 暴露过滤策略。
type ExposureFilterConfig struct {
	Allowlist []string
	Denylist  []string
	Agents    []AgentExposureRule
}

// AgentExposureRule 描述单个 agent 角色可见的 MCP tool 模式集合。
type AgentExposureRule struct {
	Agent     string
	Allowlist []string
}

// DefaultExposureFilter 是默认的 MCP 暴露过滤实现。
type DefaultExposureFilter struct {
	cfg ExposureFilterConfig
}

// NewExposureFilter 创建默认 MCP 暴露过滤器。
func NewExposureFilter(cfg ExposureFilterConfig) *DefaultExposureFilter {
	return &DefaultExposureFilter{cfg: normalizeExposureFilterConfig(cfg)}
}

// Filter 基于不可变快照执行 MCP tool 暴露过滤，并返回审计决策。
func (f *DefaultExposureFilter) Filter(
	ctx context.Context,
	snapshots []ServerSnapshot,
	input ExposureFilterInput,
) ([]ServerSnapshot, []ExposureDecision, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	cfg := ExposureFilterConfig{}
	if f != nil {
		cfg = f.cfg
	}

	normalizedAgent := strings.ToLower(strings.TrimSpace(input.Agent))
	var agentRule *AgentExposureRule
	if len(cfg.Agents) > 0 {
		for index := range cfg.Agents {
			if cfg.Agents[index].Agent == normalizedAgent {
				agentRule = &cfg.Agents[index]
				break
			}
		}
	}

	result := make([]ServerSnapshot, 0, len(snapshots))
	decisions := make([]ExposureDecision, 0)
	for _, snapshot := range snapshots {
		serverIDPattern := serverIdentity(snapshot.ServerID)
		if snapshot.Status != ServerStatusReady {
			for _, tool := range snapshot.Tools {
				decisions = append(decisions, deniedExposureDecision(snapshot.ServerID, tool.Name, ExposureFilterReasonOffline))
			}
			continue
		}

		filteredTools := make([]ToolDescriptor, 0, len(snapshot.Tools))
		for _, tool := range snapshot.Tools {
			fullName := composeToolName(snapshot.ServerID, tool.Name)
			decision := ExposureDecision{
				ServerID:     snapshot.ServerID,
				ToolName:     tool.Name,
				ToolFullName: fullName,
			}

			if matchesAnyIdentity(fullName, serverIDPattern, cfg.Denylist) {
				decision.Reason = ExposureFilterReasonPolicyDeny
				decisions = append(decisions, decision)
				continue
			}
			if len(cfg.Allowlist) > 0 && !matchesAnyIdentity(fullName, serverIDPattern, cfg.Allowlist) {
				decision.Reason = ExposureFilterReasonAllowlistMiss
				decisions = append(decisions, decision)
				continue
			}
			if len(cfg.Agents) > 0 {
				if agentRule == nil {
					decision.Reason = ExposureFilterReasonAgentMismatch
					decisions = append(decisions, decision)
					continue
				}
				if len(agentRule.Allowlist) > 0 && !matchesAnyIdentity(fullName, serverIDPattern, agentRule.Allowlist) {
					decision.Reason = ExposureFilterReasonAgentMismatch
					decisions = append(decisions, decision)
					continue
				}
			}

			decision.Allowed = true
			filteredTools = append(filteredTools, tool)
			decisions = append(decisions, decision)
		}

		if len(filteredTools) == 0 {
			continue
		}

		cloned := snapshot
		cloned.Tools = cloneToolDescriptors(filteredTools)
		result = append(result, cloned)
	}

	sort.SliceStable(decisions, func(i, j int) bool {
		if decisions[i].ServerID == decisions[j].ServerID {
			return decisions[i].ToolFullName < decisions[j].ToolFullName
		}
		return decisions[i].ServerID < decisions[j].ServerID
	})
	return result, decisions, nil
}

// deniedExposureDecision 构造一条拒绝暴露的审计记录。
func deniedExposureDecision(serverID string, toolName string, reason ExposureFilterReason) ExposureDecision {
	return ExposureDecision{
		ServerID:     strings.TrimSpace(serverID),
		ToolName:     strings.TrimSpace(toolName),
		ToolFullName: composeToolName(serverID, toolName),
		Allowed:      false,
		Reason:       reason,
	}
}

// normalizeExposureFilterConfig 规范化过滤配置，确保比较逻辑稳定一致。
func normalizeExposureFilterConfig(cfg ExposureFilterConfig) ExposureFilterConfig {
	cfg.Allowlist = normalizeExposurePatternList(cfg.Allowlist)
	cfg.Denylist = normalizeExposurePatternList(cfg.Denylist)
	if len(cfg.Agents) == 0 {
		cfg.Agents = nil
		return cfg
	}

	agents := make([]AgentExposureRule, 0, len(cfg.Agents))
	for _, rule := range cfg.Agents {
		normalizedAgent := strings.ToLower(strings.TrimSpace(rule.Agent))
		if normalizedAgent == "" {
			continue
		}
		agents = append(agents, AgentExposureRule{
			Agent:     normalizedAgent,
			Allowlist: normalizeExposurePatternList(rule.Allowlist),
		})
	}
	if len(agents) == 0 {
		cfg.Agents = nil
	} else {
		cfg.Agents = agents
	}
	return cfg
}

// normalizeExposurePatternList 规范化模式列表并剔除空项。
func normalizeExposurePatternList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeExposurePattern(value)
		if normalized == "" {
			continue
		}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// normalizeExposurePattern 将模式统一规范为 mcp 前缀的小写形式。
func normalizeExposurePattern(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return ""
	}
	if strings.HasPrefix(normalized, "mcp.") {
		return normalized
	}
	return "mcp." + normalized
}

// matchesAnyIdentity 判断工具名或 server 名是否命中任一模式。
func matchesAnyIdentity(fullName string, serverIdentity string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	normalizedFullName := strings.ToLower(strings.TrimSpace(fullName))
	normalizedServer := strings.ToLower(strings.TrimSpace(serverIdentity))
	for _, pattern := range patterns {
		if matchesExposurePattern(normalizedFullName, pattern) || matchesExposurePattern(normalizedServer, pattern) {
			return true
		}
	}
	return false
}

// matchesExposurePattern 使用 glob 语义匹配单个暴露模式。
func matchesExposurePattern(value string, pattern string) bool {
	normalizedValue := strings.ToLower(strings.TrimSpace(value))
	normalizedPattern := normalizeExposurePattern(pattern)
	if normalizedPattern == "" || normalizedValue == "" {
		return false
	}
	matched, err := filepath.Match(normalizedPattern, normalizedValue)
	if err == nil && matched {
		return true
	}
	return normalizedValue == normalizedPattern
}

// serverIdentity 返回 server 级别的匹配标识。
func serverIdentity(serverID string) string {
	server := strings.ToLower(strings.TrimSpace(serverID))
	if server == "" {
		return ""
	}
	return fmt.Sprintf("mcp.%s", server)
}
