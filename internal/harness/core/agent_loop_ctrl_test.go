package core

import (
	"context"
	"testing"
	"time"
)

// ---- preemptController tests ----

func TestPreemptController_Lifecycle(t *testing.T) {
	ctrl := newPreemptController()

	// idle -> planning
	ctrl.beginPlanningTurn()
	planningTurnID := ctrl.turnID
	if planningTurnID == 0 {
		t.Error("expected turnID > 0 after beginPlanningTurn")
	}

	// planning -> active
	ctx := context.Background()
	ctrl.beginActiveTurn(ctx, "test-turn-context")
	if ctrl.turnPhase != preemptTurnActive {
		t.Error("expected turnPhase = active after beginActiveTurn")
	}

	// active -> idle
	req := ctrl.endActiveTurn()
	if req != nil {
		t.Error("expected nil pending request on clean endActiveTurn")
	}
	if ctrl.turnPhase != preemptTurnIdle {
		t.Error("expected turnPhase = idle after endActiveTurn")
	}
}

func TestPreemptController_AbortPlanningTurn(t *testing.T) {
	ctrl := newPreemptController()
	ctrl.beginPlanningTurn()

	req := ctrl.abortPlanningTurn()
	if req != nil {
		t.Error("expected nil req on abort with no pending")
	}
	if ctrl.turnPhase != preemptTurnIdle {
		t.Error("expected turnPhase = idle after abortPlanningTurn")
	}

	// Must be able to begin again after abort
	ctrl.beginPlanningTurn()
	if ctrl.turnPhase != preemptTurnPlanning {
		t.Error("expected turnPhase = planning after second begin")
	}
}

func TestPreemptController_PushCriticalSection(t *testing.T) {
	ctrl := newPreemptController()
	ctrl.beginPlanningTurn()
	ctrl.beginActiveTurn(context.Background(), nil)

	// beginPush captures current turn state
	snap := ctrl.beginPush()
	if !snap.hasTargetTurn {
		t.Error("expected hasTargetTurn = true")
	}
	ctrl.endPush()

	// Request preempt during active phase
	ack := make(chan struct{})
	ctrl.requestPreempt(snap, ack)

	// The watcher would call receivePreempt and ack
	req, ok := ctrl.receivePreempt()
	if !ok {
		t.Fatal("expected preempt request to be available")
	}
	req.ack()

	select {
	case <-ack:
	case <-time.After(time.Second):
		t.Error("preempt ack not received within timeout")
	}
}

func TestPreemptController_StaleTurnPreempt(t *testing.T) {
	ctrl := newPreemptController()
	ctrl.beginPlanningTurn()
	ctrl.beginActiveTurn(context.Background(), nil)

	// Capture snapshot of turn 1
	snap := ctrl.beginPush()
	ctrl.endPush()

	// Complete turn 1
	ctrl.endActiveTurn().ack()

	// Start turn 2
	ctrl.beginPlanningTurn()
	ctrl.beginActiveTurn(context.Background(), nil)

	// Request preempt on stale turn 1 snapshot - should be no-op
	ack := make(chan struct{})
	done := make(chan struct{})
	go func() {
		ctrl.requestPreempt(snap, ack)
		close(done)
	}()
	select {
	case <-done:
		// requestPreempt resolved immediately with ack closed (stale turn)
	case <-time.After(time.Second):
		t.Error("stale preempt request blocked")
	}

	ctrl.endActiveTurn().ack()
}

func TestPreemptController_WaitForPushes(t *testing.T) {
	ctrl := newPreemptController()

	// Start a push and keep it in-flight
	snap := ctrl.beginPush()
	defer func() {
		// If push was ended, this is a no-op; if not, this panics
		_ = snap
	}()

	done := make(chan struct{})
	go func() {
		ctrl.waitForPushes()
		close(done)
	}()

	// waitForPushes should block while pushInFlight > 0
	select {
	case <-done:
		t.Error("waitForPushes returned while push was in-flight")
	case <-time.After(10 * time.Millisecond):
		// Expected: still blocked
	}

	// End the push - should unblock waitForPushes
	ctrl.endPush()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("waitForPushes didn't unblock after push ended")
	}
}

func TestPreemptController_CloseForLoopExit(t *testing.T) {
	ctrl := newPreemptController()
	ctrl.beginPlanningTurn()
	ctrl.beginActiveTurn(context.Background(), nil)

	ctrl.closeForLoopExit()
	if !ctrl.closed {
		t.Error("expected closed = true after closeForLoopExit")
	}
	if ctrl.turnPhase != preemptTurnIdle {
		t.Error("expected turnPhase = idle after closeForLoopExit")
	}
}

