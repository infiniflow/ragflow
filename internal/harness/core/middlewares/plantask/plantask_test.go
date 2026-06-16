package plantask

import (
	"context"
	"encoding/json"
	"testing"
)

func TestManager_CreateAndGet(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	task, err := m.Create(ctx, "Test task", "A test task description")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.ID == "" {
		t.Error("expected non-empty ID")
	}
	if task.Title != "Test task" {
		t.Errorf("expected title 'Test task', got %s", task.Title)
	}
	if task.State != TaskPending {
		t.Errorf("expected state pending, got %s", task.State)
	}

	got, err := m.Get(task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != task.ID {
		t.Errorf("got different task")
	}
}

func TestManager_GetNotFound(t *testing.T) {
	m := NewManager()
	_, err := m.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	m.Create(ctx, "Task 1", "Desc 1")
	m.Create(ctx, "Task 2", "Desc 2")

	tasks, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestManager_ListByState(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	t1, _ := m.Create(ctx, "Pending", "")
	t2, _ := m.Create(ctx, "Running", "", WithInitialState(TaskRunning))

	pending, _ := m.ListByState(TaskPending)
	running, _ := m.ListByState(TaskRunning)

	if len(pending) != 1 || pending[0].ID != t1.ID {
		t.Error("ListByState(Pending) mismatch")
	}
	if len(running) != 1 || running[0].ID != t2.ID {
		t.Error("ListByState(Running) mismatch")
	}
}

func TestManager_Update(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	task, _ := m.Create(ctx, "Original", "")

	updated, err := m.Update(task.ID,
		WithTitle("Updated"),
		WithResult("done"),
		WithState(TaskCompleted),
	)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "Updated" {
		t.Errorf("title not updated: %s", updated.Title)
	}
	if updated.Result != "done" {
		t.Errorf("result not updated: %s", updated.Result)
	}
	if updated.State != TaskCompleted {
		t.Errorf("state not completed: %s", updated.State)
	}
}

func TestManager_SetResultAndError(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	task, _ := m.Create(ctx, "Task", "")

	if err := m.SetResult(task.ID, "success output"); err != nil {
		t.Fatalf("SetResult: %v", err)
	}
	got, _ := m.Get(task.ID)
	if got.State != TaskCompleted || got.Result != "success output" {
		t.Error("SetResult did not update correctly")
	}

	task2, _ := m.Create(ctx, "Fail task", "")
	if err := m.SetError(task2.ID, "something broke"); err != nil {
		t.Fatalf("SetError: %v", err)
	}
	got2, _ := m.Get(task2.ID)
	if got2.State != TaskFailed || got2.Error != "something broke" {
		t.Error("SetError did not update correctly")
	}
}

func TestManager_Delete(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	task, _ := m.Create(ctx, "To delete", "")

	if err := m.Delete(task.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get(task.ID); err == nil {
		t.Error("expected error after delete")
	}
}

func TestManager_DeleteNotFound(t *testing.T) {
	m := NewManager()
	err := m.Delete("ghost")
	if err == nil {
		t.Error("expected error deleting nonexistent task")
	}
}

func TestManager_Subtasks(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	parent, _ := m.Create(ctx, "Parent task", "")
	sub, err := m.AddSubtask(parent.ID, "Subtask", "A sub-task")
	if err != nil {
		t.Fatalf("AddSubtask: %v", err)
	}
	if sub.ParentID != parent.ID {
		t.Errorf("subtask ParentID = %s, want %s", sub.ParentID, parent.ID)
	}

	subs, err := m.GetSubtasks(parent.ID)
	if err != nil {
		t.Fatalf("GetSubtasks: %v", err)
	}
	if len(subs) != 1 || subs[0].ID != sub.ID {
		t.Error("GetSubtasks returned wrong data")
	}
}

func TestManager_Count(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	if m.Count() != 0 {
		t.Error("empty manager should have 0 count")
	}
	m.Create(ctx, "T1", "")
	m.Create(ctx, "T2", "")
	if m.Count() != 2 {
		t.Errorf("expected count 2, got %d", m.Count())
	}
}

func TestManager_Options(t *testing.T) {
	m := NewManager()
	ctx := context.Background()

	withDeps, _ := m.Create(ctx, "", "",
		WithDependencies("dep1", "dep2"),
	)
	if len(withDeps.Dependencies) != 2 {
		t.Error("WithDependencies not applied")
	}

	withMeta, _ := m.Create(ctx, "", "",
		WithTaskMetadata(map[string]any{"priority": "high"}),
	)
	if withMeta.Metadata["priority"] != "high" {
		t.Error("WithTaskMetadata not applied")
	}

	// WithParentID creates a subtask under the designated parent.
	p, _ := m.Create(ctx, "parent", "")
	withParent, err := m.Create(ctx, "", "",
		WithParentID(p.ID),
	)
	if err != nil {
		t.Fatalf("Create with ParentID: %v", err)
	}
	if withParent.ParentID != p.ID {
		t.Errorf("WithParentID: got %s, want %s", withParent.ParentID, p.ID)
	}
}

func TestToolCreateTask(t *testing.T) {
	tool := ToolCreateTask()
	if tool.Name() != "create_task" {
		t.Errorf("tool name = %s, want create_task", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description empty")
	}
}

func TestToolListTasks(t *testing.T) {
	tool := ToolListTasks()
	if tool.Name() != "list_tasks" {
		t.Errorf("tool name = %s, want list_tasks", tool.Name())
	}
}

func TestAllTools(t *testing.T) {
	tools := AllTools()
	if len(tools) != 5 {
		t.Errorf("AllTools returned %d, want 5", len(tools))
	}
	names := map[string]bool{}
	for _, t := range tools { names[t.Name()] = true }
	for _, name := range []string{"create_task", "list_tasks", "get_task", "update_task", "delete_task"} {
		if !names[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestTaskJSON(t *testing.T) {
	m := NewManager()
	ctx := context.Background()
	task, _ := m.Create(ctx, "JSON test", "serializable")

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var roundTrip Task
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundTrip.ID != task.ID || roundTrip.Title != task.Title {
		t.Error("round-trip mismatch")
	}
}

func TestInitManager(t *testing.T) {
	// Note: GetManagerFromContext requires a valid run context from within ReActAgent.
	// This test verifies Manager creation and basic operations only.
	m := NewManager()
	if m == nil {
		t.Fatal("nil manager")
	}
	task, err := m.Create(context.Background(), "test", "desc")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.ID == "" {
		t.Error("expected non-empty ID")
	}
	// GetManagerFromContext(nil) panics — that's expected without a proper run context
}
