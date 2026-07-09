// Package pregel provides the Pregel execution algorithm for graph processing.
package pregel

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/interrupt"
	"ragflow/internal/harness/graph/types"
)

// Engine implements the Pregel (bulk-synchronous parallel) execution model
// for StateGraph. It manages channel-based state communication, concurrent
// task execution via AsyncPipeline, streaming event emission, and checkpoint
// persistence.
//
// Create an Engine via NewEngine with option functions:
//
//	engine := NewEngine(graph,
//	    WithCheckpointer(cp),
//	    WithRecursionLimit(50),
//	)
type Engine struct {
	graph               types.StateGraph
	checkpointer        checkpoint.BaseCheckpointer
	interrupts          map[string]bool
	interruptsAfter     map[string]bool
	recursionLimit      int
	debug               bool
	config              *types.RunnableConfig
	maxConcurrency      int
	retryPolicy         *types.RetryPolicy
	currentCheckpoint   *checkpoint.Checkpoint
	channelVersions     map[string]int
	versionsSeen        map[string]map[string]int
	cache               Cache
	backgroundExec      *BackgroundExecutor
	callbacks           *CallbackManager   // lifecycle callbacks (event recording, metrics)
	deferredCheckpoints []deferredCheckpoint // for DurabilityExit mode
}

// deferredCheckpoint stores checkpoint data for deferred saving (DurabilityExit mode)
type deferredCheckpoint struct {
	ThreadID     string
	CheckpointID string
	Step         int
	Checkpoint   map[string]any
}

// NewEngine creates a new Pregel engine bound to a StateGraph.
// Options configure checkpointer, recursion limit, concurrency, retry, cache, etc.
//
// The engine is reusable across multiple Run calls. Each call creates its own
// background executor for isolation.
func NewEngine(g types.StateGraph, opts ...EngineOption) *Engine {
	eng := &Engine{
		graph:           g,
		interrupts:      make(map[string]bool),
		interruptsAfter: make(map[string]bool),
		recursionLimit:  25,
		debug:           false,
		config:          types.NewRunnableConfig(),
		maxConcurrency:  10,
		retryPolicy:     nil,
		channelVersions: make(map[string]int),
		versionsSeen:    make(map[string]map[string]int),
		cache:           &NoopCache{},
	}

	for _, opt := range opts {
		opt(eng)
	}

	// Initialize background executor if not already set
	if eng.backgroundExec == nil {
		eng.backgroundExec = NewBackgroundExecutor(eng.maxConcurrency, 100)
	}

	return eng
}

// EngineOption is an option for configuring the Pregel engine.
// Available options: WithCheckpointer, WithInterrupts, WithRecursionLimit,
// WithDebug, WithConfig, WithMaxConcurrency, WithRetryPolicy, WithCache,
// WithBackgroundExecutor.
type EngineOption func(*Engine)

// WithCheckpointer sets the checkpointer.
func WithCheckpointer(cp checkpoint.BaseCheckpointer) EngineOption {
	return func(e *Engine) {
		e.checkpointer = cp
	}
}

// WithInterrupts sets the interrupt nodes.
func WithInterrupts(nodes ...string) EngineOption {
	return func(e *Engine) {
		for _, node := range nodes {
			e.interrupts[node] = true
		}
	}
}

// WithInterruptsAfter sets the after-execution interrupt nodes.
func WithInterruptsAfter(nodes ...string) EngineOption {
	return func(e *Engine) {
		for _, node := range nodes {
			e.interruptsAfter[node] = true
		}
	}
}

// WithRecursionLimit sets the recursion limit.
func WithRecursionLimit(limit int) EngineOption {
	return func(e *Engine) {
		e.recursionLimit = limit
	}
}

// WithDebug enables debug mode.
func WithDebug(debug bool) EngineOption {
	return func(e *Engine) {
		e.debug = debug
	}
}

// WithConfig sets the runnable config.
func WithConfig(cfg *types.RunnableConfig) EngineOption {
	return func(e *Engine) {
		e.config = cfg
	}
}

// WithMaxConcurrency sets the maximum concurrency for node execution.
func WithMaxConcurrency(max int) EngineOption {
	return func(e *Engine) {
		if max > 0 {
			e.maxConcurrency = max
		}
	}
}

// WithRetryPolicy sets the retry policy for node execution.
func WithRetryPolicy(policy *types.RetryPolicy) EngineOption {
	return func(e *Engine) {
		e.retryPolicy = policy
	}
}

// WithCache sets the cache for the engine.
func WithCache(cache Cache) EngineOption {
	return func(e *Engine) {
		e.cache = cache
	}
}

// WithBackgroundExecutor sets the background executor for the engine.
func WithBackgroundExecutor(exec *BackgroundExecutor) EngineOption {
	return func(e *Engine) {
		e.backgroundExec = exec
	}
}

// WithCallbacks sets the callback manager for the engine.
// Callbacks are dispatched during graph execution (run start/end, step start/end,
// node start/end, checkpoint save/load, interrupt/resume).
func WithCallbacks(cb *CallbackManager) EngineOption {
	return func(e *Engine) {
		e.callbacks = cb
	}
}

// ExecuteResult represents the result of graph execution.
type ExecuteResult struct {
	// Final state of the graph.
	State any
	// Checkpoint ID for this execution.
	CheckpointID string
	// Metadata about the execution.
	Metadata map[string]any
}

