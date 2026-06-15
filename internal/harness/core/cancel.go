package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ---- CancelMode ----

type CancelMode int

const (
	CancelImmediate      CancelMode = 0
	CancelAfterChatModel  CancelMode = 1 << iota
	CancelAfterToolCalls
)

// ---- CancelHandle ----

type CancelHandle struct{ wait func() error }
func (h *CancelHandle) Wait() error { return h.wait() }

type AgentCancelFunc func(...CancelOption) (*CancelHandle, bool)

type CancelOption func(*cancelConfig)
type cancelConfig struct {
	Mode      CancelMode
	Recursive bool
	Timeout   *time.Duration
}

func WithCancelMode(mode CancelMode) CancelOption {
	return func(c *cancelConfig) { c.Mode = mode }
}
func WithCancelTimeout(d time.Duration) CancelOption {
	return func(c *cancelConfig) { c.Timeout = &d }
}
func WithRecursiveCancel() CancelOption {
	return func(c *cancelConfig) { c.Recursive = true }
}

type AgentCancelInfo struct {
	Mode      CancelMode
	Escalated bool
	Timeout   bool
}

type CancelError struct {
	Info              *AgentCancelInfo
	InterruptContexts []*InterruptCtx
	interruptSignal   *InterruptSignal
}

func (e *CancelError) Error() string {
	if e == nil || e.Info == nil {
		return "agent canceled"
	}
	return fmt.Sprintf("agent canceled: mode=%v escalated=%v", e.Info.Mode, e.Info.Escalated)
}

type StreamCanceledError struct{}
func (e *StreamCanceledError) Error() string { return "stream canceled" }

var (
	ErrCancelTimeout  = errors.New("cancel timed out")
	ErrExecutionEnded = errors.New("execution already ended")
	ErrStreamCanceled error = &StreamCanceledError{}
)

// ---- cancelContext state machine ----

const (
	stRunning       int32 = 0
	stCancelling    int32 = 1
	stDone          int32 = 2
	stCancelHandled int32 = 5
	interruptNotSent   int32 = 0
	interruptImmediate int32 = 1
)

const cancelGracePeriod = 1 * time.Second

type cancelContext struct {
	mode            int32
	cancelChan      chan struct{}
	immediateChan   chan struct{}
	doneChan        chan struct{}
	doneOnce        sync.Once
	state           int32
	interruptSent   int32
	escalated       int32
	timeoutEscalated int32
	startedMode     int32
	deadlineUnixNano int64
	recursive       int32
	recursiveChan   chan struct{}
	root            bool
	parent          *cancelContext
	agentToolDescendant int32
	cancelMu        sync.Mutex
	timeoutOnce     sync.Once
	timeoutNotify   chan struct{}
	mu              sync.Mutex
	interruptFuncs  []func(...any)
}

func newCancelContext() *cancelContext {
	return &cancelContext{
		cancelChan:    make(chan struct{}), immediateChan: make(chan struct{}),
		doneChan: make(chan struct{}), timeoutNotify: make(chan struct{}, 1),
		recursiveChan: make(chan struct{}), root: true,
	}
}

func (cc *cancelContext) isRoot() bool                      { return cc != nil && cc.root }
func (cc *cancelContext) isRecursive() bool                  { return cc != nil && atomic.LoadInt32(&cc.recursive) == 1 }
func (cc *cancelContext) shouldCancel() bool {
	if cc == nil { return false }
	select { case <-cc.cancelChan: return true; default: return false }
}
func (cc *cancelContext) isImmediate() bool {
	if cc == nil { return false }
	select { case <-cc.immediateChan: return true; default: return false }
}
func (cc *cancelContext) getMode() CancelMode {
	if cc == nil { return CancelImmediate }
	return CancelMode(atomic.LoadInt32(&cc.mode))
}
func (cc *cancelContext) setMode(m CancelMode)  { atomic.StoreInt32(&cc.mode, int32(m)) }
func (cc *cancelContext) setRecursive(v bool) {
	if v && atomic.CompareAndSwapInt32(&cc.recursive, 0, 1) { close(cc.recursiveChan) }
}

