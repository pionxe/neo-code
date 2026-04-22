package types

// Usage 记录本次请求的 token 使用统计。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// BudgetEstimate 描述 provider 对冻结请求输入 token 的估算结果。
type BudgetEstimate struct {
	EstimatedInputTokens int    `json:"estimated_input_tokens"`
	EstimateSource       string `json:"estimate_source"`
	GatePolicy           string `json:"gate_policy"`
}
