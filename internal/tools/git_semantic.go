package tools

import "strings"

const (
	// BashGitResourceReadOnly 表示 Git 只读语义命令资源。
	BashGitResourceReadOnly = "bash_git_read_only"
	// BashGitResourceLocalMutation 表示 Git 本地变更语义命令资源。
	BashGitResourceLocalMutation = "bash_git_local_mutation"
	// BashGitResourceRemoteOp 表示 Git 远端交互语义命令资源。
	BashGitResourceRemoteOp = "bash_git_remote_op"
	// BashGitResourceDestructive 表示 Git 破坏性语义命令资源。
	BashGitResourceDestructive = "bash_git_destructive"
	// BashGitResourceUnknown 表示无法安全判定语义的 Git 命令资源。
	BashGitResourceUnknown = "bash_git_unknown"
)

// NormalizeGitSemanticClass 统一归一化 git 语义分类，避免散落逻辑分叉。
func NormalizeGitSemanticClass(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case BashIntentClassificationReadOnly:
		return BashIntentClassificationReadOnly
	case BashIntentClassificationLocalMutation:
		return BashIntentClassificationLocalMutation
	case BashIntentClassificationRemoteOp:
		return BashIntentClassificationRemoteOp
	case BashIntentClassificationDestructive:
		return BashIntentClassificationDestructive
	default:
		return BashIntentClassificationUnknown
	}
}

// BashGitResourceForClass 将语义分类映射为稳定资源名，供权限层统一使用。
func BashGitResourceForClass(raw string) string {
	switch NormalizeGitSemanticClass(raw) {
	case BashIntentClassificationReadOnly:
		return BashGitResourceReadOnly
	case BashIntentClassificationLocalMutation:
		return BashGitResourceLocalMutation
	case BashIntentClassificationRemoteOp:
		return BashGitResourceRemoteOp
	case BashIntentClassificationDestructive:
		return BashGitResourceDestructive
	default:
		return BashGitResourceUnknown
	}
}
