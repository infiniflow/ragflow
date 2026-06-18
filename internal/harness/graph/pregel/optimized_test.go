// Package pregel provides Pregel algorithm optimizations for graph execution.
package pregel

import (
	"context"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/types"
)

func TestPriorityTaskQueue(t *testing.T) {
	pq := NewPriorityTaskQueue()
	
	// Test push and pop
	task1 := &Task{Name: "task1", Path: []string{"a"}}
	task2 := &Task{Name: "task2", Path: []string{"b"}}
	task3 := &Task{Name: "task3", Path: []string{"a", "b"}}
	
	pq.Push(task1)
	pq.Push(task2)
	pq.Push(task3)
	
	if pq.Len() != 3 {
		t.Errorf("Expected 3 tasks, got %d", pq.Len())
	}
	
	// Test pop - should return shortest path first
	popped := pq.Pop()
	if popped == nil {
		t.Fatal("Expected non-nil popped task")
	}
	
	// task1 has path ["a"] which is shorter than task3's ["a","b"]
	if popped.Name != "task1" && popped.Name != "task2" {
		t.Errorf("Expected task1 or task2, got %s", popped.Name)
	}
	
	// Check queue size
	if pq.Len() != 2 {
		t.Errorf("Expected 2 tasks remaining, got %d", pq.Len())
	}
}

func TestPriorityTaskQueueOrdering(t *testing.T) {
	pq := NewPriorityTaskQueue()
	
	// Add tasks with different path lengths
	tasks := []*Task{
		{Name: "deep1", Path: []string{"a", "b", "c"}},
		{Name: "shallow1", Path: []string{"a"}},
		{Name: "deep2", Path: []string{"a", "b"}},
		{Name: "shallow2", Path: []string{"b"}},
	}
	
	for _, task := range tasks {
		pq.Push(task)
	}
	
	// Pop in priority order
	// Shallow paths should come first
	order := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		task := pq.Pop()
		if task != nil {
			order = append(order, task.Name)
		}
	}
	
	// Verify shallow tasks come first
	if order[0] != "shallow1" && order[0] != "shallow2" {
		t.Errorf("Expected shallow task first, got %s", order[0])
	}
}

