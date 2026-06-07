package keymap

import "testing"

func TestMatchInputKey(t *testing.T) {
	tests := []struct {
		key    string
		action Action
	}{
		{"enter", ActionSend},
		{"shift+enter", ActionNewline},
		{"esc", ActionEscape},
		{"ctrl+c", ActionCtrlC},
		{"ctrl+p", ActionOpenPalette},
		{"a", ActionNone},
		{"j", ActionNone},
	}
	for _, tt := range tests {
		got := MatchInputKey(tt.key)
		if got != tt.action {
			t.Errorf("MatchInputKey(%q) = %v, want %v", tt.key, got, tt.action)
		}
	}
}

func TestMatchNormalKey(t *testing.T) {
	tests := []struct {
		key    string
		action Action
	}{
		{"i", ActionEnterInput},
		{"j", ActionScrollDown},
		{"k", ActionScrollUp},
		{"ctrl+d", ActionHalfPageDown},
		{"ctrl+u", ActionHalfPageUp},
		{"g", ActionScrollTop},
		{"G", ActionScrollBottom},
		{"/", ActionSearchForward},
		{"n", ActionSearchNext},
		{"N", ActionSearchPrev},
		{":", ActionExCommand},
		{"q", ActionQuit},
		{" ", ActionLeader},
		{"ctrl+c", ActionCtrlC},
		{"enter", ActionNone},
		{"a", ActionNone},
	}
	for _, tt := range tests {
		got := MatchNormalKey(tt.key)
		if got != tt.action {
			t.Errorf("MatchNormalKey(%q) = %v, want %v", tt.key, got, tt.action)
		}
	}
}

func TestMatchLeaderKey(t *testing.T) {
	tests := []struct {
		key    string
		action Action
	}{
		{"p", ActionLeaderPalette},
		{"n", ActionLeaderNewSession},
		{"s", ActionLeaderSwitchSession},
		{"h", ActionLeaderHelp},
		{"m", ActionLeaderToggleMode},
		{"f", ActionLeaderFullAccess},
		{"l", ActionLeaderLog},
		{"c", ActionLeaderCompact},
		{"q", ActionLeaderQuit},
		{"a", ActionNone},
		{"j", ActionNone},
	}
	for _, tt := range tests {
		got := MatchLeaderKey(tt.key)
		if got != tt.action {
			t.Errorf("MatchLeaderKey(%q) = %v, want %v", tt.key, got, tt.action)
		}
	}
}

func TestIsLeaderSuffix(t *testing.T) {
	if !IsLeaderSuffix("p") {
		t.Error("IsLeaderSuffix(\"p\") = false, want true")
	}
	if !IsLeaderSuffix("q") {
		t.Error("IsLeaderSuffix(\"q\") = false, want true")
	}
	if IsLeaderSuffix("a") {
		t.Error("IsLeaderSuffix(\"a\") = true, want false")
	}
}

func TestFullHelpContainsAllGroups(t *testing.T) {
	groups := FullHelp()
	if len(groups) < 4 {
		t.Errorf("FullHelp() returned %d groups, want at least 4", len(groups))
	}
	titles := make(map[string]bool)
	for _, g := range groups {
		titles[g.Title] = true
	}
	for _, want := range []string{"Input Mode", "Normal Mode (Esc)", "Leader (Space)", "Navigation"} {
		if !titles[want] {
			t.Errorf("FullHelp() missing group %q", want)
		}
	}
}
