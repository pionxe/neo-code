package memo

import (
	"context"
	"strings"

	providertypes "neo-code/internal/provider/types"
)

// signalPhrases 包含规则提取器识别的显式记忆信号词。
var signalPhrases = []string{
	"记住", "记下来", "以后都这样", "以后都这样",
	"我偏好", "我喜欢", "我习惯", "我希望",
	"别再", "不要再", "不要使用", "避免",
	"always", "never", "prefer", "avoid",
	"remember", "make sure", "from now on",
}

// RuleExtractor 基于规则的轻量记忆提取器，检测用户消息中的显式信号词。
// 无外部依赖，适合作为默认提取器。
type RuleExtractor struct{}

// NewRuleExtractor 创建规则提取器实例。
func NewRuleExtractor() *RuleExtractor {
	return &RuleExtractor{}
}

// Extract 扫描最近的消息，检测含信号词的用户输入并构造记忆条目。
// 仅提取用户最后一条消息，避免重复。
func (r *RuleExtractor) Extract(ctx context.Context, messages []providertypes.Message) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 找到用户发送的最后一条消息
	var lastUserMsg string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == providertypes.RoleUser {
			lastUserMsg = messages[i].Content
			break
		}
	}
	if lastUserMsg == "" {
		return nil, nil
	}

	if !containsSignal(lastUserMsg) {
		return nil, nil
	}

	// 截断过长内容作为标题
	title := NormalizeTitle(lastUserMsg)
	if len(title) > 150 {
		title = title[:147] + "..."
	}

	return []Entry{
		{
			Type:    TypeUser,
			Title:   title,
			Content: lastUserMsg,
			Source:  SourceAutoExtract,
		},
	}, nil
}

// containsSignal 检查文本是否包含任意信号词。
func containsSignal(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range signalPhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return true
		}
	}
	return false
}

// ExtractAndStore 从消息中提取记忆并保存到 Service。
// 提取失败静默处理，不影响主循环。
func ExtractAndStore(ctx context.Context, extractor Extractor, svc *Service, messages []providertypes.Message) {
	if extractor == nil || svc == nil {
		return
	}

	entries, err := extractor.Extract(ctx, messages)
	if err != nil || len(entries) == 0 {
		return
	}

	for _, entry := range entries {
		_ = svc.Add(ctx, entry) // 提取失败不影响主循环，静默忽略
	}
}
