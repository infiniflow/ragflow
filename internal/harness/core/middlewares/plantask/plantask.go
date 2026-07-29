// Package plantask provides a task management middleware for core.
// It allows agents to create, list, update, and manage tasks during execution,
// with task state persisted in the run session.
//
// TODO: This package is placed under middlewares/ but does not implement
// TypedReActMiddleware[M]. It is a tool library rather than a middleware.
// Consider moving to agentcore/tools/ or integrating its TaskManager with
// prebuilt/deep's TaskManager to eliminate duplication.
package plantask

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"ragflow/internal/harness/core"
)

// TaskState represents the lifecycle state of a task.
type TaskState string

const (
	TaskPending   TaskState = "pending"
	TaskRunning   TaskState = "running"
	TaskCompleted TaskState = "completed"
	TaskFailed    TaskState = "failed"
	TaskCancelled TaskState = "cancelled"
)

// Task represents a unit of work managed by plantask.
type Task struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	State        TaskState      `json:"state"`
	Result       string         `json:"result,omitempty"`
	Error        string         `json:"error,omitempty"`
	ParentID     string         `json:"parent_id,omitempty"`
	Dependencies []string       `json:"dependencies,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Manager manages tasks for an agent run session.
type Manager struct {
	mu     sync.RWMutex
	tasks  map[string]*taskInternal
	nextID int64
}

type taskInternal struct {
	*Task
	subtasks map[string]*taskInternal
}

// NewManager creates a new task Manager.
func NewManager() *Manager {
	return &Manager{
		tasks: make(map[string]*taskInternal),
	}
}

// Create creates a new task. When ParentID is set, the task is also registered
// as a subtask of the parent within the same lock (TOCTOU-safe).
func (m *Manager) Create(ctx context.Context, title, desc string, opts ...CreateOption) (*Task, error) {
	cfg := &createConfig{State: TaskPending}
	for _, o := range opts {
		o(cfg)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg.ParentID != "" {
		if _, ok := m.tasks[cfg.ParentID]; !ok {
			return nil, fmt.Errorf("parent task '%s' not found", cfg.ParentID)
		}
	}

	m.nextID++
	id := fmt.Sprintf("task_%d", m.nextID)
	t := &Task{
		ID: id, Title: title, Description: desc,
		State: cfg.State, Dependencies: cfg.Dependencies,
		ParentID: cfg.ParentID, Metadata: cfg.Metadata,
	}
	ti := &taskInternal{Task: t, subtasks: make(map[string]*taskInternal)}
	m.tasks[id] = ti
	if cfg.ParentID != "" {
		m.tasks[cfg.ParentID].subtasks[t.ID] = ti
	}
	return t, nil
}

// Get retrieves a task by ID.
func (m *Manager) Get(id string) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task '%s' not found", id)
	}
	return t.Task, nil
}

// List returns all top-level tasks (tasks without a parent).
func (m *Manager) List() ([]*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*Task
	for _, t := range m.tasks {
		if t.ParentID == "" {
			result = append(result, t.Task)
		}
	}
	return result, nil
}

// ListByState returns tasks filtered by state.
func (m *Manager) ListByState(state TaskState) ([]*Task, error) {
	all, err := m.List()
	if err != nil {
		return nil, err
	}
	var result []*Task
	for _, t := range all {
		if t.State == state {
			result = append(result, t)
		}
	}
	return result, nil
}

// Update modifies task fields.
func (m *Manager) Update(id string, opts ...UpdateOption) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task '%s' not found", id)
	}
	for _, o := range opts {
		o(t.Task)
	}
	return t.Task, nil
}

// SetState transitions a task to a new state.
func (m *Manager) SetState(id string, state TaskState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("task '%s' not found", id)
	}
	t.State = state
	return nil
}

// SetResult marks a task as completed with a result.
func (m *Manager) SetResult(id, result string) error {
	_, err := m.Update(id, WithResult(result), WithState(TaskCompleted))
	return err
}

// SetError marks a task as failed with an error message.
func (m *Manager) SetError(id, errMsg string) error {
	_, err := m.Update(id, WithError(errMsg), WithState(TaskFailed))
	return err
}

// Delete removes a task.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tasks[id]; !ok {
		return fmt.Errorf("task '%s' not found", id)
	}
	delete(m.tasks, id)
	return nil
}

// AddSubtask adds a child task to a parent task.
// Parent-child registration happens inside Create's lock (TOCTOU-safe).
func (m *Manager) AddSubtask(parentID, title, desc string) (*Task, error) {
	return m.Create(context.Background(), title, desc, WithParentID(parentID))
}

// GetSubtasks returns all subtasks of a parent task.
func (m *Manager) GetSubtasks(parentID string) ([]*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pt, ok := m.tasks[parentID]
	if !ok {
		return nil, fmt.Errorf("task '%s' not found", parentID)
	}
	var subs []*Task
	for _, st := range pt.subtasks {
		subs = append(subs, st.Task)
	}
	return subs, nil
}

// Count returns the total number of tasks.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tasks)
}

// ---- Options ----

type createConfig struct {
	State        TaskState
	Dependencies []string
	ParentID     string
	Metadata     map[string]any
}

type CreateOption func(*createConfig)

func WithInitialState(s TaskState) CreateOption {
	return func(c *createConfig) { c.State = s }
}
func WithDependencies(deps ...string) CreateOption {
	return func(c *createConfig) { c.Dependencies = deps }
}
func WithParentID(id string) CreateOption {
	return func(c *createConfig) { c.ParentID = id }
}
func WithTaskMetadata(md map[string]any) CreateOption {
	return func(c *createConfig) { c.Metadata = md }
}

type UpdateOption func(*Task)

func WithTitle(t string) UpdateOption             { return func(task *Task) { task.Title = t } }
func WithDescription(d string) UpdateOption       { return func(task *Task) { task.Description = d } }
func WithState(s TaskState) UpdateOption          { return func(task *Task) { task.State = s } }
func WithResult(r string) UpdateOption            { return func(task *Task) { task.Result = r } }
func WithError(e string) UpdateOption             { return func(task *Task) { task.Error = e } }
func WithMetadata(md map[string]any) UpdateOption { return func(task *Task) { task.Metadata = md } }

// ---- Tools ----

// GetManagerFromContext retrieves the plantask.Manager from the run session.
// Returns nil if not found or if called outside agent execution.
func GetManagerFromContext(ctx context.Context) *Manager {
	val, ok, _ := core.GetRunLocalValue(ctx, plantaskSessionKey)
	if !ok {
		return nil
	}
	if m, ok := val.(*Manager); ok {
		return m
	}
	return nil
}

const plantaskSessionKey = "_plantask_manager"

// InitManager creates a plantask Manager and stores it in the run session.
// Call this in BeforeAgent middleware before tools that need task management are used.
func InitManager(ctx context.Context) (*Manager, error) {
	m := NewManager()
	if err := core.SetRunLocalValue(ctx, plantaskSessionKey, m); err != nil {
		return nil, err
	}
	return m, nil
}

// ToolCreateTask returns an core.Tool for creating tasks.
func ToolCreateTask() core.Tool {
	return core.NewBaseTool(
		"create_task",
		"Create a new task. Args JSON: {\"title\":\"...\",\"description\":\"...\",\"parent_id?\":\"...\"}",
		func(ctx context.Context, args string) (string, error) {
			m := GetManagerFromContext(ctx)
			if m == nil {
				return "", fmt.Errorf("plantask manager not initialized")
			}
			var in struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				ParentID    string `json:"parent_id,omitempty"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", err
			}
			opts := []CreateOption{}
			if in.ParentID != "" {
				opts = append(opts, WithParentID(in.ParentID))
			}
			t, err := m.Create(ctx, in.Title, in.Description, opts...)
			if err != nil {
				return "", err
			}
			b, _ := json.Marshal(t)
			return string(b), nil
		},
	)
}

