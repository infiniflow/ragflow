package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// TestReconcileMemoryTasksNeverStrandsOnLookupFailure is a regression guard for
// the dead-code removal in ReconcileMemoryTasks. The old `if err != nil {
// continue }` after resolveMemoryTenant could never fire (resolveMemoryTenant
// swallows lookup failures and returns a nil error) and would have stranded the
// wake-up had the seam ever errored. This asserts the two properties that must
// hold regardless: a lookup failure (here a deleted memory whose config cannot
// be resolved) is a benign fallback to the default lane — the wake-up is still
// published and no spurious reconcile error is surfaced.
func TestReconcileMemoryTasksNeverStrandsOnLookupFailure(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	pinMemoryNow(t, now)
	db := testutil.SetupTestDB(t, &entity.MemoryTask{})
	if err := db.Create(&entity.MemoryTask{
		TaskID: "due", MemoryID: "memory-broken", SourceID: 1,
		Input: entity.JSONMap{}, State: entity.MemoryTaskStatePending,
	}).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	publisher := &recordingTaskPublisher{}
	svc := NewMemoryMessageService(NewMemoryService(db, nil))
	svc.taskPublisher = publisher
	svc.tierResolver = (&fakeTierResolver{tier: common.TierDefault}).resolve
	svc.memoryConfigFn = func(_ context.Context, _ string) (*CreateMemoryResponse, error) {
		// A deleted memory (or any unresolvable lookup) returns an error; the
		// reconcile path must fall back to the default lane, not fail.
		return nil, errors.New("memory not found")
	}

	if err := svc.ReconcileMemoryTasks(t.Context(), 100); err != nil {
		t.Fatalf("ReconcileMemoryTasks error = %v, want nil (lookup failure is a benign fallback)", err)
	}
	if publisher.subject != common.TaskSubject || len(publisher.messages) != 1 {
		t.Fatalf("wake-up not published to default lane: subject=%q messages=%d", publisher.subject, len(publisher.messages))
	}
	if publisher.messages[0].TaskID != "due" {
		t.Fatalf("published wake-up TaskID = %q, want %q", publisher.messages[0].TaskID, "due")
	}
}
