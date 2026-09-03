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

func TestIngestionTaskDAODeleteAllowsScheduledTask(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	task := &entity.IngestionTask{
		ID:         "task-scheduled",
		UserID:     "user-1",
		DocumentID: "doc-scheduled",
		DatasetID:  "kb-1",
		Status:     common.SCHEDULED,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create scheduled task: %v", err)
	}

	info, err := NewIngestionTaskDAO().Delete(t.Context(), db, task.ID, nil)
	if err != nil {
		t.Fatalf("delete scheduled task: %v", err)
	}
	if info == nil || info.TaskID != task.ID {
		t.Fatalf("unexpected task info: %+v", info)
	}
}

func TestIngestionTaskDAODeleteDoesNotRemoveTaskClaimedDuringDelete(t *testing.T) {
	db := setupTaskTestDB(t)
	task := &entity.IngestionTask{
		ID:         "task-scheduled",
		UserID:     "user-1",
		DocumentID: "doc-scheduled",
		DatasetID:  "kb-1",
		Status:     common.SCHEDULED,
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create scheduled task: %v", err)
	}

	const callbackName = "test:claim-ingestion-task-before-delete"
	if err := db.Callback().Delete().Before("gorm:delete").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != task.TableName() {
			return
		}
		if err := tx.Session(&gorm.Session{NewDB: true}).Model(&entity.IngestionTask{}).Where("id = ?", task.ID).
			Update("status", common.RUNNING).Error; err != nil {
			tx.AddError(err)
		}
	}); err != nil {
		t.Fatalf("register delete callback: %v", err)
	}

	if _, err := NewIngestionTaskDAO().Delete(t.Context(), db, task.ID, nil); err == nil {
		t.Fatal("expected delete to reject task claimed during delete")
	}

	reloaded, err := NewIngestionTaskDAO().GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("reload task after rejected delete: %v", err)
	}
	if reloaded.Status != common.SCHEDULED {
		t.Fatalf("status after rejected delete = %q, want %q", reloaded.Status, common.SCHEDULED)
	}
}

