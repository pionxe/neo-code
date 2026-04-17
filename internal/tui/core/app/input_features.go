package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"neo-code/internal/config"
	tuiinfra "neo-code/internal/tui/infra"
	tuiservices "neo-code/internal/tui/services"
)

const (
	workspaceCommandPrefix = "&"
	workspaceCommandUsage  = "& <command>"
	fileReferencePrefix    = "@"
	imageReferencePrefix   = "@image:"
	imageReferenceUsage    = "@image:<path>"
	fileMenuTitle          = "Files"
	shellMenuTitle         = "Shell"
	maxWorkspaceFiles      = 4000
	maxFileSuggestions     = 6
	maxImageAttachments    = 3
)

type tokenSelector int

const (
	tokenSelectorFirst tokenSelector = iota
	tokenSelectorLast
)

var workspaceCommandExecutor = defaultWorkspaceCommandExecutor
var readClipboardImage = tuiinfra.ReadClipboardImage
var saveClipboardImageToTempFile = tuiinfra.SaveImageToTempFile

func isWorkspaceCommandInput(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), workspaceCommandPrefix)
}

func extractWorkspaceCommand(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, workspaceCommandPrefix) {
		return "", fmt.Errorf("usage: %s", workspaceCommandUsage)
	}
	command := strings.TrimSpace(strings.TrimPrefix(trimmed, workspaceCommandPrefix))
	if command == "" {
		return "", fmt.Errorf("usage: %s", workspaceCommandUsage)
	}
	return command, nil
}

func runWorkspaceCommand(configManager *config.Manager, workdir string, raw string) tea.Cmd {
	return tuiservices.RunWorkspaceCommandCmd(
		func(ctx context.Context) (string, string, error) {
			return executeWorkspaceCommand(ctx, configManager, workdir, raw)
		},
		func(command string, output string, err error) tea.Msg {
			return workspaceCommandResultMsg{
				Command: command,
				Output:  output,
				Err:     err,
			}
		},
	)
}

func executeWorkspaceCommand(ctx context.Context, configManager *config.Manager, workdir string, raw string) (string, string, error) {
	command, err := extractWorkspaceCommand(raw)
	if err != nil {
		return "", "", err
	}

	cfg := configManager.Get()
	output, execErr := workspaceCommandExecutor(ctx, cfg, workdir, command)
	return command, output, execErr
}

func defaultWorkspaceCommandExecutor(ctx context.Context, cfg config.Config, workdir string, command string) (string, error) {
	return tuiinfra.DefaultWorkspaceCommandExecutor(ctx, cfg, workdir, command)
}

func shellArgs(shell string, command string) []string {
	return tuiinfra.ShellArgs(shell, command)
}

func powershellUTF8Command(command string) string {
	return tuiinfra.PowerShellUTF8Command(command)
}

func formatWorkspaceCommandResult(command string, output string, err error) string {
	header := "Command"
	if err != nil {
		header = "Command Failed"
	}

	body := strings.TrimSpace(output)
	if body == "" && err != nil {
		body = err.Error()
	}
	if body == "" {
		body = "(no output)"
	}

	body = strings.ReplaceAll(body, "```", "` ` `")
	return fmt.Sprintf("%s: & %s\n```text\n%s\n```", header, command, body)
}

func sanitizeWorkspaceOutput(raw []byte) string {
	return tuiinfra.SanitizeWorkspaceOutput(raw)
}

func decodeWorkspaceOutput(raw []byte) string {
	return tuiinfra.DecodeWorkspaceOutput(raw)
}

func (a *App) refreshFileCandidates() error {
	candidates, err := collectWorkspaceFiles(a.state.CurrentWorkdir, maxWorkspaceFiles)
	if err != nil {
		return err
	}
	a.fileCandidates = candidates
	if absolute := tuiservices.ResolveWorkspaceDirectory(a.state.CurrentWorkdir); absolute != "" {
		a.fileBrowser.CurrentDirectory = absolute
	}
	a.refreshCommandMenu()
	return nil
}

func collectWorkspaceFiles(root string, limit int) ([]string, error) {
	return tuiservices.CollectWorkspaceFiles(root, limit)
}

func (a App) resolveFileReferenceSuggestions(input string) (start int, end int, query string, suggestions []string, ok bool) {
	start, end, token, ok := currentReferenceToken(input)
	if !ok {
		return 0, 0, "", nil, false
	}

	query = strings.ToLower(strings.TrimPrefix(token, fileReferencePrefix))
	suggestions = collectFileSuggestionMatches(query, a.fileCandidates, maxFileSuggestions)
	return start, end, query, suggestions, true
}

