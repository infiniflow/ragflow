package core

import (
	"context"
	"sync"
)

// AsyncIterator provides blocking iteration over a typed stream.
// Multiple goroutines reading from the same iterator will receive each item
// exactly once.
type AsyncIterator[T any] struct {
	ch        chan iterationItem[T]
	stop      chan struct{}
	done      <-chan struct{}
	closeOnce sync.Once
}

type iterationItem[T any] struct {
	value T
	ok    bool
}

func NewAsyncIterator[T any]() *AsyncIterator[T] {
	return &AsyncIterator[T]{
		ch:   make(chan iterationItem[T], 64),
		stop: make(chan struct{}),
	}
}

func (it *AsyncIterator[T]) Next() (T, bool) {
	select {
	case <-it.stop:
		var zero T
		return zero, false
	default:
	}

	select {
	case <-it.stop:
		var zero T
		return zero, false
	case item, ok := <-it.ch:
		if !ok {
			var zero T
			return zero, false
		}
		return item.value, item.ok
	case <-it.done:
		// The generator closes done only after all in-flight sends have ended.
		// Drain buffered values before reporting the end of the stream.
		select {
		case item, ok := <-it.ch:
			if !ok {
				var zero T
				return zero, false
			}
			return item.value, item.ok
		default:
			var zero T
			return zero, false
		}
	}
}

func (it *AsyncIterator[T]) Close() {
	it.closeOnce.Do(func() { close(it.stop) })
}

// AsyncGenerator produces items for an AsyncIterator.
// Multiple goroutines can safely Send to the same generator.
type AsyncGenerator[T any] struct {
	ch      chan iterationItem[T]
	stop    <-chan struct{}
	closing chan struct{}
	done    chan struct{}
	closed  bool
	mu      sync.Mutex
	sends   sync.WaitGroup
}

func NewAsyncIteratorPair[T any]() (*AsyncIterator[T], *AsyncGenerator[T]) {
	ch := make(chan iterationItem[T], 64)
	stop := make(chan struct{})
	closing := make(chan struct{})
	done := make(chan struct{})
	return &AsyncIterator[T]{ch: ch, stop: stop, done: done}, &AsyncGenerator[T]{
		ch:      ch,
		stop:    stop,
		closing: closing,
		done:    done,
	}
}

func (g *AsyncGenerator[T]) Send(value T) {
	if !g.beginSend() {
		return
	}
	defer g.sends.Done()
	select {
	case <-g.stop:
	case <-g.closing:
	case g.ch <- iterationItem[T]{value: value, ok: true}:
	}
}

func (g *AsyncGenerator[T]) trySend(value T) bool {
	if !g.beginSend() {
		return false
	}
	defer g.sends.Done()
	select {
	case <-g.stop:
		return false
	case <-g.closing:
		return false
	case g.ch <- iterationItem[T]{value: value, ok: true}:
		return true
	default:
		return false
	}
}

func (g *AsyncGenerator[T]) Close() {
	g.mu.Lock()
	if g.closed {
		done := g.done
		g.mu.Unlock()
		<-done
		return
	}
	g.closed = true
	close(g.closing)
	g.mu.Unlock()

	g.sends.Wait()
	close(g.done)
}

func (g *AsyncGenerator[T]) IsClosed() bool {
	g.mu.Lock()
	closed := g.closed
	g.mu.Unlock()
	if closed {
		return true
	}
	select {
	case <-g.stop:
		return true
	default:
		return false
	}
}

// SendCtx sends a value, respecting context cancellation to prevent goroutine leaks
// when the consumer stops reading from the iterator.
func (g *AsyncGenerator[T]) SendCtx(ctx context.Context, value T) bool {
	if !g.beginSend() {
		return false
	}
	defer g.sends.Done()

	select {
	case <-g.stop:
		return false
	case <-g.closing:
		return false
	case g.ch <- iterationItem[T]{value: value, ok: true}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (g *AsyncGenerator[T]) beginSend() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return false
	}
	select {
	case <-g.stop:
		return false
	default:
		g.sends.Add(1)
		return true
	}
}

// ===== Copy helpers =====

func copyTypedAgentEvent[M MessageType](event *TypedAgentEvent[M]) *TypedAgentEvent[M] {
	if event == nil {
		return nil
	}
	cp := &TypedAgentEvent[M]{
		AgentName: event.AgentName,
		Err:       event.Err,
	}
	if event.RunPath != nil {
		cp.RunPath = make([]RunStep, len(event.RunPath))
		for i, s := range event.RunPath {
			cp.RunPath[i] = RunStep{agentName: s.agentName}
		}
	}
	if event.Output != nil {
		cp.Output = &TypedAgentOutput[M]{CustomizedOutput: event.Output.CustomizedOutput}
		if event.Output.MessageOutput != nil {
			cp.Output.MessageOutput = &TypedMessageVariant[M]{
				IsStreaming: event.Output.MessageOutput.IsStreaming,
				Message:     event.Output.MessageOutput.Message,
				Role:        event.Output.MessageOutput.Role,
				AgenticRole: event.Output.MessageOutput.AgenticRole,
				ToolName:    event.Output.MessageOutput.ToolName,
			}
		}
	}
	if event.Action != nil {
		cp.Action = &AgentAction{
			Exit: event.Action.Exit, Interrupted: event.Action.Interrupted,
			TransferToAgent: event.Action.TransferToAgent, BreakLoop: event.Action.BreakLoop,
			CustomizedAction:    event.Action.CustomizedAction,
			internalInterrupted: event.Action.internalInterrupted,
		}
	}
	return cp
}

func setAutomaticClose[M MessageType](event *TypedAgentEvent[M]) {
	if event == nil {
		return
	}
	if event.Output != nil && event.Output.MessageOutput != nil {
		if event.Output.MessageOutput.MessageStream != nil {
			event.Output.MessageOutput.MessageStream.Close()
		}
	}
}
func typedSetAutomaticClose[M MessageType](event *TypedAgentEvent[M]) { setAutomaticClose(event) }
func addTypedEvent[M MessageType](s *runSession, event *TypedAgentEvent[M]) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.TypedEvents == nil {
		events := make([]*TypedAgentEvent[M], 0)
		s.TypedEvents = &events
	}
	if te, ok := s.TypedEvents.(*[]*TypedAgentEvent[M]); ok {
		*te = append(*te, event)
	}
}

func copyMap[K comparable, V any](src map[K]V) map[K]V {
	if src == nil {
		return nil
	}
	dst := make(map[K]V, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneSlice[T any](src []T) []T {
	if src == nil {
		return nil
	}
	dst := make([]T, len(src))
	copy(dst, src)
	return dst
}
