package core

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ---- AgentLoop core: struct, lifecycle, cleanup ----
//
// Configuration types (AgentLoopConfig, preemptController, stopController, etc.)
// are defined in turn_loop_config.go, turn_loop_preempt.go, and turn_loop_stop.go.
// Execution logic is split into:
//   - turn_loop_run.go     (planTurn, run, defaultTurnLoopOnAgentEvents)
//   - turn_loop_agent.go   (runAgentAndHandleEvents, watchPreempt, watchStop, setupBridgeStore)
//   - turn_loop_push.go    (Push, pushWithStrategy, pushWithConfig, appendLate)
//   - turn_loop_checkpoint.go (checkpoint serialization, tryLoadCheckpoint)

// AgentLoop executes agent turns in a push-based loop.
// See AgentLoopConfig for configuration details and AgentLoopState for results.
type AgentLoop[T any] struct {
	config AgentLoopConfig[T]

	buffer *turnBuffer[T]

	stopped int32
	started int32

	done chan struct{}

	result *AgentLoopState[T]

	runOnce sync.Once

	stopCtrl *stopController

	preemptCtrl *preemptController

	runErr error

	interruptedItems []T

	checkPointRunnerBytes []byte
	interruptContexts     []*InterruptCtx
	capturedCancelErr     *CancelError

	pendingResume *agentLoopPendingResume[T]

	loadCheckpointID string

	onAgentEvents func(ctx context.Context, tc *TurnContext[T], events *AsyncIterator[*AgentEvent]) error

	lateMu     sync.Mutex
	lateItems  []T
	lateSealed bool
}

// NewAgentLoop creates a new AgentLoop without starting it.
func NewAgentLoop[T any](cfg AgentLoopConfig[T]) *AgentLoop[T] {
	if cfg.GenInput == nil {
		panic("agentcore: NewAgentLoop: GenInput is required")
	}
	if cfg.PrepareAgent == nil {
		panic("agentcore: NewAgentLoop: PrepareAgent is required")
	}

	l := &AgentLoop[T]{
		config:      cfg,
		buffer:      newTurnBuffer[T](),
		done:        make(chan struct{}),
		stopCtrl:    newStopController(),
		preemptCtrl: newPreemptController(),
	}
	if cfg.OnAgentEvents != nil {
		l.onAgentEvents = cfg.OnAgentEvents
	} else {
		l.onAgentEvents = defaultTurnLoopOnAgentEvents[T]
	}
	return l
}

func (l *AgentLoop[T]) start(ctx context.Context) {
	l.runOnce.Do(func() {
		atomic.StoreInt32(&l.started, 1)
		go l.run(ctx)
	})
}

// Run starts the loop's processing goroutine. It is non-blocking.
func (l *AgentLoop[T]) Run(ctx context.Context) {
	l.start(ctx)
}

// Stop signals the loop to stop and returns immediately (non-blocking).
func (l *AgentLoop[T]) Stop(opts ...StopOption) {
	cfg := &stopConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.idleFor > 0 {
		cfg.agentCancelOpts = nil
	}

	decision := l.stopCtrl.requestStop(cfg)
	if decision.wakeIdle {
		l.buffer.Wakeup()
	}
	if decision.commit {
		l.finishStopCommit()
	}

	// If a stop timeout is configured, force-stop after the timeout
	if cfg.timeout != nil && *cfg.timeout > 0 {
		go func() {
			select {
			case <-time.After(*cfg.timeout):
				l.commitStop()
			case <-l.done:
			}
		}()
	}
}

func (l *AgentLoop[T]) commitStop() {
	if !l.stopCtrl.commit() {
		return
	}
	l.finishStopCommit()
}

func (l *AgentLoop[T]) finishStopCommit() {
	atomic.StoreInt32(&l.stopped, 1)
	l.buffer.Close()
}

// Wait blocks until the loop exits and returns the result.
func (l *AgentLoop[T]) Wait() *AgentLoopState[T] {
	<-l.done
	return l.result
}

// shouldSaveCheckpoint determines whether a turn-loop checkpoint should be saved.
// Checkpoints are saved when:
//  1. A stop was committed AND exit was caused by stop (runErr==nil, CancelError, or capturedCancelErr).
//  2. A business interrupt occurred (InterruptError or interruptContexts).
//  3. Checkpoint is not skipped (skipCheckpoint not set), not idle, and store is available.
//
// On normal completion (runErr==nil, no stop committed), no checkpoint is saved.
func (l *AgentLoop[T]) shouldSaveCheckpoint() bool {
	if l.config.Store == nil || l.config.CheckpointID == "" {
		return false
	}
	if l.stopCtrl.skipCheckpointEnabled() {
		return false
	}
	isIdle := len(l.checkPointRunnerBytes) == 0 && len(l.interruptedItems) == 0
	if isIdle {
		return false
	}
	exitCausedByStop := l.runErr == nil || errors.As(l.runErr, new(*CancelError)) || l.capturedCancelErr != nil
	businessInterrupt := errors.As(l.runErr, new(*InterruptError)) || l.interruptContexts != nil
	return (l.stopCtrl.isCommitted() && exitCausedByStop) || businessInterrupt
}

func (l *AgentLoop[T]) cleanup(ctx context.Context) {
	atomic.StoreInt32(&l.stopped, 1)

	unhandled := l.buffer.TakeAll()
	checkpointID := l.config.CheckpointID
	shouldSaveCheckpoint := l.shouldSaveCheckpoint()

	var checkpointed bool
	var checkpointErr error

	if shouldSaveCheckpoint {
		cp := &agentLoopCheckpoint[T]{
			RunnerCheckpoint: l.checkPointRunnerBytes,
			HasRunnerState:   len(l.checkPointRunnerBytes) > 0,
			UnhandledItems:   unhandled,
			CanceledItems:    l.interruptedItems,
		}
		checkpointed = true
		checkpointErr = l.saveTurnLoopCheckpoint(ctx, checkpointID, cp)
	} else if l.loadCheckpointID != "" {
		_ = l.deleteTurnLoopCheckpoint(ctx, l.loadCheckpointID)
	}

	var takeLateOnce sync.Once
	var takeLateResult []T

	l.result = &AgentLoopState[T]{
		ExitReason:          l.runErr,
		UnhandledItems:      unhandled,
		InterruptedItems:    l.interruptedItems,
		StopCause:           l.stopCtrl.cause(),
		CheckpointAttempted: checkpointed,
		CheckpointErr:       checkpointErr,
		TakeLateItems: func() []T {
			takeLateOnce.Do(func() {
				l.lateMu.Lock()
				takeLateResult = append([]T{}, l.lateItems...)
				l.lateSealed = true
				l.lateMu.Unlock()
			})
			return takeLateResult
		},
	}

	l.stopCtrl.closeForLoopExit()
	l.preemptCtrl.closeForLoopExit()
	l.buffer.Close()
	close(l.done)
}