// Run executes the graph using the Pregel algorithm and returns streaming events.
// outputCh yields StreamEvent values (checkpoints, task start/end, state updates,
// and a final event with the complete state). errCh receives a single error on failure
// or nil on clean completion.
//
// The caller MUST read from outputCh until it is closed to prevent goroutine leaks.
// For synchronous execution, use RunSync instead.
func (e *Engine) Run(ctx context.Context, input any, mode types.StreamMode) (<-chan any, <-chan error) {
	outputCh := make(chan any, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(errCh)

		// Create stream manager for event streaming
		streamManager := NewStreamManager(mode, 100)

		// WaitGroup ensures the forward goroutine exits before we close outputCh,
		// preventing a data race between close(outputCh) and outputCh <- event.
		var fwWg sync.WaitGroup

		// Forward stream events to output channel
		fwWg.Go(func() {
			for event := range streamManager.Events() {
				select {
				case outputCh <- event:
				case <-ctx.Done():
					return
				}
			}
		})

		// Resolve thread ID early (before deferred cleanup uses it).
		threadID := e.getThreadID()

		// reportRunEnd is defined before the deferred cleanup block so the
		// defer can capture it by closure.
		reportRunEnd := func(err error) {
			if e.callbacks == nil {
				return
			}
			gName := "state_graph"
			if e.graph != nil {
				nodes := e.graph.GetNodes()
				for name := range nodes {
					gName = name
					break
				}
			}
			e.callbacks.RunEnd(context.Background(), gName, threadID, err)
		}

		// Deferred cleanup: dispatch RunEnd, close streamManager,
		// wait for forward goroutine, then close outputCh.
		var exitErr error // captured for RunEnd callback dispatch
		defer func() {
			// Read from errCh to get the exit error for RunEnd dispatch.
			// errCh is still open here (close(errCh) runs after this defer).
			select {
			case exitErr = <-errCh:
				reportRunEnd(exitErr)
				errCh <- exitErr // put back for the caller
			default:
				reportRunEnd(nil)
			}
			streamManager.Close()
			fwWg.Wait()
			close(outputCh)
		}()

		// Create async pipeline for concurrent task execution
		retryPolicy := e.retryPolicy
		if retryPolicy == nil {
			defaultPolicy := types.DefaultRetryPolicy()
			retryPolicy = &defaultPolicy
		}
		asyncPipeline := NewAsyncPipeline(e.maxConcurrency, retryPolicy)
		pipelineCtx := asyncPipeline.Start(ctx)
		defer asyncPipeline.Stop()

		// Reset per-execution engine state.
		// Without this, reusing the same Engine across multiple RunSync calls
		// causes checkpoint maps and channel versions to accumulate indefinitely,
		// leading to unbounded memory growth (soak tests exposed this).
		e.currentCheckpoint = nil
		e.channelVersions = make(map[string]int)
		e.versionsSeen = make(map[string]map[string]int)
		e.deferredCheckpoints = nil

		// Initialize channels
		channelRegistry := channels.NewRegistry()
		graphChannels := e.getGraphChannels()
		for name, ch := range graphChannels {
			channelRegistry.Register(name, ch.Copy())
		}

		// Apply input to channels
		if err := e.applyInput(channelRegistry, input); err != nil {
			errCh <- fmt.Errorf("failed to apply input: %w", err)
			return
		}

		// Get thread ID for checkpointing
		// threadID already resolved above (before deferred cleanup).

		// Load checkpoint when one exists for this thread_id, even when
		// input is non-nil (resume from a previous run).  The canvas
		// always passes a non-nil input ({"query": ...}) on resume, so
		// a strict input==nil guard would prevent checkpoint recovery.
		// We only skip checkpoint loading if the checkpointer reports
		// no data (fresh start).
		// When a checkpoint IS loaded, do NOT apply input — the
		// channel values from the checkpoint already contain the
		// state at the point of interruption.
		var (
			didLoadCheckpoint   bool
			cpCompletedTasks    map[string]bool
			cpLastCompletedNode string
			cpData              map[string]any
		)
		if e.checkpointer != nil {
			var cpErr error
			cpConfig := map[string]any{
				constants.ConfigKeyThreadID: threadID,
			}
			// Support loading a specific checkpoint_id for replay/fork.
			var requestedCPID string
			if e.config != nil && e.config.Configurable != nil {
				if cpid, ok := e.config.Configurable[constants.ConfigKeyCheckpointID]; ok {
					if cpidStr, ok := cpid.(string); ok && cpidStr != "" {
						cpConfig[constants.ConfigKeyCheckpointID] = cpidStr
						requestedCPID = cpidStr
					}
				}
			}
			cpData, cpErr = e.checkpointer.Get(ctx, cpConfig)
			// When a specific checkpoint_id was requested, fail on missing data.
			if requestedCPID != "" && (cpErr != nil || cpData == nil) {
				cpErrMsg := "checkpoint not found"
				if cpErr != nil {
					cpErrMsg = cpErr.Error()
				}
				errCh <- fmt.Errorf("requested checkpoint_id %s: %s", requestedCPID, cpErrMsg)
				return
			}
			if cpErr == nil && cpData != nil {
				didLoadCheckpoint = true
				common.Debug("LOOP_CHECK: loaded checkpoint",
					zap.String("thread", threadID),
					zap.Bool("has_sub", cpData["__sub_state__"] != nil))
				// Restore sub-state (e.g. Loop iteration, currentInput)
				// and inject into interrupt context so Loop node can
				// read it via loadLoopSnapshot on resume.
				if raw, ok := cpData["__sub_state__"]; ok {
					switch v := raw.(type) {
					case []byte:
						pipelineCtx = context.WithValue(pipelineCtx, interrupt.SubGraphStateCtxKey, v)
					case string:
						pipelineCtx = context.WithValue(pipelineCtx, interrupt.SubGraphStateCtxKey, []byte(v))
					}
				}
				// Restore completed task tracking.
				if raw, ok := cpData["__completed_tasks__"]; ok {
					if str, ok := raw.(string); ok {
						cpCompletedTasks = deserializeStringSet(str)
					}
				}
				if raw, ok := cpData["__last_completed_node__"]; ok {
					if str, ok := raw.(string); ok {
						cpLastCompletedNode = str
					}
				}
				// Only restore keys that correspond to registered channels.
				filtered := make(map[string]any)
				for key, val := range cpData {
					if _, ok := channelRegistry.Get(key); ok {
						filtered[key] = val
					}
				}
				if len(filtered) > 0 {
					if err := channelRegistry.RestoreFromCheckpoint(filtered); err != nil {
						errCh <- fmt.Errorf("failed to restore from checkpoint: %w", err)
						return
					}
				}
				if cp, err := checkpoint.FromMap(cpData); err == nil {
					e.currentCheckpoint = cp
				}
				// Dispatch CheckpointLoad callback.
				if e.callbacks != nil {
					cpID := ""
					if cpid, _ := cpData["checkpoint_id"].(string); cpid != "" {
						cpID = cpid
					}
					e.callbacks.CheckpointLoad(ctx, threadID, cpID, 0)
				}
			}
		}
		// Apply input only when no checkpoint was loaded.
		if !didLoadCheckpoint {
			if err := e.applyInput(channelRegistry, input); err != nil {
				errCh <- fmt.Errorf("failed to apply input: %w", err)
				return
			}
		}

		// Initialize new checkpoint if none exists
		if e.currentCheckpoint == nil {
			e.currentCheckpoint = checkpoint.NewCheckpoint(threadID, 0)
		}

		// Create per-run background executor (not shared, so concurrent calls are safe)
		backgroundExec := NewBackgroundExecutor(e.maxConcurrency, 100)
		backgroundExec.Start(ctx)
		defer backgroundExec.Stop()
		// Replace engine-level backgroundExec reference for use by async pipeline
		e.backgroundExec = backgroundExec

		// Execute Pregel loop
		step := 0
		var completedTasks map[string]bool
		lastCompletedNode := cpLastCompletedNode
		if didLoadCheckpoint && cpCompletedTasks != nil {
			completedTasks = cpCompletedTasks
		} else {
			completedTasks = make(map[string]bool)
		}
		var lastState any
		if didLoadCheckpoint {
			if raw, ok := cpData["__last_state__"]; ok {
				var jsonBytes []byte
				switch val := raw.(type) {
				case string:
					jsonBytes = []byte(val)
				case []byte:
					jsonBytes = val
				}
				if jsonBytes != nil {
					var decoded map[string]any
					if json.Unmarshal(jsonBytes, &decoded) == nil {
						lastState = decoded
					}
				}
			}
		} else {
			lastState = input
		}

		// Dispatch RunStart callback.
		if e.callbacks != nil {
			gName := "state_graph"
			if e.graph != nil {
				nodes := e.graph.GetNodes()
				for name := range nodes {
					gName = name
					break
				}
			}
			e.callbacks.RunStart(ctx, gName, threadID)
		}
		for {
			// Check context cancellation at each superstep.
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
			}

			// Check recursion limit
			if step >= e.recursionLimit {
				errCh <- &errors.GraphRecursionError{Limit: e.recursionLimit}
				return
			}

			// Dispatch StepStart callback.
			if e.callbacks != nil {
				e.callbacks.StepStart(ctx, step, 0) // taskCount filled after prepareNextTasks
			}

			// Emit checkpoint event via stream manager
			streamManager.EmitCheckpoint(step, channelRegistry.CreateCheckpoint())

			// Determine next tasks
			tasks, triggers, err := e.prepareNextTasks(ctx, channelRegistry, completedTasks, lastCompletedNode, lastState)
			if err != nil {
				errCh <- fmt.Errorf("failed to prepare next tasks: %w", err)
				return
			}

			// Emit task start events
			for _, task := range tasks {
				streamManager.EmitTaskStart(step, task.Name, task.ID)
			}

			// If no tasks, we're done
			if len(tasks) == 0 {
				break
			}

			// Check for interrupts
			interruptedTasks := e.shouldInterrupt(channelRegistry, tasks, triggers)
			if len(interruptedTasks) > 0 {
				// Save checkpoint
				if e.checkpointer != nil {
					checkpoint := channelRegistry.CreateCheckpoint()
					if err := e.checkpointer.Put(ctx, map[string]any{
						constants.ConfigKeyThreadID: threadID,
					}, checkpoint); err != nil {
						errCh <- fmt.Errorf("failed to save checkpoint: %w", err)
						return
					}
				}

				// Emit interrupt event
				interruptNames := make([]string, len(interruptedTasks))
				for i, task := range interruptedTasks {
					interruptNames[i] = task.Name
				}
				streamManager.EmitInterrupt(step, interruptNames)

				// Dispatch Interrupt callback.
				if e.callbacks != nil {
					e.callbacks.Interrupt(ctx, interruptNames, step)
				}

				errCh <- &errors.GraphInterrupt{}
				return
			}

			// Execute tasks using async pipeline
			results, err := e.executeTasksAsync(pipelineCtx, tasks, channelRegistry, asyncPipeline, streamManager, step)
			if err != nil {
				errCh <- fmt.Errorf("failed to execute tasks: %w", err)
				return
			}

			// Mark tasks as completed and track last state
			allFailed := len(results) > 0
			var interruptTaskNames []string
			for _, result := range results {
				if errors.IsGraphInterrupt(result.Err) {
					interruptTaskNames = append(interruptTaskNames, result.Name)
					continue
				}
				if result.Err == nil {
					allFailed = false
					completedTasks[result.Name] = true
					lastCompletedNode = result.Name
					// Merge result into lastState
					lastState = e.mergeStates(lastState, result.Output)
				}
			}
			// If any task was interrupted, handle the interrupt.
			if len(interruptTaskNames) > 0 {
				common.Debug("engine interrupt path",
					zap.Int("step", step),
					zap.Strings("tasks", interruptTaskNames),
					zap.Bool("allFailed", allFailed))
				// Save checkpoint with completed_tasks and sub_state.
				if e.checkpointer != nil {
					checkpointData := channelRegistry.CreateCheckpoint()
					cpPayload := make(map[string]any, len(checkpointData)+4)
					for key, val := range checkpointData {
						cpPayload[key] = val
					}
					cpPayload["__completed_tasks__"] = serializeStringSet(completedTasks)
					cpPayload["__last_completed_node__"] = lastCompletedNode
					cpPayload["__step__"] = float64(step)
					// Persist lastState as string (not []byte) to avoid
					// JSON double-base64-encoding when the checkpointer
					// adapter serializes the whole payload.
					if lastState != nil {
						if ls, err := json.Marshal(lastState); err == nil {
							cpPayload["__last_state__"] = string(ls)
						}
					}
					// Extract sub-state from GraphInterrupt value.
					for _, r := range results {
						if gi, ok := r.Err.(*errors.GraphInterrupt); ok && len(gi.Interrupts) > 0 {
							if intr, ok := gi.Interrupts[0].(*types.Interrupt); ok && intr.Value != nil {
								if b, e := json.Marshal(intr.Value); e == nil {
									cpPayload["__sub_state__"] = b
								}
							}
							break
						}
					}
					if err := e.checkpointer.Put(ctx, map[string]any{
						constants.ConfigKeyThreadID: threadID,
					}, cpPayload); err != nil {
						errCh <- fmt.Errorf("failed to save checkpoint on interrupt: %w", err)
						return
					}
				}
				streamManager.EmitInterrupt(step, interruptTaskNames)
				// Dispatch Interrupt callback.
				if e.callbacks != nil {
					e.callbacks.Interrupt(ctx, interruptTaskNames, step)
				}
				// Preserve the first interrupted task's GraphInterrupt value
				// (with Interrupts populated) instead of creating a bare one,
				// so MustExtractInterruptContexts can extract the original
				// UserFillUp spec / tips / cpn_id from it.
				for _, r := range results {
					if gi, ok := r.Err.(*errors.GraphInterrupt); ok && len(gi.Interrupts) > 0 {
						errCh <- gi
						return
					}
				}
				errCh <- &errors.GraphInterrupt{}
				return
			}
			// If every task in this step failed, the graph cannot make progress.
			// Terminate immediately rather than infinitely re-scheduling the
			// same failing nodes (e.g. a panicking node caught by recover()).
			if allFailed {
				var why string
				for _, r := range results {
					why += fmt.Sprintf(" %s=%T(%v)", r.Name, r.Err, r.Err)
				}
				common.Debug("allFailed",
					zap.Int("step", step),
					zap.String("results", why))
				errCh <- fmt.Errorf("all %d tasks failed in step %d: %s", len(results), step, why)
				return
			}

			// Apply writes to channels
			_, err = e.applyWrites(channelRegistry, results, triggers)
			if err != nil {
				errCh <- fmt.Errorf("failed to apply writes: %w", err)
				return
			}

			// Emit values event
			if values, err := channelRegistry.GetValues(); err == nil {
				streamManager.EmitValues(step, values)
			}

			// Save checkpoint based on durability mode
			if e.checkpointer != nil {
				checkpoint := channelRegistry.CreateCheckpoint()
				checkpointID := uuid.New().String()

				switch e.config.Durability {
				case types.DurabilitySync:
					// Synchronous save - block until complete
					if err := e.saveCheckpoint(ctx, threadID, checkpointID, step, checkpoint); err != nil {
						errCh <- fmt.Errorf("failed to save checkpoint: %w", err)
						return
					}
					// Dispatch CheckpointSave callback.
					if e.callbacks != nil {
						e.callbacks.CheckpointSave(ctx, threadID, checkpointID, step)
					}
				case types.DurabilityAsync:
					// Asynchronous save - don't block next step
					go func(cp map[string]any, cpID string, s int) {
						if err := e.saveCheckpoint(context.Background(), threadID, cpID, s, cp); err != nil {
							// Log async error but don't fail execution
							common.Error("async checkpoint save failed", err, zap.String("thread_id", threadID), zap.String("checkpoint_id", cpID), zap.Int("step", s))
						}
					}(checkpoint, checkpointID, step)
				case types.DurabilityExit:
					// Defer save until exit - accumulate checkpoints in memory
					// Will be saved in final state
					e.deferCheckpoint(threadID, checkpointID, step, checkpoint)
					// Dispatch CheckpointSave callback (deferred save still counts as saved).
					if e.callbacks != nil {
						e.callbacks.CheckpointSave(ctx, threadID, checkpointID, step)
					}
				default:
					// Default to sync behavior
					if err := e.saveCheckpoint(ctx, threadID, checkpointID, step, checkpoint); err != nil {
						errCh <- fmt.Errorf("failed to save checkpoint: %w", err)
						return
					}
				}
			}

			// Check for after-node interrupts. The checkpoint above already
			// captures this step's output.
			if e.shouldInterruptAfter(results) {
				if e.callbacks != nil {
					e.callbacks.Interrupt(ctx, []string{"after_node"}, step)
				}
				errCh <- &errors.GraphInterrupt{}
				return
			}

			// Dispatch StepEnd callback.
			if e.callbacks != nil {
				e.callbacks.StepEnd(ctx, step, nil)
			}

			step++
		}

		// Get final state
		finalState, err := e.buildOutput(channelRegistry, lastState)
		if err != nil {
			errCh <- fmt.Errorf("failed to build output: %w", err)
			return
		}

		// Save deferred checkpoints for DurabilityExit mode
		if e.config.Durability == types.DurabilityExit {
			if err := e.saveDeferredCheckpoints(ctx); err != nil {
				errCh <- fmt.Errorf("failed to save deferred checkpoints: %w", err)
				return
			}
		}

		// Emit final event
		streamManager.EmitFinal(step, finalState)
	}()

	return outputCh, errCh
}