func collectFileSuggestionMatches(query string, candidates []string, limit int) []string {
	return tuiservices.SuggestFileMatches(query, candidates, limit)
}

func tokenRange(input string, selector tokenSelector) (start int, end int, token string, ok bool) {
	if strings.TrimSpace(input) == "" {
		return 0, 0, "", false
	}

	switch selector {
	case tokenSelectorFirst:
		start = 0
		for start < len(input) {
			switch input[start] {
			case ' ', '\t', '\r', '\n':
				start++
			default:
				goto parse
			}
		}
		return 0, 0, "", false
	case tokenSelectorLast:
		end = len(input)
		start = strings.LastIndexAny(input, " \t\r\n")
		if start < 0 {
			start = 0
		} else {
			start++
		}
		if start >= end {
			return 0, 0, "", false
		}
		token = input[start:end]
		return start, end, token, true
	default:
		return 0, 0, "", false
	}

parse:
	end = start
	for end < len(input) {
		switch input[end] {
		case ' ', '\t', '\r', '\n':
			token = input[start:end]
			return start, end, token, true
		default:
			end++
		}
	}
	token = input[start:end]
	return start, end, token, true
}

func currentReferenceToken(input string) (start int, end int, token string, ok bool) {
	start, end, token, ok = tokenRange(input, tokenSelectorLast)
	if !ok {
		return 0, 0, "", false
	}
	if !strings.HasPrefix(token, fileReferencePrefix) && !strings.HasPrefix(token, imageReferencePrefix) {
		return 0, 0, "", false
	}
	return start, end, token, true
}

func (a *App) applyImageReference(input string) error {
	path := extractImageReference(input)
	if path == "" {
		return fmt.Errorf("invalid image reference")
	}
	return a.addImageAttachment(path)
}

// absorbInlineImageReferences 会把输入文本中的 @<image-path> 令牌吸收到附件队列，并返回移除令牌后的文本。
// 仅根据令牌语法与扩展名做轻量识别，避免把文件系统硬校验放到 TUI 层。
func (a *App) absorbInlineImageReferences(input string) (string, int, error) {
	tokens := strings.Fields(input)
	if len(tokens) == 0 {
		return strings.TrimSpace(input), 0, nil
	}

	kept := make([]string, 0, len(tokens))
	absorbed := 0
	for _, token := range tokens {
		imagePath, ok := a.parseInlineImagePathToken(token)
		if !ok {
			kept = append(kept, token)
			continue
		}
		if err := a.queueImageAttachmentForPrepare(imagePath); err != nil {
			return "", absorbed, err
		}
		absorbed++
	}

	return strings.TrimSpace(strings.Join(kept, " ")), absorbed, nil
}

// parseInlineImagePathToken 识别 @<path> 形式的图片路径令牌，并映射为待发送路径。
func (a *App) parseInlineImagePathToken(token string) (string, bool) {
	trimmed := strings.TrimSpace(token)
	if !strings.HasPrefix(trimmed, fileReferencePrefix) || strings.HasPrefix(trimmed, imageReferencePrefix) {
		return "", false
	}

	path := strings.TrimPrefix(trimmed, fileReferencePrefix)
	path = strings.Trim(path, `"'`)
	path = strings.TrimSpace(path)
	if path == "" || !looksLikeImagePath(path) {
		return "", false
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		base := strings.TrimSpace(a.state.CurrentWorkdir)
		if base == "" {
			return "", false
		}
		resolved = filepath.Join(base, resolved)
	}
	return resolved, true
}

// queueImageAttachmentForPrepare 将图片路径排队为待发送附件，不在 TUI 层做文件系统和 MIME 硬校验。
// 真正的可用性校验与错误语义统一在 runtime/session 归一化阶段完成。
func (a *App) queueImageAttachmentForPrepare(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("image path is empty")
	}
	if len(a.pendingImageAttachments) >= maxImageAttachments {
		return fmt.Errorf("maximum %d image attachments allowed", maxImageAttachments)
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		base := strings.TrimSpace(a.state.CurrentWorkdir)
		if base != "" {
			resolved = filepath.Join(base, resolved)
		}
	}
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return fmt.Errorf("invalid image path: %w", err)
	}

	a.pendingImageAttachments = append(a.pendingImageAttachments, pendingImageAttachment{
		Path:     absPath,
		MimeType: "",
		Size:     0,
		Name:     filepath.Base(absPath),
	})
	a.refreshImageAttachmentDisplay()
	return nil
}