// ToolListTasks returns an core.Tool for listing all tasks.
func ToolListTasks() core.Tool {
	return core.NewBaseTool(
		"list_tasks",
		"List all tasks. Optionally filter by state. Args JSON: {\"state?\":\"pending|running|completed|failed\"}",
		func(ctx context.Context, args string) (string, error) {
			m := GetManagerFromContext(ctx)
			if m == nil {
				return "", fmt.Errorf("plantask manager not initialized")
			}
			var in struct {
				State *string `json:"state,omitempty"`
			}
			json.Unmarshal([]byte(args), &in) // ignore error - optional field
			var tasks []*Task
			var err error
			if in.State != nil && *in.State != "" {
				tasks, err = m.ListByState(TaskState(*in.State))
			} else {
				tasks, err = m.List()
			}
			if err != nil {
				return "", err
			}
			b, _ := json.Marshal(tasks)
			return string(b), nil
		},
	)
}

// ToolGetTask returns an core.Tool for getting a specific task.
func ToolGetTask() core.Tool {
	return core.NewBaseTool(
		"get_task",
		"Get task details by ID. Args JSON: {\"id\":\"task_1\"}",
		func(ctx context.Context, args string) (string, error) {
			m := GetManagerFromContext(ctx)
			if m == nil {
				return "", fmt.Errorf("plantask manager not initialized")
			}
			var in struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", err
			}
			t, err := m.Get(in.ID)
			if err != nil {
				return "", err
			}
			b, _ := json.Marshal(t)
			return string(b), nil
		},
	)
}

