package dao

import (
	"errors"
	"testing"

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

	updated, err := NewIngestionTaskDAO().UpdateStatusIfCurrent("task-1", common.CREATED, common.RUNNING)
	if err != nil {
		t.Fatalf("UpdateStatusIfCurrent failed: %v", err)
	}
	if !updated {
		t.Fatal("expected update to succeed")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID("task-1")
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.Where("id = ?", "task-1").Delete(&entity.IngestionTask{}).Error; err != nil {
				t.Fatalf("clear task: %v", err)
			}
			task := &entity.IngestionTask{ID: "task-1", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: tc.status}
			if err := db.Create(task).Error; err != nil {
				t.Fatalf("create task: %v", err)
			}
			_, err := NewIngestionTaskDAO().Create(&entity.IngestionTask{ID: "task-2", UserID: "user-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.CREATED})
			if err == nil {
				t.Fatal("expected Create to reject duplicate document task")
			}
			reloaded, err := NewIngestionTaskDAO().GetByID("task-1")
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

	updated, err := NewIngestionTaskDAO().UpdateStatusIfCurrent("task-1", common.CREATED, common.RUNNING)
	if err != nil {
		t.Fatalf("UpdateStatusIfCurrent failed: %v", err)
	}
	if updated {
		t.Fatal("expected update to be rejected")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID("task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if reloaded.Status != common.STOPPING {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.STOPPING)
	}
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

	// DeleteIfTerminal should delete terminal tasks (idx 3=COMPLETED, 4=STOPPED, 5=FAILED),
	// not non-terminal ones (idx 0=CREATED, 1=RUNNING, 2=STOPPING).
	deleted, err := NewIngestionTaskDAO().DeleteIfTerminal("doc-3")
	if err != nil {
		t.Fatalf("DeleteIfTerminal: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted doc-3(COMPLETED) = %d, want 1", deleted)
	}
	_, err = NewIngestionTaskDAO().DeleteIfTerminal("doc-4")
	if err != nil {
		t.Fatalf("DeleteIfTerminal: %v", err)
	}
	_, err = NewIngestionTaskDAO().DeleteIfTerminal("doc-5")
	if err != nil {
		t.Fatalf("DeleteIfTerminal: %v", err)
	}

	// Verify non-terminal tasks survived.
	for i, status := range []string{common.CREATED, common.RUNNING, common.STOPPING} {
		docID := fmt.Sprintf("doc-%d", i)
		task, err := NewIngestionTaskDAO().GetByDocumentID(docID)
		if err != nil {
			t.Fatalf("GetByDocumentID %s: %v", docID, err)
		}
		if task == nil {
			t.Fatalf("%s task (doc=%s) must not be deleted", status, docID)
		}
	}
	// Verify terminal tasks are gone.
	for i, status := range []string{common.COMPLETED, common.STOPPED, common.FAILED} {
		docID := fmt.Sprintf("doc-%d", i+3)
		task, err := NewIngestionTaskDAO().GetByDocumentID(docID)
		if err != nil {
			t.Fatalf("GetByDocumentID %s: %v", docID, err)
		}
		if task != nil {
			t.Fatalf("%s task (doc=%s) should be deleted, still present", status, docID)
		}
	}
}
