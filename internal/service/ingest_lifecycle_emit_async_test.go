package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// slowRecordingEmitter records emitted ingest logs but delays each emit so we can
// assert that emitLifecycleEvent returns immediately (fire-and-forget) instead of
// blocking the caller on the instrumentation work.
type slowRecordingEmitter struct {
	mu     sync.Mutex
	events []common.IngestionLog
	delay  time.Duration
}

func (s *slowRecordingEmitter) EmitIngestLog(_ context.Context, log common.IngestionLog) {
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, log)
}

func (s *slowRecordingEmitter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *slowRecordingEmitter) last() common.IngestionLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) == 0 {
		return common.IngestionLog{}
	}
	return s.events[len(s.events)-1]
}

// panickingEmitter makes the instrumentation path blow up, to prove a failure
// inside emit never escapes into the business flow.
type panickingEmitter struct{}

func (p *panickingEmitter) EmitIngestLog(_ context.Context, _ common.IngestionLog) {
	panic("instrumentation boom")
}

// ingestTestEmitTimeout bounds how long a test waits for the fire-and-forget
// emit goroutine to actually emit.
const ingestTestEmitTimeout = 2 * time.Second

// waitForEmitCount polls until countFn reports at least want, used to await the
// background instrumentation goroutine after a fire-and-forget emit.
func waitForEmitCount(countFn func() int, want int) bool {
	deadline := time.Now().Add(ingestTestEmitTimeout)
	for time.Now().Before(deadline) {
		if countFn() >= want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// TestEmitLifecycleEventIsFireAndForgetNonBlocking verifies the core contract:
// emitLifecycleEvent must return immediately and never block the business flow on
// instrumentation work, even when the sink itself is slow.
func TestEmitLifecycleEventIsFireAndForgetNonBlocking(t *testing.T) {
	rec := &slowRecordingEmitter{delay: 500 * time.Millisecond}
	svc := &IngestionTaskService{eventEmitter: rec}
	task := &entity.IngestionTask{ID: "task1"}

	start := time.Now()
	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "")
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("emitLifecycleEvent blocked for %v; must be fire-and-forget and return immediately", elapsed)
	}

	if !waitForEmitCount(rec.count, 1) {
		t.Fatalf("event not emitted by background goroutine within timeout")
	}
}

// TestEmitLifecycleEventIsolatesPanic verifies that a panic inside the
// instrumentation path is contained and never propagates to the business flow
// caller.
func TestEmitLifecycleEventIsolatesPanic(t *testing.T) {
	svc := &IngestionTaskService{eventEmitter: &panickingEmitter{}}
	task := &entity.IngestionTask{ID: "task1"}

	// Must not panic the caller even though the emitter panics.
	svc.emitLifecycleEvent(context.Background(), task, "completed", "COMPLETED", "")
	// Reaching here means the panic was isolated to the instrumentation goroutine.
	time.Sleep(50 * time.Millisecond)
}
