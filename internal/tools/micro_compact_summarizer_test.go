package tools

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// stubMetadata 快速构建测试用 metadata map。
func stubMetadata(keyValue ...string) map[string]string {
	m := make(map[string]string, len(keyValue)/2)
	for i := 0; i+1 < len(keyValue); i += 2 {
		m[keyValue[i]] = keyValue[i+1]
	}
	return m
}

func TestBashSummarizer(t *testing.T) {
	t.Parallel()

	t.Run("normal_output", func(t *testing.T) {
		content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"
		meta := stubMetadata("workdir", "/home/user/project")
		got := bashSummarizer(content, meta, false)
		if !strings.Contains(got, "[exit=0]") {
			t.Fatalf("expected exit=0 in summary, got %q", got)
		}
		if !strings.Contains(got, "workdir=/home/user/project") {
			t.Fatalf("expected workdir in summary, got %q", got)
		}
		if !strings.Contains(got, "line8") {
			t.Fatalf("expected last line preserved, got %q", got)
		}
		if utf8.RuneCountInString(got) > 200 {
			t.Fatalf("summary exceeds 200 runes: %d", utf8.RuneCountInString(got))
		}
	})

	t.Run("error_output", func(t *testing.T) {
		content := "error: command not found"
		meta := stubMetadata("workdir", "/tmp")
		got := bashSummarizer(content, meta, true)
		if !strings.Contains(got, "[exit=non-zero]") {
			t.Fatalf("expected exit=non-zero in summary, got %q", got)
		}
	})

	t.Run("short_output", func(t *testing.T) {
		content := "ok"
		got := bashSummarizer(content, nil, false)
		if !strings.Contains(got, "ok") {
			t.Fatalf("expected content preserved for short output, got %q", got)
		}
	})

	t.Run("empty_content", func(t *testing.T) {
		got := bashSummarizer("", nil, false)
		if !strings.Contains(got, "[exit=0]") {
			t.Fatalf("expected summary even with empty content, got %q", got)
		}
	})
}

func TestReadFileSummarizer(t *testing.T) {
	t.Parallel()

	t.Run("normal_file", func(t *testing.T) {
		content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
		meta := stubMetadata("path", "/home/user/main.go")
		got := readFileSummarizer(content, meta, false)
		if !strings.Contains(got, "/home/user/main.go") {
			t.Fatalf("expected path in summary, got %q", got)
		}
		if !strings.Contains(got, "lines=") {
			t.Fatalf("expected lines count in summary, got %q", got)
		}
		if !strings.Contains(got, "first=package main") {
			t.Fatalf("expected first line in summary, got %q", got)
		}
		if utf8.RuneCountInString(got) > 200 {
			t.Fatalf("summary exceeds 200 runes: %d", utf8.RuneCountInString(got))
		}
	})

	t.Run("missing_path", func(t *testing.T) {
		got := readFileSummarizer("content", nil, false)
		if got != "" {
			t.Fatalf("expected empty string for missing path, got %q", got)
		}
	})
}

func TestWriteFileSummarizer(t *testing.T) {
	t.Parallel()

	t.Run("normal", func(t *testing.T) {
		meta := stubMetadata("path", "/home/user/test.go", "bytes", "1024")
		got := writeFileSummarizer("", meta, false)
		if !strings.Contains(got, "/home/user/test.go") {
			t.Fatalf("expected path in summary, got %q", got)
		}
		if !strings.Contains(got, "1024 bytes") {
			t.Fatalf("expected bytes in summary, got %q", got)
		}
	})

	t.Run("missing_path", func(t *testing.T) {
		got := writeFileSummarizer("", stubMetadata("bytes", "100"), false)
		if got != "" {
			t.Fatalf("expected empty for missing path, got %q", got)
		}
	})
}

func TestEditSummarizer(t *testing.T) {
	t.Parallel()

	t.Run("with_relative_path", func(t *testing.T) {
		meta := stubMetadata("relative_path", "src/main.go", "path", "/abs/src/main.go", "search_length", "50", "replacement_length", "60")
		got := editSummarizer("", meta, false)
		if !strings.Contains(got, "src/main.go") {
			t.Fatalf("expected relative_path preferred, got %q", got)
		}
		if !strings.Contains(got, "search=50") {
			t.Fatalf("expected search_length, got %q", got)
		}
	})

	t.Run("fallback_to_abs_path", func(t *testing.T) {
		meta := stubMetadata("path", "/abs/src/main.go", "search_length", "10", "replacement_length", "20")
		got := editSummarizer("", meta, false)
		if !strings.Contains(got, "/abs/src/main.go") {
			t.Fatalf("expected abs path fallback, got %q", got)
		}
	})

	t.Run("missing_path", func(t *testing.T) {
		got := editSummarizer("", stubMetadata("search_length", "10"), false)
		if got != "" {
			t.Fatalf("expected empty for missing path, got %q", got)
		}
	})
}

