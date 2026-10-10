package syncer

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSchedulerExpiredCallbackKeepsReplacementTimer verifies the stale
// callback cleanup cannot remove a newer timer for the same task.
func TestSchedulerExpiredCallbackKeepsReplacementTimer(t *testing.T) {
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), nil, &fakeSyncTaskBroker{})
	defer scheduler.stopTimers()

	if err := scheduler.ScheduleTaskAfter(t.Context(), "task-1", time.Hour); err != nil {
		t.Fatalf("schedule first timer: %v", err)
	}
	scheduler.timerMu.Lock()
	original := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if original == nil {
		t.Fatal("original timer not registered")
	}

	if err := scheduler.ScheduleTaskAfter(t.Context(), "task-1", 2*time.Hour); err != nil {
		t.Fatalf("schedule replacement timer: %v", err)
	}
	scheduler.timerMu.Lock()
	replacement := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if replacement == nil || replacement == original {
		t.Fatalf("replacement timer = %p, original = %p", replacement, original)
	}
	defer replacement.Stop()

	// Simulate the original callback reaching its cleanup after replacement.
	// Invoke the exact same method used by the real timer callback: unlike a
	// sleep-based assertion, this cannot pass before cleanup has executed.
	scheduler.retireTimer("task-1", &original)

	scheduler.timerMu.Lock()
	got := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if got != replacement {
		t.Fatalf("stale callback removed replacement: got %p, want %p", got, replacement)
	}

	// The current callback must still be allowed to retire its own timer.
	replacement.Stop()
	scheduler.retireTimer("task-1", &replacement)
	scheduler.timerMu.Lock()
	_, exists := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if exists {
		t.Fatal("current callback did not remove its timer")
	}
}

// TestSchedulerStaleFailureKeepsReplacementTimer verifies that the retry
// path cannot overwrite a newer timer when an old callback fails.
func TestSchedulerStaleFailureKeepsReplacementTimer(t *testing.T) {
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), nil, &fakeSyncTaskBroker{})
	defer scheduler.stopTimers()

	if err := scheduler.ScheduleTaskAfter(t.Context(), "task-1", time.Hour); err != nil {
		t.Fatalf("schedule first timer: %v", err)
	}
	scheduler.timerMu.Lock()
	original := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()

	if err := scheduler.ScheduleTaskAfter(t.Context(), "task-1", 2*time.Hour); err != nil {
		t.Fatalf("schedule replacement timer: %v", err)
	}
	scheduler.timerMu.Lock()
	replacement := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if original == nil || replacement == nil || original == replacement {
		t.Fatalf("invalid timer replacement: old %p, new %p", original, replacement)
	}
	defer replacement.Stop()

	// Simulate the stale callback's retry, using the same path as the
	// production callback. The newer schedule must not be overwritten.
	scheduler.retryTimerIfCurrent(t.Context(), "task-1", &original)
	scheduler.timerMu.Lock()
	got := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if got != replacement {
		t.Fatalf("stale retry replaced newer timer: got %p, want %p", got, replacement)
	}

	// A genuinely current timer must still be retried on publish failure.
	scheduler.retryTimerIfCurrent(t.Context(), "task-1", &replacement)
	scheduler.timerMu.Lock()
	retry := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if retry == nil || retry == replacement {
		t.Fatalf("current timer not retried: got %p, previous %p", retry, replacement)
	}
}

// TestSchedulerCallbackPreservesReplacement exercises the actual callback
// path through publication and cleanup, rather than just the cleanup helper.
func TestSchedulerCallbackPreservesReplacement(t *testing.T) {
	ctx := t.Context()
	var scheduler *Scheduler
	broker := &fakeSyncTaskBroker{}
	broker.onPublish = func(taskID string) {
		if err := scheduler.ScheduleTaskAfter(ctx, taskID, 2*time.Hour); err != nil {
			t.Errorf("schedule replacement: %v", err)
		}
	}
	scheduler = NewNATSScheduler(make(chan TaskEnvelope, 1), nil, broker)
	defer scheduler.stopTimers()

	if err := scheduler.ScheduleTaskAfter(ctx, "task-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	scheduler.timerMu.Lock()
	original := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	original.Stop()

	// The old callback's publish synchronously schedules the replacement.
	// The handler cannot return until the stale cleanup has also completed.
	scheduler.handleTimerWakeup(ctx, "task-1", &original)
	scheduler.timerMu.Lock()
	replacement := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if replacement == nil || replacement == original {
		t.Fatalf("callback removed the replacement: old=%p new=%p", original, replacement)
	}
}

type failingTimerWakeupBroker struct {
	*fakeSyncTaskBroker
	onWakeup func(string)
}

func (b *failingTimerWakeupBroker) PublishSyncerTaskWakeup(taskID string) error {
	if b.onWakeup != nil {
		b.onWakeup(taskID)
	}
	return errors.New("temporary publish failure")
}

// TestSchedulerCallbackFailurePreservesReplacement covers the case in which
// a stale publish failure must not overwrite an already replaced timer.
func TestSchedulerCallbackFailurePreservesReplacement(t *testing.T) {
	ctx := context.Background()
	var scheduler *Scheduler
	broker := &failingTimerWakeupBroker{fakeSyncTaskBroker: &fakeSyncTaskBroker{}}
	broker.onWakeup = func(taskID string) {
		if err := scheduler.ScheduleTaskAfter(ctx, taskID, 2*time.Hour); err != nil {
			t.Errorf("schedule replacement: %v", err)
		}
	}
	scheduler = NewNATSScheduler(make(chan TaskEnvelope, 1), nil, broker)
	defer scheduler.stopTimers()
	if err := scheduler.ScheduleTaskAfter(ctx, "task-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	scheduler.timerMu.Lock()
	original := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	original.Stop()

	scheduler.handleTimerWakeup(ctx, "task-1", &original)
	scheduler.timerMu.Lock()
	replacement := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if replacement == nil || replacement == original {
		t.Fatalf("stale failure replaced the new schedule: old=%p new=%p", original, replacement)
	}
}