func (cc *cancelContext) markDone() {
	if cc == nil { return }
	if atomic.CompareAndSwapInt32(&cc.state, stRunning, stDone) || atomic.CompareAndSwapInt32(&cc.state, stCancelling, stDone) {
		cc.doneOnce.Do(func() { close(cc.doneChan) })
	}
}
func (cc *cancelContext) markHandled() bool {
	if cc == nil { return false }
	if atomic.CompareAndSwapInt32(&cc.state, stCancelling, stCancelHandled) {
		cc.doneOnce.Do(func() { close(cc.doneChan) })
		return true
	}
	return false
}
func (cc *cancelContext) createError() *CancelError {
	info := &AgentCancelInfo{Mode: cc.getMode()}
	if atomic.LoadInt32(&cc.escalated) == 1 {
		info.Escalated = true
		info.Timeout = atomic.LoadInt32(&cc.timeoutEscalated) == 1
	}
	return &CancelError{Info: info}
}
func (cc *cancelContext) createAndMarkHandled() (*CancelError, bool) {
	cc.cancelMu.Lock()
	defer cc.cancelMu.Unlock()
	err := cc.createError()
	ok := cc.markHandled()
	return err, ok
}

func (cc *cancelContext) triggerCancel(m CancelMode) {
	cc.setMode(m)
	if atomic.CompareAndSwapInt32(&cc.state, stRunning, stCancelling) { close(cc.cancelChan) }
}
func (cc *cancelContext) triggerImmediate() {
	atomic.StoreInt32(&cc.escalated, 1)
	cc.setMode(CancelImmediate)
	// If state is still Running, transition to Cancelling and close channels.
	// If already Cancelling (set by buildCancelFunc), just send the interrupt signal.
	if atomic.CompareAndSwapInt32(&cc.state, stRunning, stCancelling) {
		close(cc.cancelChan)
	}
	cc.sendInterrupt()
}
func (cc *cancelContext) sendInterrupt() bool {
	cc.mu.Lock()
	if !atomic.CompareAndSwapInt32(&cc.interruptSent, interruptNotSent, interruptImmediate) {
		cc.mu.Unlock()
		return false
	}
	close(cc.immediateChan)
	// Snapshot callbacks under lock, invoke outside to avoid callback-induced deadlocks.
	funcs := append([]func(...any){}, cc.interruptFuncs...)
	cc.mu.Unlock()

	for _, fn := range funcs {
		fn()
	}

	// Grace period for recursive cancellation with agent-tool descendants.
	// This is best-effort; cancel() itself returns immediately, the grace wait
	// is advisory for the sub-agent to observe the cancellation signal.
	if cc.isRecursive() && atomic.LoadInt32(&cc.agentToolDescendant) == 1 {
		select { case <-cc.doneChan: case <-time.After(cancelGracePeriod): }
	}
	return true
}
func (cc *cancelContext) markAgentToolDescendant() {
	for cur := cc; cur != nil; cur = cur.parent { atomic.StoreInt32(&cur.agentToolDescendant, 1) }
}

