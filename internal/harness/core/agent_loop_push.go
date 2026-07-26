package core

import (
	"context"
	"sync/atomic"
	"time"
)

// ---- AgentLoop push operations ----

func (l *AgentLoop[T]) appendLate(item T) {
	l.lateMu.Lock()
	defer l.lateMu.Unlock()
	if l.lateSealed {
		panic("AgentLoop: Push called after TakeLateItems")
	}
	l.lateItems = append(l.lateItems, item)
}

// Push adds an item to the loop's buffer for processing.
// Returns false if the loop has stopped. When preemptive, returns an ack channel.
func (l *AgentLoop[T]) Push(item T, opts ...PushOption[T]) (bool, <-chan struct{}) {
	cfg := &pushConfig[T]{}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.pushStrategy != nil {
		return l.pushWithStrategy(item, cfg)
	}

	return l.pushWithConfig(item, cfg)
}

// pushWithStrategy snapshots the current target turn while the strategy decides
// how to enqueue the item.
//
// When the loop is idle (no active turn), snapshot.ctx is nil and the strategy
// receives context.TODO() — it cannot observe caller cancellation or deadlines
// at that point. If the strategy needs the caller's context, use the Push overload
// that accepts ctx (not yet available; pass via closure instead).
func (l *AgentLoop[T]) pushWithStrategy(item T, cfg *pushConfig[T]) (bool, <-chan struct{}) {
	strategy := cfg.pushStrategy

	snapshot := l.preemptCtrl.beginPush()
	defer l.preemptCtrl.endPush()

	runCtx := snapshot.ctx
	if runCtx == nil {
		runCtx = context.TODO()
	}
	var tc *TurnContext[T]
	if snapshot.tc != nil {
		tc = snapshot.tc.(*TurnContext[T])
	}
	realOpts := strategy(runCtx, tc)
	cfg = &pushConfig[T]{}
	for _, opt := range realOpts {
		opt(cfg)
	}
	cfg.pushStrategy = nil

	if !cfg.preempt {
		if !l.buffer.TrySend(item) {
			l.appendLate(item)
			return false, nil
		}
		return true, nil
	}

	if atomic.LoadInt32(&l.stopped) != 0 {
		l.appendLate(item)
		return false, nil
	}

	if !l.buffer.TrySend(item) {
		l.appendLate(item)
		return false, nil
	}

	ack := make(chan struct{})
	if atomic.LoadInt32(&l.started) == 0 {
		close(ack)
		return true, ack
	}

	if cfg.preemptDelay > 0 {
		go func() {
			select {
			case <-time.After(cfg.preemptDelay):
				l.preemptCtrl.requestPreempt(snapshot, ack, cfg.agentCancelOpts...)
			case <-l.done:
				close(ack)
			}
		}()
	} else {
		l.preemptCtrl.requestPreempt(snapshot, ack, cfg.agentCancelOpts...)
	}
	return true, ack
}

func (l *AgentLoop[T]) pushWithConfig(item T, cfg *pushConfig[T]) (bool, <-chan struct{}) {
	if atomic.LoadInt32(&l.stopped) != 0 {
		l.appendLate(item)
		return false, nil
	}

	if cfg.preempt {
		snapshot := l.preemptCtrl.beginPush()
		defer l.preemptCtrl.endPush()

		if !l.buffer.TrySend(item) {
			l.appendLate(item)
			return false, nil
		}

		ack := make(chan struct{})
		if atomic.LoadInt32(&l.started) == 0 {
			close(ack)
			return true, ack
		}

		if cfg.preemptDelay > 0 {
			go func() {
				select {
				case <-time.After(cfg.preemptDelay):
					l.preemptCtrl.requestPreempt(snapshot, ack, cfg.agentCancelOpts...)
				case <-l.done:
					close(ack)
				}
			}()
		} else {
			l.preemptCtrl.requestPreempt(snapshot, ack, cfg.agentCancelOpts...)
		}
		return true, ack
	}

	if !l.buffer.TrySend(item) {
		l.appendLate(item)
		return false, nil
	}
	return true, nil
}
