// Package pregel provides the Pregel execution algorithm for graph processing.
package pregel

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/graph"
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
	graph               *graph.StateGraph
	checkpointer        graph.Checkpointer
	interrupts          map[string]bool
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
	deferredCheckpoints []deferredCheckpoint // for DurabilityExit mode
}

// deferredCheckpoint stores checkpoint data for deferred saving (DurabilityExit mode)
type deferredCheckpoint struct {
	ThreadID    string
	CheckpointID string
	Step        int
	Checkpoint  map[string]interface{}
}

// NewEngine creates a new Pregel engine bound to a StateGraph.
// Options configure checkpointer, recursion limit, concurrency, retry, cache, etc.
//
// The engine is reusable across multiple Run calls. Each call creates its own
// background executor for isolation.
func NewEngine(g *graph.StateGraph, opts ...EngineOption) *Engine {
	eng := &Engine{
		graph:           g,
		interrupts:      make(map[string]bool),
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
func WithCheckpointer(cp graph.Checkpointer) EngineOption {
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

// ExecuteResult represents the result of graph execution.
type ExecuteResult struct {
	// Final state of the graph.
	State interface{}
	// Checkpoint ID for this execution.
	CheckpointID string
	// Metadata about the execution.
	Metadata map[string]interface{}
}

// Run executes the graph using the Pregel algorithm and returns streaming events.
// outputCh yields StreamEvent values (checkpoints, task start/end, state updates,
// and a final event with the complete state). errCh receives a single error on failure
// or nil on clean completion.
//
// The caller MUST read from outputCh until it is closed to prevent goroutine leaks.
// For synchronous execution, use RunSync instead.
func (e *Engine) Run(ctx context.Context, input interface{}, mode types.StreamMode) (<-chan interface{}, <-chan error) {
	outputCh := make(chan interface{}, 100)
	errCh := make(chan error, 1)
	
	go func() {
		defer close(errCh)

		// Create stream manager for event streaming
		streamManager := NewStreamManager(mode, 100)

		// WaitGroup ensures the forward goroutine exits before we close outputCh,
		// preventing a data race between close(outputCh) and outputCh <- event.
		var fwWg sync.WaitGroup
		fwWg.Add(1)

		// Forward stream events to output channel
		go func() {
			defer fwWg.Done()
			for event := range streamManager.Events() {
				select {
				case outputCh <- event:
				case <-ctx.Done():
					return
				}
			}
		}()

		// Deferred cleanup: close streamManager first (unblocks forward goroutine),
		// then wait for forward goroutine to exit, then close outputCh.
		defer func() {
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
		threadID := e.getThreadID()

		// Load checkpoint only when resuming (input == nil).
		// New executions (input != nil) start from scratch — checkpoint is not loaded,
		// preventing state from bleeding across independent runs on the same Engine.
		if input == nil && e.checkpointer != nil {
			cpData, err := e.checkpointer.Get(ctx, map[string]interface{}{
				constants.ConfigKeyThreadID: threadID,
			})
			if err == nil && cpData != nil {
				if err := channelRegistry.RestoreFromCheckpoint(cpData); err != nil {
					errCh <- fmt.Errorf("failed to restore from checkpoint: %w", err)
					return
				}
				// Load checkpoint object
				if cp, err := checkpoint.FromMap(cpData); err == nil {
					e.currentCheckpoint = cp
				}
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
		completedTasks := make(map[string]bool)
		lastCompletedNode := ""
		lastState := input
		
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
			
			// Emit checkpoint event via stream manager
			streamManager.EmitCheckpoint(step, channelRegistry.CreateCheckpoint())
			
			// Determine next tasks
			tasks, triggers, err := e.prepareNextTasks(channelRegistry, completedTasks, lastCompletedNode, lastState)
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
					if err := e.checkpointer.Put(ctx, map[string]interface{}{
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
			for _, result := range results {
				if result.Err == nil {
					allFailed = false
					completedTasks[result.Name] = true
					lastCompletedNode = result.Name
					// Merge result into lastState
					lastState = e.mergeStates(lastState, result.Output)
				}
			}
			// If every task in this step failed, the graph cannot make progress.
			// Terminate immediately rather than infinitely re-scheduling the
			// same failing nodes (e.g. a panicking node caught by recover()).
			if allFailed {
				errCh <- fmt.Errorf("all %d tasks failed in step %d", len(results), step)
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
				case types.DurabilityAsync:
					// Asynchronous save - don't block next step
					go func(cp map[string]interface{}, cpID string, s int) {
						if err := e.saveCheckpoint(context.Background(), threadID, cpID, s, cp); err != nil {
							// Log async error but don't fail execution
							log.Printf("async checkpoint save failed: %v", err)
						}
					}(checkpoint, checkpointID, step)
				case types.DurabilityExit:
					// Defer save until exit - accumulate checkpoints in memory
					// Will be saved in final state
					e.deferCheckpoint(threadID, checkpointID, step, checkpoint)
				default:
					// Default to sync behavior
					if err := e.saveCheckpoint(ctx, threadID, checkpointID, step, checkpoint); err != nil {
						errCh <- fmt.Errorf("failed to save checkpoint: %w", err)
						return
					}
				}
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
	registry *channels.Registry,
	completedTasks map[string]bool,
	lastCompletedNode string,
	currentState interface{},
) ([]*Task, map[string]struct{}, error) {
	return e.prepareNextTasksWithMode(registry, completedTasks, lastCompletedNode, currentState, true)
}

// prepareNextTasksWithMode determines which tasks to execute next with for_execution mode.
// When forExecution is true, tasks are prepared for actual execution.
// When forExecution is false, only task information is prepared (for inspection/planning).
//
// In AllPredecessor (DAG) mode, a node is triggered only when ALL of its incoming edges'
// source nodes have completed. In AnyPredecessor (Pregel/BSP) mode (default), a node is
// triggered when any predecessor completes. AllPredecessor does not support cycles.
func (e *Engine) prepareNextTasksWithMode(
	registry *channels.Registry,
	completedTasks map[string]bool,
	lastCompletedNode string,
	currentState interface{},
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
		triggers := e.getTriggers(node)
		task := e.createTask(node, currentState, triggers, []string{})
		tasks = append(tasks, task)
		triggerToNodes["__start__"] = struct{}{}
		return tasks, triggerToNodes, nil
	}
	
	// AllPredecessor (DAG) mode: scan all uncompleted nodes and check if
	// ALL of their incoming-edge source nodes have completed.
	if e.graph.NodeTriggerMode == types.NodeTriggerAllPredecessor {
		return e.prepareNextTasksDAG(completedTasks, currentState, forExecution)
	}
	
	// AnyPredecessor (Pregel/BSP) mode: determine next nodes from the
	// last completed node's outgoing edges.
	nextNodes := e.getNextNodes(lastCompletedNode, currentState)
	
	for nodeName := range nextNodes {
		node := e.getNode(nodeName)
		if node == nil {
			continue
		}
		
		// Determine triggers for this node
		triggers := e.getTriggers(node)
		
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
	currentState interface{},
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
	
	// Check if any triggered node should interrupt
	if len(e.interrupts) == 0 {
		return interrupted
	}
	
	// Check if "*" is set (interrupt all)
	interruptAll := e.interrupts[types.All]
	
	for _, task := range tasks {
		shouldInterrupt := false
		if interruptAll {
			shouldInterrupt = true
		} else {
			shouldInterrupt = e.interrupts[task.Name]
		}
		
		if shouldInterrupt {
			// Check if this task was triggered by a channel update
			triggered := false
			for trigger := range task.Triggers {
				if _, ok := triggerToNodes[trigger]; ok {
					triggered = true
					break
				}
			}
			
			if triggered {
				interrupted = append(interrupted, task)
			}
		}
	}
	
	return interrupted
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
	writesByChannel := make(map[string][]interface{})
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
		if ch, ok := registry.Get(channelName); ok {
			// Filter out nil values
			filtered := make([]interface{}, 0, len(values))
			for _, v := range values {
				if v != nil {
					filtered = append(filtered, v)
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
			
			// Define the function to execute
			executeFn := func(ctx context.Context) (interface{}, error) {
				return t.Func(ctx, input)
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
	
	// Use RetryExecutor for retry logic
	retryPolicy := task.RetryPolicy
	if retryPolicy == nil {
		defaultPolicy := types.DefaultRetryPolicy()
		retryPolicy = &defaultPolicy
	}
	
	retryExecutor := NewRetryExecutor(retryPolicy)
	
	// Define the function to execute
	executeFn := func(ctx context.Context) (interface{}, error) {
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
func (e *Engine) readTaskInput(registry *channels.Registry, task *Task) (interface{}, error) {
	if len(task.Channels) == 0 {
		return nil, nil
	}
	
	// Read values from specified channels
	values := make(map[string]interface{})
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
	ID         string
	Name       string
	Func       types.NodeFunc
	Channels   []string
	Path       []string
	Triggers   map[string]struct{}
	RetryPolicy *types.RetryPolicy
}

// TaskResult represents the result of executing a task.
type TaskResult struct {
	Name   string
	Output interface{}
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
func BuildTaskPath(components ...interface{}) []string {
	path := make([]string, 0, len(components))
	for _, comp := range components {
		switch v := comp.(type) {
		case string:
			path = append(path, v)
		case int:
			path = append(path, fmt.Sprintf("%d", v))
		case []string:
			path = append(path, v...)
		default:
			if s, ok := v.(fmt.Stringer); ok {
				path = append(path, s.String())
			} else {
				path = append(path, fmt.Sprintf("%v", v))
			}
		}
	}
	return path
}

// Helper methods that access the StateGraph
func (e *Engine) getGraphChannels() map[string]channels.Channel {
	return e.graph.GetChannels()
}

func (e *Engine) getEntryPoint() string {
	return e.graph.GetEntryPoint()
}

func (e *Engine) getNode(name string) *graph.Node {
	n, _ := e.graph.GetNode(name)
	return n
}

func (e *Engine) getNextNodes(node string, state interface{}) map[string]bool {
	nextNodes := make(map[string]bool)

	// Check conditional edges
	for _, condEdge := range e.graph.GetConditionalEdges() {
		if condEdge.From == node {
			conditionResult, err := condEdge.Condition(nil, state)
			if err != nil {
				continue
			}
			conditionKey := fmt.Sprintf("%v", conditionResult)
			targetNode, ok := condEdge.Mapping[conditionKey]
			if !ok {
				continue
			}
			if targetNode == constants.End {
				return nextNodes // Return empty to signal end
			}
			nextNodes[targetNode] = true
		}
	}

	// Check regular edges if no conditional edge was found
	if len(nextNodes) == 0 {
		for _, edge := range e.graph.GetEdges() {
			if edge.From == node {
				if edge.To == constants.End {
					return nextNodes
				}
				nextNodes[edge.To] = true
			}
		}
	}

	// Check branches
	for _, branch := range e.graph.GetBranches() {
		if branch.From == node {
			branchResult, err := branch.Condition(nil, state)
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

func (e *Engine) getTriggers(node *graph.Node) []string {
	if node == nil {
		return []string{}
	}
	return node.Triggers
}

func (e *Engine) createTask(node *graph.Node, state interface{}, channels []string, triggers []string) *Task {
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
func (e *Engine) createTaskInfo(node *graph.Node, state interface{}, channels []string, triggers []string) *Task {
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
	registry *channels.Registry,
	completedTasks map[string]bool,
	lastCompletedNode string,
	currentState interface{},
) ([]*Task, map[string]struct{}, error) {
	return e.prepareNextTasksWithMode(registry, completedTasks, lastCompletedNode, currentState, false)
}

func (e *Engine) applyInput(registry *channels.Registry, input interface{}) error {
	// Convert input to map
	inputMap, err := toMap(input)
	if err != nil {
		return err
	}
	
	// Apply each key to corresponding channel
	writes := make(map[string][]interface{})
	for key, value := range inputMap {
		writes[key] = []interface{}{value}
	}
	
	return registry.UpdateChannels(writes)
}

func (e *Engine) getThreadID() string {
	if e.config != nil && e.config.Configurable != nil {
		if tid, ok := e.config.Configurable["thread_id"].(string); ok {
			return tid
		}
	}
	return uuid.New().String()
}

func (e *Engine) buildOutput(registry *channels.Registry, lastState interface{}) (interface{}, error) {
	values, err := registry.GetValues()
	if err != nil {
		return lastState, nil
	}
	
	if len(values) > 0 {
		return values, nil
	}
	
	return lastState, nil
}

func (e *Engine) mergeStates(existing, new interface{}) interface{} {
	if existing == nil {
		return new
	}
	
	if new == nil {
		return existing
	}
	
	// Try to merge maps
	existingMap, ok1 := existing.(map[string]interface{})
	newMap, ok2 := new.(map[string]interface{})
	
	if ok1 && ok2 {
		result := make(map[string]interface{})
		for k, v := range existingMap {
			result[k] = v
		}
		for k, v := range newMap {
			result[k] = v
		}
		return result
	}
	
	return new
}

// toMap converts a struct or map to a map[string]interface{}.
func toMap(v interface{}) (map[string]interface{}, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value")
	}
	
	// If it's already a map
	if m, ok := v.(map[string]interface{}); ok {
		return m, nil
	}
	
	// Use reflection to convert struct to map
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	
	if rv.Kind() != reflect.Struct && rv.Kind() != reflect.Map {
		return map[string]interface{}{"__root__": v}, nil
	}
	
	result := make(map[string]interface{})
	
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
		value := rv.Field(i).Interface()
		
		// Convert field name to snake_case for consistency
		fieldName := toSnakeCase(field.Name)
		result[fieldName] = value
	}
	
	return result, nil
}

// toSnakeCase converts CamelCase to snake_case.
func toSnakeCase(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// saveCheckpoint saves a checkpoint to the checkpointer.
func (e *Engine) saveCheckpoint(ctx context.Context, threadID, checkpointID string, step int, checkpoint map[string]interface{}) error {
	if e.checkpointer == nil {
		return nil
	}
	return e.checkpointer.Put(ctx, map[string]interface{}{
		constants.ConfigKeyThreadID:     threadID,
		constants.ConfigKeyCheckpointID: checkpointID,
		"step":          step,
	}, checkpoint)
}

// deferCheckpoint defers a checkpoint save for DurabilityExit mode.
func (e *Engine) deferCheckpoint(threadID, checkpointID string, step int, checkpoint map[string]interface{}) {
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
func (e *Engine) RunSync(ctx context.Context, input interface{}) (interface{}, error) {
	outputCh, errCh := e.Run(ctx, input, types.StreamModeValues)
	var finalState interface{}
	for {
		select {
		case result, ok := <-outputCh:
			if !ok {
				return finalState, nil
			}
			// Extract final state from StreamEvent wrapping
			if se, ok := result.(*StreamEvent); ok && se.Type == EventTypeFinal {
				if data, ok := se.Data.(map[string]interface{}); ok {
					if state, ok := data["state"]; ok {
						finalState = state
					}
				}
			}
		case err := <-errCh:
			if err != nil {
				return nil, err
			}
			return finalState, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// applyFieldMapping filters and remaps an output map according to FieldMapping rules.
// If no mappings are specified, the entire output map is passed through unchanged.
// Each mapping specifies a source field path (From) and a target field path (To).
func applyFieldMapping(output map[string]interface{}, mappings []graph.FieldMapping) map[string]interface{} {
	if len(mappings) == 0 {
		return output
	}
	result := make(map[string]interface{}, len(mappings))
	for _, m := range mappings {
		val := getNestedField(output, m.From)
		if val != nil {
			setNestedField(result, m.To, val)
		}
	}
	return result
}

// getNestedField retrieves a value from a nested map using a dot-separated path.
func getNestedField(m map[string]interface{}, path string) interface{} {
	if path == "" {
		return m // return entire map
	}
	parts := strings.Split(path, ".")
	var cur interface{} = m
	for _, part := range parts {
		cm, ok := cur.(map[string]interface{})
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
func setNestedField(m map[string]interface{}, path string, val interface{}) {
	if path == "" {
		for k, v := range val.(map[string]interface{}) {
			m[k] = v
		}
		return
	}
	parts := strings.Split(path, ".")
	for i := 0; i < len(parts)-1; i++ {
		sub, ok := m[parts[i]]
		if !ok {
			sub = make(map[string]interface{})
			m[parts[i]] = sub
		}
		var ok2 bool
		m, ok2 = sub.(map[string]interface{})
		if !ok2 {
			nm := make(map[string]interface{})
			m[parts[i]] = nm
			m = nm
		}
	}
	m[parts[len(parts)-1]] = val
}