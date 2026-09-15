package service

import (
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
)

// controllableHandle is a TaskHandle whose InProgress behavior is delegated,
// for testing heartbeat timing and shutdown without a live broker.
type controllableHandle struct {
	inProgressFn func() error
}

func (h *controllableHandle) GetMessage() common.TaskMessage { return common.TaskMessage{} }
func (h *controllableHandle) Ack() error                     { return nil }
func (h *controllableHandle) Nack() error                    { return nil }
func (h *controllableHandle) InProgress() error              { return h.inProgressFn() }

// TestHeartbeat_TicksInProgressUntilStop: with a short interval the heartbeat
// goroutine calls InProgress repeatedly; Stop halts it.
func TestHeartbeat_TicksInProgressUntilStop(t *testing.T) {
	handle := &fakeTaskHandle{}
	hb := NewHeartbeat("task-1", handle, 2*time.Millisecond)
	hb.Start()

	deadline := time.Now().Add(2 * time.Second)
	for handle.inProgress.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if handle.inProgress.Load() == 0 {
		t.Fatal("expected InProgress heartbeats, got 0")
	}

	hb.Stop()
	after := handle.inProgress.Load()
	time.Sleep(10 * time.Millisecond)
	if got := handle.inProgress.Load(); got != after {
		t.Fatalf("InProgress calls continued after Stop: before=%d after=%d", after, got)
	}
}

// TestHeartbeat_StopWaitsForInFlightInProgress: Stop must block until an
// in-flight InProgress call returns, so the caller can ack/nack with no
// concurrent InProgress on the same message. Regression guard for the
// close-without-wait heartbeat shutdown.
func TestHeartbeat_StopWaitsForInFlightInProgress(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	h := &controllableHandle{
		inProgressFn: func() error {
			startedOnce.Do(func() { close(started) })
			<-release
			return nil
		},
	}
	hb := NewHeartbeat("task-1", h, time.Millisecond)
	hb.Start()
	<-started // heartbeat goroutine is inside InProgress

	stopDone := make(chan struct{})
	go func() { hb.Stop(); close(stopDone) }()

	select {
	case <-stopDone:
		t.Fatal("Stop returned before in-flight InProgress completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(release) // let InProgress complete

	select {
	case <-stopDone:
		// good: Stop returned only after InProgress completed
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after in-flight InProgress completed")
	}
}

// TestHeartbeat_NoOpWhenNoHandle: with no MQ handle, Start starts no goroutine
// and Stop returns immediately (never blocks).
func TestHeartbeat_NoOpWhenNoHandle(t *testing.T) {
	hb := NewHeartbeat("task-1", nil, time.Millisecond)
	hb.Start()
	hb.Stop() // must not block or panic
}

// TestHeartbeat_StopBeforeStart: Stop before Start is a no-op and never blocks,
// covering the "lease never started" path (e.g. a task context that admission
// rejected before starting a heartbeat).
func TestHeartbeat_StopBeforeStart(t *testing.T) {
	hb := NewHeartbeat("task-1", &fakeTaskHandle{}, time.Millisecond)
	done := make(chan struct{})
	go func() { hb.Stop(); close(done) }()
	select {
	case <-done:
		// good: returned immediately
	case <-time.After(time.Second):
		t.Fatal("Stop before Start blocked")
	}
}

// TestHeartbeat_StopBeforeStartPreventsLaterStart ensures a stopped heartbeat
// cannot launch a renewal goroutine afterwards.
func TestHeartbeat_StopBeforeStartPreventsLaterStart(t *testing.T) {
	handle := &fakeTaskHandle{}
	hb := NewHeartbeat("task-1", handle, time.Millisecond)

	hb.Stop()
	hb.Start()
	time.Sleep(10 * time.Millisecond)

	if got := handle.inProgress.Load(); got != 0 {
		t.Fatalf("InProgress calls after Stop then Start = %d, want 0", got)
	}
}
