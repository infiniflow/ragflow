package syncer

import (
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
	scheduler.retireTimer("task-1", original)

	scheduler.timerMu.Lock()
	got := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if got != replacement {
		t.Fatalf("stale callback removed replacement: got %p, want %p", got, replacement)
	}

	// The current callback must still be allowed to retire its own timer.
	replacement.Stop()
	scheduler.retireTimer("task-1", replacement)
	scheduler.timerMu.Lock()
	_, exists := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if exists {
		t.Fatal("current callback did not remove its timer")
	}
}
