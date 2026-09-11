package components

import (
	"strings"
	"testing"
)

// TestPaletteCommandsSortedByCategory 验证 PaletteCommands 按 Category 优先级单调非递减排序。
func TestPaletteCommandsSortedByCategory(t *testing.T) {
	cmds := PaletteCommands()
	if len(cmds) != 15 {
		t.Fatalf("PaletteCommands len=%d, want 15", len(cmds))
	}
	for i := 1; i < len(cmds); i++ {
		if cmds[i].Category < cmds[i-1].Category {
			t.Fatalf("Category not sorted at %d: %d < %d", i, cmds[i].Category, cmds[i-1].Category)
		}
	}
}

// TestPaletteCommandsNoDuplicate 验证命令 Name 与 Action 无重复。
func TestPaletteCommandsNoDuplicate(t *testing.T) {
	cmds := PaletteCommands()
	seenName := map[string]bool{}
	seenAction := map[PaletteAction]bool{}
	for _, c := range cmds {
		if seenName[c.Name] {
			t.Fatalf("duplicate Name: %s", c.Name)
		}
		if seenAction[c.Action] {
			t.Fatalf("duplicate Action: %s", c.Action)
		}
		seenName[c.Name] = true
		seenAction[c.Action] = true
	}
}

// TestUnimplementedCommandMarked 验证未实现命令 Description 含 [未实现]。
func TestUnimplementedCommandMarked(t *testing.T) {
	cmds := PaletteCommands()
	for _, c := range cmds {
		if c.Action == PaletteActionCheckpoint || c.Action == PaletteActionSkills {
			if !strings.Contains(c.Description, "[未实现]") {
				t.Fatalf("unimplemented %s Description=%q should contain [未实现]", c.Action, c.Description)
			}
		}
	}
}

// TestPaletteCommandsReturnsCopy 验证 PaletteCommands 返回副本，修改不影响全局。
func TestPaletteCommandsReturnsCopy(t *testing.T) {
	cmds := PaletteCommands()
	cmds[0].Name = "MUTATED"
	again := PaletteCommands()
	if again[0].Name == "MUTATED" {
		t.Fatal("PaletteCommands should return a copy, not shared slice")
	}
}
