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

	// DeleteIfTerminal deletes everything except RUNNING and STOPPING.
	// CREATED is safe to delete (no worker has claimed it yet);
	// COMPLETED/STOPPED/FAILED are terminal.
	// Call it for every doc and verify the negative cases survived.
	for i := 0; i < len(statuses); i++ {
		docID := fmt.Sprintf("doc-%d", i)
		_, err := NewIngestionTaskDAO().DeleteIfTerminal(docID)
		if err != nil {
			t.Fatalf("DeleteIfTerminal(doc-%d): %v", i, err)
		}
	}

	// RUNNING and STOPPING must survive.
	for _, i := range []int{1, 2} {
		docID := fmt.Sprintf("doc-%d", i)
		task, err := NewIngestionTaskDAO().GetByDocumentID(docID)
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
		task, err := NewIngestionTaskDAO().GetByDocumentID(docID)
		if err != nil {
			t.Fatalf("GetByDocumentID %s: %v", docID, err)
		}
		if task != nil {
			t.Fatalf("%s task (doc=%d) should be deleted, still present", statuses[i], i)
		}
	}
}