// prepareNextTasks determines which tasks to execute next.
// This is the standard version that prepares tasks for execution.
func (e *Engine) prepareNextTasks(
	ctx context.Context,
	registry *channels.Registry,
	completedTasks map[string]bool,
	lastCompletedNode string,
	currentState any,
) ([]*Task, map[string]struct{}, error) {
	return e.prepareNextTasksWithMode(ctx, registry, completedTasks, lastCompletedNode, currentState, true)
}

// prepareNextTasksWithMode determines which tasks to execute next with for_execution mode.
// When forExecution is true, tasks are prepared for actual execution.
// When forExecution is false, only task information is prepared (for inspection/planning).
//
// In AllPredecessor (DAG) mode, a node is triggered only when ALL of its incoming edges'
// source nodes have completed. In AnyPredecessor (Pregel/BSP) mode (default), a node is
// triggered when any predecessor completes. AllPredecessor does not support cycles.
func (e *Engine) prepareNextTasksWithMode(
	ctx context.Context,
	registry *channels.Registry,
	completedTasks map[string]bool,
	lastCompletedNode string,
	currentState any,
	forExecution bool,
) ([]*Task, map[string]struct{}, error) {
	tasks := make([]*Task, 0)
	triggerToNodes := make(map[string]struct{})

	// If this is the first step
	if len(completedTasks) == 0 {
		entryPoint := e.getEntryPoint()
		if entryPoint == "" {
			return nil, nil, fmt.Errorf("no entry point set")
		}

		// Handle direct edge Start → End (empty/trivial graph)
		if entryPoint == constants.End {
			return tasks, triggerToNodes, nil
		}

		node := e.getNode(entryPoint)
		if node == nil {
			return nil, nil, &errors.NodeNotFoundError{NodeName: entryPoint}
		}

		// Pass node Triggers as task Channels so the first task reads from
		// registered channels rather than receiving a nil state.
		// When the entry point has no explicit triggers, use all registered
		// channel names so it can read the initial input values. This is
		// needed by systems (e.g. canvas) that route data via context
		// rather than channels but still register input channels for
		// the engine's input validation.
		triggers := e.getTriggers(node)
		if len(triggers) == 0 {
			triggers = registry.Names()
		}
		task := e.createTask(node, currentState, triggers, []string{})
		tasks = append(tasks, task)
		triggerToNodes["__start__"] = struct{}{}
		return tasks, triggerToNodes, nil
	}

	// AllPredecessor (DAG) mode: scan all uncompleted nodes and check if
	// ALL of their incoming-edge source nodes have completed.
	if e.graph.GetNodeTriggerMode() == types.NodeTriggerAllPredecessor {
		return e.prepareNextTasksDAG(completedTasks, currentState, forExecution)
	}

	// AnyPredecessor (Pregel/BSP) mode: determine next nodes from the
	// last completed node's outgoing edges.
	nextNodes := e.getNextNodes(ctx, lastCompletedNode, currentState)

	for nodeName := range nextNodes {
		node := e.getNode(nodeName)
		if node == nil {
			continue
		}

		// Determine triggers for this node
		triggers := e.getTriggers(node)
		if len(triggers) == 0 {
			triggers = registry.Names()
		}

		// BSP mode: always schedule, even if previously completed (supports loops).
		var task *Task
		if forExecution {
			task = e.createTask(node, currentState, triggers, []string{})
		} else {
			task = e.createTaskInfo(node, currentState, triggers, []string{})
		}
		tasks = append(tasks, task)

		// Build trigger to nodes mapping
		for _, trigger := range triggers {
			triggerToNodes[trigger] = struct{}{}
		}
	}

	return tasks, triggerToNodes, nil
}

