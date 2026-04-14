package session

import (
	"strings"
	"testing"
	"time"
)

func TestSessionAddTodoFindTodoAndDeleteTodo(t *testing.T) {
	t.Parallel()

	session := New("Todo Helpers")
	if err := session.AddTodo(TodoItem{
		ID:           "todo-1",
		Content:      "  implement todos  ",
		Dependencies: []string{"todo-2", "todo-2", " "},
	}); err == nil || !strings.Contains(err.Error(), `unknown dependency "todo-2"`) {
		t.Fatalf("expected dependency validation error, got %v", err)
	}

	if err := session.AddTodo(TodoItem{
		ID:      "todo-2",
		Content: "write tests",
		Status:  TodoStatusCompleted,
	}); err != nil {
		t.Fatalf("add todo-2: %v", err)
	}
	if err := session.AddTodo(TodoItem{
		ID:           "todo-1",
		Content:      "  implement todos  ",
		Dependencies: []string{"todo-2", "todo-2", " "},
	}); err != nil {
		t.Fatalf("add todo-1: %v", err)
	}

	found, ok := session.FindTodo("todo-1")
	if !ok {
		t.Fatalf("expected to find todo-1")
	}
	if found.Content != "implement todos" {
		t.Fatalf("expected normalized content, got %q", found.Content)
	}
	if len(found.Dependencies) != 1 || found.Dependencies[0] != "todo-2" {
		t.Fatalf("expected normalized dependencies, got %+v", found.Dependencies)
	}

	if err := session.DeleteTodo("todo-2"); err == nil || !strings.Contains(err.Error(), `still required by todo-1`) {
		t.Fatalf("expected delete of depended-on todo to fail with dependent info, got %v", err)
	}
	if err := session.DeleteTodo("todo-1"); err != nil {
		t.Fatalf("delete todo-1: %v", err)
	}
	if err := session.DeleteTodo("todo-2"); err != nil {
		t.Fatalf("delete todo-2: %v", err)
	}
	if len(session.Todos) != 0 {
		t.Fatalf("expected empty todos after deletions, got %+v", session.Todos)
	}
}

func TestSessionUpdateTodoStatus(t *testing.T) {
	t.Parallel()

	session := New("Update Todo Status")
	createdAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	if err := session.AddTodo(TodoItem{
		ID:        "todo-1",
		Content:   "implement status update",
		Status:    TodoStatusPending,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("add todo: %v", err)
	}

	before := session.Todos[0].UpdatedAt
	if err := session.UpdateTodoStatus("todo-1", TodoStatusInProgress); err != nil {
		t.Fatalf("update todo status: %v", err)
	}

	got, ok := session.FindTodo("todo-1")
	if !ok {
		t.Fatalf("expected to find updated todo")
	}
	if got.Status != TodoStatusInProgress {
		t.Fatalf("expected status %q, got %q", TodoStatusInProgress, got.Status)
	}
	if !got.UpdatedAt.After(before) {
		t.Fatalf("expected updated_at to advance, got before=%v after=%v", before, got.UpdatedAt)
	}
}

func TestSessionUpdateTodoStatusRejectsUnknownTodoAndInvalidStatus(t *testing.T) {
	t.Parallel()

	session := New("Update Todo Status Errors")
	if err := session.AddTodo(TodoItem{ID: "todo-1", Content: "existing"}); err != nil {
		t.Fatalf("add todo: %v", err)
	}

	if err := session.UpdateTodoStatus("missing", TodoStatusCompleted); err == nil || !strings.Contains(err.Error(), `todo "missing" not found`) {
		t.Fatalf("expected unknown todo error, got %v", err)
	}
	if err := session.UpdateTodoStatus("todo-1", TodoStatus("paused")); err == nil || !strings.Contains(err.Error(), `invalid todo status`) {
		t.Fatalf("expected invalid status error, got %v", err)
	}
}

func TestSessionTodoHelpersRejectEmptyID(t *testing.T) {
	t.Parallel()

	session := New("Todo Empty ID")
	if _, ok := session.FindTodo("  "); ok {
		t.Fatalf("expected FindTodo to reject empty id")
	}
	if err := session.UpdateTodoStatus(" ", TodoStatusCompleted); err == nil || !strings.Contains(err.Error(), "todo id is empty") {
		t.Fatalf("expected update empty id error, got %v", err)
	}
	if err := session.DeleteTodo("\n\t "); err == nil || !strings.Contains(err.Error(), "todo id is empty") {
		t.Fatalf("expected delete empty id error, got %v", err)
	}
}

func TestSessionAddTodoRejectsDuplicateID(t *testing.T) {
	t.Parallel()

	session := New("Duplicate Todo")
	if err := session.AddTodo(TodoItem{ID: "todo-1", Content: "first"}); err != nil {
		t.Fatalf("add first todo: %v", err)
	}
	err := session.AddTodo(TodoItem{ID: "todo-1", Content: "second"})
	if err == nil || !strings.Contains(err.Error(), `duplicate todo id "todo-1"`) {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestNormalizeAndValidateTodosRejectsSelfDependencyAndInvalidStatus(t *testing.T) {
	t.Parallel()

	if _, err := normalizeAndValidateTodos([]TodoItem{
		{ID: "todo-1", Content: "self", Dependencies: []string{"todo-1"}},
	}); err == nil || !strings.Contains(err.Error(), `cannot depend on itself`) {
		t.Fatalf("expected self dependency error, got %v", err)
	}

	if _, err := normalizeAndValidateTodos([]TodoItem{
		{ID: "todo-1", Content: "bad status", Status: TodoStatus("paused")},
	}); err == nil || !strings.Contains(err.Error(), `invalid todo status`) {
		t.Fatalf("expected invalid status error, got %v", err)
	}
}

func TestFindTodoDependentsPreservesOrder(t *testing.T) {
	t.Parallel()

	got := findTodoDependents([]TodoItem{
		{ID: "todo-1", Dependencies: []string{"todo-9"}},
		{ID: "todo-2", Dependencies: []string{"other", "todo-9"}},
		{ID: "todo-3", Dependencies: []string{"other"}},
	}, "todo-9")

	if len(got) != 2 || got[0] != "todo-1" || got[1] != "todo-2" {
		t.Fatalf("expected ordered dependents [todo-1 todo-2], got %+v", got)
	}
}

func TestSessionDeleteTodoReportsAllDependentsAndNotFound(t *testing.T) {
	t.Parallel()

	session := New("Delete Todo Errors")
	for _, item := range []TodoItem{
		{ID: "todo-9", Content: "shared dependency"},
		{ID: "todo-1", Content: "first dependent", Dependencies: []string{"todo-9"}},
		{ID: "todo-2", Content: "second dependent", Dependencies: []string{"todo-9"}},
	} {
		if err := session.AddTodo(item); err != nil {
			t.Fatalf("add todo %q: %v", item.ID, err)
		}
	}

	if err := session.DeleteTodo("todo-9"); err == nil || !strings.Contains(err.Error(), `still required by todo-1, todo-2`) {
		t.Fatalf("expected dependent list error, got %v", err)
	}
	if err := session.DeleteTodo("missing"); err == nil || !strings.Contains(err.Error(), `todo "missing" not found`) {
		t.Fatalf("expected not found error, got %v", err)
	}
}