func TestPreemptController_RequestPreemptOnIdleTurn(t *testing.T) {
	ctrl := newPreemptController()
	ctrl.beginPlanningTurn()
	ctrl.beginActiveTurn(context.Background(), nil)
	ctrl.endActiveTurn().ack()

	// Turn is now idle
	snap := ctrl.beginPush()
	ctrl.endPush()

	ack := make(chan struct{})
	done := make(chan struct{})
	go func() {
		ctrl.requestPreempt(snap, ack)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("preempt request on idle turn blocked")
	}
}

// ---- stopController tests ----

func TestStopController_Lifecycle(t *testing.T) {
	ctrl := newStopController()

	if ctrl.isCommitted() {
		t.Error("expected not committed initially")
	}

	// Commit
	committed := ctrl.commit()
	if !committed {
		t.Error("expected commit = true on first commit")
	}
	if !ctrl.isCommitted() {
		t.Error("expected isCommitted = true after commit")
	}

	// Double commit is no-op
	committed = ctrl.commit()
	if committed {
		t.Error("expected commit = false on second commit")
	}
}

func TestStopController_ActiveTurnLifecycle(t *testing.T) {
	ctrl := newStopController()

	ctrl.beginActiveTurn()
	ctrl.endActiveTurn()
	// endActiveTurn returns nil pending, which is fine
}

func TestStopController_RequestStop(t *testing.T) {
	ctrl := newStopController()

	decision := ctrl.requestStop(&stopConfig{})
	if !decision.commit {
		t.Error("expected commit = true on first requestStop")
	}
	if !ctrl.isCommitted() {
		t.Error("expected committed after requestStop")
	}
}

func TestStopController_StopCause(t *testing.T) {
	ctrl := newStopController()

	ctrl.requestStop(&stopConfig{stopCause: "user_requested"})
	if ctrl.cause() != "user_requested" {
		t.Errorf("expected cause = user_requested, got %q", ctrl.cause())
	}

	// Second cause is ignored
	ctrl.requestStop(&stopConfig{stopCause: "other"})
	if ctrl.cause() != "user_requested" {
		t.Errorf("expected cause to remain user_requested, got %q", ctrl.cause())
	}
}

func TestStopController_SkipCheckpoint(t *testing.T) {
	ctrl := newStopController()

	if ctrl.skipCheckpointEnabled() {
		t.Error("expected skipCheckpoint = false initially")
	}

	ctrl.requestStop(&stopConfig{skipCheckpoint: true})
	if !ctrl.skipCheckpointEnabled() {
		t.Error("expected skipCheckpoint = true after request")
	}
}

func TestStopController_IdleDuration(t *testing.T) {
	ctrl := newStopController()

	if ctrl.idleDuration() != 0 {
		t.Error("expected idleDuration = 0 initially")
	}

	ctrl.requestStop(&stopConfig{idleFor: 5 * time.Second})
	if ctrl.idleDuration() != 5*time.Second {
		t.Errorf("expected idleDuration = 5s, got %v", ctrl.idleDuration())
	}

	// After commit, idleFor is cleared
	ctrl.requestStop(&stopConfig{})
	if ctrl.idleDuration() != 0 {
		t.Error("expected idleDuration = 0 after commit")
	}
}

func TestStopController_ReceiveCancel(t *testing.T) {
	ctrl := newStopController()

	_, ok := ctrl.receiveCancel()
	if ok {
		t.Error("expected no cancel without pending request")
	}

	ctrl.beginActiveTurn()
	ctrl.requestStop(&stopConfig{agentCancelOpts: []CancelOption{
		WithCancelMode(CancelAfterChatModel),
	}})
	_, ok = ctrl.receiveCancel()
	if !ok {
		t.Error("expected cancel to be available after requestStop with agentCancelOpts")
	}

	ctrl.endActiveTurn()
}

func TestStopController_CloseForLoopExit(t *testing.T) {
	ctrl := newStopController()

	ctrl.closeForLoopExit()
	if !ctrl.closed {
		t.Error("expected closed after closeForLoopExit")
	}

	// After close, commit should be no-op
	if ctrl.commit() {
		t.Error("expected commit = false after close")
	}
}

func TestStopController_RequestStopOnClosed(t *testing.T) {
	ctrl := newStopController()
	ctrl.closeForLoopExit()

	decision := ctrl.requestStop(&stopConfig{})
	if decision.commit {
		t.Error("expected no commit on closed controller")
	}
}

func TestStopController_UntilIdleFlow(t *testing.T) {
	ctrl := newStopController()

	// Request idle-for stop
	decision := ctrl.requestStop(&stopConfig{idleFor: 100 * time.Millisecond})
	if !decision.wakeIdle {
		t.Error("expected wakeIdle = true")
	}
	if ctrl.idleDuration() != 100*time.Millisecond {
		t.Errorf("expected idleDuration = 100ms, got %v", ctrl.idleDuration())
	}
}
