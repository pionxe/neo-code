package subagent

import (
	"slices"
	"testing"
	"time"

	agentsession "neo-code/internal/session"
)

func TestBuildTaskContextSliceIncludesExpectedSections(t *testing.T) {
	t.Parallel()

	now := time.Unix(1710000000, 0).UTC()
	task := agentsession.TodoItem{
		ID:           "task-main",
		Content:      "实现子代理任务调度",
		Status:       agentsession.TodoStatusInProgress,
		Priority:     4,
		Dependencies: []string{"dep-a", "dep-b"},
		Acceptance:   []string{"通过单元测试", "通过集成测试"},
		CreatedAt:    now,
	}
	todos := map[string]agentsession.TodoItem{
		"dep-a": {
			ID:        "dep-a",
			Content:   "补齐权限能力",
			Status:    agentsession.TodoStatusCompleted,
			Priority:  2,
			Artifacts: []string{"docs/security.md", "artifacts/security-report.json"},
			CreatedAt: now.Add(-2 * time.Hour),
		},
		"dep-b": {
			ID:        "dep-b",
			Content:   "补齐 TODO 模型",
			Status:    agentsession.TodoStatusCompleted,
			Priority:  3,
			Artifacts: []string{"docs/todo.md"},
			CreatedAt: now.Add(-time.Hour),
		},
		"peer": {
			ID:        "peer",
			Content:   "并行修复覆盖率",
			Status:    agentsession.TodoStatusInProgress,
			Priority:  5,
			CreatedAt: now.Add(-30 * time.Minute),
		},
	}

	slice := BuildTaskContextSlice(TaskContextSliceInput{
		Task:            task,
		Todos:           todos,
		ActivatedSkills: []string{"go-review", "go-review", "todo-planner"},
		RelatedFiles: []TaskContextFileSummary{
			{Path: "internal/subagent/scheduler.go", Summary: "调度主循环"},
		},
		MaxChars: 6000,
	})

	if slice.TaskID != "task-main" {
		t.Fatalf("TaskID = %q, want task-main", slice.TaskID)
	}
	if slice.Goal != "实现子代理任务调度" {
		t.Fatalf("Goal = %q, want 实现子代理任务调度", slice.Goal)
	}
	if len(slice.DependencyArtifacts) != 3 {
		t.Fatalf("DependencyArtifacts len = %d, want 3", len(slice.DependencyArtifacts))
	}
	if len(slice.ActivatedSkills) != 2 {
		t.Fatalf("ActivatedSkills len = %d, want 2", len(slice.ActivatedSkills))
	}
	if len(slice.TodoFragment) < 3 {
		t.Fatalf("TodoFragment len = %d, want >= 3", len(slice.TodoFragment))
	}
	if slice.TodoFragment[0].ID != "task-main" {
		t.Fatalf("TodoFragment[0].ID = %q, want task-main", slice.TodoFragment[0].ID)
	}
	if slice.Descriptor.TaskID != "task-main" {
		t.Fatalf("Descriptor.TaskID = %q, want task-main", slice.Descriptor.TaskID)
	}
	if !slices.Equal(slice.Descriptor.DependencyTaskIDs, []string{"dep-a", "dep-b"}) {
		t.Fatalf("Descriptor.DependencyTaskIDs = %v, want [dep-a dep-b]", slice.Descriptor.DependencyTaskIDs)
	}
	if rendered := slice.Render(); rendered == "" {
		t.Fatalf("Render() should not be empty")
	}
}

func TestBuildTaskContextSliceBudgetTrimming(t *testing.T) {
	t.Parallel()

	task := agentsession.TodoItem{
		ID:           "task-budget",
		Content:      "这是一个很长很长的任务目标，需要在预算内裁剪但保持关键信息",
		Status:       agentsession.TodoStatusInProgress,
		Priority:     3,
		Dependencies: []string{"dep"},
		Acceptance:   []string{"验收条件一", "验收条件二", "验收条件三"},
	}
	todos := map[string]agentsession.TodoItem{
		"dep": {
			ID:        "dep",
			Content:   "依赖任务",
			Status:    agentsession.TodoStatusCompleted,
			Priority:  1,
			Artifacts: []string{"very/long/path/that/consumes/context/budget.md"},
		},
		"extra": {
			ID:       "extra",
			Content:  "额外任务应在裁剪时优先被移除",
			Status:   agentsession.TodoStatusPending,
			Priority: 1,
		},
	}

	cases := []struct {
		name              string
		maxChars          int
		requireAcceptance bool
	}{
		{name: "tight budget", maxChars: 140, requireAcceptance: true},
		{name: "very tight budget", maxChars: 90, requireAcceptance: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			slice := BuildTaskContextSlice(TaskContextSliceInput{
				Task:            task,
				Todos:           todos,
				MaxChars:        tc.maxChars,
				MaxRelatedFiles: 4,
			})

			if !slice.Truncated {
				t.Fatalf("Truncated = false, want true")
			}
			if len([]rune(slice.Render())) > tc.maxChars {
				t.Fatalf("rendered chars exceed budget: got %d, budget %d", len([]rune(slice.Render())), tc.maxChars)
			}
			if slice.Goal == "" {
				t.Fatalf("Goal should remain non-empty after trimming")
			}
			if len(slice.TodoFragment) == 0 || slice.TodoFragment[0].ID != "task-budget" {
				t.Fatalf("first todo fragment should keep current task, got %+v", slice.TodoFragment)
			}
			if tc.requireAcceptance && len(slice.Acceptance) == 0 {
				t.Fatalf("Acceptance should keep at least one item")
			}
		})
	}
}

