package service

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
	servicepkg "ragflow/internal/service"
)

// TestExecuteMemoryTask_PanicDoesNotPropagate verifies that a panic raised inside
// the memory-extraction path (HandleSaveToMemoryTask) is recovered by
// executeMemoryTask. Before the fix, executeMemoryTask had no recover guard
// (unlike the ingestion path's settleMessage), so a panic crashed the worker
// goroutine. With max_concurrent_workers defaults that can be as low as 1, a
// single panicking memory task permanently removed the only worker, after which
// every normal document parse (PDF etc.) stalled in the queue — exactly the
// "parsing works at first, then breaks after some memory tasks" symptom.
//
// The test injects a panic through the runMemoryTask seam (mirrors runDocumentTask)
// so it does not depend on a live DB or a real MemoryMessageService. The contract
// asserted:
//   - executeMemoryTask returns normally (no panic propagates to the caller/worker)
//   - the MQ message is Nacked so the broker redelivers it (never silently dropped)
func TestExecuteMemoryTask_PanicDoesNotPropagate(t *testing.T) {
	_ = testutil.SetupTestDB(t) // ensure DB helpers are initialized; not exercised here

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	// Real (non-nil) memory service so executeMemoryTask reaches the
	// runMemoryTask dispatch path instead of early-Acking on nil service.
	ingestor.SetMemoryMessageService(servicepkg.NewMemoryMessageService(servicepkg.NewMemoryService()))
	// Inject a panicking memory runner through the same seam used in production
	// (defaultRunMemoryTask calls memorySvc.HandleSaveToMemoryTask).
	ingestor.runMemoryTask = func(_ context.Context, _ string, _ map[string]any) error {
		panic("simulated memory extraction panic")
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-panic-1", TaskType: common.TaskTypeMemory}}
	taskCtx := taskpkg.NewMemoryTaskContextForScheduling(context.Background(), "mem-panic-1", map[string]any{
		"memory_id": "mem-p", "source_id": 1,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	}, handle)

	// If the panic is not recovered, this call itself will panic and the test
	// fails. We also guard with a recover here only to convert a propagated
	// panic into a clear test failure (the real recovery belongs in production).
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("executeMemoryTask let a memory panic propagate: %v", r)
			}
		}()
		ingestor.executeMemoryTask(context.Background(), taskCtx)
	}()

	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("panicking memory task: expected 0 Ack/1 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestWorkerLoop_SurvivesMemoryPanicAndKeepsServing proves the production
// failure mode end-to-end: a panicking memory task must NOT kill the worker
// goroutine, so a subsequent normal document parse is still processed. This is
// the exact "parsing works at first, then breaks after some memory tasks"
// symptom — at max_concurrent_workers=1 the only worker is lost on the first
// panicking memory task, and every later PDF stalls in the queue.
//
// The test drives the real workerLoop (not executeMemoryTask directly): it
// starts one worker, enqueues (1) a panicking memory task, then (2) a normal
// ingestion task with a stubbed runDocumentTask, and asserts the worker both
// Nacked the poison memory task and Acked the document task before exiting.
func TestWorkerLoop_SurvivesMemoryPanicAndKeepsServing(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.SetMemoryMessageService(servicepkg.NewMemoryMessageService(servicepkg.NewMemoryService()))
	ingestor.runMemoryTask = func(_ context.Context, _ string, _ map[string]any) error {
		panic("simulated memory extraction panic")
	}
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return nil
	}

	memHandle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-panic-2", TaskType: common.TaskTypeMemory}}
	memCtx := taskpkg.NewMemoryTaskContextForScheduling(context.Background(), "mem-panic-2", map[string]any{
		"memory_id": "mem-p2", "source_id": 1,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	}, memHandle)

	docHandle := &fakeTaskHandle{}
	docCtx := newAckTaskCtx(context.Background(), taskID, docID, docHandle)

	ingestor.startWorkerPool()
	// Feed the poison memory task first, then a normal document task.
	ingestor.taskChan <- memCtx
	ingestor.taskChan <- docCtx

	// Wait for the worker to drain both tasks (poison Nacked, document Acked)
	// before cancelling, so the loop does not exit on ctx.Done() first.
	deadline := time.After(5 * time.Second)
	for memHandle.nacks.Load() != 1 || docHandle.acks.Load() != 1 {
		select {
		case <-deadline:
			t.Fatalf("worker did not drain tasks in time: mem(acks=%d nacks=%d) doc(acks=%d nacks=%d)",
				memHandle.acks.Load(), memHandle.nacks.Load(), docHandle.acks.Load(), docHandle.nacks.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// Cancel the ingestor context so the worker loop exits; then wait for the
	// worker goroutine to finish cleanly (proves it was not crashed by the panic).
	ingestor.cancel()
	ingestor.workerWg.Wait()

	if memHandle.nacks.Load() != 1 || memHandle.acks.Load() != 0 {
		t.Fatalf("poison memory task: expected 0 Ack/1 Nack, got acks=%d nacks=%d", memHandle.acks.Load(), memHandle.nacks.Load())
	}
	if docHandle.acks.Load() != 1 || docHandle.nacks.Load() != 0 {
		t.Fatalf("document task after memory panic: expected 1 Ack/0 Nack (worker survived), got acks=%d nacks=%d", docHandle.acks.Load(), docHandle.nacks.Load())
	}
}