// prepareNextTasksDAG prepares tasks in DAG (AllPredecessor) mode.
// It scans all nodes and schedules those whose incoming-edge sources
// have all completed. This is O(n) per call but correct for fan-in patterns.
func (e *Engine) prepareNextTasksDAG(
	completedTasks map[string]bool,
	currentState any,
	forExecution bool,
) ([]*Task, map[string]struct{}, error) {
	tasks := make([]*Task, 0)
	triggerToNodes := make(map[string]struct{})

	// Build reverse adjacency: for each node, which nodes have edges TO it.
	incomingEdges := e.buildIncomingEdges()

	for _, node := range e.graph.GetNodes() {
		n := e.getNode(node.Name)
		if n == nil {
			continue
		}
		if completedTasks[node.Name] {
			continue
		}

		// Check if all incoming-edge sources have completed.
		predecessors := incomingEdges[node.Name]
		allDone := true
		for _, pred := range predecessors {
			// constants.Start and constants.End are always considered completed.
			if pred == constants.Start || pred == constants.End {
				continue
			}
			if !completedTasks[pred] {
				allDone = false
				break
			}
		}
		// Nodes with no incoming edges (beyond start) can run.
		if !allDone {
			continue
		}

		triggers := e.getTriggers(n)
		if len(triggers) == 0 {
			chMap := e.graph.GetChannels()
			triggers = make([]string, 0, len(chMap))
			for name := range chMap {
				triggers = append(triggers, name)
			}
		}
		var task *Task
		if forExecution {
			task = e.createTask(n, currentState, triggers, []string{})
		} else {
			task = e.createTaskInfo(n, currentState, triggers, []string{})
		}
		tasks = append(tasks, task)
		for _, trigger := range triggers {
			triggerToNodes[trigger] = struct{}{}
		}
	}

	// No tasks means all reachable nodes are done.
	return tasks, triggerToNodes, nil
}

