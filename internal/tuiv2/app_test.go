package tuiv2

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

func TestAppWindowSizeUpdatesLayoutState(t *testing.T) {
	app := NewApp(StartupConfig{Backend: "fake", Scenario: "default", Debug: true}).(*App)

	updated, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	app = updated.(*App)
	if app.state.Layout.Width != 120 || app.state.Layout.Height != 30 {
		t.Fatalf("layout = %dx%d, want 120x30", app.state.Layout.Width, app.state.Layout.Height)
	}

	view := app.View()
	for _, want := range []string{"Layout size:120x30", "size:120x30"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
}

func TestAppViewShowsViewStatePlaceholder(t *testing.T) {
	app := NewApp(StartupConfig{Backend: "fake", Scenario: "default"}).(*App)
	view := app.View()

	for _, want := range []string{
		"NEOCODE",
		"○ idle",
		"fake",
		"ghost-console",
		"ViewState mode:input input:message cursor:0",
		"Gateway connected:false sessions:0",
		"Runtime phase:idle run:- tokens:0/0/0",
		"Layout size:0x0 inspector:false/0",
		"Stream entries:0 last:-",
		"› ",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in:\n%s", want, view)
		}
	}
}
