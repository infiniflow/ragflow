package dao

import (
	"errors"
	"testing"
	"time"

	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func TestIngestionTaskDAOUpdateStatusIfCurrentSucceeds(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	task := &entity.IngestionTask{
		ID:         "task-1",
		UserID:     "user-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	ctx := t.Context()
	updated, err := NewIngestionTaskDAO().UpdateStatusIfCurrent(ctx, db, "task-1", common.CREATED, common.RUNNING)
	if err != nil {
		t.Fatalf("UpdateStatusIfCurrent failed: %v", err)
	}
	if !updated {
		t.Fatal("expected update to succeed")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.RUNNING {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.RUNNING)
	}
}

func TestIngestionTaskDAOCreateRejectsExistingTerminalTask(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	testCases := []struct {
		name   string
		status string
	}{
		{name: "failed", status: common.FAILED},
		{name: "stopped", status: common.STOPPED},
	}

	ctx := t.Context()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.WithContext(ctx).Where("id = ?", "task-1").Delete(&entity.IngestionTask{}).Error; err != nil {
				t.Fatalf("clear task: %v", err)
			}
			task := &entity.IngestionTask{ID: "task-1", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: tc.status}
			if err := db.WithContext(ctx).Create(task).Error; err != nil {
				t.Fatalf("create task: %v", err)
			}
			_, err := NewIngestionTaskDAO().Create(ctx, db, &entity.IngestionTask{ID: "task-2", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.CREATED})
			if err == nil {
				t.Fatal("expected Create to reject duplicate document task")
			}
			reloaded, err := NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
			if err != nil {
				t.Fatalf("reload task: %v", err)
			}
			if reloaded.Status != tc.status {
				t.Fatalf("status = %q, want %q", reloaded.Status, tc.status)
			}
		})
	}
}

func TestIngestionTaskDAODocumentIDIsUniqueAtDBLevel(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	first := &entity.IngestionTask{ID: "task-1", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.CREATED}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("create first task: %v", err)
	}

	second := &entity.IngestionTask{ID: "task-2", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.CREATED}
	err := db.Create(second).Error
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Fatalf("expected duplicated key error, got %v", err)
	}
}