// buildIncomingEdges builds a reverse-adjacency map: node → list of nodes with edges TO it.
func (e *Engine) buildIncomingEdges() map[string][]string {
	adj := make(map[string][]string)
	for _, edge := range e.graph.GetEdges() {
		adj[edge.To] = append(adj[edge.To], edge.From)
	}
	return adj
}

// shouldInterrupt checks if graph should be interrupted.
func (e *Engine) shouldInterrupt(
	registry *channels.Registry,
	tasks []*Task,
	triggerToNodes map[string]struct{},
) []*Task {
	interrupted := make([]*Task, 0)

	if len(e.interrupts) == 0 {
		return interrupted
	}

	interruptAll := e.interrupts[types.All]

	for _, task := range tasks {
		if interruptAll || e.interrupts[task.Name] {
			interrupted = append(interrupted, task)
		}
	}

	return interrupted
}

// shouldInterruptAfter checks if any SUCCESSFULLY completed task's node name
// is in interruptsAfter. Called AFTER execution and checkpoint save so the
// checkpoint already captures the node's output.
func (e *Engine) shouldInterruptAfter(results []*TaskResult) bool {
	if len(e.interruptsAfter) == 0 {
		return false
	}
	interruptAll := e.interruptsAfter[types.All]
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		if interruptAll || e.interruptsAfter[r.Name] {
			return true
		}
	}
	return false
}

// applyWrites applies task outputs to channels with version management and write merging.
func (e *Engine) applyWrites(
	registry *channels.Registry,
	results []*TaskResult,
	triggerToNodes map[string]struct{},
) (map[string]struct{}, error) {
	updatedChannels := make(map[string]struct{})

	// Sort results for deterministic order
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	// Group writes by channel with write merging
	writesByChannel := make(map[string][]any)
	pendingWrites := make(map[string]*checkpoint.PendingWrite)

	for _, result := range results {
		if result.Err != nil {
			continue
		}
		// Skip nil outputs (node returned nil, nil — no state update)
		if result.Output == nil {
			continue
		}

		// Convert output to map of writes
		outputMap, err := toMap(result.Output)
		if err != nil {
			return nil, fmt.Errorf("failed to convert output to map: %w", err)
		}

		// Apply FieldMapping if the node has field-level routing configured.
		if node := e.getNode(result.Name); node != nil && len(node.FieldMapping) > 0 {
			outputMap = applyFieldMapping(outputMap, node.FieldMapping)
		}

		for key, value := range outputMap {
			// Skip nil values
			if value == nil {
				continue
			}

			// Check for Overwrite wrapper
			overwrite := false
			if ow, ok := value.(*types.Overwrite); ok {
				value = ow.Value
				overwrite = true
			}

			// Add to writes
			writesByChannel[key] = append(writesByChannel[key], value)

			// Track pending write
			pendingWrites[key] = &checkpoint.PendingWrite{
				Channel:   key,
				Value:     value,
				Overwrite: overwrite,
				Node:      result.Name,
			}
		}
	}

	// Apply writes to channels with version management
	for channelName, values := range writesByChannel {
		ch, ok := registry.Get(channelName)
		if !ok {
			// Auto-create a LastValue channel for map-based schemas where no
			// channels were pre-configured (e.g. map[string]any{} schema).
			newCh := channels.NewLastValue(nil)
			registry.Register(channelName, newCh)
			ch = newCh
		}

		// Filter out nil values
		filtered := make([]any, 0, len(values))
		for _, val := range values {
			if val != nil {
				filtered = append(filtered, val)
			}
		}

		// When multiple values target a LastValue channel in the same step
		// (star-topology pattern), keep only the last value to avoid channel
		// conflict errors.  BinaryOperatorAggregate and ReducerChannel handle
		// multiple writes via their accumulator logic.
		if len(filtered) > 1 {
			_, isBO := ch.(*channels.BinaryOperatorAggregate)
			_, isRC := ch.(*channels.ReducerChannel)
			if !isBO && !isRC {
				last := filtered[len(filtered)-1]
				filtered = filtered[:1]
				filtered[0] = last
			}
		}

		// Update channel
		updated, err := ch.Update(filtered)
		if err != nil {
			return nil, fmt.Errorf("failed to update channel %s: %w", channelName, err)
		}

		if updated && ch.IsAvailable() {
			updatedChannels[channelName] = struct{}{}

			// Increment channel version (engine-level tracking).
			e.channelVersions[channelName]++

			// Also bump the version on the channel itself for ChannelChangedTrigger.
			if vc, ok := ch.(interface{ SetVersion(int) }); ok {
				vc.SetVersion(e.channelVersions[channelName])
			}

			// Update checkpoint if available
			if e.currentCheckpoint != nil {
				e.currentCheckpoint.IncrementChannel(channelName)
			}
		}
	}

	// Store pending writes to checkpoint
	if e.currentCheckpoint != nil {
		for _, pw := range pendingWrites {
			e.currentCheckpoint.AddPendingWrite(pw.Channel, pw.Value, pw.Overwrite, pw.Node)
		}
	}

	// Mark channels as seen by nodes
	for resultName := range writesByChannel {
		if _, ok := triggerToNodes[resultName]; ok {
			for channelName := range updatedChannels {
				e.markSeen(resultName, channelName)
			}
		}
	}

	return updatedChannels, nil
}

// markSeen marks that a node has seen a channel's version.
func (e *Engine) markSeen(node, channel string) {
	if e.versionsSeen[node] == nil {
		e.versionsSeen[node] = make(map[string]int)
	}
	e.versionsSeen[node][channel] = e.channelVersions[channel]

	if e.currentCheckpoint != nil {
		e.currentCheckpoint.MarkSeen(node, channel)
	}
}

// hasSeen checks if a node has seen a channel's current version.
func (e *Engine) hasSeen(node, channel string) bool {
	if versions, ok := e.versionsSeen[node]; ok {
		if version, ok := versions[channel]; ok {
			return version == e.channelVersions[channel]
		}
	}
	return false
}

// executeTasks executes the given tasks concurrently.
func (e *Engine) executeTasks(
	ctx context.Context,
	tasks []*Task,
	registry *channels.Registry,
) ([]*TaskResult, error) {
	results := make([]*TaskResult, len(tasks))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t *Task) {
			defer wg.Done()

			result := e.executeTask(ctx, t, registry)

			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, task)
	}

	wg.Wait()

	return results, nil
}

