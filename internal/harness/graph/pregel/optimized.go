// Package pregel provides Pregel algorithm optimizations for graph execution.
package pregel

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"ragflow/internal/harness/graph/channels"
)

// OptimizedEngineConfig configures the optimized Pregel engine.
type OptimizedEngineConfig struct {
	BumpStep          bool
	FinishNotification bool
	TaskPriority      bool
}

// PregelOptimizedEngine extends Engine with Python-style Pregel algorithm optimizations.
type PregelOptimizedEngine struct {
	*Engine
	config            *OptimizedEngineConfig
	taskPriorityQueue  []*TaskWithPriority
	stepQueue         map[int][]string
	seenChannels      map[string]map[string]bool
	readyChannels     map[string]bool
	finishedTasks     map[string]bool
	taskDependencies  map[string][]string
	channelVersions   map[string]int
	mu                sync.RWMutex
}

// TaskWithPriority extends Task with priority information.
type TaskWithPriority struct {
	*Task
	Priority  int
	Namespace string
	Path      []string
}

// NewPregelOptimizedEngine creates an optimized Pregel engine.
func NewPregelOptimizedEngine(baseEngine *Engine, config *OptimizedEngineConfig) *PregelOptimizedEngine {
	if config == nil {
		config = &OptimizedEngineConfig{
			BumpStep:          true,
			FinishNotification: true,
			TaskPriority:      true,
		}
	}
	
	return &PregelOptimizedEngine{
		Engine:             baseEngine,
		config:            config,
		taskPriorityQueue:  make([]*TaskWithPriority, 0, 100),
		stepQueue:         make(map[int][]string),
		seenChannels:      make(map[string]map[string]bool),
		readyChannels:     make(map[string]bool),
		finishedTasks:     make(map[string]bool),
		taskDependencies:  make(map[string][]string),
		channelVersions:   make(map[string]int),
	}
}

// BumpStep implements Python-style bump_step optimization.
// When a task finishes, bump step for all dependent tasks
// that haven't seen the latest channel values.
func (e *PregelOptimizedEngine) BumpStep(
	ctx context.Context,
	taskName string,
	completedStep int,
	updatedChannels map[string]struct{},
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Mark task as finished
	e.finishedTasks[taskName] = true
	
	// Find dependent tasks
	dependencies, exists := e.taskDependencies[taskName]
	if !exists || len(dependencies) == 0 {
		// No dependent tasks, nothing to bump
		return nil
	}
	
	for _, depTask := range dependencies {
		// Check if dependent task has seen all updated channels
		ready := true
		seenChannels := e.seenChannels[depTask]
		
		for channel := range updatedChannels {
			if seenChannels != nil && !seenChannels[channel] {
				// This dependent task needs to be bumped
				ready = false
				break
			}
		}
		
		if ready {
			// Bump task to current step
			e.stepQueue[completedStep+1] = append(e.stepQueue[completedStep+1], depTask)
			
			// Mark channels as seen for this task
			if e.seenChannels[depTask] == nil {
				e.seenChannels[depTask] = make(map[string]bool)
			}
			for ch := range updatedChannels {
				e.seenChannels[depTask][ch] = true
			}
		}
	}
	
	return nil
}

// FinishNotification sends Python-style finish notifications.
// When a task completes, notify all waiting tasks and streams.
func (e *PregelOptimizedEngine) FinishNotification(
	ctx context.Context,
	taskName string,
	result interface{},
	err error,
	completedStep int,
) {
	if !e.config.FinishNotification {
		return
	}
	
	// Build finish notification
	notification := &FinishNotification{
		TaskName:      taskName,
		Output:        result,
		Error:         err,
		Step:          completedStep,
		Timestamp:     time.Now(),
		Namespace:      e.getNamespace(ctx),
	}
	
	// Send to stream manager if available
	// Note: This requires the Engine to have access to streamManager
	// For now, we'll just log it
	if err != nil {
		fmt.Printf("[FinishNotification] Task %s failed at step %d: %v\n", taskName, completedStep, err)
	} else {
		fmt.Printf("[FinishNotification] Task %s completed at step %d\n", taskName, completedStep)
	}
	
	_ = notification // Mark as used to avoid unused variable error
}

