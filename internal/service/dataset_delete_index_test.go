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
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type deleteIndexDocEngine struct {
	fakeChatDocEngine
	deleteCalls []deleteIndexDocEngineCall
}

type deleteIndexDocEngineCall struct {
	condition map[string]interface{}
	indexName string
	datasetID string
}

func (e *deleteIndexDocEngine) DeleteChunks(_ context.Context, condition map[string]interface{}, indexName string, datasetID string) (int64, error) {
	e.deleteCalls = append(e.deleteCalls, deleteIndexDocEngineCall{
		condition: condition,
		indexName: indexName,
		datasetID: datasetID,
	})
	return 1, nil
}

func testDatasetServiceForDeleteIndex(docEngine *deleteIndexDocEngine) *DatasetService {
	return &DatasetService{
		kbDAO:     dao.NewKnowledgebaseDAO(),
		taskDAO:   dao.NewTaskDAO(),
		docEngine: docEngine,
	}
}

func insertDeleteIndexKB(t *testing.T, indexType string, taskID string) {
	t.Helper()

	finishAt := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)
	kb := &entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "user-1",
		Name:         "test-kb",
		EmbdID:       "embedding@OpenAI",
		CreatedBy:    "user-1",
		Permission:   string(entity.TenantPermissionMe),
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Status:       sptr("1"),
	}

	switch indexType {
	case "graph":
		kb.GraphragTaskID = &taskID
		kb.GraphragTaskFinishAt = &finishAt
	case "raptor":
		kb.RaptorTaskID = &taskID
		kb.RaptorTaskFinishAt = &finishAt
	case "mindmap":
		kb.MindmapTaskID = &taskID
		kb.MindmapTaskFinishAt = &finishAt
	}

	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if taskID != "" {
		if err := dao.DB.Create(&entity.Task{ID: taskID, DocID: "doc-1", TaskType: indexTypeToTaskType[indexType]}).Error; err != nil {
			t.Fatalf("insert task: %v", err)
		}
	}
}

func TestDatasetServiceDeleteIndexGraphWipeFalseOnlyCancelsTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDeleteIndexKB(t, "graph", "graph-task")

	docEngine := &deleteIndexDocEngine{}
	code, err := testDatasetServiceForDeleteIndex(docEngine).DeleteIndex("user-1", "kb-1", "graph", false)
	if err != nil {
		t.Fatalf("DeleteIndex failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if len(docEngine.deleteCalls) != 0 {
		t.Fatalf("wipe=false should not delete doc-store artefacts, got %#v", docEngine.deleteCalls)
	}

	assertDeleteIndexTaskDeleted(t, "graph-task")
	kb := getDeleteIndexKB(t)
	if kb.GraphragTaskID == nil || *kb.GraphragTaskID != "" {
		t.Fatalf("expected graphrag_task_id to be cleared to empty string, got %#v", kb.GraphragTaskID)
	}
	if kb.GraphragTaskFinishAt != nil {
		t.Fatalf("expected graphrag_task_finish_at to be cleared, got %#v", kb.GraphragTaskFinishAt)
	}
}