func TestRebuildTaskContextSliceFromDescriptor(t *testing.T) {
	t.Parallel()

	task := agentsession.TodoItem{
		ID:           "task-rebuild",
		Content:      "重建上下文",
		Status:       agentsession.TodoStatusInProgress,
		Dependencies: []string{"dep-a"},
		Acceptance:   []string{"保留核心上下文"},
	}
	todos := map[string]agentsession.TodoItem{
		"task-rebuild": task,
		"dep-a": {
			ID:        "dep-a",
			Content:   "先完成依赖",
			Status:    agentsession.TodoStatusCompleted,
			Artifacts: []string{"artifacts/dep-a.txt"},
		},
	}

	origin := BuildTaskContextSlice(TaskContextSliceInput{
		Task:            task,
		Todos:           todos,
		ActivatedSkills: []string{"skill-a"},
		RelatedFiles: []TaskContextFileSummary{
			{Path: "internal/subagent/context_slice.go", Summary: "上下文切片实现"},
		},
		MaxChars: 4000,
	})

	rebuilt, err := RebuildTaskContextSlice(TaskContextRebuildInput{
		Descriptor:      origin.Descriptor,
		Todos:           todos,
		ActivatedSkills: []string{"skill-a", "skill-b"},
		RelatedFiles: []TaskContextFileSummary{
			{Path: "internal/subagent/context_slice.go", Summary: "上下文切片实现"},
			{Path: "internal/ignored.go", Summary: "不应被带入"},
		},
		MaxChars: 4000,
	})
	if err != nil {
		t.Fatalf("RebuildTaskContextSlice() error = %v", err)
	}
	if rebuilt.TaskID != origin.TaskID {
		t.Fatalf("TaskID = %q, want %q", rebuilt.TaskID, origin.TaskID)
	}
	if !slices.Equal(rebuilt.Descriptor.DependencyTaskIDs, origin.Descriptor.DependencyTaskIDs) {
		t.Fatalf("dependency ids = %v, want %v", rebuilt.Descriptor.DependencyTaskIDs, origin.Descriptor.DependencyTaskIDs)
	}
	if !slices.Equal(rebuilt.Descriptor.ArtifactRefs, origin.Descriptor.ArtifactRefs) {
		t.Fatalf("artifact refs = %v, want %v", rebuilt.Descriptor.ArtifactRefs, origin.Descriptor.ArtifactRefs)
	}
	if !slices.Equal(rebuilt.ActivatedSkills, []string{"skill-a"}) {
		t.Fatalf("ActivatedSkills = %v, want [skill-a]", rebuilt.ActivatedSkills)
	}
	if len(rebuilt.RelatedFiles) == 0 {
		t.Fatalf("RelatedFiles should not be empty")
	}
	if rebuilt.RelatedFiles[0].Path != "internal/subagent/context_slice.go" {
		t.Fatalf("RelatedFiles[0].Path = %q, want internal/subagent/context_slice.go", rebuilt.RelatedFiles[0].Path)
	}
}

func TestBuildTaskContextSliceStableOrder(t *testing.T) {
	t.Parallel()

	now := time.Unix(1711000000, 0).UTC()
	task := agentsession.TodoItem{
		ID:       "order-task",
		Content:  "稳定排序验证",
		Status:   agentsession.TodoStatusInProgress,
		Priority: 2,
	}
	todos := map[string]agentsession.TodoItem{
		"a": {ID: "a", Content: "A", Status: agentsession.TodoStatusPending, Priority: 1, CreatedAt: now.Add(time.Minute)},
		"b": {ID: "b", Content: "B", Status: agentsession.TodoStatusInProgress, Priority: 1, CreatedAt: now.Add(2 * time.Minute)},
		"c": {ID: "c", Content: "C", Status: agentsession.TodoStatusInProgress, Priority: 3, CreatedAt: now.Add(3 * time.Minute)},
	}

	first := BuildTaskContextSlice(TaskContextSliceInput{
		Task:            task,
		Todos:           todos,
		ActivatedSkills: []string{"skill-b", "skill-a", "skill-b"},
		MaxChars:        4000,
	})
	second := BuildTaskContextSlice(TaskContextSliceInput{
		Task:            task,
		Todos:           todos,
		ActivatedSkills: []string{"skill-b", "skill-a", "skill-b"},
		MaxChars:        4000,
	})

	firstIDs := make([]string, 0, len(first.TodoFragment))
	for _, item := range first.TodoFragment {
		firstIDs = append(firstIDs, item.ID)
	}
	secondIDs := make([]string, 0, len(second.TodoFragment))
	for _, item := range second.TodoFragment {
		secondIDs = append(secondIDs, item.ID)
	}
	if !slices.Equal(firstIDs, secondIDs) {
		t.Fatalf("todo order unstable: first=%v second=%v", firstIDs, secondIDs)
	}
	if !slices.Equal(first.ActivatedSkills, []string{"skill-b", "skill-a"}) {
		t.Fatalf("ActivatedSkills = %v, want [skill-b skill-a]", first.ActivatedSkills)
	}
}

func TestRebuildTaskContextSliceErrors(t *testing.T) {
	t.Parallel()

	if _, err := RebuildTaskContextSlice(TaskContextRebuildInput{
		Descriptor: TaskContextDescriptor{},
		Todos:      map[string]agentsession.TodoItem{},
	}); err == nil {
		t.Fatalf("expected missing task id error")
	}

	if _, err := RebuildTaskContextSlice(TaskContextRebuildInput{
		Descriptor: TaskContextDescriptor{TaskID: "missing"},
		Todos:      map[string]agentsession.TodoItem{},
	}); err == nil {
		t.Fatalf("expected task not found error")
	}
}