// compareTaskPriority compares two tasks for priority ordering.
// Returns negative if t1 has higher priority, positive if t2 has higher priority, zero if equal.
func (e *PregelOptimizedEngine) compareTaskPriority(t1, t2 *Task) int {
	// Compare by path length (shorter path = higher priority)
	if len(t1.Path) != len(t2.Path) {
		return len(t1.Path) - len(t2.Path)
	}
	// If same path length, compare by name lexicographically
	for i := 0; i < len(t1.Path); i++ {
		if t1.Path[i] != t2.Path[i] {
			if t1.Path[i] < t2.Path[i] {
				return -1
			}
			return 1
		}
	}
	// If paths are identical, compare by task name
	if t1.Name != t2.Name {
		if t1.Name < t2.Name {
			return -1
		}
		return 1
	}
	return 0
}

// OptimizedApplyWrites implements optimized apply_writes with bump_step and finish notification.
// This corresponds to Python's apply_writes function in _algo.py
func (e *PregelOptimizedEngine) OptimizedApplyWrites(
	ctx context.Context,
	registry *channels.Registry,
	results []*TaskResult,
	step int,
	triggerToNodes map[string]struct{},
) (map[string]struct{}, error) {
	updatedChannels := make(map[string]struct{})

	// Sort results by task path for deterministic execution (like Python's task sorting)
	// This ensures consistent ordering across distributed executions
	sort.Slice(results, func(i, j int) bool {
		// First compare by path length (shorter first)
		if len(results[i].Path) != len(results[j].Path) {
			return len(results[i].Path) < len(results[j].Path)
		}
		// Then compare by path lexicographically
		for k := 0; k < len(results[i].Path) && k < len(results[j].Path); k++ {
			if results[i].Path[k] != results[j].Path[k] {
				return results[i].Path[k] < results[j].Path[k]
			}
		}
		// Finally by name
		return results[i].Name < results[j].Name
	})

	// Group and apply writes
	writesByChannel := make(map[string][]interface{})

	for _, result := range results {
		if result.Err != nil {
			continue
		}

		outputMap, err := toMap(result.Output)
		if err != nil {
			return nil, fmt.Errorf("failed to convert output to map: %w", err)
		}

		for key, value := range outputMap {
			if value == nil {
				continue
			}

			writesByChannel[key] = append(writesByChannel[key], value)
		}
	}

	// Apply writes with channel version management
	for channelName, values := range writesByChannel {
		ch, ok := registry.Get(channelName)
		if !ok {
			continue
		}

		filtered := make([]interface{}, 0, len(values))
		for _, v := range values {
			if v != nil {
				filtered = append(filtered, v)
			}
		}

		updated, err := ch.Update(filtered)
		if err != nil {
			return nil, fmt.Errorf("failed to update channel %s: %w", channelName, err)
		}

		if updated {
			updatedChannels[channelName] = struct{}{}
			e.readyChannels[channelName] = true

			// Bump step optimization
			if e.config.BumpStep {
				e.channelVersions[channelName]++
				if e.currentCheckpoint != nil {
					e.currentCheckpoint.IncrementChannel(channelName)
				}
			}
		}
	}

	// Finish notification - notify all channels that this superstep is finishing
	// This corresponds to Python's finish notification in apply_writes
	if e.config.FinishNotification && len(updatedChannels) > 0 {
		// Check if this might be the last superstep
		// (all triggers have been processed)
		allTriggersProcessed := true
		for trigger := range triggerToNodes {
			if _, ok := updatedChannels[trigger]; !ok {
				// This trigger hasn't been updated yet
				allTriggersProcessed = false
				break
			}
		}

		if allTriggersProcessed {
			// Notify all channels that the run is finishing
			for _, channelName := range registry.List() {
				if ch, ok := registry.Get(channelName); ok {
					ch.Finish()
				}
			}
		}
	}

	return updatedChannels, nil
}

// AddTaskDependency adds a dependency relationship between tasks.
func (e *PregelOptimizedEngine) AddTaskDependency(fromTask, toTask string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if e.taskDependencies[toTask] == nil {
		e.taskDependencies[toTask] = make([]string, 0)
	}
	
	// Add dependency (fromTask depends on toTask)
	e.taskDependencies[toTask] = append(e.taskDependencies[toTask], fromTask)
}

// GetTaskDependencies returns dependencies for a task.
func (e *PregelOptimizedEngine) GetTaskDependencies(taskName string) []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if deps, exists := e.taskDependencies[taskName]; exists {
		return append([]string{}, deps...)
	}
	return []string{}
}