func TestDatasetServiceDeleteIndexGraphWipeTrueDeletesArtefacts(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDeleteIndexKB(t, "graph", "graph-task")

	docEngine := &deleteIndexDocEngine{}
	code, err := testDatasetServiceForDeleteIndex(docEngine).DeleteIndex("user-1", "kb-1", "graph", true)
	if err != nil {
		t.Fatalf("DeleteIndex failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if len(docEngine.deleteCalls) != 1 {
		t.Fatalf("expected one doc-store delete call, got %#v", docEngine.deleteCalls)
	}

	call := docEngine.deleteCalls[0]
	if call.indexName != "ragflow_user-1" || call.datasetID != "kb-1" {
		t.Fatalf("unexpected delete target: %#v", call)
	}
	if call.condition["kb_id"] != "kb-1" {
		t.Fatalf("delete condition must include kb_id, got %#v", call.condition)
	}
	assertStringSet(t, call.condition["knowledge_graph_kwd"], []string{"graph", "subgraph", "entity", "relation", "community_report"})
	assertDeleteIndexTaskDeleted(t, "graph-task")
}

func TestDatasetServiceDeleteIndexRaptorWipeTrueDeletesRaptorArtefacts(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDeleteIndexKB(t, "raptor", "raptor-task")

	docEngine := &deleteIndexDocEngine{}
	code, err := testDatasetServiceForDeleteIndex(docEngine).DeleteIndex("user-1", "kb-1", "raptor", true)
	if err != nil {
		t.Fatalf("DeleteIndex failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if len(docEngine.deleteCalls) != 1 {
		t.Fatalf("expected one doc-store delete call, got %#v", docEngine.deleteCalls)
	}
	call := docEngine.deleteCalls[0]
	if call.condition["kb_id"] != "kb-1" {
		t.Fatalf("delete condition must include kb_id, got %#v", call.condition)
	}
	assertStringSet(t, call.condition["raptor_kwd"], []string{"raptor"})
	assertDeleteIndexTaskDeleted(t, "raptor-task")

	kb := getDeleteIndexKB(t)
	if kb.RaptorTaskID == nil || *kb.RaptorTaskID != "" {
		t.Fatalf("expected raptor_task_id to be cleared to empty string, got %#v", kb.RaptorTaskID)
	}
	if kb.RaptorTaskFinishAt != nil {
		t.Fatalf("expected raptor_task_finish_at to be cleared, got %#v", kb.RaptorTaskFinishAt)
	}
}

func TestDatasetServiceDeleteIndexMindmapDoesNotDeleteDocStore(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDeleteIndexKB(t, "mindmap", "mindmap-task")

	docEngine := &deleteIndexDocEngine{}
	code, err := testDatasetServiceForDeleteIndex(docEngine).DeleteIndex("user-1", "kb-1", "mindmap", true)
	if err != nil {
		t.Fatalf("DeleteIndex failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %d", code)
	}
	if len(docEngine.deleteCalls) != 0 {
		t.Fatalf("mindmap delete should not delete doc-store artefacts, got %#v", docEngine.deleteCalls)
	}
	assertDeleteIndexTaskDeleted(t, "mindmap-task")

	kb := getDeleteIndexKB(t)
	if kb.MindmapTaskID == nil || *kb.MindmapTaskID != "" {
		t.Fatalf("expected mindmap_task_id to be cleared to empty string, got %#v", kb.MindmapTaskID)
	}
	if kb.MindmapTaskFinishAt != nil {
		t.Fatalf("expected mindmap_task_finish_at to be cleared, got %#v", kb.MindmapTaskFinishAt)
	}
}

func TestDatasetServiceDeleteIndexRejectsInvalidType(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	code, err := testDatasetServiceForDeleteIndex(&deleteIndexDocEngine{}).DeleteIndex("user-1", "kb-1", "invalid", true)
	if err == nil {
		t.Fatal("expected invalid index type error")
	}
	if code != common.CodeArgumentError {
		t.Fatalf("expected argument error code, got %d", code)
	}
}

func assertDeleteIndexTaskDeleted(t *testing.T, taskID string) {
	t.Helper()
	var task entity.Task
	err := dao.DB.Where("id = ?", taskID).First(&task).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected task %s to be deleted, got err=%v task=%#v", taskID, err, task)
	}
}

func getDeleteIndexKB(t *testing.T) entity.Knowledgebase {
	t.Helper()
	var kb entity.Knowledgebase
	if err := dao.DB.Where("id = ?", "kb-1").First(&kb).Error; err != nil {
		t.Fatalf("fetch kb: %v", err)
	}
	return kb
}

func assertStringSet(t *testing.T, actual interface{}, expected []string) {
	t.Helper()

	items, ok := actual.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %#v", actual)
	}
	if len(items) != len(expected) {
		t.Fatalf("expected %d items, got %#v", len(expected), items)
	}

	seen := make(map[string]bool, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("expected string item, got %#v", item)
		}
		seen[value] = true
	}
	for _, item := range expected {
		if !seen[item] {
			t.Fatalf("missing %q in %#v", item, items)
		}
	}
}
