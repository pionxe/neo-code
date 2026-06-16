package theme

import "testing"

// 覆盖所有样式工厂函数与剩余符号/工具分支。
func TestStyleFactoriesRenderNonEmpty(t *testing.T) {
	styles := []func() string{
		func() string { return BaseStyle().Render("x") },
		func() string { return AccentStyle().Render("x") },
		func() string { return SuccessStyle().Render("x") },
		func() string { return WarningStyle().Render("x") },
		func() string { return ErrorStyle().Render("x") },
		func() string { return MutedStyle().Render("x") },
		func() string { return SubtleStyle().Render("x") },
		func() string { return ToolNameStyle().Render("x") },
		func() string { return FilePathStyle().Render("x") },
		func() string { return CodeBlockStyle().Render("x") },
		func() string { return InfoStyle().Render("x") },
		func() string { return TimestampStyle().Render("x") },
	}
	for i, s := range styles {
		if s() == "" {
			t.Fatalf("style %d rendered empty", i)
		}
	}
}

func TestPadRightAndTruncateBranches(t *testing.T) {
	if PadRight("ab", 5) != "ab   " {
		t.Fatal("PadRight should pad")
	}
	if PadRight("abcdef", 3) != "abcdef" {
		t.Fatal("PadRight should not truncate")
	}
	if Truncate("abc", 0) != "" {
		t.Fatal("Truncate max<=0 should return empty")
	}
	if Separator() == "" {
		t.Fatal("Separator empty")
	}
}

func TestStatusSymbolAllPhases(t *testing.T) {
	for _, phase := range []string{
		PhaseRunning,
		PhaseWaiting,
		PhaseWaitingPermission,
		PhaseWaitingUser,
		PhaseError,
		PhaseCancelled,
		PhaseIdle,
		"unknown", // default 分支
	} {
		if StatusSymbol(phase) == "" {
			t.Fatalf("StatusSymbol(%q) empty", phase)
		}
	}
}

func TestStreamPrefixAllBranches(t *testing.T) {
	cases := map[string]struct{}{
		"tool_end":             {},
		"tool_finished":        {},
		"run_finished":         {},
		"run_cancelled":        {},
		"tool_start":           {},
		"agent_chunk":          {},
		"run_started":          {},
		"permission_requested": {},
		"error":                {},
		"gateway_offline":      {},
		"message":              {},
		"unknown_type":         {}, // default 分支
	}
	for entryType := range cases {
		if StreamPrefix(entryType) == "" {
			t.Fatalf("StreamPrefix(%q) empty", entryType)
		}
	}
}

func TestSymbolsSetsAndDetection(t *testing.T) {
	// 真实环境 Symbols() 不为空
	set := Symbols()
	if set.AccentBar == "" {
		t.Fatal("Symbols() returned empty AccentBar")
	}
	// 两个符号集合字段都可用
	if UnicodeSymbols.Success == "" || ASCIISymbols.Success == "" {
		t.Fatal("symbol sets empty")
	}
	// TERM=linux 也应降级 ASCII
	if !DetectASCIISymbolsFromEnv(func(key string) string {
		if key == "TERM" {
			return "linux"
		}
		return ""
	}) {
		t.Fatal("TERM=linux should force ASCII")
	}
	// 普通终端不降级
	if DetectASCIISymbolsFromEnv(func(key string) string {
		if key == "TERM" {
			return "xterm-256color"
		}
		return ""
	}) {
		t.Fatal("xterm should not force ASCII")
	}
}

func TestDisplayWidthEdgeCases(t *testing.T) {
	if DisplayWidth("") != 0 {
		t.Fatal("empty width should be 0")
	}
	if DisplayWidth("abc") != 3 {
		t.Fatal("ascii width wrong")
	}
}
