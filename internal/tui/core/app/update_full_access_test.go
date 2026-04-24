package tui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	agentruntime "neo-code/internal/tui/services"
)

func expectPermissionResolutionFinishedMsg(t *testing.T, cmd tea.Cmd) permissionResolutionFinishedMsg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected non-nil command")
	}

	msg := cmd()
	done, ok := msg.(permissionResolutionFinishedMsg)
	if !ok {
		t.Fatalf("expected permissionResolutionFinishedMsg, got %T", msg)
	}
	return done
}

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

func TestUpdateConsumesPendingFullAccessPromptAndBatchesResolveCmd(t *testing.T) {
	runtime := &permissionTestRuntime{}
	app := newPermissionTestApp(runtime)
	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 0}
	app.pendingPermission = &permissionPromptState{
		Request:  agentruntime.PermissionRequestPayload{RequestID: "perm-from-update"},
		Selected: 0,
	}

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := model.(App)
	if next.pendingFullAccessPrompt != nil {
		t.Fatalf("expected pending full access prompt to be cleared")
	}
	done := expectPermissionResolutionFinishedMsg(t, cmd)
	if done.RequestID != "perm-from-update" || done.Decision != string(agentruntime.DecisionAllowSession) {
		t.Fatalf("unexpected batched permission resolution message: %+v", done)
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

	done := expectPermissionResolutionFinishedMsg(t, cmd)
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
	if app.pendingAutoPermission == nil || app.pendingAutoPermission.Request.RequestID != "perm-auto" {
		t.Fatalf("expected full access mode to track pending auto-approval request")
	}
	if app.deferredEventCmd == nil {
		t.Fatalf("expected full access mode to schedule auto-approval command")
	}

	done := expectPermissionResolutionFinishedMsg(t, app.deferredEventCmd)
	if done.RequestID != "perm-auto" || done.Decision != string(agentruntime.DecisionAllowSession) {
		t.Fatalf("unexpected auto-approval payload: %+v", done)
	}
	if runtime.lastResolved.RequestID != "perm-auto" || runtime.lastResolved.Decision != agentruntime.DecisionAllowSession {
		t.Fatalf("unexpected runtime resolve input: %+v", runtime.lastResolved)
	}
}

func TestUpdatePermissionResolutionFinishedMessageRestoresPromptAfterFullAccessAutoApproveFailure(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	app.pendingAutoPermission = &autoPermissionApprovalState{
		Request: agentruntime.PermissionRequestPayload{
			RequestID: "perm-auto-fail",
			ToolName:  "bash",
			Target:    "rm -rf /tmp/x",
		},
	}
	app.fullAccessModeEnabled = true

	model, _ := app.Update(permissionResolutionFinishedMsg{
		RequestID: "perm-auto-fail",
		Decision:  string(agentruntime.DecisionAllowSession),
		Err:       errors.New("submit failed"),
	})
	next := model.(App)
	if next.pendingAutoPermission != nil {
		t.Fatalf("expected pending auto-approval state to be cleared after callback")
	}
	if next.pendingPermission == nil || next.pendingPermission.Request.RequestID != "perm-auto-fail" {
		t.Fatalf("expected failed auto-approval to restore manual permission prompt")
	}
	if next.pendingPermission.Submitting {
		t.Fatalf("expected restored permission prompt to be interactive")
	}
	if next.state.StatusText != statusPermissionRequired {
		t.Fatalf("expected permission required status after fallback, got %q", next.state.StatusText)
	}
	if next.state.ExecutionError == "" {
		t.Fatalf("expected execution error to be preserved for failed auto-approval")
	}
}

func TestUpdatePendingFullAccessPromptInputGuardAndNavigation(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	if cmd, handled := app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyEnter}); handled || cmd != nil {
		t.Fatalf("expected nil prompt state to return not handled")
	}

	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 0}
	cmd, handled := app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyDown})
	if !handled || cmd != nil {
		t.Fatalf("expected down key to be handled without cmd")
	}
	if app.pendingFullAccessPrompt.Selected != 1 {
		t.Fatalf("expected selection moved to 1, got %d", app.pendingFullAccessPrompt.Selected)
	}
	if app.state.StatusText != statusFullAccessPrompt {
		t.Fatalf("expected status full access prompt after navigation, got %q", app.state.StatusText)
	}

	cmd, handled = app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyUp})
	if !handled || cmd != nil {
		t.Fatalf("expected up key to be handled without cmd")
	}
	if app.pendingFullAccessPrompt.Selected != 0 {
		t.Fatalf("expected selection moved back to 0, got %d", app.pendingFullAccessPrompt.Selected)
	}
}

