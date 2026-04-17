package subagent

import "testing"

func TestTaskValidateContextSliceTaskIDMismatch(t *testing.T) {
	t.Parallel()

	err := (Task{
		ID:   "task-a",
		Goal: "goal",
		ContextSlice: TaskContextSlice{
			TaskID: "task-b",
		},
	}).Validate()
	if err == nil {
		t.Fatalf("expected context slice task id mismatch error")
	}
}

func TestTaskValidateAllowsEmptyOrMatchedContextSliceTaskID(t *testing.T) {
	t.Parallel()

	cases := []Task{
		{ID: "task-a", Goal: "goal", ContextSlice: TaskContextSlice{}},
		{ID: "task-a", Goal: "goal", ContextSlice: TaskContextSlice{TaskID: "task-a"}},
		{ID: "task-a", Goal: "goal", ContextSlice: TaskContextSlice{TaskID: " TASK-A "}},
	}
	for _, task := range cases {
		task := task
		t.Run(task.ContextSlice.TaskID, func(t *testing.T) {
			t.Parallel()
			if err := task.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
