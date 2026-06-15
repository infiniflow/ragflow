package core

import (
	"context"
	"sync"
	"time"
)

// stopController owns global Stop state and optional active-turn cancel requests.
type stopController struct {
	mu sync.Mutex

	phase stopPhase

	hasActiveCancelTarget bool
	pending               *stopCancelRequest
	notify                chan struct{}

	idleFor        time.Duration
	skipCheckpoint bool
	stopCause      string

	closed bool
}

func newStopController() *stopController {
	return &stopController{notify: make(chan struct{}, 1)}
}

func (c *stopController) requestStop(cfg *stopConfig) stopDecision {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return stopDecision{}
	}
	if cfg.skipCheckpoint {
		c.skipCheckpoint = true
	}
	if cfg.stopCause != "" && c.stopCause == "" {
		c.stopCause = cfg.stopCause
	}
	if cfg.idleFor > 0 {
		if c.phase != stopCommitted && c.idleFor == 0 {
			c.phase = stopIdleWaiting
			c.idleFor = cfg.idleFor
		}
		return stopDecision{wakeIdle: c.phase == stopIdleWaiting}
	}

	committed := c.commitLocked()
	if cfg.agentCancelOpts != nil {
		now := time.Now()
		if c.pending == nil {
			c.pending = newStopCancelRequest(cfg.agentCancelOpts, now)
		} else {
			c.pending.merge(cfg.agentCancelOpts, now)
		}
		if c.hasActiveCancelTarget {
			c.notifyWatcherLocked()
		}
	}
	return stopDecision{commit: committed}
}

func (c *stopController) commit() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.commitLocked()
}

func (c *stopController) commitLocked() bool {
	if c.closed || c.phase == stopCommitted {
		return false
	}
	c.phase = stopCommitted
	c.idleFor = 0
	return true
}

func (c *stopController) isCommitted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.phase == stopCommitted
}

func (c *stopController) idleDuration() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != stopIdleWaiting {
		return 0
	}
	return c.idleFor
}

func (c *stopController) skipCheckpointEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.skipCheckpoint
}

func (c *stopController) cause() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopCause
}

func (c *stopController) beginActiveTurn() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.hasActiveCancelTarget = true
	if c.pending != nil {
		c.notifyWatcherLocked()
	}
}

func (c *stopController) endActiveTurn() *stopCancelRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hasActiveCancelTarget = false
	req := c.pending
	c.pending = nil
	return req
}

func (c *stopController) receiveCancel() (*stopCancelRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasActiveCancelTarget || c.pending == nil {
		return nil, false
	}
	req := c.pending
	c.pending = nil
	return req, true
}

func (c *stopController) closeForLoopExit() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.hasActiveCancelTarget = false
	c.pending = nil
	select {
	case <-c.notify:
	default:
	}
}

func (c *stopController) notifyWatcherLocked() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

// ---- StopOption constructors ----

func WithGraceful() StopOption {
	return func(cfg *stopConfig) {
		cfg.agentCancelOpts = []CancelOption{
			WithCancelMode(CancelAfterChatModel | CancelAfterToolCalls),
			WithRecursiveCancel(),
		}
	}
}

func WithImmediate() StopOption {
	return func(cfg *stopConfig) {
		cfg.agentCancelOpts = []CancelOption{
			WithRecursiveCancel(),
		}
	}
}

func WithGracefulTimeout(gracePeriod time.Duration) StopOption {
	if gracePeriod <= 0 {
		panic("agentcore: WithGracefulTimeout: gracePeriod must be positive")
	}
	return func(cfg *stopConfig) {
		cfg.agentCancelOpts = []CancelOption{
			WithCancelMode(CancelAfterChatModel | CancelAfterToolCalls),
			WithRecursiveCancel(),
			WithCancelTimeout(gracePeriod),
		}
	}
}

func WithStopTimeout(d time.Duration) StopOption {
	return func(cfg *stopConfig) { cfg.timeout = &d }
}

func WithSkipCheckpoint() StopOption {
	return func(cfg *stopConfig) {
		cfg.skipCheckpoint = true
	}
}

func WithStopCause(cause string) StopOption {
	return func(cfg *stopConfig) {
		cfg.stopCause = cause
	}
}

func UntilIdleFor(duration time.Duration) StopOption {
	if duration <= 0 {
		panic("agentcore: UntilIdleFor: duration must be positive")
	}
	return func(cfg *stopConfig) {
		cfg.idleFor = duration
	}
}

// ---- PushOption constructors ----

func WithPreempt[T any](safePoint SafePoint) PushOption[T] {
	if safePoint == 0 {
		panic("agentcore: SafePoint must not be zero; use AfterToolCalls, AfterChatModel, or AnySafePoint")
	}
	return func(cfg *pushConfig[T]) {
		cfg.preempt = true
		cfg.agentCancelOpts = []CancelOption{
			WithCancelMode(safePoint.toCancelMode()),
			WithRecursiveCancel(),
		}
	}
}

func WithPreemptTimeout[T any](safePoint SafePoint, timeout time.Duration) PushOption[T] {
	if safePoint == 0 {
		panic("agentcore: SafePoint must not be zero; use AfterToolCalls, AfterChatModel, or AnySafePoint")
	}
	return func(cfg *pushConfig[T]) {
		cfg.preempt = true
		cfg.agentCancelOpts = []CancelOption{
			WithCancelMode(safePoint.toCancelMode()),
			WithCancelTimeout(timeout),
			WithRecursiveCancel(),
		}
	}
}

func WithPreemptDelay[T any](delay time.Duration) PushOption[T] {
	return func(cfg *pushConfig[T]) {
		cfg.preemptDelay = delay
	}
}

func WithPushStrategy[T any](fn func(ctx context.Context, tc *TurnContext[T]) []PushOption[T]) PushOption[T] {
	return func(cfg *pushConfig[T]) {
		cfg.pushStrategy = fn
	}
}

// ---- Deprecated aliases ----

func WithImmediateStop() StopOption { return WithImmediate() }
func WithGracefulStop() StopOption { return WithGraceful() }
