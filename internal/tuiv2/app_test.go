package tuiv2

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"neo-code/internal/tuiv2/fakegateway"
	"neo-code/internal/tuiv2/state"
)

func TestNewAppBuildsRootModel(t *testing.T) {
	model := NewApp(StartupConfig{Backend: "fake", Scenario: "default"})

	app, ok := model.(*App)
	if !ok {
		t.Fatalf("NewApp() = %T, want *App", model)
	}
	if app.client != nil {
		t.Fatal("client = non-nil, want nil when config has no client")
	}
	if app.state == nil {
		t.Fatal("state = nil")
	}
	if app.state.Runtime.Phase != state.RuntimePhaseIdle {
		t.Fatalf("runtime phase = %q, want %q", app.state.Runtime.Phase, state.RuntimePhaseIdle)
	}
	if app.ambientStatus == nil || app.agentStream == nil || app.commandPrompt == nil || app.softInspector == nil {
		t.Fatal("component placeholders must be initialized")
	}
}

func TestAppInitLoadsViewStateFromGatewayClient(t *testing.T) {
	client, err := fakegateway.NewFakeClient(fakegateway.ScenarioEmptySessions)
	if err != nil {
		t.Fatalf("NewFakeClient() error = %v", err)
	}
	app := NewApp(StartupConfig{
		Backend:  "fake",
		Scenario: fakegateway.ScenarioEmptySessions,
		Client:   client,
	}).(*App)

	cmd := app.Init()
	if cmd == nil {
		t.Fatal("Init() command = nil, want load command")
	}
	updated, _ := app.Update(cmd())
	app = updated.(*App)

	if !app.state.Gateway.Connected {
		t.Fatal("Gateway.Connected = false, want true")
	}
	if len(app.state.Gateway.Sessions) != 0 {
		t.Fatalf("sessions = %d, want 0", len(app.state.Gateway.Sessions))
	}
}

func TestAppWindowSizeBreakpoints(t *testing.T) {
	tests := []struct {
		name           string
		width          int
		showInspector  bool
		inspectorWidth int
	}{
		{name: "hidden below 80", width: 79, showInspector: false, inspectorWidth: 0},
		{name: "compressed 80 to 99", width: 80, showInspector: true, inspectorWidth: 80},
		{name: "wide at 100", width: 100, showInspector: true, inspectorWidth: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := NewApp(StartupConfig{Backend: "fake", Scenario: "default", Debug: true}).(*App)
			updated, cmd := app.Update(tea.WindowSizeMsg{Width: tt.width, Height: 30})
			app = updated.(*App)
			if cmd == nil {
				t.Fatal("WindowSizeMsg command = nil, want clear screen command")
			}

			if app.state.Layout.Width != tt.width || app.state.Layout.Height != 30 {
				t.Fatalf("layout = %dx%d, want %dx30", app.state.Layout.Width, app.state.Layout.Height, tt.width)
			}
			if app.state.Layout.ShowInspector != tt.showInspector {
				t.Fatalf("ShowInspector = %t, want %t", app.state.Layout.ShowInspector, tt.showInspector)
			}
			if app.state.Layout.InspectorWidth != tt.inspectorWidth {
				t.Fatalf("InspectorWidth = %d, want %d", app.state.Layout.InspectorWidth, tt.inspectorWidth)
			}
		})
	}
}

func TestAppViewFitsTerminalWidthAndHeight(t *testing.T) {
	app := NewApp(StartupConfig{Backend: "fake", Scenario: "default", Debug: true}).(*App)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	app = updated.(*App)
	app.state.Gateway.ActiveModel = "a-very-long-model-name-that-must-not-wrap"
	app.state.Stream = append(app.state.Stream, state.StreamEntry{
		ID:      "long",
		Type:    "message",
		Content: "this is a very long stream entry that must be truncated before the terminal wraps it",
	})

	lines := strings.Split(app.View(), "\n")
	if len(lines) != 10 {
		t.Fatalf("line count = %d, want 10", len(lines))
	}
	for index, line := range lines {
		if width := printableWidth(line); width != 39 {
			t.Fatalf("line %d width = %d, want 39: %q", index, width, line)
		}
	}
}

func TestAppViewShowsFocusOnlyLayout(t *testing.T) {
	app := NewApp(StartupConfig{Backend: "fake", Scenario: "default", Debug: true}).(*App)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app = updated.(*App)
	view := app.View()

	for _, want := range []string{
		"NEOCODE",
		"idle",
		"ghost-console",
		"Agent Stream",
		"Soft Inspector",
		"Command Prompt",
		"> _",
		"[debug] mode:input",
		"size:120x30",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
	if strings.Contains(view, "┌") || strings.Contains(view, "┐") || strings.Contains(view, "└") || strings.Contains(view, "┘") {
		t.Fatalf("View() contains box border rune:\n%s", view)
	}
}

func TestAppViewHidesInspectorBelow80Columns(t *testing.T) {
	app := NewApp(StartupConfig{Backend: "fake", Scenario: "default"}).(*App)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	app = updated.(*App)
	view := app.View()

	if strings.Contains(view, "Soft Inspector") {
		t.Fatalf("View() shows inspector below 80 columns:\n%s", view)
	}
}

func printableWidth(line string) int {
	return ansi.StringWidth(line)
}