func (cc *cancelContext) deriveAgentToolCancelContext(ctx context.Context) *cancelContext {
	if cc == nil { return nil }
	child := newCancelContext()
	child.root = false
	child.parent = cc

	// Propagate cancel signal to child (goroutine exits cleanly when any case fires)
	go func() {
		select {
		case <-cc.cancelChan:
			if cc.isRecursive() {
				child.setRecursive(true)
				child.triggerCancel(cc.getMode())
				return
			}
			select {
			case <-cc.recursiveChan:
				child.setRecursive(true)
				child.triggerCancel(cc.getMode())
			case <-child.doneChan:
			case <-ctx.Done():
			}
		case <-child.doneChan:
		case <-ctx.Done():
		}
	}()

	// Propagate immediate cancel signal to child (goroutine exits cleanly when any case fires)
	go func() {
		select {
		case <-cc.immediateChan:
			if cc.isRecursive() {
				child.setRecursive(true)
				child.triggerImmediate()
				return
			}
			select {
			case <-cc.recursiveChan:
				child.setRecursive(true)
				child.triggerImmediate()
			case <-child.doneChan:
			case <-ctx.Done():
			}
		case <-child.doneChan:
		case <-ctx.Done():
		}
	}()

	return child
}
func (cc *cancelContext) buildCancelFunc() AgentCancelFunc {
	join := func(a, b CancelMode) CancelMode {
		if a == CancelImmediate || b == CancelImmediate { return CancelImmediate }
		return a | b
	}
	parse := func(opts ...CancelOption) *cancelConfig {
		c := &cancelConfig{Mode: CancelImmediate}
		for _, o := range opts { o(c) }
		return c
	}
	waitDone := func() error {
		<-cc.doneChan
		switch atomic.LoadInt32(&cc.state) {
		case stDone: return ErrExecutionEnded
		default:
			if atomic.LoadInt32(&cc.timeoutEscalated) == 1 { return ErrCancelTimeout }
			return nil
		}
	}
	return func(callOpts ...CancelOption) (*CancelHandle, bool) {
		req := parse(callOpts...)
		st := atomic.LoadInt32(&cc.state)
		switch st {
		case stCancelHandled: return &CancelHandle{func() error { return nil }}, false
		case stDone: return &CancelHandle{func() error { return ErrExecutionEnded }}, false
		}
		cc.cancelMu.Lock()
		st = atomic.LoadInt32(&cc.state)
		switch st {
		case stCancelHandled: cc.cancelMu.Unlock(); return &CancelHandle{func() error { return nil }}, false
		case stDone: cc.cancelMu.Unlock(); return &CancelHandle{func() error { return ErrExecutionEnded }}, false
		}
		if st == stRunning {
			if !atomic.CompareAndSwapInt32(&cc.state, stRunning, stCancelling) {
				st = atomic.LoadInt32(&cc.state)
				cc.cancelMu.Unlock()
				if st == stDone { return &CancelHandle{func() error { return ErrExecutionEnded }}, false }
				return &CancelHandle{waitDone}, true
			}
			cc.setMode(req.Mode)
			atomic.StoreInt32(&cc.startedMode, int32(req.Mode))
			cc.setRecursive(req.Recursive)
			close(cc.cancelChan)
		} else {
			cc.setMode(join(cc.getMode(), req.Mode))
			if req.Recursive { cc.setRecursive(true) }
		}
		var needImmediate, needTimeout bool
		if cc.getMode() == CancelImmediate { needImmediate = true
		} else if req.Timeout != nil && *req.Timeout > 0 {
			// Use minimum (earliest) non-zero deadline so a later cancel cannot
			// extend an earlier timeout.
			nextDeadline := time.Now().Add(*req.Timeout).UnixNano()
			cc.setDeadlineMinUnixNano(nextDeadline)
			cc.wakeTimeout()
			needTimeout = true
		}
		cc.cancelMu.Unlock()
		if needImmediate { cc.triggerImmediate() }
		if needTimeout { cc.startTimeout() }
		return &CancelHandle{waitDone}, true
	}
}

func (cc *cancelContext) startTimeout() {
	cc.timeoutOnce.Do(func() {
		go func() {
			for {
				select {
				case <-cc.doneChan:
					return
				default:
				}
				dl := atomic.LoadInt64(&cc.deadlineUnixNano)
				if dl == 0 { return }
				wait := time.Duration(dl - time.Now().UnixNano())
				if wait <= 0 {
					atomic.StoreInt32(&cc.escalated, 1)
					atomic.StoreInt32(&cc.timeoutEscalated, 1)
					cc.triggerImmediate()
					return
				}
				timer := time.NewTimer(wait)
				select {
				case <-timer.C:
					atomic.StoreInt32(&cc.escalated, 1)
					atomic.StoreInt32(&cc.timeoutEscalated, 1)
					cc.triggerImmediate()
					return
				case <-cc.timeoutNotify:
					timer.Stop()
					continue
				case <-cc.doneChan:
					timer.Stop()
					return
				}
			}
		}()
	})
}

func (cc *cancelContext) wakeTimeout() {
	select {
	case cc.timeoutNotify <- struct{}{}:
	default:
	}
}

func (cc *cancelContext) setDeadlineUnixNano(t int64) { atomic.StoreInt64(&cc.deadlineUnixNano, t) }
func (cc *cancelContext) setDeadlineMinUnixNano(next int64) {
	for {
		cur := atomic.LoadInt64(&cc.deadlineUnixNano)
		if cur != 0 && cur <= next {
			return
		}
		if atomic.CompareAndSwapInt64(&cc.deadlineUnixNano, cur, next) {
			return
		}
	}
}
func (cc *cancelContext) agentToolSeen() bool          { return cc != nil && atomic.LoadInt32(&cc.agentToolDescendant) == 1 }

// ---- Context propagation ----

type cancelCtxKey struct{}

func withCancelContext(ctx context.Context, cc *cancelContext) context.Context {
	if cc == nil { return ctx }
	return context.WithValue(ctx, cancelCtxKey{}, cc)
}

func getCancelContext(ctx context.Context) *cancelContext {
	if v := ctx.Value(cancelCtxKey{}); v != nil { return v.(*cancelContext) }
	return nil
}

// ---- Iterator wrapper ----

