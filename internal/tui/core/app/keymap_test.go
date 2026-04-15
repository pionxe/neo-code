package tui

import "testing"

func TestFullHelpIncludesPasteImage(t *testing.T) {
	keys := newKeyMap()
	help := keys.FullHelp()
	for _, row := range help {
		for _, binding := range row {
			if binding.Help().Key == keys.PasteImage.Help().Key {
				return
			}
		}
	}
	t.Fatalf("expected full help to include paste image binding")
}
