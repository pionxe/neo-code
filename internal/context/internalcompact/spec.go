package internalcompact

import "strings"

const (
	// SummaryMarker 是 compact 摘要协议要求的固定起始标记。
	SummaryMarker = "[compact_summary]"
	// SectionDone 标识已完成事项 section。
	SectionDone = "done"
	// SectionInProgress 标识进行中事项 section。
	SectionInProgress = "in_progress"
	// SectionDecisions 标识关键决策 section。
	SectionDecisions = "decisions"
	// SectionCodeChanges 标识代码变更 section。
	SectionCodeChanges = "code_changes"
	// SectionConstraints 标识约束条件 section。
	SectionConstraints = "constraints"
)

var summarySections = []string{
	SectionDone,
	SectionInProgress,
	SectionDecisions,
	SectionCodeChanges,
	SectionConstraints,
}

// SummarySections 返回 compact 摘要协议要求的 section 顺序副本。
func SummarySections() []string {
	return append([]string(nil), summarySections...)
}

// FormatTemplate 渲染 compact 摘要协议的固定格式示例，供 prompt 与校验逻辑复用。
func FormatTemplate() string {
	var builder strings.Builder
	builder.WriteString(SummaryMarker)
	builder.WriteString("\n")

	for index, section := range summarySections {
		if index > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(section)
		builder.WriteString(":\n- ...")
	}

	return builder.String()
}