// ToolUpdateTask returns an core.Tool for updating a task.
func ToolUpdateTask() core.Tool {
	return core.NewBaseTool(
		"update_task",
		"Update a task. Args JSON: {\"id\":\"...\",\"state?\":\"completed\",\"result?\":\"...\"}",
		func(ctx context.Context, args string) (string, error) {
			m := GetManagerFromContext(ctx)
			if m == nil {
				return "", fmt.Errorf("plantask manager not initialized")
			}
			var in struct {
				ID     string `json:"id"`
				State  string `json:"state,omitempty"`
				Result string `json:"result,omitempty"`
				Error  string `json:"error,omitempty"`
				Title  string `json:"title,omitempty"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", err
			}
			var opts []UpdateOption
			if in.State != "" {
				opts = append(opts, WithState(TaskState(in.State)))
			}
			if in.Result != "" {
				opts = append(opts, WithResult(in.Result))
			}
			if in.Error != "" {
				opts = append(opts, WithError(in.Error))
			}
			if in.Title != "" {
				opts = append(opts, WithTitle(in.Title))
			}
			t, err := m.Update(in.ID, opts...)
			if err != nil {
				return "", err
			}
			b, _ := json.Marshal(t)
			return string(b), nil
		},
	)
}

// ToolDeleteTask returns an core.Tool for deleting a task.
func ToolDeleteTask() core.Tool {
	return core.NewBaseTool(
		"delete_task",
		"Delete a task by ID. Args JSON: {\"id\":\"task_1\"}",
		func(ctx context.Context, args string) (string, error) {
			m := GetManagerFromContext(ctx)
			if m == nil {
				return "", fmt.Errorf("plantask manager not initialized")
			}
			var in struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return "", err
			}
			if err := m.Delete(in.ID); err != nil {
				return "", err
			}
			return fmt.Sprintf(`{"deleted":true,"id":"%s"}`, in.ID), nil
		},
	)
}

// AllTools returns all plantask tool definitions as a slice.
func AllTools() []core.Tool {
	return []core.Tool{
		ToolCreateTask(),
		ToolListTasks(),
		ToolGetTask(),
		ToolUpdateTask(),
		ToolDeleteTask(),
	}
}