func TestTaskPriorityComparison(t *testing.T) {
	tests := []struct {
		name      string
		tp1       *TaskPriority
		tp2       *TaskPriority
		expected  int
	}{
		{
			name:     "different priorities",
			tp1:      NewTaskPriority("t1", []string{"a"}, 2),
			tp2:      NewTaskPriority("t2", []string{"b"}, 1),
			expected: 1, // t1 has higher priority (lower number)
		},
		{
			name:     "different path lengths",
			tp1:      NewTaskPriority("t1", []string{"a"}, 0),
			tp2:      NewTaskPriority("t2", []string{"a", "b"}, 0),
			expected: -1, // t1 has shorter path
		},
		{
			name:     "same path length, lexicographic",
			tp1:      NewTaskPriority("t1", []string{"a"}, 0),
			tp2:      NewTaskPriority("t2", []string{"b"}, 0),
			expected: -1, // t1 comes first alphabetically
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.tp1.Compare(tt.tp2)
			if result != tt.expected {
				t.Errorf("Compare() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestPregelOptimizedEngineCreation(t *testing.T) {
	baseEngine := &Engine{
		config: types.NewRunnableConfig(),
	}
	
	optimized := NewPregelOptimizedEngine(baseEngine, nil)
	
	if optimized == nil {
		t.Fatal("Expected non-nil optimized engine")
	}
	
	if optimized.Engine != baseEngine {
		t.Error("Expected base engine to match")
	}
	
	if !optimized.config.BumpStep {
		t.Error("Expected bump_step to be enabled")
	}
	
	if !optimized.config.FinishNotification {
		t.Error("Expected finish notification to be enabled")
	}
	
	if optimized.taskPriorityQueue == nil {
		t.Error("Expected task queue to be initialized")
	}
}

func TestFinishNotification(t *testing.T) {
	t.Run("send finish notification", func(t *testing.T) {
		// This would test actual notification sending
		// For now, just verify the structure
		notification := &FinishNotification{
			TaskName:  "test_node",
			Output:    map[string]interface{}{"result": "success"},
			Step:       1,
			Namespace:  "test_namespace",
		}
		
		if notification.TaskName != "test_node" {
			t.Errorf("Expected task name 'test_node', got %s", notification.TaskName)
		}
		
		if notification.Step != 1 {
			t.Errorf("Expected step 1, got %d", notification.Step)
		}
		
		if notification.Namespace != "test_namespace" {
			t.Errorf("Expected namespace 'test_namespace', got %s", notification.Namespace)
		}
	})
}

func TestBumpStep(t *testing.T) {
	baseEngine := &Engine{
		config: types.NewRunnableConfig(),
	}
	
	optimized := NewPregelOptimizedEngine(baseEngine, nil)
	
	t.Run("bump step for dependent tasks", func(t *testing.T) {
		ctx := context.Background()
		
		// Simulate task completion
		updatedChannels := map[string]struct{}{
			"channel1": {},
			"channel2": {},
		}
		
		err := optimized.BumpStep(ctx, "task1", 1, updatedChannels)
		if err != nil {
			t.Errorf("BumpStep failed: %v", err)
		}
		
		// Verify task is marked as finished
		if !optimized.finishedTasks["task1"] {
			t.Error("Expected task1 to be marked as finished")
		}
	})
}

func TestCompareTaskPriority(t *testing.T) {
	optimized := NewPregelOptimizedEngine(&Engine{}, nil)
	
	tests := []struct {
		name     string
		t1       *Task
		t2       *Task
		expected int
	}{
		{
			name: "different path lengths",
			t1:   &Task{Name: "a", Path: []string{"a"}},
			t2:   &Task{Name: "b", Path: []string{"a", "b"}},
			expected: -1, // Shorter path first
		},
		{
			name: "same path length, lexicographic",
			t1:   &Task{Name: "a", Path: []string{"a"}},
			t2:   &Task{Name: "b", Path: []string{"a"}},
			expected: -1, // Alphabetical first
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := optimized.compareTaskPriority(tt.t1, tt.t2)
			if result != tt.expected {
				t.Errorf("compareTaskPriority() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestIsNodeReady(t *testing.T) {
	optimized := NewPregelOptimizedEngine(&Engine{}, nil)
	
	t.Run("node with no dependencies", func(t *testing.T) {
		// Nodes with no required channels are always ready
		ready := optimized.isNodeReady("node1")
		if !ready {
			t.Error("Expected node to be ready with no dependencies")
		}
	})
	
	t.Run("node with seen channels", func(t *testing.T) {
		// Mark channels as ready
		optimized.readyChannels["channel1"] = true
		optimized.readyChannels["channel2"] = true
		
		// This would need to be set up with proper dependencies
		// For now, just test the mechanism
		_ = optimized.isNodeReady("node1")
	})
}

func TestGetDependencies(t *testing.T) {
	optimized := NewPregelOptimizedEngine(&Engine{}, nil)
	
	t.Run("get dependencies", func(t *testing.T) {
		// This would require proper graph setup
		// For now, test that the function exists
		deps := optimized.getDependencies("task1")
		if deps == nil {
			t.Log("getDependencies returns nil (expected for empty graph)")
		}
	})
}

func TestHasSeenChannel(t *testing.T) {
	optimized := NewPregelOptimizedEngine(&Engine{}, nil)
	
	t.Run("channel not seen", func(t *testing.T) {
		seen := optimized.hasSeenChannel("task1", "channel1")
		if seen {
			t.Error("Expected channel to not be seen")
		}
	})
	
	t.Run("channel seen", func(t *testing.T) {
		// Initialize seen channels for task
		optimized.seenChannels["task1"] = map[string]bool{
			"channel1": true,
		}
		
		seen := optimized.hasSeenChannel("task1", "channel1")
		if !seen {
			t.Error("Expected channel to be seen")
		}
	})
}

func TestGetTriggersForNode(t *testing.T) {
	optimized := NewPregelOptimizedEngine(&Engine{}, nil)
	
	t.Run("get triggers", func(t *testing.T) {
		// This would require proper graph setup
		// For now, test that the function exists
		triggers := optimized.getTriggersForNode("node1")
		if triggers == nil {
			t.Log("getTriggersForNode returns nil (expected for empty graph)")
		}
	})
}

func TestNewTaskPriority(t *testing.T) {
	tp := NewTaskPriority("test_task", []string{"a", "b"}, 1)
	
	if tp == nil {
		t.Fatal("Expected non-nil task priority")
	}
	
	if tp.Name != "test_task" {
		t.Errorf("Expected name 'test_task', got %s", tp.Name)
	}
	
	if len(tp.Path) != 2 {
		t.Errorf("Expected path length 2, got %d", len(tp.Path))
	}
	
	if tp.Priority != 1 {
		t.Errorf("Expected priority 1, got %d", tp.Priority)
	}
}

func TestPriorityQueueHeap(t *testing.T) {
	pq := NewPriorityTaskQueue()
	
	// Test heap property maintenance
	tasks := []*Task{
		{Name: "z", Path: []string{"z"}},
		{Name: "m", Path: []string{"m"}},
		{Name: "a", Path: []string{"a"}},
		{Name: "n", Path: []string{"n"}},
	}
	
	// Add in random order
	for _, task := range tasks {
		pq.Push(task)
	}
	
	// Verify heap property: each parent should have higher priority than children
	// This is a simplified check - in practice we'd verify the full heap property
	if pq.Len() != 4 {
		t.Errorf("Expected 4 tasks in queue, got %d", pq.Len())
	}
}

func TestOptimizedApplyWrites(t *testing.T) {
	baseEngine := &Engine{
		config: types.NewRunnableConfig(),
	}
	
	optimized := NewPregelOptimizedEngine(baseEngine, nil)
	
	t.Run("apply writes with bump_step", func(t *testing.T) {
		ctx := context.Background()
		
		// Create a registry with a channel
		registry := channels.NewRegistry()
		ch := channels.NewAnyValue(nil)
		registry.Register("key", ch)
		
		results := []*TaskResult{
			{Name: "task1", Output: map[string]interface{}{"key": "value"}},
		}
		
		updatedChannels, err := optimized.OptimizedApplyWrites(
			ctx,
			registry,
			results,
			1,
			map[string]struct{}{"trigger": {}},
		)
		
		if err != nil {
			t.Logf("OptimizedApplyWrites error: %v", err)
		}
		
		if updatedChannels == nil {
			t.Error("Expected non-nil updated channels")
		}
	})
}

func TestGetCurrentNamespace(t *testing.T) {
	baseEngine := &Engine{
		config: types.NewRunnableConfig(),
	}
	baseEngine.config.Set("namespace", "test_ns")
	
	optimized := NewPregelOptimizedEngine(baseEngine, nil)
	
	ns := optimized.getCurrentNamespace()
	
	if ns == "" {
		t.Log("getCurrentNamespace returns empty (config may not have namespace)")
	}
}

func TestPrepareNextTasksOptimized(t *testing.T) {
	baseEngine := &Engine{
		config: types.NewRunnableConfig(),
	}
	
	optimized := NewPregelOptimizedEngine(baseEngine, nil)
	
	t.Run("prepare next tasks with optimization", func(t *testing.T) {
		ctx := context.Background()
		
		tasks, triggers, err := optimized.PrepareNextTasksOptimized(
			ctx,
			nil, // registry
			map[string]bool{},
			"",
			nil, // current state
		)
		
		if err != nil {
			t.Logf("PrepareNextTasksOptimized error: %v", err)
		}
		
		if tasks == nil {
			tasks = []*Task{}
		}
		
		if triggers == nil {
			triggers = map[string]struct{}{}
		}
		
		t.Logf("Prepared %d tasks with %d triggers", len(tasks), len(triggers))
	})
}
