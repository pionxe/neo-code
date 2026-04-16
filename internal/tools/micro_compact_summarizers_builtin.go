package tools

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// RegisterBuiltinSummarizers 将所有内置工具的内容摘要器注册到 Registry。
// 应在所有工具注册完成后调用一次。
func RegisterBuiltinSummarizers(registry *Registry) {
	if registry == nil {
		return
	}
	registry.RegisterSummarizer(ToolNameBash, bashSummarizer)
	registry.RegisterSummarizer(ToolNameFilesystemReadFile, readFileSummarizer)
	registry.RegisterSummarizer(ToolNameFilesystemWriteFile, writeFileSummarizer)
	registry.RegisterSummarizer(ToolNameFilesystemEdit, editSummarizer)
	registry.RegisterSummarizer(ToolNameFilesystemGrep, grepSummarizer)
	registry.RegisterSummarizer(ToolNameFilesystemGlob, globSummarizer)
	registry.RegisterSummarizer(ToolNameWebFetch, webfetchSummarizer)
}

const summaryMaxRunes = 200

// bashSummarizer 保留退出状态 + 末尾若干行 + 工作目录。
func bashSummarizer(content string, metadata map[string]string, isError bool) string {
	var parts []string

	if isError {
		parts = append(parts, "[exit=non-zero]")
	} else {
		parts = append(parts, "[exit=0]")
	}

	if workdir := metadata["workdir"]; workdir != "" {
		parts = append(parts, "workdir="+workdir)
	}

	const tailLines = 5
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) > tailLines {
		body := "...(truncated)\n" + strings.Join(lines[len(lines)-tailLines:], "\n")
		parts = append(parts, body)
	} else if len(lines) > 0 && strings.TrimSpace(content) != "" {
		parts = append(parts, content)
	}

	return truncateRunes(strings.Join(parts, " "), summaryMaxRunes)
}

// readFileSummarizer 保留文件路径 + 行数 + 首尾行片段。
func readFileSummarizer(content string, metadata map[string]string, isError bool) string {
	path := metadata["path"]
	if path == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	lineCount := len(lines)

	var parts []string
	parts = append(parts, "[summary]", path, "lines="+strconv.Itoa(lineCount))

	if len(lines) > 0 {
		first := truncateRunes(strings.TrimSpace(lines[0]), 60)
		if first != "" {
			parts = append(parts, "first="+first)
		}
	}
	if len(lines) > 1 {
		last := truncateRunes(strings.TrimSpace(lines[len(lines)-1]), 60)
		if last != "" && last != strings.TrimSpace(lines[0]) {
			parts = append(parts, "last="+last)
		}
	}

	return truncateRunes(strings.Join(parts, " "), summaryMaxRunes)
}

// writeFileSummarizer 保留文件路径与写入字节数。
func writeFileSummarizer(content string, metadata map[string]string, isError bool) string {
	path := metadata["path"]
	if path == "" {
		return ""
	}
	bytes := metadata["bytes"]
	return "[summary] wrote " + path + " (" + bytes + " bytes)"
}

// editSummarizer 保留编辑路径与替换范围。
func editSummarizer(content string, metadata map[string]string, isError bool) string {
	path := metadata["relative_path"]
	if path == "" {
		path = metadata["path"]
	}
	if path == "" {
		return ""
	}
	searchLen := metadata["search_length"]
	replaceLen := metadata["replacement_length"]
	return "[summary] edited " + path + " (search=" + searchLen + " chars, replace=" + replaceLen + " chars)"
}

// grepSummarizer 保留搜索根目录、匹配计数与前若干文件名。
func grepSummarizer(content string, metadata map[string]string, isError bool) string {
	var parts []string
	parts = append(parts, "[summary] grep")

	if root := metadata["root"]; root != "" {
		parts = append(parts, "root="+root)
	}

	if matchedFiles := metadata["matched_files"]; matchedFiles != "" {
		parts = append(parts, "files="+matchedFiles)
	}
	if matchedLines := metadata["matched_lines"]; matchedLines != "" {
		parts = append(parts, "lines="+matchedLines)
	}

	// 从 content 中提取前几个不重复文件名
	contentLines := strings.Split(strings.TrimSpace(content), "\n")
	fileSet := make(map[string]struct{})
	var fileNames []string
	for _, line := range contentLines {
		if len(fileSet) >= 3 {
			break
		}
		idx := strings.Index(line, ":")
		if idx > 0 {
			f := line[:idx]
			if _, ok := fileSet[f]; !ok {
				fileSet[f] = struct{}{}
				fileNames = append(fileNames, f)
			}
		}
	}
	if len(fileNames) > 0 {
		parts = append(parts, "matches="+strings.Join(fileNames, ", "))
	}

	return truncateRunes(strings.Join(parts, " "), summaryMaxRunes)
}

// globSummarizer 保留匹配计数与前若干文件名。
func globSummarizer(content string, metadata map[string]string, isError bool) string {
	count := metadata["count"]
	if count == "" {
		count = "?"
	}

	contentLines := strings.Split(strings.TrimSpace(content), "\n")
	const previewLimit = 3
	var preview []string
	for i, line := range contentLines {
		if i >= previewLimit {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			preview = append(preview, trimmed)
		}
	}

	var parts []string
	parts = append(parts, "[summary] glob", count+" files")
	if len(preview) > 0 {
		parts = append(parts, strings.Join(preview, ", "))
	}

	return truncateRunes(strings.Join(parts, " "), summaryMaxRunes)
}

// webfetchSummarizer 保留 URL、截断标记等持久化元数据。
func webfetchSummarizer(content string, metadata map[string]string, isError bool) string {
	var parts []string
	parts = append(parts, "[summary] webfetch")

	if url := metadata["url"]; url != "" {
		parts = append(parts, url)
	}
	if truncated := metadata["truncated"]; truncated == "true" {
		parts = append(parts, "truncated=true")
	}

	return truncateRunes(strings.Join(parts, " "), summaryMaxRunes)
}

// truncateRunes 按 rune 数量截断字符串，超出时追加 "..."。
func truncateRunes(text string, maxRunes int) string {
	if maxRunes <= 0 || text == "" {
		return text
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxRunes]) + "..."
}