// IsTaskReady checks if a task is ready to execute.
func (e *PregelOptimizedEngine) IsTaskReady(taskName string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	// Check if task has been seen all required channels
	// This is a simplified check - in practice you'd check specific channels
	if len(e.taskDependencies[taskName]) == 0 {
		return true
	}
	
	// Check if any dependencies are still unfinished
	for _, dep := range e.taskDependencies[taskName] {
		if !e.finishedTasks[dep] {
			return false
		}
	}
	
	return true
}

// getNamespace retrieves the current namespace from context.
func (e *PregelOptimizedEngine) getNamespace(ctx context.Context) string {
	// Simplified implementation - in practice this would use context values
	// For now, return empty namespace
	return ""
}

// FinishNotification represents a task completion notification.
type FinishNotification struct {
	TaskName  string      `json:"task_name"`
	Output    interface{} `json:"output"`
	Error      error       `json:"error,omitempty"`
	Step       int         `json:"step"`
	Timestamp  time.Time   `json:"timestamp"`
	Namespace  string      `json:"namespace,omitempty"`
}

// TaskPriority represents task execution priority.
type TaskPriority struct {
	Name     string
	Path     []string
	Priority int
}

// NewTaskPriority creates a new task priority.
func NewTaskPriority(name string, path []string, priority int) *TaskPriority {
	return &TaskPriority{
		Name:     name,
		Path:     path,
		Priority: priority,
	}
}

// Compare compares two task priorities.
func (tp *TaskPriority) Compare(other *TaskPriority) int {
	// Compare by priority first
	if tp.Priority != other.Priority {
		return tp.Priority - other.Priority
	}
	
	// Then by path length
	if len(tp.Path) != len(other.Path) {
		return len(tp.Path) - len(other.Path)
	}
	
	// Finally by path lexicographically
	for i := 0; i < len(tp.Path); i++ {
		if tp.Path[i] != other.Path[i] {
			if tp.Path[i] < other.Path[i] {
				return -1
			}
			return 1
		}
	}
	
	return 0
}

// OptimizedRun executes the graph with optimizations enabled.
// It delegates to the base Engine's RunSync, which applies the full Pregel execution
// loop with async pipeline, streaming, checkpoint, and interrupt support.
// BumpStep, FinishNotification, and other optimized methods are available for
// callers to integrate into custom execution flows.
func (e *PregelOptimizedEngine) OptimizedRun(
	ctx context.Context,
	input interface{},
) (interface{}, error) {
	return e.RunSync(ctx, input)
}

// ExecuteTaskWithPriority executes a task with priority queue support.
func (e *PregelOptimizedEngine) ExecuteTaskWithPriority(
	ctx context.Context,
	task *Task,
	priority int,
	namespace string,
) *TaskResult {
	// Mark task as executing
	e.mu.Lock()
	taskWithPriority := &TaskWithPriority{
		Task:     task,
		Priority:  priority,
		Namespace: namespace,
		Path:      []string{namespace, task.Name},
	}
	e.taskPriorityQueue = append(e.taskPriorityQueue, taskWithPriority)
	e.mu.Unlock()
	
		// Execute task
		output, err := task.Func(ctx, nil)
	
	return &TaskResult{
		Name:   task.Name,
		Output: output,
		Err:    err,
	}
}

// GetNextPriorityTask gets the next task from priority queue.
func (e *PregelOptimizedEngine) GetNextPriorityTask() *TaskWithPriority {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	if len(e.taskPriorityQueue) == 0 {
		return nil
	}
	
		// Sort by priority (could use heap for better performance)
	sort.Slice(e.taskPriorityQueue, func(i, j int) bool {
		tp1 := &TaskPriority{
			Name:     e.taskPriorityQueue[i].Name,
			Path:     e.taskPriorityQueue[i].Path,
			Priority: e.taskPriorityQueue[i].Priority,
		}
		tp2 := &TaskPriority{
			Name:     e.taskPriorityQueue[j].Name,
			Path:     e.taskPriorityQueue[j].Path,
			Priority: e.taskPriorityQueue[j].Priority,
		}
		return tp1.Compare(tp2) < 0
	})
	
	// Get first task
	if len(e.taskPriorityQueue) == 0 {
		return nil
	}
	
	task := e.taskPriorityQueue[0]
	e.taskPriorityQueue = e.taskPriorityQueue[1:]
	
	return task
}

// ClearFinishedTasks clears the finished tasks map.
func (e *PregelOptimizedEngine) ClearFinishedTasks() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.finishedTasks = make(map[string]bool)
}

