package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

// transientTaskHandle adds the optional broker capabilities the transient
// settle path uses (paced nack + delivery budget) on top of fakeTaskHandle.
type transientTaskHandle struct {
	fakeTaskHandle
	delivered   uint64
	limit       uint64
	budgetKnown bool
	delays      []time.Duration
}

func (h *transientTaskHandle) NackWithDelay(delay time.Duration) error {
	h.delays = append(h.delays, delay)
	h.nacks.Add(1)
	return nil
}

func (h *transientTaskHandle) RedeliveryBudget() (uint64, uint64, bool) {
	return h.delivered, h.limit, h.budgetKnown
}

func newTransientTaskCtx(ctx context.Context, taskID, docID string, handle *transientTaskHandle) *taskpkg.TaskContext {
	taskCtx := taskpkg.NewTaskContextForScheduling(ctx, &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})
	taskCtx.Handle = handle
	return taskCtx
}

// A bare Nack is redelivered at once, so the transient path must ask for the
// paced delay that mirrors the consumer BackOff.
func TestDeferTransientTaskNacksWithPacedDelay(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	handle := &transientTaskHandle{delivered: 2, limit: 16, budgetKnown: true}
	taskCtx := newTransientTaskCtx(t.Context(), taskID, docID, handle)

	if handled := ingestor.deferTransientTask(t.Context(), taskCtx, nil, errors.New("Infinity is initing")); !handled {
		t.Fatal("expected the transient failure to be settled with a nack")
	}
	if len(handle.delays) != 1 || handle.delays[0] != 15*time.Second {
		t.Fatalf("delays = %v, want one 15s delay for delivery 2", handle.delays)
	}
	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("expected 1 Nack / 0 Ack, got nacks=%d acks=%d", handle.nacks.Load(), handle.acks.Load())
	}
}

// Once the broker stops delivering, leaving the task RUNNING would be a dead
// end: the last delivery must mark it FAILED so the caller Acks.
func TestDeferTransientTaskFailsAtDeliveryLimit(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	handle := &transientTaskHandle{delivered: 16, limit: 16, budgetKnown: true}
	taskCtx := newTransientTaskCtx(t.Context(), taskID, docID, handle)

	if handled := ingestor.deferTransientTask(t.Context(), taskCtx, nil, errors.New("Infinity is initing")); handled {
		t.Fatal("expected an exhausted budget to settle as terminal, not nack")
	}
	if handle.nacks.Load() != 0 || handle.acks.Load() != 0 {
		t.Fatalf("expected no direct settlement, got nacks=%d acks=%d", handle.nacks.Load(), handle.acks.Load())
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %s, want FAILED at the delivery limit", task.Status)
	}
}

// The terminal write is the last chance to move the task out of RUNNING, so a
// single transient DB error must not be accepted as final.
func TestRetryTerminalWriteRecoversAfterTransientFailure(t *testing.T) {
	attempts := 0
	ok := retryTerminalWrite(t.Context(), nil, func() bool {
		attempts++
		return attempts == 2
	}, []time.Duration{time.Millisecond})
	if !ok || attempts != 2 {
		t.Fatalf("retryTerminalWrite = %v after %d attempts, want success on the second", ok, attempts)
	}
}

// The delivery limit leaves no later delivery, so the retry keeps going after
// the delay schedule is exhausted instead of giving up on the task.
func TestRetryTerminalWriteKeepsRetryingAfterTheSchedule(t *testing.T) {
	attempts := 0
	ok := retryTerminalWrite(t.Context(), nil, func() bool {
		attempts++
		return attempts == 5
	}, []time.Duration{time.Millisecond, time.Millisecond})
	if !ok || attempts != 5 {
		t.Fatalf("retryTerminalWrite = %v after %d attempts, want success on the fifth", ok, attempts)
	}
}

// Holding the last delivery means renewing its in-flight deadline; otherwise
// the broker would take the message back while the write is still retrying.
func TestRetryTerminalWriteRenewsTheDelivery(t *testing.T) {
	handle := &fakeTaskHandle{}
	attempts := 0
	ok := retryTerminalWrite(t.Context(), handle, func() bool {
		attempts++
		return attempts == 3
	}, []time.Duration{time.Millisecond})
	if !ok {
		t.Fatal("expected the write to succeed on the third attempt")
	}
	if got := handle.inProgress.Load(); got != 2 {
		t.Fatalf("InProgress calls = %d, want one per retry", got)
	}
}

func TestRetryTerminalWriteStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	attempts := 0
	ok := retryTerminalWrite(ctx, nil, func() bool {
		attempts++
		return false
	}, []time.Duration{time.Hour})
	if ok || attempts != 2 {
		t.Fatalf("retryTerminalWrite = %v after %d attempts, want the initial try plus one final try", ok, attempts)
	}
}

// When the broker metadata is unavailable there is no delivery budget to
// exhaust, so the failure stays on the paced-redelivery path.
func TestDeferTransientTaskUnknownBudgetNacksWithDelay(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	handle := &transientTaskHandle{budgetKnown: false}
	taskCtx := newTransientTaskCtx(t.Context(), taskID, docID, handle)

	if handled := ingestor.deferTransientTask(t.Context(), taskCtx, nil, errors.New("Infinity is initing")); !handled {
		t.Fatal("expected an unknown budget to settle with a nack")
	}
	if len(handle.delays) != 1 || handle.delays[0] != 5*time.Second {
		t.Fatalf("delays = %v, want one 5s delay when the budget is unknown", handle.delays)
	}
	if handle.acks.Load() != 0 {
		t.Fatalf("expected no Ack, got %d", handle.acks.Load())
	}
}

