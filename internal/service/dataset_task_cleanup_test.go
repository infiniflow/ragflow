//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestCleanupFailedDatasetIndexTaskDeletesTaskAndRestoresDocument(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	previousMsg := "previous progress"
	previousBeginAt := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	queuedMsg := "Task is queued..."
	queuedBeginAt := previousBeginAt.Add(time.Hour)
	taskID := "task-1"

	kb := &entity.Knowledgebase{
		ID:             "kb-1",
		TenantID:       "user-1",
		Name:           "test-kb",
		EmbdID:         "embedding@OpenAI",
		CreatedBy:      "user-1",
		Permission:     string(entity.TenantPermissionMe),
		ParserID:       "naive",
		ParserConfig:   entity.JSONMap{},
		GraphragTaskID: &taskID,
		Status:         sptr("1"),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}

	doc := &entity.Document{
		ID:             "doc-1",
		KbID:           "kb-1",
		ParserID:       "naive",
		ParserConfig:   entity.JSONMap{},
		SourceType:     "local",
		Type:           "pdf",
		CreatedBy:      "user-1",
		Suffix:         ".pdf",
		ProgressMsg:    &queuedMsg,
		ProcessBeginAt: &queuedBeginAt,
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert document: %v", err)
	}

	task := &entity.Task{ID: taskID, DocID: doc.ID, TaskType: "graphrag"}
	if err := dao.DB.Create(task).Error; err != nil {
		t.Fatalf("insert task: %v", err)
	}

	snapshot := &entity.Document{
		ID:             doc.ID,
		ProgressMsg:    &previousMsg,
		ProcessBeginAt: &previousBeginAt,
	}
	if err := cleanupFailedDatasetIndexTask(task.ID, snapshot, kb.ID, "graph"); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	var persistedTask entity.Task
	err := dao.DB.Where("id = ?", task.ID).First(&persistedTask).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected task to be deleted, got err=%v task=%#v", err, persistedTask)
	}

	persistedDoc, err := dao.NewDocumentDAO().GetByID(doc.ID)
	if err != nil {
		t.Fatalf("fetch document: %v", err)
	}
	if persistedDoc.ProgressMsg == nil || *persistedDoc.ProgressMsg != previousMsg {
		t.Fatalf("expected progress_msg %q, got %#v", previousMsg, persistedDoc.ProgressMsg)
	}
	if persistedDoc.ProcessBeginAt == nil || !persistedDoc.ProcessBeginAt.Equal(previousBeginAt) {
		t.Fatalf("expected process_begin_at %v, got %#v", previousBeginAt, persistedDoc.ProcessBeginAt)
	}

	var persistedKB entity.Knowledgebase
	if err := dao.DB.Where("id = ?", kb.ID).First(&persistedKB).Error; err != nil {
		t.Fatalf("fetch kb: %v", err)
	}
	if persistedKB.GraphragTaskID != nil {
		t.Fatalf("expected graphrag_task_id to be cleared, got %#v", *persistedKB.GraphragTaskID)
	}
}