func TestIngestionTaskDAOListByStatus(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	for _, task := range []*entity.IngestionTask{
		{ID: "task-created", UserID: "user-1", DocumentID: "doc-created", DatasetID: "kb-1", Status: common.CREATED},
		{ID: "task-scheduled", UserID: "user-1", DocumentID: "doc-scheduled", DatasetID: "kb-1", Status: common.SCHEDULED},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	tasks, err := NewIngestionTaskDAO().ListByStatus(t.Context(), db, common.CREATED)
	if err != nil {
		t.Fatalf("list CREATED tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-created" {
		t.Fatalf("CREATED tasks = %+v, want only task-created", tasks)
	}
}

func TestIngestionTaskDAOListsTasks(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	for _, task := range []*entity.IngestionTask{
		{ID: "task-created", UserID: "user-1", DocumentID: "doc-created", DatasetID: "kb-1", Status: common.CREATED},
		{ID: "task-scheduled", UserID: "user-1", DocumentID: "doc-scheduled", DatasetID: "kb-1", Status: common.SCHEDULED},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	d := NewIngestionTaskDAO()
	tasks, err := d.ListByUserID(t.Context(), db, "user-1", 0, 0)
	if err != nil {
		t.Fatalf("list tasks by user: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("listed tasks = %+v, want 2 tasks", tasks)
	}

	tasks, err = d.ListByUserIDAndDatasetID(t.Context(), db, "user-1", "kb-1", 0, 0)
	if err != nil {
		t.Fatalf("list tasks by user and dataset: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("listed tasks by dataset = %+v, want 2 tasks", tasks)
	}

	tasks, err = d.GetAllTasks(t.Context(), db, 0, 0)
	if err != nil {
		t.Fatalf("list all tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("all listed tasks = %+v, want 2 tasks", tasks)
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

func TestIngestionTaskDAODeleteIfTerminal_RemovesOnlyTerminal(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	// Create tasks in different statuses, each with a unique docID.
	statuses := []string{common.CREATED, common.SCHEDULED, common.RUNNING, common.STOPPING, common.COMPLETED, common.STOPPED, common.FAILED}
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
	// CREATED and SCHEDULED are safe to delete (no worker has claimed them yet);
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
	for _, i := range []int{2, 3} {
		docID := fmt.Sprintf("doc-%d", i)
		task, err := NewIngestionTaskDAO().GetByDocumentID(ctx, db, docID)
		if err != nil {
			t.Fatalf("GetByDocumentID %s: %v", docID, err)
		}
		if task == nil {
			t.Fatalf("%s task (doc=%d) must not be deleted", statuses[i], i)
		}
	}
	// CREATED, SCHEDULED, COMPLETED, STOPPED, FAILED must be gone.
	for _, i := range []int{0, 1, 4, 5, 6} {
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

func TestIngestionTaskDAOCountActiveByDatasetID(t *testing.T) {
	db := setupTaskTestDB(t)
	for _, task := range []*entity.IngestionTask{
		{ID: "task-active-1", UserID: "user-1", DocumentID: "doc-active-1", DatasetID: "kb-1", Status: common.SCHEDULED},
		{ID: "task-active-2", UserID: "user-1", DocumentID: "doc-active-2", DatasetID: "kb-1", Status: common.RUNNING},
		{ID: "task-active-3", UserID: "user-1", DocumentID: "doc-active-3", DatasetID: "kb-1", Status: common.CREATED},
		{ID: "task-active-4", UserID: "user-1", DocumentID: "doc-active-4", DatasetID: "kb-1", Status: common.STOPPING},
		{ID: "task-terminal", UserID: "user-1", DocumentID: "doc-terminal", DatasetID: "kb-1", Status: common.COMPLETED},
		{ID: "task-other-kb", UserID: "user-1", DocumentID: "doc-other", DatasetID: "kb-2", Status: common.SCHEDULED},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	ctx := t.Context()
	count, err := NewIngestionTaskDAO().CountActiveByDatasetID(ctx, db, "kb-1")
	if err != nil {
		t.Fatalf("CountActiveByDatasetID: %v", err)
	}
	if count != 4 {
		t.Fatalf("active count for kb-1 = %d, want 4", count)
	}
	count, err = NewIngestionTaskDAO().CountActiveByDatasetID(ctx, db, "kb-2")
	if err != nil {
		t.Fatalf("CountActiveByDatasetID kb-2: %v", err)
	}
	if count != 1 {
		t.Fatalf("active count for kb-2 = %d, want 1", count)
	}
	count, err = NewIngestionTaskDAO().CountActiveByDatasetID(ctx, db, "kb-unknown")
	if err != nil {
		t.Fatalf("CountActiveByDatasetID unknown: %v", err)
	}
	if count != 0 {
		t.Fatalf("active count for unknown = %d, want 0", count)
	}
}

func TestIngestionTaskDAOCountActiveByDatasetIDUsesLatestTask(t *testing.T) {
	db := setupTaskTestDB(t)
	if err := db.Exec("DROP INDEX idx_ingestion_task_document_id").Error; err != nil {
		t.Fatalf("drop ingestion task unique index: %v", err)
	}

	oldTaskTime := int64(100)
	newTaskTime := int64(200)
	for _, task := range []*entity.IngestionTask{
		{
			ID:         "task-old-active",
			UserID:     "user-1",
			DocumentID: "doc-retried",
			DatasetID:  "kb-1",
			Status:     common.RUNNING,
			BaseModel:  entity.BaseModel{CreateTime: &oldTaskTime},
		},
		{
			ID:         "task-new-terminal",
			UserID:     "user-1",
			DocumentID: "doc-retried",
			DatasetID:  "kb-1",
			Status:     common.COMPLETED,
			BaseModel:  entity.BaseModel{CreateTime: &newTaskTime},
		},
		{
			ID:         "task-current-active",
			UserID:     "user-1",
			DocumentID: "doc-current",
			DatasetID:  "kb-1",
			Status:     common.SCHEDULED,
		},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	count, err := NewIngestionTaskDAO().CountActiveByDatasetID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatalf("CountActiveByDatasetID: %v", err)
	}
	if count != 1 {
		t.Fatalf("active count for kb-1 = %d, want 1 (latest task per document)", count)
	}
}

func TestIngestionTaskDAOGetByDocumentIDUsesLatestTask(t *testing.T) {
	db := setupTaskTestDB(t)
	if err := db.Exec("DROP INDEX idx_ingestion_task_document_id").Error; err != nil {
		t.Fatalf("drop ingestion task unique index: %v", err)
	}

	oldTaskTime := int64(100)
	newTaskTime := int64(200)
	for _, task := range []*entity.IngestionTask{
		{
			ID:         "task-old",
			UserID:     "user-1",
			DocumentID: "doc-retried",
			DatasetID:  "kb-1",
			Status:     common.RUNNING,
			BaseModel:  entity.BaseModel{CreateTime: &oldTaskTime},
		},
		{
			ID:         "task-new",
			UserID:     "user-1",
			DocumentID: "doc-retried",
			DatasetID:  "kb-1",
			Status:     common.COMPLETED,
			BaseModel:  entity.BaseModel{CreateTime: &newTaskTime},
		},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	task, err := NewIngestionTaskDAO().GetByDocumentID(t.Context(), db, "doc-retried")
	if err != nil {
		t.Fatalf("GetByDocumentID: %v", err)
	}
	if task == nil || task.ID != "task-new" {
		t.Fatalf("latest task = %+v, want task-new", task)
	}
}

func TestIngestionTaskDAOGetByDocumentIDUsesIDAsTieBreaker(t *testing.T) {
	db := setupTaskTestDB(t)
	if err := db.Exec("DROP INDEX idx_ingestion_task_document_id").Error; err != nil {
		t.Fatalf("drop ingestion task unique index: %v", err)
	}

	createTime := int64(100)
	for _, task := range []*entity.IngestionTask{
		{
			ID:         "task-z",
			UserID:     "user-1",
			DocumentID: "doc-retried",
			DatasetID:  "kb-1",
			Status:     common.RUNNING,
			BaseModel:  entity.BaseModel{CreateTime: &createTime},
		},
		{
			ID:         "task-a",
			UserID:     "user-1",
			DocumentID: "doc-retried",
			DatasetID:  "kb-1",
			Status:     common.COMPLETED,
			BaseModel:  entity.BaseModel{CreateTime: &createTime},
		},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	task, err := NewIngestionTaskDAO().GetByDocumentID(t.Context(), db, "doc-retried")
	if err != nil {
		t.Fatalf("GetByDocumentID: %v", err)
	}
	if task == nil || task.ID != "task-z" {
		t.Fatalf("latest task = %+v, want task-z", task)
	}
}

func TestIngestionTaskDAOHasDatasetStatusIndex(t *testing.T) {
	db := setupTaskTestDB(t)
	if !db.Migrator().HasIndex(&entity.IngestionTask{}, "idx_ingestion_task_dataset_status") {
		t.Fatal("expected composite dataset/status index on ingestion_task")
	}
}
