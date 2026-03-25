package components

import (
	"strings"
	"testing"
)

func TestFenceHelpers(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
		lang string
	}{
		{name: "plain fence", line: "```", want: true, lang: ""},
		{name: "fence with language", line: "```python", want: true, lang: "python"},
		{name: "indented fence", line: "   ```go   ", want: true, lang: "go"},
		{name: "non fence", line: "```go fmt.Println()", want: true, lang: "go fmt.Println()"},
		{name: "plain text", line: "hello", want: false, lang: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trimmed := strings.TrimSpace(tt.line)
			if got := isFenceLine(trimmed); got != tt.want {
				t.Fatalf("expected fence=%v, got %v", tt.want, got)
			}
			if tt.want {
				if got := parseFenceLanguage(trimmed); got != tt.lang {
					t.Fatalf("expected lang %q, got %q", tt.lang, got)
				}
			}
		})
	}
}

func TestRenderContentRendersClosedCodeBlock(t *testing.T) {
	content := "before\n```python\ndef bubble_sort(arr):\n    for i in range(len(arr)):\n        pass\n```\nafter"
	rendered := RenderContent(content, 80)

	for _, want := range []string{"before", "```python", "def bubble_sort(arr):", "for i in range(len(arr)):", "```", "after"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected rendered content to contain %q, got %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "def bubble_sort(arr):    for i in range(len(arr)):") {
		t.Fatalf("expected code lines not to collapse into one line, got %q", rendered)
	}
}

func TestRenderContentRendersUnclosedCodeBlockWhileStreaming(t *testing.T) {
	content := "before\n```python\ndef bubble_sort(arr):\n    return arr"
	rendered := RenderContent(content, 80)

	for _, want := range []string{"before", "```python", "def bubble_sort(arr):", "return arr"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected rendered content to contain %q, got %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "def bubble_sort(arr):    return arr") {
		t.Fatalf("expected streaming code lines not to collapse into one line, got %q", rendered)
	}
}

func TestRenderContentDetectsLanguageWhenFenceDoesNotSpecifyOne(t *testing.T) {
	content := "```\ndef greet():\n    return 1\n```"
	rendered := RenderContent(content, 80)

	if !strings.Contains(rendered, "```python") {
		t.Fatalf("expected detected language header, got %q", rendered)
	}
	if !strings.Contains(rendered, "def greet():") {
		t.Fatalf("expected code content, got %q", rendered)
	}
}

func TestRenderContentSupportsIndentedFence(t *testing.T) {
	content := "  ```bash  \necho hi\n  ```"
	rendered := RenderContent(content, 80)

	for _, want := range []string{"```bash", "echo hi"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected rendered content to contain %q, got %q", want, rendered)
		}
	}
}

func TestParseContentSegmentsExtractsTextAndCodeBlocks(t *testing.T) {
	segments := ParseContentSegments("before\n```go\nfmt.Println(1)\n```\nafter")
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segments))
	}
	if segments[0].Type != SegmentText || segments[0].Text != "before" {
		t.Fatalf("unexpected first segment: %+v", segments[0])
	}
	if segments[1].Type != SegmentCodeBlock || segments[1].Lang != "go" || segments[1].Code != "fmt.Println(1)" || !segments[1].Closed {
		t.Fatalf("unexpected code segment: %+v", segments[1])
	}
	if segments[2].Type != SegmentText || segments[2].Text != "after" {
		t.Fatalf("unexpected last segment: %+v", segments[2])
	}
}

func TestRenderCodeBlockIncludesCopyHeader(t *testing.T) {
	rendered := RenderCodeBlock(ContentSegment{Type: SegmentCodeBlock, Lang: "go", Code: "fmt.Println(1)", Closed: true}, 80, CopyActionLabel())
	if !strings.Contains(rendered, "[Copy] go") {
		t.Fatalf("expected copy header, got %q", rendered)
	}
}

func TestHighlightCodePreservesNewlines(t *testing.T) {
	code := "def bubble_sort(arr):\n    for i in range(len(arr)):\n        pass"
	highlighted := HighlightCode(code, "python")

	if highlighted != code {
		t.Fatalf("expected highlighted code to preserve line structure, got %q", highlighted)
	}
}

func TestHighlightCodeBlockIncludesClosingFenceOnlyWhenClosed(t *testing.T) {
	closed := HighlightCodeBlock([]string{"print(1)"}, "python", 80, true)
	if strings.Count(closed, "```") != 2 {
		t.Fatalf("expected closed block to contain closing fence, got %q", closed)
	}

	unclosed := HighlightCodeBlock([]string{"print(1)"}, "python", 80, false)
	if strings.Count(unclosed, "```") != 1 {
		t.Fatalf("expected unclosed block to contain only opening fence, got %q", unclosed)
	}
}

func TestFormatCopyNoticeUsesEnglishSummary(t *testing.T) {
	notice := FormatCopyNotice(CodeBlockRef{
		Lang: "go",
		Code: "fmt.Println(1)\nfmt.Println(2)\n",
	})

	if notice != "Copied go code block (2 lines)" {
		t.Fatalf("unexpected copy notice %q", notice)
	}
}
