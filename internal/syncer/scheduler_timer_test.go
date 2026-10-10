package syncer

import (
	"context"
	"testing"
	"time"
)

// TestSchedulerExpiredCallbackKeepsReplacementTimer covers a timer callback
// finishing after another caller has scheduled the same task again.
func TestSchedulerExpiredCallbackKeepsReplacementTimer(t *testing.T) {
	firstPublish := make(chan struct{})
	releaseFirst := make(chan struct{})
	broker := &fakeSyncTaskBroker{
		onPublish: func(string) {
			close(firstPublish)
			<-releaseFirst
		},
	}
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), nil, broker)
	defer scheduler.stopTimers()

	if err := scheduler.ScheduleTaskAfter(context.Background(), "task-1", 10*time.Millisecond); err != nil {
		t.Fatalf("schedule first timer: %v", err)
	}
	select {
	case <-firstPublish:
	case <-time.After(time.Second):
		t.Fatal("first timer did not start publishing")
	}

	if err := scheduler.ScheduleTaskAfter(context.Background(), "task-1", time.Hour); err != nil {
		close(releaseFirst)
		t.Fatalf("schedule replacement timer: %v", err)
	}
	scheduler.timerMu.Lock()
	replacement := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if replacement == nil {
		close(releaseFirst)
		t.Fatal("replacement timer missing before original callback finished")
	}

	close(releaseFirst)
	// The old callback's completion must not delete the replacement.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		scheduler.timerMu.Lock()
		got := scheduler.timers["task-1"]
		scheduler.timerMu.Unlock()
		if got != replacement {
			t.Fatalf("old callback removed replacement timer: got %p, want %p", got, replacement)
		}
		time.Sleep(time.Millisecond)
	}
}