// GetFinishedTasks returns all finished task names.
func (e *PregelOptimizedEngine) GetFinishedTasks() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	tasks := make([]string, 0, len(e.finishedTasks))
	for name := range e.finishedTasks {
		tasks = append(tasks, name)
	}
	
	return tasks
}

// Reset clears all optimization state.
func (e *PregelOptimizedEngine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.taskPriorityQueue = make([]*TaskWithPriority, 0, 100)
	e.stepQueue = make(map[int][]string)
	e.seenChannels = make(map[string]map[string]bool)
	e.readyChannels = make(map[string]bool)
	e.finishedTasks = make(map[string]bool)
}

// PriorityTaskQueue implements a priority queue for tasks based on path length.
type PriorityTaskQueue struct {
	tasks []*Task
}

// NewPriorityTaskQueue creates a new priority task queue.
func NewPriorityTaskQueue() *PriorityTaskQueue {
	return &PriorityTaskQueue{
		tasks: make([]*Task, 0),
	}
}

// Push adds a task to the queue.
func (pq *PriorityTaskQueue) Push(task *Task) {
	pq.tasks = append(pq.tasks, task)
	// Simple insertion sort by path length (shorter first)
	// This is inefficient for large queues but fine for testing
	for i := len(pq.tasks) - 1; i > 0; i-- {
		if len(pq.tasks[i].Path) < len(pq.tasks[i-1].Path) {
			pq.tasks[i], pq.tasks[i-1] = pq.tasks[i-1], pq.tasks[i]
		} else {
			break
		}
	}
}

// Pop removes and returns the highest priority task.
func (pq *PriorityTaskQueue) Pop() *Task {
	if len(pq.tasks) == 0 {
		return nil
	}
	task := pq.tasks[0]
	pq.tasks = pq.tasks[1:]
	return task
}

// Len returns the number of tasks in the queue.
func (pq *PriorityTaskQueue) Len() int {
	return len(pq.tasks)
}

// isNodeReady checks if a node is ready to execute (alias for IsTaskReady).
func (e *PregelOptimizedEngine) isNodeReady(nodeName string) bool {
	return e.IsTaskReady(nodeName)
}

// getDependencies returns dependencies for a task (alias for GetTaskDependencies).
func (e *PregelOptimizedEngine) getDependencies(taskName string) []string {
	return e.GetTaskDependencies(taskName)
}

// hasSeenChannel checks if a task has seen a specific channel.
func (e *PregelOptimizedEngine) hasSeenChannel(taskName, channel string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	if channels, exists := e.seenChannels[taskName]; exists {
		return channels[channel]
	}
	return false
}

// getTriggersForNode returns triggers for a node.
func (e *PregelOptimizedEngine) getTriggersForNode(nodeName string) map[string]struct{} {
	// Guard against uninitialized engine (e.g., in tests).
	if e.Engine == nil || e.Engine.graph == nil {
		return make(map[string]struct{})
	}
	node := e.getNode(nodeName)
	if node == nil {
		return make(map[string]struct{})
	}
	triggers := e.getTriggers(node)
	result := make(map[string]struct{}, len(triggers))
	for _, t := range triggers {
		result[t] = struct{}{}
	}
	return result
}

// getCurrentNamespace returns the current namespace.
func (e *PregelOptimizedEngine) getCurrentNamespace() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	// Simplified - check config for namespace
	if e.Engine != nil && e.Engine.config != nil {
		if ns, ok := e.Engine.config.Get("namespace"); ok {
			if nsStr, ok := ns.(string); ok {
				return nsStr
			}
		}
	}
	return ""
}

// PrepareNextTasksOptimized prepares next tasks with optimization.
// It delegates to the base Engine's prepareNextTasks for standard task discovery,
// which handles entry points, conditional edges, regular edges, and branches.
func (e *PregelOptimizedEngine) PrepareNextTasksOptimized(
	ctx context.Context,
	registry interface{},
	visited map[string]bool,
	trigger string,
	currentState interface{},
) ([]*Task, map[string]struct{}, error) {
	if e.Engine == nil || e.Engine.graph == nil {
		return nil, nil, fmt.Errorf("PrepareNextTasksOptimized: engine not initialized")
	}
	reg, ok := registry.(*channels.Registry)
	if !ok {
		return nil, nil, fmt.Errorf("PrepareNextTasksOptimized: invalid registry type %T", registry)
	}
	return e.prepareNextTasks(reg, visited, trigger, currentState)
}
