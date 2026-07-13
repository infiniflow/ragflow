//go:build integration

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

package task

import (
	"context"
	"encoding/json"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// TestRealProducerConsumer exercises the project's real producer and consumer code paths:
//
//	Producer: document.go pattern — Create(IngestionTask) → PublishTask(NATS)
//	Consumer: Ingestor.Start() core logic — calls each actual function in sequence
func TestRealProducerConsumer(t *testing.T) {
	// ── 1. NATS (embedded in-process server) ──
	natsEngine := testutil.SetupNatsEngine(t)
	if err := natsEngine.InitConsumer("tasks.>"); err != nil {
		t.Fatalf("InitConsumer: %v", err)
	}

	// Purge stale messages
	for {
		h, _ := natsEngine.GetMessages(1)
		if len(h) == 0 {
			break
		}
		h[0].Ack()
	}

	// ── 2. SQLite DB ──
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	db.Create(&entity.Tenant{ID: "t1", LLMID: "gpt-4", Status: testutil.StrPtr("1")})
	db.Create(&entity.Knowledgebase{ID: "kb1", TenantID: "t1", EmbdID: "e1", Status: testutil.StrPtr("1"), ParserConfig: entity.JSONMap{}})
	docName := "doc-real.pdf"
	db.Create(&entity.Document{ID: "doc-real", KbID: "kb1", ParserID: "naive", ParserConfig: entity.JSONMap{}, Name: &docName})

	// ── 3. Producer: Mirrors document.go:1062-1085 exactly ──
	ingestionTask := &entity.IngestionTask{
		ID:         "ingest-task-1",
		UserID:     "u1",
		DocumentID: "doc-real",
		DatasetID:  "kb1",
		Status:     common.CREATED,
	}
	created, err := dao.NewIngestionTaskDAO().Create(ingestionTask)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Logf("Producer: IngestionTask created id=%s status=%s", created.ID, created.Status)

	taskMessage := common.TaskMessage{
		TaskID:   created.ID,
		TaskType: common.TaskTypeIngestionTask,
	}
	payload, _ := json.Marshal(taskMessage)
	if err := natsEngine.PublishTask("tasks.RAGFLOW", payload); err != nil {
		t.Fatalf("PublishTask: %v", err)
	}
	t.Logf("Producer: Published %s", payload)

	// ── 4. Consumer: Mirrors Ingestor.Start():131-189 exactly ──
	handles, err := natsEngine.GetMessages(1)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected 1 message, got %d", len(handles))
	}
	taskHandle := handles[0]
	taskMsg := taskHandle.GetMessage()
	t.Logf("Consumer: Received TaskID=%s TaskType=%s", taskMsg.TaskID, taskMsg.TaskType)

	// Mirrors Start():133 — type filter
	if taskMsg.TaskType != common.TaskTypeIngestionTask {
		taskHandle.Ack()
		t.Fatalf("unexpected task type: %s", taskMsg.TaskType)
	}

	// Mirrors Start():142-143 — UpdateStatusIfCurrent
	ingestionTaskDAO := dao.NewIngestionTaskDAO()
	_, err = ingestionTaskDAO.UpdateStatusIfCurrent(taskMsg.TaskID, common.CREATED, common.RUNNING)
	if err != nil {
		t.Fatalf("UpdateStatusIfCurrent: %v", err)
	}
	task, err := ingestionTaskDAO.GetByID(taskMsg.TaskID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if task == nil {
		t.Logf("Consumer: task %s not found in ingestion_task table — skipped", taskMsg.TaskID)
		taskHandle.Ack()
		return
	}
	t.Logf("Consumer: UpdateStatusIfCurrent status=%s", task.Status)

	// Mirrors Start():167-180 — status check
	switch task.Status {
	case common.COMPLETED, common.STOPPED, common.FAILED:
		taskHandle.Ack()
		t.Fatalf("task already terminal: %s", task.Status)
	case common.STOPPING, common.CREATED:
		t.Fatalf("unexpected status: %s", task.Status)
	case common.RUNNING:
		t.Logf("Consumer: task is RUNNING — dispatching to executeTask")
	}

	// ── 5. executeTask (our modified version) ──
	// Set a pipeline ID so the handler can resolve the canvas.
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-real").Update("pipeline_id", "pipeline-real").Error; err != nil {
		t.Fatalf("set pipeline_id: %v", err)
	}

	tc, err := LoadFromIngestionTask(task)
	if err != nil {
		t.Fatalf("LoadFromIngestionTask: %v", err)
	}
	tc.Ctx = context.Background()
	t.Logf("Consumer: Loaded Doc=%s Parser=%s KB=%s Tenant=%s",
		tc.Doc.ID, tc.Doc.ParserID, tc.KB.ID, tc.Tenant.ID)

	svc, err := NewPipelineExecutor(tc, tc.PipelineID, 0)
	if err != nil {
		t.Fatalf("NewPipelineExecutor: %v", err)
	}
	svc.WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
		return `{"nodes":[{"id":"test","type":"parser"}],"edges":[]}`, canvasID, nil
	})
	svc.WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
		return nil, "", nil
	})
	svc.WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
		return nil, nil
	})
	if _, err := svc.Execute(tc.Ctx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	t.Log("Consumer: PipelineExecutor.Execute() - OK")

	// Mirrors executeTask — mark as completed
	if _, err := ingestionTaskDAO.UpdateStatusIfCurrent(task.ID, common.RUNNING, common.COMPLETED); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	// Mirrors Start():135 — Ack
	taskHandle.Ack()

	// ── 6. Verify ──
	final, _ := ingestionTaskDAO.GetByID(task.ID)
	if final.Status != common.COMPLETED {
		t.Errorf("final status = %s, want %s", final.Status, common.COMPLETED)
	}
	t.Logf("Final: IngestionTask status=%s ✅", final.Status)
}