// executeTasksAsync executes tasks using async pipeline with streaming.
func (e *Engine) executeTasksAsync(
	ctx context.Context,
	tasks []*Task,
	registry *channels.Registry,
	asyncPipeline *AsyncPipeline,
	streamManager *StreamManager,
	step int,
) ([]*TaskResult, error) {
	results := make([]*TaskResult, len(tasks))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t *Task) {
			defer wg.Done()

			// Read input for this task
			input, err := e.readTaskInput(registry, t)
			if err != nil {
				mu.Lock()
				results[idx] = &TaskResult{
					Name: t.Name,
					Err:  fmt.Errorf("failed to read task input: %w", err),
				}
				mu.Unlock()
				return
			}

			// Convert map input to struct type if state schema is a struct
			convertedInput := e.mapToStateSchema(input)

			// Define the function to execute
			executeFn := func(ctx context.Context) (any, error) {
				return t.Func(ctx, convertedInput)
			}

			// Use task's retry policy or default
			retryPolicy := t.RetryPolicy
			if retryPolicy == nil {
				defaultPolicy := types.DefaultRetryPolicy()
				retryPolicy = &defaultPolicy
			}

			// Execute with async pipeline
			resultCh := asyncPipeline.ExecuteNode(ctx, t.Name, executeFn, &RetryConfig{Policy: retryPolicy})

			// Wait for result
			select {
			case <-ctx.Done():
				mu.Lock()
				results[idx] = &TaskResult{
					Name: t.Name,
					Err:  ctx.Err(),
				}
				mu.Unlock()
			case asyncResult, ok := <-resultCh:
				if !ok {
					mu.Lock()
					results[idx] = &TaskResult{
						Name: t.Name,
						Err:  fmt.Errorf("async result channel closed unexpectedly"),
					}
					mu.Unlock()
					return
				}

				// Convert async result to task result
				taskResult := &TaskResult{
					Name:   t.Name,
					Output: asyncResult.Output,
					Err:    asyncResult.Err,
				}

				// Emit task end event
				streamManager.EmitTaskEnd(step, t.Name, t.ID, asyncResult.Output, asyncResult.Duration, asyncResult.Err)

				// Emit update event if successful
				if asyncResult.Err == nil {
					streamManager.EmitUpdate(step, t.Name, asyncResult.Output)
				} else {
					// Emit error event
					streamManager.EmitError(step, asyncResult.Err, t.Name)
				}

				mu.Lock()
				results[idx] = taskResult
				mu.Unlock()
			}
		}(i, task)
	}

	wg.Wait()
	return results, nil
}

// executeTask executes a single task with retry logic.
func (e *Engine) executeTask(
	ctx context.Context,
	task *Task,
	registry *channels.Registry,
) *TaskResult {
	// Read input for this task
	input, err := e.readTaskInput(registry, task)
	if err != nil {
		return &TaskResult{
			Name: task.Name,
			Err:  fmt.Errorf("failed to read task input: %w", err),
		}
	}

	// Convert map input to struct type if the state schema is a struct
	input = e.mapToStateSchema(input)

	// Use RetryExecutor for retry logic
	retryPolicy := task.RetryPolicy
	if retryPolicy == nil {
		defaultPolicy := types.DefaultRetryPolicy()
		retryPolicy = &defaultPolicy
	}

	retryExecutor := NewRetryExecutor(retryPolicy)

	// Define the function to execute
	executeFn := func(ctx context.Context) (any, error) {
		return task.Func(ctx, input)
	}

	// Execute with retry
	output, err := retryExecutor.Execute(ctx, task.Name, executeFn)
	if err != nil {
		// Check if it's a retry exhausted error
		if IsRetryExhausted(err) {
			return &TaskResult{
				Name: task.Name,
				Err:  fmt.Errorf("max retries exceeded: %w", err),
			}
		}
		// Check for interrupt
		if errors.IsGraphInterrupt(err) {
			return &TaskResult{
				Name: task.Name,
				Err:  err,
			}
		}
		// Other errors
		return &TaskResult{
			Name: task.Name,
			Err:  err,
		}
	}

	// Success
	return &TaskResult{
		Name:   task.Name,
		Output: output,
		Err:    nil,
	}
}

// readTaskInput reads the input for a task from channels.
// mapToStateSchema converts a map[string]any state to the graph's state schema
// type if it is a struct (or pointer to struct). If the schema is a map or
// nil, the map input is returned as-is.
func (e *Engine) mapToStateSchema(input any) any {
	if input == nil {
		return nil
	}
	inputMap, ok := input.(map[string]any)
	if !ok {
		return input
	}

	schema := e.graph.GetStateSchema()
	if schema == nil {
		return inputMap
	}

	rv := reflect.ValueOf(schema)
	for rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return inputMap
	}

	// State schema is a struct (possibly wrapped in pointer): create a new
	// instance and populate fields from the input map.
	// Preserve whether the original schema was a pointer or value.
	schemaVal := reflect.ValueOf(schema)
	isPtr := schemaVal.Kind() == reflect.Ptr
	structType := rv.Type() // underlying struct type
	structPtr := reflect.New(structType)
	structVal := structPtr.Elem()

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if val, exists := inputMap[field.Name]; exists {
			fv := structVal.Field(i)
			if fv.CanSet() {
				rvVal := reflect.ValueOf(val)
				if rvVal.Type().AssignableTo(fv.Type()) {
					fv.Set(rvVal)
				} else if rvVal.Type().ConvertibleTo(fv.Type()) {
					fv.Set(rvVal.Convert(fv.Type()))
				}
			}
		}
	}

	if isPtr {
		return structPtr.Interface() // *StructType
	}
	return structVal.Interface() // StructType (value)
}

func (e *Engine) readTaskInput(registry *channels.Registry, task *Task) (any, error) {
	if len(task.Channels) == 0 {
		// Return empty map instead of nil so that node functions expecting
		// map[string]any receive a usable zero value rather than nil.
		return map[string]any{}, nil
	}

	// Read values from specified channels
	values := make(map[string]any)
	for _, channelName := range task.Channels {
		if ch, ok := registry.Get(channelName); ok {
			value, err := ch.Get()
			if err != nil {
				if _, isEmpty := err.(*errors.EmptyChannelError); !isEmpty {
					return nil, err
				}
				// Empty channels are OK
				continue
			}
			values[channelName] = value
		}
	}

	return values, nil
}

// Task represents a task to execute.
type Task struct {
	ID          string
	Name        string
	Func        types.NodeFunc
	Channels    []string
	Path        []string
	Triggers    map[string]struct{}
	RetryPolicy *types.RetryPolicy
}

// TaskResult represents the result of executing a task.
type TaskResult struct {
	Name   string
	Output any
	Err    error
	Path   []string // Task path for deterministic ordering (like Python's task_path)
}

// TaskPathStr generates a deterministic string representation of the task path.
// This corresponds to Python's task_path_str function in _algo.py
func TaskPathStr(path []string) string {
	if len(path) == 0 {
		return ""
	}
	// Join path components with separator for deterministic ordering
	return strings.Join(path, "/")
}

