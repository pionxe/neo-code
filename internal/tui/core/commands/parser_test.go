package commands

import "testing"

func TestMatchSlashCommands(t *testing.T) {
	commands := []SlashCommand{
		{Usage: "/help", Description: "show help"},
		{Usage: "/provider", Description: "pick provider"},
		{Usage: "/model", Description: "pick model"},
	}

	got := MatchSlashCommands("/pro", "/", commands)
	if len(got) != 1 {
		t.Fatalf("expected one suggestion for /pro, got %d", len(got))
	}
	if got[0].Command.Usage != "/provider" || !got[0].Match {
		t.Fatalf("unexpected suggestion: %+v", got[0])
	}

	if complete := MatchSlashCommands("/help", "/", commands); complete != nil {
		t.Fatalf("expected nil suggestion when command is complete, got %+v", complete)
	}
}

func TestIsCompleteSlashCommand(t *testing.T) {
	commands := []SlashCommand{{Usage: "/help"}, {Usage: "/provider"}}
	if !IsCompleteSlashCommand("/help", commands) {
		t.Fatalf("expected /help to be complete")
	}
	if IsCompleteSlashCommand("/hel", commands) {
		t.Fatalf("expected /hel to be incomplete")
	}
}

func TestSplitFirstWord(t *testing.T) {
	first, rest := SplitFirstWord(" /cwd   ./tmp/project ")
	if first != "/cwd" || rest != "./tmp/project" {
		t.Fatalf("unexpected split result: first=%q rest=%q", first, rest)
	}

	first, rest = SplitFirstWord("   ")
	if first != "" || rest != "" {
		t.Fatalf("expected empty split for blank input, got first=%q rest=%q", first, rest)
	}
}