func TestIngestionTaskDAOUpdateStatusIfCurrentRejectsMismatchedStatus(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	task := &entity.IngestionTask{
		ID:         "task-1",
		UserID:     "user-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		Status:     common.STOPPING,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	ctx := t.Context()
	updated, err := NewIngestionTaskDAO().UpdateStatusIfCurrent(ctx, db, "task-1", common.CREATED, common.RUNNING)
	if err != nil {
		t.Fatalf("UpdateStatusIfCurrent failed: %v", err)
	}
	if updated {
		t.Fatal("expected update to be rejected")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.STOPPING {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.STOPPING)
	}
}

// seedStaleTask inserts an ingestion task whose create/update timestamps are
// aged back so ListStaleByStatus staleness windows can be asserted.
func seedStaleTask(t *testing.T, db *gorm.DB, id, status string, age time.Duration) {
	t.Helper()
	ts := time.Now().Add(-age).UnixMilli()
	task := &entity.IngestionTask{
		ID:         id,
		UserID:     "user-1",
		DocumentID: "doc-" + id,
		DatasetID:  "kb-1",
		Status:     status,
		BaseModel:  entity.BaseModel{CreateTime: &ts, UpdateTime: &ts},
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task %s: %v", id, err)
	}
}

// TestIngestionTaskDAOListStaleByStatus covers the three contract axes of the
// startup-reconciliation query: status filtering, the staleness time window
// (per column), and the result limit.
func TestIngestionTaskDAOListStaleByStatus(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	seedStaleTask(t, db, "run-stale", common.RUNNING, 20*time.Minute)
	seedStaleTask(t, db, "run-fresh", common.RUNNING, 1*time.Minute)
	seedStaleTask(t, db, "created-stale", common.CREATED, 20*time.Minute)
	seedStaleTask(t, db, "created-fresh", common.CREATED, 1*time.Minute)
	seedStaleTask(t, db, "completed-stale", common.COMPLETED, 20*time.Minute)

	ctx := t.Context()
	d := NewIngestionTaskDAO()
	threshold := time.Now().Add(-15 * time.Minute)

	// Status filter + update_time window: only the stale RUNNING row matches.
	running, err := d.ListStaleByStatus(ctx, db, []string{common.RUNNING}, "update_time", threshold, 0)
	if err != nil {
		t.Fatalf("ListStaleByStatus(RUNNING): %v", err)
	}
	if len(running) != 1 || running[0].ID != "run-stale" {
		t.Fatalf("RUNNING stale rows = %v, want [run-stale]", idsOfTasks(running))
	}

	// CREATED is judged by create_time: fresh CREATED rows stay outside the
	// window even though their status is in the filter.
	created, err := d.ListStaleByStatus(ctx, db, []string{common.CREATED}, "create_time", threshold, 0)
	if err != nil {
		t.Fatalf("ListStaleByStatus(CREATED): %v", err)
	}
	if len(created) != 1 || created[0].ID != "created-stale" {
		t.Fatalf("CREATED stale rows = %v, want [created-stale]", idsOfTasks(created))
	}

	// Rows in unlisted statuses (COMPLETED) are never returned.
	all, err := d.ListStaleByStatus(ctx, db, []string{common.RUNNING, common.CREATED}, "create_time", threshold, 0)
	if err != nil {
		t.Fatalf("ListStaleByStatus(RUNNING+CREATED): %v", err)
	}
	for _, task := range all {
		if task.ID == "completed-stale" {
			t.Fatal("COMPLETED row must not be returned for RUNNING/CREATED filter")
		}
	}

	// Limit caps the (oldest-first) result set.
	limited, err := d.ListStaleByStatus(ctx, db, []string{common.RUNNING, common.CREATED}, "create_time", threshold, 1)
	if err != nil {
		t.Fatalf("ListStaleByStatus(limit): %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limited rows = %v, want exactly 1", idsOfTasks(limited))
	}

	// An unsupported staleness column is rejected, not silently ignored.
	if _, err = d.ListStaleByStatus(ctx, db, []string{common.RUNNING}, "status; --", threshold, 0); err == nil {
		t.Fatal("expected error for unsupported staleness column")
	}
}

func idsOfTasks(tasks []*entity.IngestionTask) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func TestIngestionTaskDAODeleteIfTerminal_RemovesOnlyTerminal(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	// Create tasks in different statuses, each with a unique docID.
	statuses := []string{common.CREATED, common.RUNNING, common.STOPPING, common.COMPLETED, common.STOPPED, common.FAILED}
	for i, status := range statuses {
		docID := fmt.Sprintf("doc-%d", i)
		task := &entity.IngestionTask{
			ID:         fmt.Sprintf("task-%d", i),
			UserID:     "user-1",
			DocumentID: docID,
			DatasetID:  "kb-1",
			Status:     status,
		}
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", status, err)
		}
	}

	ctx := t.Context()
	// DeleteIfTerminal deletes everything except RUNNING and STOPPING.
	// CREATED is safe to delete (no worker has claimed it yet);
	// COMPLETED/STOPPED/FAILED are terminal.
	// Call it for every doc and verify the negative cases survived.
	for i := 0; i < len(statuses); i++ {
		docID := fmt.Sprintf("doc-%d", i)
		_, err := NewIngestionTaskDAO().DeleteIfTerminal(ctx, db, docID)
		if err != nil {
			t.Fatalf("DeleteIfTerminal(doc-%d): %v", i, err)
		}
	}

	// RUNNING and STOPPING must survive.
	for _, i := range []int{1, 2} {
		docID := fmt.Sprintf("doc-%d", i)
		task, err := NewIngestionTaskDAO().GetByDocumentID(ctx, db, docID)
		if err != nil {
			t.Fatalf("GetByDocumentID %s: %v", docID, err)
		}
		if task == nil {
			t.Fatalf("%s task (doc=%d) must not be deleted", statuses[i], i)
		}
	}
	// CREATED, COMPLETED, STOPPED, FAILED must be gone.
	for _, i := range []int{0, 3, 4, 5} {
		docID := fmt.Sprintf("doc-%d", i)
		task, err := NewIngestionTaskDAO().GetByDocumentID(ctx, db, docID)
		if err != nil {
			t.Fatalf("GetByDocumentID %s: %v", docID, err)
		}
		if task != nil {
			t.Fatalf("%s task (doc=%d) should be deleted, still present", statuses[i], i)
		}
	}
}
