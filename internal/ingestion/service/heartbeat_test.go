package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
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

// TestStartHeartbeat_TicksInProgressUntilStop: with a short interval the
// heartbeat goroutine calls InProgress repeatedly; stop() halts it.
func TestStartHeartbeat_TicksInProgressUntilStop(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.heartbeatInterval = 2 * time.Millisecond

	handle := &fakeTaskHandle{}
	taskCtx := newAckTaskCtx(context.Background(), "task-1", "doc-1", handle)

	stop := ingestor.startHeartbeat(taskCtx)
	time.Sleep(15 * time.Millisecond)
	stop()

	if handle.inProgress.Load() == 0 {
		t.Fatal("expected InProgress heartbeats, got 0")
	}
}

// TestStartHeartbeat_StopWaitsForInFlightInProgress: stop() must block until an
// in-flight InProgress call returns, so the caller can ack/nack with no
// concurrent InProgress on the same message. Regression guard for the
// close-without-wait heartbeat shutdown (problem 1).
func TestStartHeartbeat_StopWaitsForInFlightInProgress(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.heartbeatInterval = time.Millisecond

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
	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		&entity.IngestionTask{ID: "task-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.RUNNING},
	)
	taskCtx.Handle = h

	stop := ingestor.startHeartbeat(taskCtx)
	<-started // heartbeat goroutine is inside InProgress

	stopDone := make(chan struct{})
	go func() { stop(); close(stopDone) }()

	select {
	case <-stopDone:
		t.Fatal("stop() returned before in-flight InProgress completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(release) // let InProgress complete

	select {
	case <-stopDone:
		// good: stop() returned only after InProgress completed
	case <-time.After(time.Second):
		t.Fatal("stop() did not return after in-flight InProgress completed")
	}
}

// TestStartHeartbeat_NoOpWhenNoHandle: with no MQ handle (standalone/test path)
// startHeartbeat returns a no-op stop and starts no goroutine.
func TestStartHeartbeat_NoOpWhenNoHandle(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.heartbeatInterval = time.Millisecond

	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		&entity.IngestionTask{ID: "task-1"},
	)
	// taskCtx.Handle is nil
	stop := ingestor.startHeartbeat(taskCtx)
	stop() // must not block or panic
}

// TestStartHeartbeat_TouchesClaimAndCancelsOnOwnershipLoss ensures the worker
// lease is renewed independently of the broker heartbeat, and that a zero-row
// TouchClaim immediately cancels the pipeline instead of letting a stale owner
// keep writing after another worker has claimed the task.
func TestStartHeartbeat_TouchesClaimAndCancelsOnOwnershipLoss(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	originalExpiry := time.Now().Add(30 * time.Millisecond).UnixMilli()
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"claim_token":      "owner-a",
		"claim_expires_at": originalExpiry,
	}).Error; err != nil {
		t.Fatalf("set claim: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.heartbeatInterval = 2 * time.Millisecond
	ingestor.claimTTL = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	taskCtx := taskpkg.NewTaskContextForScheduling(ctx, &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING, ClaimToken: "owner-a",
	})
	taskCtx.Cancel = cancel

	stop := ingestor.startHeartbeat(taskCtx)
	defer stop()

	deadline := time.Now().Add(time.Second)
	for {
		task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
		if err != nil {
			t.Fatalf("load task: %v", err)
		}
		if task.ClaimExpiresAt > originalExpiry {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("claim expiry was not extended")
		}
		time.Sleep(time.Millisecond)
	}

	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Update("claim_token", "owner-b").Error; err != nil {
		t.Fatalf("replace owner: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("lost claim did not cancel task context")
	}
}
