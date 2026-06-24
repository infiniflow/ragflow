// Package pregel provides lifecycle callbacks for graph execution.
//
// Callbacks enable instrumentation, logging, and custom hook points
// throughout the Pregel execution lifecycle.
package pregel

import (
	"context"
	"sync"
)

// ---- Callback types ----

// RunCallback is called at the start/end of a full graph run.
type RunCallback interface {
	// OnRunStart is called when a graph run begins.
	OnRunStart(ctx context.Context, graphName string, threadID string)
	// OnRunEnd is called when a graph run completes (or errors).
	OnRunEnd(ctx context.Context, graphName string, threadID string, err error)
}

// StepCallback is called at the start/end of each Pregel superstep.
type StepCallback interface {
	// OnStepStart is called before a superstep begins.
	OnStepStart(ctx context.Context, step int, taskCount int)
	// OnStepEnd is called after a superstep completes.
	OnStepEnd(ctx context.Context, step int, err error)
}

// NodeCallback is called before/after each node execution.
type NodeCallback interface {
	// OnNodeStart is called before a node executes.
	OnNodeStart(ctx context.Context, nodeName string, step int)
	// OnNodeEnd is called after a node completes.
	OnNodeEnd(ctx context.Context, nodeName string, step int, output interface{}, err error)
}

// CheckpointCallback is called when checkpoints are created or loaded.
type CheckpointCallback interface {
	// OnCheckpointSave is called after a checkpoint is saved.
	OnCheckpointSave(ctx context.Context, threadID, checkpointID string, step int)
	// OnCheckpointLoad is called after a checkpoint is loaded.
	OnCheckpointLoad(ctx context.Context, threadID, checkpointID string, step int)
	// OnCheckpointUpdate is called when state is manually updated (UpdateState).
	OnCheckpointUpdate(ctx context.Context, threadID string, asNode string)
}

// InterruptCallback is called when execution is interrupted.
type InterruptCallback interface {
	// OnInterrupt is called when the graph is interrupted.
	OnInterrupt(ctx context.Context, nodeNames []string, step int)
	// OnResume is called when the graph resumes from an interrupt.
	OnResume(ctx context.Context, threadID string)
}

// GraphCallback aggregates all callback interfaces into one.
type GraphCallback interface {
	RunCallback
	StepCallback
	NodeCallback
	CheckpointCallback
	InterruptCallback
}

// ---- Callback manager ----

// CallbackManager manages a collection of callbacks.
// All methods are safe for concurrent use.
type CallbackManager struct {
	mu                  sync.RWMutex
	runCallbacks        []RunCallback
	stepCallbacks       []StepCallback
	nodeCallbacks       []NodeCallback
	checkpointCallbacks []CheckpointCallback
	interruptCallbacks  []InterruptCallback
}

// NewCallbackManager creates a new callback manager.
func NewCallbackManager() *CallbackManager {
	return &CallbackManager{}
}

// AddRunCallback adds a run callback.
func (m *CallbackManager) AddRunCallback(cb RunCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCallbacks = append(m.runCallbacks, cb)
}

// AddStepCallback adds a step callback.
func (m *CallbackManager) AddStepCallback(cb StepCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stepCallbacks = append(m.stepCallbacks, cb)
}

// AddNodeCallback adds a node callback.
func (m *CallbackManager) AddNodeCallback(cb NodeCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeCallbacks = append(m.nodeCallbacks, cb)
}

// AddCheckpointCallback adds a checkpoint callback.
func (m *CallbackManager) AddCheckpointCallback(cb CheckpointCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkpointCallbacks = append(m.checkpointCallbacks, cb)
}

// AddInterruptCallback adds an interrupt callback.
func (m *CallbackManager) AddInterruptCallback(cb InterruptCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interruptCallbacks = append(m.interruptCallbacks, cb)
}

// AddCallback adds a GraphCallback (implements all callback interfaces).
func (m *CallbackManager) AddCallback(cb GraphCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCallbacks = append(m.runCallbacks, cb)
	m.stepCallbacks = append(m.stepCallbacks, cb)
	m.nodeCallbacks = append(m.nodeCallbacks, cb)
	m.checkpointCallbacks = append(m.checkpointCallbacks, cb)
	m.interruptCallbacks = append(m.interruptCallbacks, cb)
}

// ---- Dispatch methods ----

// RunStart dispatches OnRunStart to all run callbacks.
func (m *CallbackManager) RunStart(ctx context.Context, graphName, threadID string) {
	m.mu.RLock()
	cbs := m.runCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnRunStart(ctx, graphName, threadID)
	}
}

