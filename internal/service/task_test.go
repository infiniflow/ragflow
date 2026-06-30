package service

import (
	"context"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestCancelTaskRejectsUnauthorizedDocumentTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	run := string(entity.TaskStatusRunning)
	kb := &entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "owner-1",
		Name:         "test-kb",
		EmbdID:       "embedding@OpenAI",
		CreatedBy:    "owner-1",
		Permission:   string(entity.TenantPermissionMe),
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Status:       sptr(string(entity.StatusValid)),
	}
	doc := &entity.Document{
		ID:           "doc-1",
		KbID:         kb.ID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   string(entity.FileSourceLocal),
		Type:         "pdf",
		CreatedBy:    "owner-1",
		Suffix:       ".pdf",
		Run:          &run,
		Status:       sptr(string(entity.StatusValid)),
	}
	task := &entity.Task{
		ID:       "task-1",
		DocID:    doc.ID,
		Progress: 0.5,
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	if err := dao.DB.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	code, err := NewTaskService().CancelTask(context.Background(), "attacker-1", task.ID)
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("code = %v, want %v", code, common.CodeAuthenticationError)
	}

	var gotTask entity.Task
	if err := dao.DB.First(&gotTask, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("get task: %v", err)
	}
	if gotTask.Progress != task.Progress {
		t.Fatalf("task progress = %v, want %v", gotTask.Progress, task.Progress)
	}

	var gotDoc entity.Document
	if err := dao.DB.First(&gotDoc, "id = ?", doc.ID).Error; err != nil {
		t.Fatalf("get document: %v", err)
	}
	if gotDoc.Run == nil || *gotDoc.Run != run {
		t.Fatalf("document run = %v, want %q", gotDoc.Run, run)
	}
}

func TestCancelTaskReturnsErrorOnTaskLookupFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql db: %v", err)
	}

	code, err := NewTaskService().CancelTask(context.Background(), "user-1", "task-1")
	if err == nil {
		t.Fatal("expected lookup error")
	}
	if code == common.CodeSuccess {
		t.Fatalf("code = %v, want non-success", code)
	}
}