func TestUpdatePendingFullAccessPromptInputCancelPaths(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 1}

	cmd, handled := app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("expected enter on No option to cancel without cmd")
	}
	if app.pendingFullAccessPrompt != nil {
		t.Fatalf("expected prompt cleared after cancel")
	}
	if app.fullAccessModeEnabled {
		t.Fatalf("expected cancel to keep full access disabled")
	}
	if app.state.StatusText != statusFullAccessCanceled {
		t.Fatalf("expected canceled status, got %q", app.state.StatusText)
	}

	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 0}
	cmd, handled = app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled || cmd != nil {
		t.Fatalf("expected esc to cancel prompt")
	}
	if app.state.StatusText != statusFullAccessCanceled {
		t.Fatalf("expected canceled status from esc, got %q", app.state.StatusText)
	}

	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 0}
	cmd, handled = app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if !handled || cmd != nil {
		t.Fatalf("expected n shortcut to cancel prompt")
	}

	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 0}
	cmd, handled = app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !handled || cmd != nil {
		t.Fatalf("expected unknown key to be consumed without command")
	}
}

func TestUpdatePendingFullAccessPromptInputEnterEnablePath(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 0}

	cmd, handled := app.updatePendingFullAccessPromptInput(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command without pending permission request")
	}
	if !app.fullAccessModeEnabled {
		t.Fatalf("expected full access mode to be enabled")
	}
	if app.state.StatusText != statusFullAccessEnabled {
		t.Fatalf("expected enabled status, got %q", app.state.StatusText)
	}
}

func TestApplyFullAccessPromptSelectionWithSubmittingPermissionDoesNotResubmit(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	app.pendingFullAccessPrompt = &fullAccessPromptState{Selected: 0}
	app.pendingPermission = &permissionPromptState{
		Request:    agentruntime.PermissionRequestPayload{RequestID: "perm-submitting"},
		Selected:   0,
		Submitting: true,
	}

	cmd := app.applyFullAccessPromptSelection(true)
	if cmd != nil {
		t.Fatalf("expected no command when pending permission is already submitting")
	}
	if !app.fullAccessModeEnabled {
		t.Fatalf("expected full access mode enabled")
	}
}

func TestHandleAutoPermissionResolutionFinishedGuardsAndSuccess(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	if handled := app.handleAutoPermissionResolutionFinished(permissionResolutionFinishedMsg{RequestID: "none"}); handled {
		t.Fatalf("expected no handling without pending auto permission")
	}

	app.pendingAutoPermission = &autoPermissionApprovalState{
		Request: agentruntime.PermissionRequestPayload{RequestID: "perm-auto"},
	}
	if handled := app.handleAutoPermissionResolutionFinished(permissionResolutionFinishedMsg{RequestID: "perm-other"}); handled {
		t.Fatalf("expected mismatched request id to be ignored")
	}

	handled := app.handleAutoPermissionResolutionFinished(permissionResolutionFinishedMsg{
		RequestID: "perm-auto",
		Decision:  string(agentruntime.DecisionAllowSession),
	})
	if !handled {
		t.Fatalf("expected matching auto permission callback to be handled")
	}
	if app.pendingAutoPermission != nil {
		t.Fatalf("expected pending auto permission to be cleared")
	}
	if app.state.StatusText != statusPermissionSubmitted {
		t.Fatalf("expected submitted status after success, got %q", app.state.StatusText)
	}
}

func TestBeginAutoPermissionApprovalGuards(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	if started := app.beginAutoPermissionApproval(agentruntime.PermissionRequestPayload{RequestID: "perm"}); started {
		t.Fatalf("expected disabled full access mode to skip auto approval")
	}

	app.fullAccessModeEnabled = true
	if started := app.beginAutoPermissionApproval(agentruntime.PermissionRequestPayload{RequestID: "   "}); started {
		t.Fatalf("expected empty request id to skip auto approval")
	}
}

func TestRuntimeEventPermissionResolvedHandlerClearsPendingAutoPermission(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	app.pendingAutoPermission = &autoPermissionApprovalState{
		Request: agentruntime.PermissionRequestPayload{RequestID: "perm-auto-clear"},
	}
	event := agentruntime.RuntimeEvent{
		Type: agentruntime.EventPermissionResolved,
		Payload: agentruntime.PermissionResolvedPayload{
			RequestID: "perm-auto-clear",
		},
	}
	if dirty := runtimeEventPermissionResolvedHandler(app, event); dirty {
		t.Fatalf("permission resolved should not mark transcript dirty")
	}
	if app.pendingAutoPermission != nil {
		t.Fatalf("expected resolved event to clear pending auto permission")
	}
}

func TestRuntimeEventPermissionResolvedHandlerGuardPaths(t *testing.T) {
	app := newPermissionTestApp(&permissionTestRuntime{})
	if dirty := runtimeEventPermissionResolvedHandler(app, agentruntime.RuntimeEvent{
		Type:    agentruntime.EventPermissionResolved,
		Payload: "invalid",
	}); dirty {
		t.Fatalf("invalid payload should not mark transcript dirty")
	}

	app.pendingAutoPermission = &autoPermissionApprovalState{
		Request: agentruntime.PermissionRequestPayload{RequestID: "perm-auto-keep"},
	}
	runtimeEventPermissionResolvedHandler(app, agentruntime.RuntimeEvent{
		Type: agentruntime.EventPermissionResolved,
		Payload: agentruntime.PermissionResolvedPayload{
			RequestID: "other-id",
		},
	})
	if app.pendingAutoPermission == nil {
		t.Fatalf("mismatched request id should keep pending auto permission")
	}
}
