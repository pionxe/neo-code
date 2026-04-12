package commands

import "strings"

// SlashCommand 描述单个 slash 命令定义。
type SlashCommand struct {
	Usage       string
	Description string
}

// CommandSuggestion 表示输入匹配后的命令建议。
type CommandSuggestion struct {
	Command SlashCommand
	Match   bool
}

// MatchSlashCommands 根据输入匹配可展示的 slash 命令建议。
func MatchSlashCommands(input string, slashPrefix string, commands []SlashCommand) []CommandSuggestion {
	if !strings.HasPrefix(input, slashPrefix) {
		return nil
	}

	query := strings.ToLower(strings.TrimSpace(input))
	if IsCompleteSlashCommand(query, commands) {
		return nil
	}
	out := make([]CommandSuggestion, 0, len(commands))
	for _, command := range commands {
		normalized := strings.ToLower(command.Usage)
		match := query == slashPrefix || strings.HasPrefix(normalized, query)
		if query == slashPrefix || match || strings.Contains(normalized, query) {
			out = append(out, CommandSuggestion{Command: command, Match: match})
		}
	}
	return out
}

// IsCompleteSlashCommand 判断输入是否已完整匹配某个命令。
func IsCompleteSlashCommand(input string, commands []SlashCommand) bool {
	for _, command := range commands {
		if strings.EqualFold(strings.TrimSpace(command.Usage), strings.TrimSpace(input)) {
			return true
		}
	}
	return false
}

// SplitFirstWord 拆分首个 token 与其后续参数。
func SplitFirstWord(input string) (string, string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ""
	}
	index := strings.IndexAny(input, " \t")
	if index < 0 {
		return input, ""
	}
	return input[:index], strings.TrimSpace(input[index+1:])
}
