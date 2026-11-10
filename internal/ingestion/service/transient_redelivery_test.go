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

	if handled := ingestor.deferTransientTask(t.Context(), taskCtx, errors.New("Infinity is initing")); !handled {
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

	if handled := ingestor.deferTransientTask(t.Context(), taskCtx, errors.New("Infinity is initing")); handled {
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

	if handled := ingestor.deferTransientTask(t.Context(), taskCtx, errors.New("Infinity is initing")); !handled {
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

	if handled := ingestor.deferTransientTask(ctx, taskCtx, errors.New("Infinity is initing")); !handled {
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
