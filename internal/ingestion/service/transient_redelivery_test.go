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