// An unwritable terminal state keeps the message in the stream instead of
// Acking it away once the worker stops retrying.
func TestDeferTransientTaskKeepsMessageWhenTerminalWriteNeverLands(t *testing.T) {
	oldDelays := terminalWriteRetryDelays
	terminalWriteRetryDelays = nil
	t.Cleanup(func() { terminalWriteRetryDelays = oldDelays })

	defer testutil.ReplaceDBForTest(t, nil)()

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	handle := &transientTaskHandle{delivered: 16, limit: 16, budgetKnown: true}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	taskCtx := newTransientTaskCtx(ctx, "task-1", "doc-1", handle)

	if handled := ingestor.deferTransientTask(ctx, taskCtx, nil, errors.New("Infinity is initing")); !handled {
		t.Fatal("expected the nack fallback when the FAILED write cannot land")
	}
	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("expected 1 Nack / 0 Ack, got nacks=%d acks=%d", handle.nacks.Load(), handle.acks.Load())
	}
	if handle.inProgress.Load() == 0 {
		t.Fatal("expected the delivery to be renewed while the write was retried")
	}
}

func TestTransientRedeliveryDelay(t *testing.T) {
	cases := map[uint64]time.Duration{
		0:  5 * time.Second,
		1:  5 * time.Second,
		2:  15 * time.Second,
		3:  30 * time.Second,
		4:  60 * time.Second,
		16: 60 * time.Second,
	}
	for delivered, want := range cases {
		if got := transientRedeliveryDelay(delivered); got != want {
			t.Fatalf("transientRedeliveryDelay(%d) = %v, want %v", delivered, got, want)
		}
	}
}

// A stop request that lands while the last delivery is failing must settle the
// task STOPPED. MarkFailed only accepts RUNNING, so without the re-read the
// retry loop would hold the worker until shutdown and leave the task STOPPING.
func TestDeferTransientTaskSettlesStoppingTaskAsStopped(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.STOPPING).Error; err != nil {
		t.Fatalf("set status STOPPING: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	handle := &transientTaskHandle{delivered: 16, limit: 16, budgetKnown: true}
	taskCtx := newTransientTaskCtx(t.Context(), taskID, docID, handle)

	if handled := ingestor.deferTransientTask(t.Context(), taskCtx, nil, errors.New("Infinity is initing")); handled {
		t.Fatal("expected a stopped task to settle as terminal, not nack")
	}
	final, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if final.Status != common.STOPPED {
		t.Fatalf("task status = %s, want STOPPED", final.Status)
	}
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("deferTransientTask must not settle the message, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// A task another worker already settled (any terminal status) must settle the
// message without a failure record: the service's idempotent MarkFailed no-op
// reports changed=false, and surfacing that as a new failure would write a
// FAILED terminal entry over a status this worker never observed.
func TestMarkFailedOrStoppedSkipsFailureLogOnAlreadyTerminalTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.COMPLETED).Error; err != nil {
		t.Fatalf("set status COMPLETED: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	settled, failed := ingestor.markFailedOrStopped(t.Context(), taskID, "Task failed: timeout")
	if !settled {
		t.Fatal("an already-terminal task should settle without a retry")
	}
	if failed {
		t.Fatal("an idempotent MarkFailed no-op must not be reported as a new failure")
	}
	final, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if final.Status != common.COMPLETED {
		t.Fatalf("task status = %s, want the pre-existing COMPLETED", final.Status)
	}
}

// The last delivery has no successor, so retrying the terminal write must keep
// renewing the message's in-flight deadline; otherwise the broker takes the
// message back while the write is still running. The heartbeat stops only when
// the message is settled.
func TestSettleMessageRenewsDeliveryWhileTheTerminalWriteRetries(t *testing.T) {
	// A wait longer than the test: the heartbeat is the only renewal that can
	// fire while the retry sits in the delay, so a stopped heartbeat shows up
	// as a missing InProgress below.
	oldDelays := terminalWriteRetryDelays
	terminalWriteRetryDelays = []time.Duration{time.Hour}
	t.Cleanup(func() { terminalWriteRetryDelays = oldDelays })

	defer testutil.ReplaceDBForTest(t, nil)()

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	handle := &transientTaskHandle{delivered: 16, limit: 16, budgetKnown: true}
	taskCtx := newTransientTaskCtx(t.Context(), "task-1", "doc-1", handle)

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	hb := NewHeartbeat("task-1", handle, 5*time.Millisecond)
	hb.Start()

	if terminal := ingestor.settleMessage(ctx, taskCtx, hb, func(context.Context) taskOutcome {
		return taskOutcome{transientErr: errors.New("Infinity is initing")}
	}); terminal {
		t.Fatal("expected the unsettled delivery to stay non-terminal")
	}
	if renewed := handle.inProgress.Load(); renewed < 5 {
		t.Fatalf("InProgress calls = %d, want the heartbeat to keep renewing during the retries", renewed)
	}
	after := handle.inProgress.Load()
	time.Sleep(30 * time.Millisecond)
	if got := handle.inProgress.Load(); got != after {
		t.Fatalf("heartbeat kept renewing after settlement: %d -> %d", after, got)
	}
	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("expected 1 Nack / 0 Ack, got nacks=%d acks=%d", handle.nacks.Load(), handle.acks.Load())
	}
}