func wrapIterWithCancelCtx[M MessageType](iter *AsyncIterator[*TypedAgentEvent[M]], cc *cancelContext) *AsyncIterator[*TypedAgentEvent[M]] {
	if cc == nil { return iter }
	it, gen := NewAsyncIteratorPair[*TypedAgentEvent[M]]()
	go func() {
		defer gen.Close()
		endedByCancel := false
		defer func() {
			// Only mark done on actual cancellation, not on normal completion.
			// This prevents a shared cancelContext from being marked done by a
			// sub-agent that finishes naturally, which would block later cancel calls.
			if endedByCancel || cc.shouldCancel() {
				cc.markDone()
			}
		}()
		for {
			event, ok := iter.Next()
			if !ok { break }
			if cc.isRoot() && event.Action != nil && event.Action.internalInterrupted != nil && cc.shouldCancel() {
				if err, ok := cc.createAndMarkHandled(); ok {
					err.interruptSignal = event.Action.internalInterrupted
					gen.Send(&TypedAgentEvent[M]{Err: err})
				}
				endedByCancel = true
				return
			}
			gen.Send(event)
		}
	}()
	return it
}

type cancelMonitoredModel[M MessageType] struct {
	inner Model[M]
	cc    *cancelContext
}

func (m *cancelMonitoredModel[M]) Generate(ctx context.Context, input []M, opts ...modelOption) (M, error) { return m.inner.Generate(ctx, input, opts...) }
func (m *cancelMonitoredModel[M]) Stream(ctx context.Context, input []M, opts ...modelOption) (*schema.StreamReader[M], error) {
	s, err := m.inner.Stream(ctx, input, opts...)
	if err != nil { return nil, err }
	return wrapStreamWithCancel(s, m.cc), nil
}
func (m *cancelMonitoredModel[M]) BindTools(tools []*schema.ToolInfo) error { return m.inner.BindTools(tools) }

func wrapStreamWithCancel[T any](s *schema.StreamReader[T], cc *cancelContext) *schema.StreamReader[T] {
	if cc == nil { return s }
	select {
	case <-cc.immediateChan:
		s.Close()
		r := schema.NewStreamReader[T]()
		var zero T
		r.Send(zero, ErrStreamCanceled)
		r.Close()
		return r
	default:
	}
	r := schema.NewStreamReader[T]()
	go func() {
		defer r.Close()
		defer s.Close()
		ch := make(chan struct{ Data T; Err error }, 64)
		done := make(chan struct{})
		defer close(done)
		go func() {
			defer close(ch)
			for {
				d, e := s.Recv()
				select {
				case ch <- struct{ Data T; Err error }{d, e}:
				case <-done:
					return
				}
				if e != nil { return }
			}
		}()
		for {
			select {
			case <-cc.immediateChan:
				s.Close()
				var z T
				r.Send(z, ErrStreamCanceled)
				return
			case v, ok := <-ch:
				if !ok || v.Err != nil { return }
				r.Send(v.Data, nil)
			}
		}
	}()
	return r
}

// ---- Graph interrupt integration ----

// InterruptSignalInfo carries information from a graph interrupt.
type InterruptSignalInfo struct {
	Signal    *InterruptSignal
	OrigError error
}

// CancelFromGraphInfo carries the cancel config from graph-level interrupt.
type CancelFromGraphInfo struct {
	Mode      CancelMode
	Timeout   time.Duration
	Recursive bool
}

// SetGraphInterruptFunc registers a callback invoked on graph interrupt signal.
func (cc *cancelContext) SetGraphInterruptFunc(fn func(...any)) {
	if cc == nil { return }
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.interruptFuncs = append(cc.interruptFuncs, fn)
}

// InterruptFromGraph coordinates a graph interrupt with the cancel state machine.
func (cc *cancelContext) InterruptFromGraph(ctx context.Context, info *CancelFromGraphInfo) bool {
	if cc == nil || info == nil { return false }
	cc.cancelMu.Lock()
	defer cc.cancelMu.Unlock()
	st := atomic.LoadInt32(&cc.state)
	if st != stRunning { return false }
	if !atomic.CompareAndSwapInt32(&cc.state, stRunning, stCancelling) {
		return false
	}
	cc.setMode(info.Mode)
	cc.setRecursive(info.Recursive)
	close(cc.cancelChan)
	if info.Mode == CancelImmediate {
		cc.triggerImmediate()
	} else if info.Timeout > 0 {
		cc.setDeadlineUnixNano(time.Now().Add(info.Timeout).UnixNano())
		cc.startTimeout()
	}
	return true
}
