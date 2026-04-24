package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	agentruntime "neo-code/internal/tui/services"
)

func TestToggleFullAccessModeOpensPromptAndDisables(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})

	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	next := model.(App)
	if next.pendingFullAccessPrompt == nil {
		t.Fatalf("expected ctrl+f to open full access risk prompt")
	}
	if next.state.StatusText != statusFullAccessPrompt {
		t.Fatalf("expected full access prompt status, got %q", next.state.StatusText)
	}

	next.fullAccessModeEnabled = true
	next.pendingFullAccessPrompt = nil
	model, _ = next.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	disabled := model.(App)
	if disabled.fullAccessModeEnabled {
		t.Fatalf("expected ctrl+f to disable full access mode")
	}
	if disabled.pendingFullAccessPrompt != nil {
		t.Fatalf("expected disable path to close full access prompt")
	}
	if disabled.state.StatusText != statusFullAccessDisabled {
		t.Fatalf("expected full access disabled status, got %q", disabled.state.StatusText)
	}
}

func TestUpdatePendingFullAccessPromptInputEnableAutoApprovesPendingPermission(t *testing.T) {
	runtime := &permissionTestRuntime{}
	app := newPermissionTestApp(runtime)
	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 0}
	app.pendingPermission = &permissionPromptState{
		Request:  agentruntime.PermissionRequestPayload{RequestID: "perm-full-access"},
		Selected: 0,
	}

	cmd, handled := app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if !handled || cmd == nil {
		t.Fatalf("expected y shortcut to enable full access and submit permission")
	}
	if !app.fullAccessModeEnabled {
		t.Fatalf("expected full access mode enabled after approval")
	}
	if app.pendingFullAccessPrompt != nil {
		t.Fatalf("expected full access prompt to be cleared after approval")
	}

	msg := cmd()
	done, ok := msg.(permissionResolutionFinishedMsg)
	if !ok {
		t.Fatalf("expected permissionResolutionFinishedMsg, got %T", msg)
	}
	if done.RequestID != "perm-full-access" || done.Decision != string(agentruntime.DecisionAllowSession) {
		t.Fatalf("unexpected auto-approval payload: %+v", done)
	}
	if runtime.lastResolved.Decision != agentruntime.DecisionAllowSession {
		t.Fatalf("expected runtime resolve decision allow_session, got %+v", runtime.lastResolved)
	}
}

func TestRuntimePermissionRequestHandlerAutoApprovesWhenFullAccessEnabled(t *testing.T) {
	runtime := &permissionTestRuntime{}
	app := newPermissionTestApp(runtime)
	app.fullAccessModeEnabled = true

	event := agentruntime.RuntimeEvent{
		Type: agentruntime.EventPermissionRequested,
		Payload: agentruntime.PermissionRequestPayload{
			RequestID: "perm-auto",
			ToolName:  "bash",
			Target:    "git status",
		},
	}
	if dirty := runtimeEventPermissionRequestHandler(app, event); dirty {
		t.Fatalf("permission request should not mark transcript dirty")
	}
	if app.pendingPermission != nil {
		t.Fatalf("expected no pending permission prompt in full access mode")
	}
	if app.deferredEventCmd == nil {
		t.Fatalf("expected full access mode to schedule auto-approval command")
	}

	msg := app.deferredEventCmd()
	done, ok := msg.(permissionResolutionFinishedMsg)
	if !ok {
		t.Fatalf("expected permissionResolutionFinishedMsg, got %T", msg)
	}
	if done.RequestID != "perm-auto" || done.Decision != string(agentruntime.DecisionAllowSession) {
		t.Fatalf("unexpected auto-approval payload: %+v", done)
	}
	if runtime.lastResolved.RequestID != "perm-auto" || runtime.lastResolved.Decision != agentruntime.DecisionAllowSession {
		t.Fatalf("unexpected runtime resolve input: %+v", runtime.lastResolved)
	}
}
