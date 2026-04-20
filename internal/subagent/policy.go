package subagent

import (
	"strings"
	"time"

	"neo-code/internal/promptasset"
)

const (
	defaultPolicyMaxSteps = 6
	defaultPolicyTimeout  = 30
	defaultToolCallsLimit = 6
)

// ToolUseMode 描述子代理在单步中是否允许模型发起工具调用。
type ToolUseMode string

const (
	// ToolUseModeAuto 表示模型按需自行决定是否调用工具。
	ToolUseModeAuto ToolUseMode = "auto"
	// ToolUseModeRequired 表示模型必须至少调用一次工具再结束。
	ToolUseModeRequired ToolUseMode = "required"
	// ToolUseModeDisabled 表示禁止模型发起任何工具调用。
	ToolUseModeDisabled ToolUseMode = "disabled"
)

// Valid 判断工具调用模式是否受支持。
func (m ToolUseMode) Valid() bool {
	switch m {
	case ToolUseModeAuto, ToolUseModeRequired, ToolUseModeDisabled:
		return true
	default:
		return false
	}
}

// RolePolicy 定义不同角色的执行策略。
type RolePolicy struct {
	Role                Role
	SystemPrompt        string
	AllowedTools        []string
	ToolUseMode         ToolUseMode
	MaxToolCallsPerStep int
	DefaultBudget       Budget
	RequiredSections    []string
}

// Validate 校验角色策略是否合法。
func (p RolePolicy) Validate() error {
	if !p.Role.Valid() {
		return errorsf("invalid policy role %q", p.Role)
	}
	if strings.TrimSpace(p.SystemPrompt) == "" {
		return errorsf("role policy prompt is required")
	}
	if len(dedupeAndTrim(p.AllowedTools)) == 0 {
		return errorsf("role policy allowed tools is empty")
	}
	if len(dedupeAndTrim(p.RequiredSections)) == 0 {
		return errorsf("role policy required sections is empty")
	}
	if p.ToolUseMode == "" {
		p.ToolUseMode = ToolUseModeAuto
	}
	if !p.ToolUseMode.Valid() {
		return errorsf("role policy tool use mode %q is invalid", p.ToolUseMode)
	}
	if p.MaxToolCallsPerStep <= 0 {
		return errorsf("role policy max tool calls per step must be greater than zero")
	}
	if _, err := normalizeRequiredSections(p.RequiredSections); err != nil {
		return err
	}
	return nil
}

// DefaultRolePolicy 返回内置角色策略。
func DefaultRolePolicy(role Role) (RolePolicy, error) {
	if !role.Valid() {
		return RolePolicy{}, errorsf("unsupported role %q", role)
	}

	policy := RolePolicy{
		Role: role,
		DefaultBudget: Budget{
			MaxSteps: defaultPolicyMaxSteps,
			Timeout:  defaultPolicyTimeout * time.Second,
		},
		ToolUseMode:         ToolUseModeAuto,
		MaxToolCallsPerStep: defaultToolCallsLimit,
		RequiredSections: []string{
			"summary",
			"findings",
			"patches",
			"risks",
			"next_actions",
			"artifacts",
		},
	}

	switch role {
	case RoleResearcher:
		policy.SystemPrompt = promptasset.ResearcherRolePrompt()
		policy.AllowedTools = []string{"filesystem_read_file", "filesystem_glob", "filesystem_grep", "webfetch"}
	case RoleCoder:
		policy.SystemPrompt = promptasset.CoderRolePrompt()
		policy.AllowedTools = []string{
			"filesystem_read_file",
			"filesystem_write_file",
			"filesystem_edit",
			"filesystem_glob",
			"filesystem_grep",
			"bash",
		}
	case RoleReviewer:
		policy.SystemPrompt = promptasset.ReviewerRolePrompt()
		policy.AllowedTools = []string{"filesystem_read_file", "filesystem_glob", "filesystem_grep"}
	}

	policy.AllowedTools = dedupeAndTrim(policy.AllowedTools)
	policy.RequiredSections = dedupeAndTrim(policy.RequiredSections)
	return policy, nil
}
