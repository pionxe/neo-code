package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type SegmentType int

const (
	SegmentText SegmentType = iota
	SegmentCodeBlock
)

const copyActionLabel = "[Copy]"

var codeBlockStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ABB2BF")).
	Background(lipgloss.Color("#282C34")).
	Padding(0, 1)

var codeBlockHeaderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#61AFEF")).
	Background(lipgloss.Color("#21252B")).
	Bold(true).
	Padding(0, 1)

type ContentSegment struct {
	Type   SegmentType
	Text   string
	Lang   string
	Code   string
	Closed bool
}

type CodeBlockRef struct {
	MessageIndex int
	BlockIndex   int
	Lang         string
	Code         string
}

type ClickableRegion struct {
	Kind      string
	StartRow  int
	EndRow    int
	StartCol  int
	EndCol    int
	CodeBlock CodeBlockRef
}

type RenderedChatLayout struct {
	Content string
	Regions []ClickableRegion
}

func RenderContent(content string, width int) string {
	return RenderSegments(ParseContentSegments(content), width)
}

func ParseContentSegments(content string) []ContentSegment {
	if content == "" {
		return nil
	}

	lines := strings.Split(content, "\n")
	segments := make([]ContentSegment, 0)
	textLines := make([]string, 0)
	inCodeBlock := false
	codeLang := ""
	codeLines := make([]string, 0)

	flushText := func() {
		if len(textLines) == 0 {
			return
		}
		segments = append(segments, ContentSegment{
			Type: SegmentText,
			Text: strings.Join(textLines, "\n"),
		})
		textLines = textLines[:0]
	}

	flushCode := func(closed bool) {
		segments = append(segments, ContentSegment{
			Type:   SegmentCodeBlock,
			Lang:   codeLang,
			Code:   strings.Join(codeLines, "\n"),
			Closed: closed,
		})
		codeLang = ""
		codeLines = codeLines[:0]
	}

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if isFenceLine(trimmedLine) {
			if !inCodeBlock {
				flushText()
				inCodeBlock = true
				codeLang = parseFenceLanguage(trimmedLine)
				codeLines = codeLines[:0]
			} else {
				inCodeBlock = false
				flushCode(true)
			}
			continue
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		textLines = append(textLines, line)
	}

	flushText()
	if inCodeBlock {
		flushCode(false)
	}

	return segments
}

func RenderSegments(segments []ContentSegment, width int) string {
	if len(segments) == 0 {
		return "..."
	}
	if width <= 0 {
		width = 80
	}

	var b strings.Builder
	textStyle := lipgloss.NewStyle().MaxWidth(width)

	for _, segment := range segments {
		switch segment.Type {
		case SegmentCodeBlock:
			b.WriteString(RenderCodeBlock(segment, width, ""))
		case SegmentText:
			lines := strings.Split(segment.Text, "\n")
			for _, line := range lines {
				b.WriteString(textStyle.Render(line))
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

func RenderCodeBlock(segment ContentSegment, width int, actionLabel string) string {
	var b strings.Builder
	resolvedLang := strings.TrimSpace(segment.Lang)
	if resolvedLang == "" {
		resolvedLang = DetectLanguage(segment.Code)
	}
	header, resolvedLang := renderCodeBlockHeader(resolvedLang, width, actionLabel)
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(HighlightCodeBlock(strings.Split(segment.Code, "\n"), resolvedLang, width, segment.Closed))
	return b.String()
}

func renderCodeBlockHeader(lang string, width int, actionLabel string) (string, string) {
	resolvedLang := strings.TrimSpace(lang)
	if width <= 0 {
		width = 80
	}
	parts := make([]string, 0, 2)
	if strings.TrimSpace(actionLabel) != "" {
		parts = append(parts, strings.TrimSpace(actionLabel))
	}
	if resolvedLang == "" {
		resolvedLang = "text"
	}
	parts = append(parts, resolvedLang)
	return codeBlockHeaderStyle.MaxWidth(width).Render(strings.Join(parts, " ")), resolvedLang
}

func BuildCopyRegion(messageIndex, blockIndex, row int, code string, lang string) ClickableRegion {
	return ClickableRegion{
		Kind:     "copy",
		StartRow: row,
		EndRow:   row,
		StartCol: 1,
		EndCol:   len(copyActionLabel),
		CodeBlock: CodeBlockRef{
			MessageIndex: messageIndex,
			BlockIndex:   blockIndex,
			Lang:         strings.TrimSpace(lang),
			Code:         code,
		},
	}
}

func CopyActionLabel() string {
	return copyActionLabel
}

func HighlightCodeBlock(lines []string, lang string, width int, closed bool) string {
	var b strings.Builder
	code := strings.Join(lines, "\n")
	resolvedLang := strings.TrimSpace(lang)
	if resolvedLang == "" {
		resolvedLang = DetectLanguage(code)
	}
	if resolvedLang == "" {
		resolvedLang = "text"
	}

	highlighted := HighlightCode(code, resolvedLang)
	b.WriteString("```")
	b.WriteString(resolvedLang)
	b.WriteString("\n")
	b.WriteString(highlighted)
	if !strings.HasSuffix(highlighted, "\n") {
		b.WriteString("\n")
	}
	if closed {
		b.WriteString("```\n")
	}

	blockStyle := codeBlockStyle.MaxWidth(width)
	return blockStyle.Render(b.String()) + "\n"
}

func FormatCopyNotice(ref CodeBlockRef) string {
	lang := strings.TrimSpace(ref.Lang)
	if lang == "" {
		lang = "text"
	}
	lineCount := 0
	trimmed := strings.TrimSuffix(ref.Code, "\n")
	if trimmed != "" {
		lineCount = strings.Count(trimmed, "\n") + 1
	}
	if lineCount == 0 && strings.TrimSpace(ref.Code) != "" {
		lineCount = 1
	}
	return fmt.Sprintf("Copied %s code block (%d lines)", lang, lineCount)
}

func isFenceLine(line string) bool {
	return strings.HasPrefix(line, "```")
}

func parseFenceLanguage(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(line, "```"))
}