// ParseTaskPath parses a task path string back into a path array.
func ParseTaskPath(pathStr string) []string {
	if pathStr == "" {
		return []string{}
	}
	return strings.Split(pathStr, "/")
}

// BuildTaskPath builds a task path from components.
// Supports nested paths like Python's tuple-based paths.
func BuildTaskPath(components ...any) []string {
	path := make([]string, 0, len(components))
	for _, comp := range components {
		switch val := comp.(type) {
		case string:
			path = append(path, val)
		case int:
			path = append(path, fmt.Sprintf("%d", val))
		case []string:
			path = append(path, val...)
		default:
			if stringer, ok := val.(fmt.Stringer); ok {
				path = append(path, stringer.String())
			} else {
				path = append(path, fmt.Sprintf("%v", val))
			}
		}
	}
	return path
}

// Helper methods that access the StateGraph
func (e *Engine) getGraphChannels() map[string]channels.Channel {
	raw := e.graph.GetChannels()
	result := make(map[string]channels.Channel, len(raw))
	for k, v := range raw {
		if ch, ok := v.(channels.Channel); ok {
			result[k] = ch
		}
	}
	return result
}

func (e *Engine) getEntryPoint() string {
	return e.graph.GetEntryPoint()
}

func (e *Engine) getNode(name string) *types.Node {
	node, _ := e.graph.GetNode(name)
	return node
}

func (e *Engine) getNextNodes(ctx context.Context, node string, state any) map[string]bool {
	common.Debug("getNextNodes",
		zap.String("node", node),
		zap.Any("state", state))
	nextNodes := make(map[string]bool)

	// (1) Check conditional edges.  When a node has conditional edges,
	// ONLY the matched target(s) are scheduled — the regular-edge
	// fallback is skipped entirely so branchable nodes (Switch,
	// Categorize) route exclusively via the _next value.
	hasConditional := false
	for _, condEdge := range e.graph.GetConditionalEdges() {
		if condEdge.From != node {
			continue
		}
		hasConditional = true
		conditionResult, err := condEdge.Condition(ctx, state)
		if err != nil {
			common.Debug("conditional edge failed", zap.String("from", node), zap.Error(err))
		}
		conditionKey := fmt.Sprintf("%v", conditionResult)
		targetNode, ok := condEdge.Mapping[conditionKey]
		if !ok {
			continue
		}
		if targetNode == constants.End {
			return nextNodes
		}
		nextNodes[targetNode] = true
	}

	// (2) Regular edges: ONLY when this node has no conditional edges.
	if !hasConditional && len(nextNodes) == 0 {
		for _, edge := range e.graph.GetEdges() {
			if edge.From == node {
				if edge.To == constants.End {
					return nextNodes
				}
				nextNodes[edge.To] = true
			}
		}
	}

	// (3) Resume fallback: when the last completed node has no outgoing
	// edges but the graph state contains _next (persisted from a
	// Switch/Categorize branch), route directly from _next.  This
	// happens on checkpoint resume because the conditional edge is
	// registered on the Switch node, not on __loop_init__.
	if len(nextNodes) == 0 {
		if st, ok := state.(map[string]any); ok {
			if raw, has := st["_next"]; has && raw != nil {
				switch tv := raw.(type) {
				case string:
					if _, exists := e.graph.GetNode(tv); exists {
						nextNodes[tv] = true
					}
				case []any:
					if len(tv) > 0 {
						if str, ok := tv[0].(string); ok {
							if _, exists := e.graph.GetNode(str); exists {
								nextNodes[str] = true
							}
						}
					}
				}
			}
		}
	}

	// (4) Branches: always included on top of whatever was scheduled.
	for _, branch := range e.graph.GetBranches() {
		if branch.From == node {
			branchResult, err := branch.Condition(ctx, state)
			if err != nil {
				continue
			}
			targets := branch.Then(branchResult)
			for _, target := range targets {
				if target == constants.End {
					continue
				}
				nextNodes[target] = true
			}
		}
	}

	return nextNodes
}

func (e *Engine) getTriggers(node *types.Node) []string {
	if node == nil {
		return []string{}
	}
	return node.Triggers
}

func (e *Engine) createTask(node *types.Node, state any, channels []string, triggers []string) *Task {
	task := &Task{
		ID:       uuid.New().String(),
		Name:     node.Name,
		Channels: channels,
		Triggers: make(map[string]struct{}),
	}
	if node.Function != nil {
		task.Func = node.Function
	}
	for _, trigger := range triggers {
		task.Triggers[trigger] = struct{}{}
	}
	return task
}

// createTaskInfo creates a task info object for inspection/planning (for_execution=false mode).
// This is similar to Python's prepare_next_tasks with for_execution=False.
func (e *Engine) createTaskInfo(node *types.Node, state any, channels []string, triggers []string) *Task {
	task := &Task{
		ID:       uuid.New().String(),
		Name:     node.Name,
		Channels: channels,
		Triggers: make(map[string]struct{}),
		Func:     nil,
	}
	for _, trigger := range triggers {
		task.Triggers[trigger] = struct{}{}
	}
	return task
}

// PrepareNextTasksForInspection prepares tasks for inspection/planning only (for_execution=false).
// This corresponds to Python's prepare_next_tasks with for_execution=False.
func (e *Engine) PrepareNextTasksForInspection(
	ctx context.Context,
	registry *channels.Registry,
	completedTasks map[string]bool,
	lastCompletedNode string,
	currentState any,
) ([]*Task, map[string]struct{}, error) {
	return e.prepareNextTasksWithMode(ctx, registry, completedTasks, lastCompletedNode, currentState, false)
}

func (e *Engine) applyInput(registry *channels.Registry, input any) error {
	inputMap, err := toMap(input)
	if err != nil {
		return err
	}

	// Auto-create channels for any input keys not yet registered, then write.
	for key, value := range inputMap {
		if _, ok := registry.Get(key); ok {
			continue
		}
		guessed := caseFoldKey(registry, key)
		if guessed != "" {
			delete(inputMap, key)
			inputMap[guessed] = value
		} else {
			registry.Register(key, channels.NewLastValue(value))
		}
	}

	writes := make(map[string][]any, len(inputMap))
	for key, value := range inputMap {
		writes[key] = []any{value}
	}

	if len(writes) > 0 {
		return registry.UpdateChannels(writes)
	}
	return nil
}

// caseFoldKey attempts to locate a registered channel whose name differs from
// key only by the case of the first character (e.g. struct field "Counter" vs
// input map key "counter").  Returns the matched channel name, or "".
func caseFoldKey(registry *channels.Registry, key string) string {
	if len(key) == 0 {
		return ""
	}
	// Try uppercase first (e.g. "counter" → "Counter")
	bs := []byte(key)
	if bs[0] >= 'a' && bs[0] <= 'z' {
		bs[0] -= 32
		candidate := string(bs)
		if _, ok := registry.Get(candidate); ok {
			return candidate
		}
	}
	// Try lowercase first (e.g. "Counter" → "counter")
	bs[0] = key[0]
	if bs[0] >= 'A' && bs[0] <= 'Z' {
		bs[0] += 32
		candidate := string(bs)
		if _, ok := registry.Get(candidate); ok {
			return candidate
		}
	}
	return ""
}

