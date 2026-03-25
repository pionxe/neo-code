package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type Message struct {
	Role      string
	Content   string
	Timestamp time.Time
	Streaming bool
}

type MessageList struct {
	Messages []Message
	Width    int
}

func (ml MessageList) Render() string {
	return ml.RenderLayout().Content
}

func (ml MessageList) RenderLayout() RenderedChatLayout {
	if len(ml.Messages) == 0 {
		return RenderedChatLayout{}
	}
	contentWidth := ml.Width - 4
	if contentWidth < 20 {
		contentWidth = ml.Width
	}

	userMsgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#98C379")).
		Bold(true)

	assistantMsgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5C07B"))

	systemMsgStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#C678DD"))

	var b strings.Builder
	regions := make([]ClickableRegion, 0)
	row := 0

	wrapStyle := lipgloss.NewStyle().MaxWidth(contentWidth)

	for i, msg := range ml.Messages {
		idx := i + 1
		switch msg.Role {
		case "user":
			line := userMsgStyle.Render(fmt.Sprintf("你 [%d]:", idx)) + " " + wrapStyle.Render(msg.Content)
			b.WriteString(line)
			b.WriteString("\n\n")
			row += strings.Count(line, "\n") + 2

		case "assistant":
			label := assistantMsgStyle.Render(fmt.Sprintf("Neo [%d]:", idx))
			b.WriteString(label)
			b.WriteString("\n")
			row++

			segments := ParseContentSegments(msg.Content)
			if len(segments) == 0 {
				segments = []ContentSegment{{Type: SegmentText, Text: "..."}}
			}
			blockIndex := 0
			for _, segment := range segments {
				switch segment.Type {
				case SegmentCodeBlock:
					blockIndex++
					headerRow := row
					codeLang := strings.TrimSpace(segment.Lang)
					if codeLang == "" {
						codeLang = DetectLanguage(segment.Code)
					}
					if codeLang == "" {
						codeLang = "text"
					}
					b.WriteString(RenderCodeBlock(segment, contentWidth, CopyActionLabel()))
					regions = append(regions, BuildCopyRegion(i, blockIndex, headerRow, segment.Code, codeLang))
					row += strings.Count(RenderCodeBlock(segment, contentWidth, CopyActionLabel()), "\n")
				case SegmentText:
					text := renderTextSegment(segment.Text, contentWidth)
					b.WriteString(text)
					row += strings.Count(text, "\n")
				}
			}

			b.WriteString("\n\n")
			row += 2

		case "system":
			line := systemMsgStyle.Render("[系统]") + " " + wrapStyle.Render(msg.Content)
			b.WriteString(line)
			b.WriteString("\n\n")
			row += strings.Count(line, "\n") + 2
		}
	}

	return RenderedChatLayout{Content: b.String(), Regions: regions}
}

func renderTextSegment(text string, width int) string {
	if text == "" {
		return ""
	}
	var b strings.Builder
	style := lipgloss.NewStyle().MaxWidth(width)
	for _, line := range strings.Split(text, "\n") {
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}
	return b.String()
}
