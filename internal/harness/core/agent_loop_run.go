package core

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// ---- AgentLoop main run loop and turn planning ----

func (l *AgentLoop[T]) planTurn(
	ctx context.Context,
	isResume bool,
	items []T,
	pr *agentLoopPendingResume[T],
) (*turnPlan[T], error) {
	if !isResume {
		result, err := l.config.GenInput(ctx, l, items)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, errors.New("GenInputResult is nil")
		}
		if result.Input == nil {
			return nil, errors.New("agent input is nil")
		}
		turnCtx := ctx
		if result.RunCtx != nil {
			turnCtx = result.RunCtx
		}
		return &turnPlan[T]{
			turnCtx:   turnCtx,
			remaining: result.Remaining,
			spec: &turnRunSpec[T]{
				runCtx:   result.RunCtx,
				input:    result.Input,
				runOpts:  result.RunOpts,
				consumed: result.Consumed,
			},
		}, nil
	}
	if pr == nil {
		return nil, errors.New("resume payload is nil")
	}
	if l.config.GenResume == nil {
		return nil, errors.New("GenResume is required for resume")
	}
	resumeResult, err := l.config.GenResume(ctx, l, pr.interrupted, pr.unhandled, pr.newItems)
	if err != nil {
		return nil, err
	}
	if resumeResult == nil {
		return nil, errors.New("GenResumeResult is nil")
	}
	turnCtx := ctx
	if resumeResult.RunCtx != nil {
		turnCtx = resumeResult.RunCtx
	}
	return &turnPlan[T]{
		turnCtx:   turnCtx,
		remaining: resumeResult.Remaining,
		spec: &turnRunSpec[T]{
			runCtx:       resumeResult.RunCtx,
			runOpts:      resumeResult.RunOpts,
			resumeParams: resumeResult.ResumeParams,
			isResume:     true,
			consumed:     resumeResult.Consumed,
			resumeBytes:  pr.resumeBytes,
		},
	}, nil
}

func defaultTurnLoopOnAgentEvents[T any](_ context.Context, _ *TurnContext[T], events *AsyncIterator[*AgentEvent]) error {
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return event.Err
		}
	}
	return nil
}

func (l *AgentLoop[T]) run(ctx context.Context) {
	defer l.cleanup(ctx)

	if err := l.tryLoadCheckpoint(ctx); err != nil {
		l.runErr = err
		return
	}

	// Monitor context cancellation: close the buffer so that a blocking
	// Receive() unblocks.
	go func() {
		select {
		case <-ctx.Done():
			l.buffer.Close()
		case <-l.done:
		}
	}()

	for {
		if l.stopCtrl.isCommitted() {
			return
		}

		isResume := false
		var pr *agentLoopPendingResume[T]
		var items []T
		var pushBack []T

		if l.pendingResume != nil {
			isResume = true
			pr = l.pendingResume
			l.pendingResume = nil

			l.preemptCtrl.waitForPushes()
			pr.newItems = append(pr.newItems, l.buffer.TakeAll()...)

			pushBack = make([]T, 0, len(pr.interrupted)+len(pr.unhandled)+len(pr.newItems))
			pushBack = append(pushBack, pr.interrupted...)
			pushBack = append(pushBack, pr.unhandled...)
			pushBack = append(pushBack, pr.newItems...)
		} else {
			var first T
			var ok bool

			if idleFor := l.stopCtrl.idleDuration(); idleFor > 0 {
				l.buffer.ClearWakeup()
				idleTimer := time.NewTimer(idleFor)
				cancelIdle := make(chan struct{})
				go func() {
					select {
					case <-idleTimer.C:
						l.commitStop()
					case <-cancelIdle:
					}
				}()

				first, ok = l.buffer.Receive()

				// Drain the timer channel to avoid race with commitStop
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				close(cancelIdle)

				if !ok && !l.buffer.IsClosed() {
					if err := ctx.Err(); err != nil {
						l.runErr = err
						return
					}
					continue
				}

				// If commitStop fired via idle timer, exit
				if atomic.LoadInt32(&l.stopped) != 0 {
					return
				}
			} else {
				first, ok = l.buffer.Receive()
				if !ok && l.stopCtrl.idleDuration() > 0 {
					continue
				}
			}

			if !ok {
				if err := ctx.Err(); err != nil {
					l.runErr = err
				}
				return
			}

			if err := ctx.Err(); err != nil {
				l.buffer.PushFront([]T{first})
				l.runErr = err
				return
			}

			if l.stopCtrl.isCommitted() {
				l.buffer.PushFront([]T{first})
				return
			}

			l.preemptCtrl.waitForPushes()
			rest := l.buffer.TakeAll()
			items = append([]T{first}, rest...)
			pushBack = items
		}

		l.preemptCtrl.beginPlanningTurn()
		abortPlanning := func() {
			l.preemptCtrl.abortPlanningTurn().ack()
		}

		plan, err := l.planTurn(ctx, isResume, items, pr)
		if err != nil {
			abortPlanning()
			if len(pushBack) > 0 {
				l.buffer.PushFront(pushBack)
			}
			l.runErr = err
			return
		}

		if l.stopCtrl.isCommitted() {
			abortPlanning()
			if len(pushBack) > 0 {
				l.buffer.PushFront(pushBack)
			}
			return
		}

		agent, err := l.config.PrepareAgent(plan.turnCtx, l, plan.spec.consumed)
		if err != nil {
			abortPlanning()
			if len(pushBack) > 0 {
				l.buffer.PushFront(pushBack)
			}
			l.runErr = err
			return
		}

		if l.stopCtrl.isCommitted() {
			abortPlanning()
			if len(pushBack) > 0 {
				l.buffer.PushFront(pushBack)
			}
			return
		}

		l.buffer.PushFront(plan.remaining)

		runErr := l.runAgentAndHandleEvents(plan.turnCtx, agent, plan.spec)

		if runErr != nil {
			if l.capturedCancelErr != nil || l.interruptContexts != nil {
				// Assignment (not append) is intentional: only the interrupting
				// turn's consumed items matter — the loop exits immediately after.
				l.interruptedItems = append([]T{}, plan.spec.consumed...)
			}
			l.runErr = runErr
			return
		}

		// Business interrupt: agent produced an Interrupted action
		if l.interruptContexts != nil {
			l.interruptedItems = append([]T{}, plan.spec.consumed...)
			l.runErr = &InterruptError{InterruptContexts: l.interruptContexts}
			return
		}
	}
}