// RunEnd dispatches OnRunEnd to all run callbacks.
func (m *CallbackManager) RunEnd(ctx context.Context, graphName, threadID string, err error) {
	m.mu.RLock()
	cbs := m.runCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnRunEnd(ctx, graphName, threadID, err)
	}
}

// StepStart dispatches OnStepStart to all step callbacks.
func (m *CallbackManager) StepStart(ctx context.Context, step, taskCount int) {
	m.mu.RLock()
	cbs := m.stepCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnStepStart(ctx, step, taskCount)
	}
}

// StepEnd dispatches OnStepEnd to all step callbacks.
func (m *CallbackManager) StepEnd(ctx context.Context, step int, err error) {
	m.mu.RLock()
	cbs := m.stepCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnStepEnd(ctx, step, err)
	}
}

// NodeStart dispatches OnNodeStart to all node callbacks.
func (m *CallbackManager) NodeStart(ctx context.Context, nodeName string, step int) {
	m.mu.RLock()
	cbs := m.nodeCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnNodeStart(ctx, nodeName, step)
	}
}

// NodeEnd dispatches OnNodeEnd to all node callbacks.
func (m *CallbackManager) NodeEnd(ctx context.Context, nodeName string, step int, output interface{}, err error) {
	m.mu.RLock()
	cbs := m.nodeCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnNodeEnd(ctx, nodeName, step, output, err)
	}
}

// CheckpointSave dispatches OnCheckpointSave to all checkpoint callbacks.
func (m *CallbackManager) CheckpointSave(ctx context.Context, threadID, checkpointID string, step int) {
	m.mu.RLock()
	cbs := m.checkpointCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnCheckpointSave(ctx, threadID, checkpointID, step)
	}
}

// CheckpointLoad dispatches OnCheckpointLoad to all checkpoint callbacks.
func (m *CallbackManager) CheckpointLoad(ctx context.Context, threadID, checkpointID string, step int) {
	m.mu.RLock()
	cbs := m.checkpointCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnCheckpointLoad(ctx, threadID, checkpointID, step)
	}
}

// CheckpointUpdate dispatches OnCheckpointUpdate to all checkpoint callbacks.
func (m *CallbackManager) CheckpointUpdate(ctx context.Context, threadID, asNode string) {
	m.mu.RLock()
	cbs := m.checkpointCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnCheckpointUpdate(ctx, threadID, asNode)
	}
}

// Interrupt dispatches OnInterrupt to all interrupt callbacks.
func (m *CallbackManager) Interrupt(ctx context.Context, nodeNames []string, step int) {
	m.mu.RLock()
	cbs := m.interruptCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnInterrupt(ctx, nodeNames, step)
	}
}

// Resume dispatches OnResume to all interrupt callbacks.
func (m *CallbackManager) Resume(ctx context.Context, threadID string) {
	m.mu.RLock()
	cbs := m.interruptCallbacks
	m.mu.RUnlock()
	for _, cb := range cbs {
		cb.OnResume(ctx, threadID)
	}
}

// ---- NoopCallback provides default no-op implementations ----

// NoopCallback implements GraphCallback with empty methods.
type NoopCallback struct{}

func (NoopCallback) OnRunStart(_ context.Context, _, _ string)                            {}
func (NoopCallback) OnRunEnd(_ context.Context, _, _ string, _ error)                     {}
func (NoopCallback) OnStepStart(_ context.Context, _ int, _ int)                          {}
func (NoopCallback) OnStepEnd(_ context.Context, _ int, _ error)                          {}
func (NoopCallback) OnNodeStart(_ context.Context, _ string, _ int)                       {}
func (NoopCallback) OnNodeEnd(_ context.Context, _ string, _ int, _ interface{}, _ error) {}
func (NoopCallback) OnCheckpointSave(_ context.Context, _, _ string, _ int)               {}
func (NoopCallback) OnCheckpointLoad(_ context.Context, _, _ string, _ int)               {}
func (NoopCallback) OnCheckpointUpdate(_ context.Context, _, _ string)                    {}
func (NoopCallback) OnInterrupt(_ context.Context, _ []string, _ int)                     {}
func (NoopCallback) OnResume(_ context.Context, _ string)                                 {}

// Ensure noop implements the interfaces.
var (
	_ RunCallback        = NoopCallback{}
	_ StepCallback       = NoopCallback{}
	_ NodeCallback       = NoopCallback{}
	_ CheckpointCallback = NoopCallback{}
	_ InterruptCallback  = NoopCallback{}
	_ GraphCallback      = NoopCallback{}
)