func TestGrepSummarizer(t *testing.T) {
	t.Parallel()

	t.Run("with_matches", func(t *testing.T) {
		content := "src/a.go:10:match1\nsrc/b.go:20:match2\nsrc/c.go:30:match3\nsrc/d.go:40:match4"
		meta := stubMetadata("root", "/home/user", "matched_files", "4", "matched_lines", "4")
		got := grepSummarizer(content, meta, false)
		if !strings.Contains(got, "root=/home/user") {
			t.Fatalf("expected root in summary, got %q", got)
		}
		if !strings.Contains(got, "files=4") {
			t.Fatalf("expected files count, got %q", got)
		}
		if utf8.RuneCountInString(got) > 200 {
			t.Fatalf("summary exceeds 200 runes: %d", utf8.RuneCountInString(got))
		}
	})

	t.Run("empty_content", func(t *testing.T) {
		meta := stubMetadata("root", "/home", "matched_files", "0", "matched_lines", "0")
		got := grepSummarizer("", meta, false)
		if !strings.Contains(got, "files=0") {
			t.Fatalf("expected files count, got %q", got)
		}
	})
}

func TestGlobSummarizer(t *testing.T) {
	t.Parallel()

	t.Run("with_files", func(t *testing.T) {
		content := "src/a.go\nsrc/b.go\nsrc/c.go\nsrc/d.go"
		meta := stubMetadata("count", "4")
		got := globSummarizer(content, meta, false)
		if !strings.Contains(got, "4 files") {
			t.Fatalf("expected file count, got %q", got)
		}
		if utf8.RuneCountInString(got) > 200 {
			t.Fatalf("summary exceeds 200 runes: %d", utf8.RuneCountInString(got))
		}
	})

	t.Run("no_matches", func(t *testing.T) {
		meta := stubMetadata("count", "0")
		got := globSummarizer("", meta, false)
		if !strings.Contains(got, "0 files") {
			t.Fatalf("expected 0 files, got %q", got)
		}
	})
}

func TestWebfetchSummarizer(t *testing.T) {
	t.Parallel()

	t.Run("with_url", func(t *testing.T) {
		meta := stubMetadata("url", "https://example.com/api", "truncated", "true")
		got := webfetchSummarizer("", meta, false)
		if !strings.Contains(got, "https://example.com/api") {
			t.Fatalf("expected url in summary, got %q", got)
		}
		if !strings.Contains(got, "truncated=true") {
			t.Fatalf("expected truncated flag, got %q", got)
		}
	})

	t.Run("minimal", func(t *testing.T) {
		got := webfetchSummarizer("", nil, false)
		if !strings.Contains(got, "[summary] webfetch") {
			t.Fatalf("expected minimal summary, got %q", got)
		}
	})
}

func TestRegisterBuiltinSummarizers(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	RegisterBuiltinSummarizers(registry)

	toolNames := []string{
		ToolNameBash, ToolNameFilesystemReadFile, ToolNameFilesystemWriteFile,
		ToolNameFilesystemEdit, ToolNameFilesystemGrep, ToolNameFilesystemGlob,
		ToolNameWebFetch,
	}
	for _, name := range toolNames {
		if registry.MicroCompactSummarizer(name) == nil {
			t.Errorf("expected summarizer for %q to be registered", name)
		}
	}

	// 不在注册列表中的工具应返回 nil
	if registry.MicroCompactSummarizer("unknown_tool") != nil {
		t.Fatal("expected nil for unknown tool")
	}
}

func TestRegisterSummarizer(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()

	// 注册
	called := false
	registry.RegisterSummarizer("test_tool", func(content string, metadata map[string]string, isError bool) string {
		called = true
		return "summary"
	})

	s := registry.MicroCompactSummarizer("test_tool")
	if s == nil {
		t.Fatal("expected summarizer to be registered")
	}
	result := s("content", nil, false)
	if !called {
		t.Fatal("expected summarizer to be called")
	}
	if result != "summary" {
		t.Fatalf("expected 'summary', got %q", result)
	}

	// 移除
	registry.RegisterSummarizer("test_tool", nil)
	if registry.MicroCompactSummarizer("test_tool") != nil {
		t.Fatal("expected nil after removal")
	}
}

func TestTruncateRunes(t *testing.T) {
	t.Parallel()

	t.Run("short", func(t *testing.T) {
		got := truncateRunes("hello", 10)
		if got != "hello" {
			t.Fatalf("expected unchanged, got %q", got)
		}
	})

	t.Run("exact", func(t *testing.T) {
		got := truncateRunes("hello", 5)
		if got != "hello" {
			t.Fatalf("expected unchanged, got %q", got)
		}
	})

	t.Run("truncated", func(t *testing.T) {
		got := truncateRunes("hello world", 5)
		if got != "hello..." {
			t.Fatalf("expected 'hello...', got %q", got)
		}
	})

	t.Run("chinese", func(t *testing.T) {
		got := truncateRunes("你好世界测试", 3)
		if got != "你好世..." {
			t.Fatalf("expected '你好世...', got %q", got)
		}
	})
}