func (e *Engine) getThreadID() string {
	if e.config != nil && e.config.Configurable != nil {
		if tid, ok := e.config.Configurable["thread_id"].(string); ok {
			return tid
		}
	}
	return uuid.New().String()
}

func (e *Engine) buildOutput(registry *channels.Registry, lastState any) (any, error) {
	values, err := registry.GetValues()
	if err != nil {
		return lastState, nil
	}

	if len(values) > 0 {
		return values, nil
	}

	return lastState, nil
}

func (e *Engine) mergeStates(existing, next any) any {
	if existing == nil {
		return next
	}

	if next == nil {
		return existing
	}

	// Try to merge maps
	existingMap, ok1 := existing.(map[string]any)
	nextMap, ok2 := next.(map[string]any)

	if ok1 && ok2 {
		result := make(map[string]any)
		for key, val := range existingMap {
			result[key] = val
		}
		for key, val := range nextMap {
			result[key] = val
		}
		return result
	}

	return next
}

// toMap converts a struct or map to a map[string]any.
func toMap(val any) (map[string]any, error) {
	if val == nil {
		return nil, fmt.Errorf("nil value")
	}

	// If it's already a map
	if m, ok := val.(map[string]any); ok {
		return m, nil
	}

	// Use reflection to convert struct to map
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	if rv.Kind() != reflect.Struct && rv.Kind() != reflect.Map {
		return map[string]any{"__root__": val}, nil
	}

	result := make(map[string]any)

	if rv.Kind() == reflect.Map {
		for _, key := range rv.MapKeys() {
			result[fmt.Sprintf("%v", key.Interface())] = rv.MapIndex(key).Interface()
		}
		return result, nil
	}

	// Struct
	rt := rv.Type()
	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}
		val := rv.Field(i).Interface()

		// Use original field name to match channel registration
		// (configureChannelsFromSchema registers channels with field.Name).
		result[field.Name] = val
	}

	return result, nil
}

// saveCheckpoint saves a checkpoint to the checkpointer.
func (e *Engine) saveCheckpoint(ctx context.Context, threadID, checkpointID string, step int, checkpoint map[string]any) error {
	if e.checkpointer == nil {
		return nil
	}
	return e.checkpointer.Put(ctx, map[string]any{
		constants.ConfigKeyThreadID:     threadID,
		constants.ConfigKeyCheckpointID: checkpointID,
		"step":                          step,
	}, checkpoint)
}

// deferCheckpoint defers a checkpoint save for DurabilityExit mode.
func (e *Engine) deferCheckpoint(threadID, checkpointID string, step int, checkpoint map[string]any) {
	e.deferredCheckpoints = append(e.deferredCheckpoints, deferredCheckpoint{
		ThreadID:     threadID,
		CheckpointID: checkpointID,
		Step:         step,
		Checkpoint:   checkpoint,
	})
}

// saveDeferredCheckpoints saves all deferred checkpoints (called at exit for DurabilityExit mode).
func (e *Engine) saveDeferredCheckpoints(ctx context.Context) error {
	if e.checkpointer == nil || len(e.deferredCheckpoints) == 0 {
		return nil
	}

	var lastErr error
	for _, dc := range e.deferredCheckpoints {
		if err := e.saveCheckpoint(ctx, dc.ThreadID, dc.CheckpointID, dc.Step, dc.Checkpoint); err != nil {
			lastErr = err
			// Continue saving other checkpoints even if one fails
		}
	}

	// Clear deferred checkpoints after attempting to save
	e.deferredCheckpoints = nil
	return lastErr
}

// RunSync executes the graph synchronously and returns the final state.
// This is a convenience wrapper around Run() for callers that want a blocking API.
//
// RunSync first drains all events from outputCh (reading until it is closed),
// then checks errCh for any execution error. This ordering avoids a race
// between the EventTypeFinal arriving on outputCh and errCh being closed
// (the defer calling close(errCh) runs AFTER close(outputCh)).
func (e *Engine) RunSync(ctx context.Context, input any) (any, error) {
	outputCh, errCh := e.Run(ctx, input, types.StreamModeValues)
	var finalState any

	// Drain outputCh to capture the final state event.
	// Must read until closed to avoid leaking the forward goroutine.
	for result := range outputCh {
		if se, ok := result.(*StreamEvent); ok && se.Type == EventTypeFinal {
			if data, ok := se.Data.(map[string]any); ok {
				if state, ok := data["state"]; ok {
					finalState = state
				}
			}
		}
	}

	// Check for execution errors (non-blocking; errCh is closed after outputCh).
	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	default:
	}

	return finalState, nil
}

// applyFieldMapping filters and remaps an output map according to FieldMapping rules.
// If no mappings are specified, the entire output map is passed through unchanged.
// Each mapping specifies a source field path (From) and a target field path (To).
func applyFieldMapping(output map[string]any, mappings []types.FieldMapping) map[string]any {
	if len(mappings) == 0 {
		return output
	}
	result := make(map[string]any, len(mappings))
	for _, mapping := range mappings {
		val := getNestedField(output, mapping.From)
		if val != nil {
			setNestedField(result, mapping.To, val)
		}
	}
	return result
}

// getNestedField retrieves a value from a nested map using a dot-separated path.
func getNestedField(m map[string]any, path string) any {
	if path == "" {
		return m // return entire map
	}
	parts := strings.Split(path, ".")
	var cur any = m
	for _, part := range parts {
		cm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = cm[part]
		if cur == nil {
			return nil
		}
	}
	return cur
}

// setNestedField sets a value in a nested map using a dot-separated path.
func setNestedField(m map[string]any, path string, val any) {
	if path == "" {
		for k, v := range val.(map[string]any) {
			m[k] = v
		}
		return
	}
	parts := strings.Split(path, ".")
	for i := 0; i < len(parts)-1; i++ {
		sub, ok := m[parts[i]]
		if !ok {
			sub = make(map[string]any)
			m[parts[i]] = sub
		}
		var ok2 bool
		m, ok2 = sub.(map[string]any)
		if !ok2 {
			nm := make(map[string]any)
			m[parts[i]] = nm
			m = nm
		}
	}
	m[parts[len(parts)-1]] = val
}

// serializeStringSet encodes a map[string]bool to a NUL-separated string
// for storage in the checkpoint payload.
func serializeStringSet(set map[string]bool) string {
	if len(set) == 0 {
		return ""
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]byte, 0, 256)
	for i, key := range keys {
		if i > 0 {
			out = append(out, 0)
		}
		out = append(out, key...)
	}
	return string(out)
}

// deserializeStringSet decodes a NUL-separated string back to a
// map[string]bool.
func deserializeStringSet(encoded string) map[string]bool {
	if encoded == "" {
		return nil
	}
	parts := strings.Split(encoded, "\x00")
	out := make(map[string]bool, len(parts))
	for _, part := range parts {
		out[part] = true
	}
	return out
}