// looksLikeImagePath 使用扩展名快速判断路径是否是常见图片文件。
func looksLikeImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp":
		return true
	default:
		return false
	}
}

func (a *App) applyFileReference(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("file path is empty")
	}

	resolved := filepath.ToSlash(path)
	if workdir := strings.TrimSpace(a.state.CurrentWorkdir); workdir != "" {
		base, errBase := filepath.Abs(workdir)
		target, errTarget := filepath.Abs(path)
		if errBase == nil && errTarget == nil {
			if rel, errRel := filepath.Rel(base, target); errRel == nil && !strings.HasPrefix(rel, "..") {
				resolved = filepath.ToSlash(rel)
			} else {
				resolved = filepath.ToSlash(target)
			}
		}
	}
	resolved = strings.TrimPrefix(resolved, "./")
	reference := fileReferencePrefix + resolved

	current := a.input.Value()
	if start, end, _, ok := currentReferenceToken(current); ok {
		current = current[:start] + reference + current[end:]
	} else if strings.TrimSpace(current) == "" {
		current = reference
	} else {
		separator := " "
		if strings.HasSuffix(current, " ") || strings.HasSuffix(current, "\t") {
			separator = ""
		}
		current = current + separator + reference
	}

	a.input.SetValue(current)
	a.state.InputText = current
	a.normalizeComposerHeight()
	a.applyComponentLayout(false)
	a.refreshCommandMenu()
	a.state.StatusText = fmt.Sprintf("[System] Added file reference %s.", reference)
	return nil
}

func isImageReferenceInput(input string) bool {
	return strings.HasPrefix(strings.TrimSpace(input), imageReferencePrefix)
}

func extractImageReference(input string) string {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, imageReferencePrefix) {
		return ""
	}
	return strings.TrimPrefix(trimmed, imageReferencePrefix)
}

func (a *App) addImageAttachment(path string) error {
	if err := a.queueImageAttachmentForPrepare(path); err != nil {
		return err
	}
	if count := len(a.pendingImageAttachments); count > 0 {
		a.state.StatusText = fmt.Sprintf("[System] Added image: %s", a.pendingImageAttachments[count-1].Name)
	}
	return nil
}

func (a *App) removeImageAttachment(index int) error {
	if index < 0 || index >= len(a.pendingImageAttachments) {
		return fmt.Errorf("invalid attachment index")
	}

	removed := a.pendingImageAttachments[index]
	a.pendingImageAttachments = append(a.pendingImageAttachments[:index], a.pendingImageAttachments[index+1:]...)

	a.refreshImageAttachmentDisplay()
	a.state.StatusText = fmt.Sprintf("[System] Removed image: %s", removed.Name)
	return nil
}

func (a *App) clearImageAttachments() {
	a.pendingImageAttachments = nil
}

func (a *App) getImageAttachmentCount() int {
	return len(a.pendingImageAttachments)
}

func (a *App) refreshImageAttachmentDisplay() {
	a.normalizeComposerHeight()
	a.applyComponentLayout(false)
}

func (a *App) hasImageAttachments() bool {
	return len(a.pendingImageAttachments) > 0
}

func (a *App) getImageAttachments() []pendingImageAttachment {
	return a.pendingImageAttachments
}

func (a *App) loadImageAttachmentData(index int) ([]byte, error) {
	if index < 0 || index >= len(a.pendingImageAttachments) {
		return nil, fmt.Errorf("invalid attachment index")
	}
	return tuiinfra.ReadImageFile(a.pendingImageAttachments[index].Path)
}

func (a *App) addImageFromClipboard() error {
	if len(a.pendingImageAttachments) >= maxImageAttachments {
		return fmt.Errorf("maximum %d image attachments allowed", maxImageAttachments)
	}

	data, err := readClipboardImage()
	if err != nil {
		return fmt.Errorf("failed to read clipboard image: %w", err)
	}

	if data == nil || len(data) == 0 {
		return fmt.Errorf("no image in clipboard")
	}

	tmpPath, err := saveClipboardImageToTempFile(data, "paste")
	if err != nil {
		return fmt.Errorf("failed to save clipboard image: %w", err)
	}
	if err := a.queueImageAttachmentForPrepare(tmpPath); err != nil {
		return err
	}
	a.state.StatusText = "[System] Added image from clipboard"
	return nil
}
