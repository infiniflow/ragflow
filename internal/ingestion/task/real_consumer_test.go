//go:build manual
// +build manual

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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/nats"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestRealProducerConsumer exercises the project's real producer and consumer code paths:
//
//	Producer: document.go pattern — CheckAndCreate(IngestionTask) → PublishTask(NATS)
//	Consumer: Ingestor.Start() core logic — calls each actual function in sequence
func TestRealProducerConsumer(t *testing.T) {
	// ── 1. NATS ──
	natsEngine := nats.NewNatsEngine("localhost", 4222)
	if err := natsEngine.Init(); err != nil {
		t.Fatalf("NATS Init: %v", err)
	}
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
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	db.AutoMigrate(
		&entity.IngestionTask{}, &entity.Task{},
		&entity.Document{}, &entity.Knowledgebase{}, &entity.Tenant{},
	)
	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })

	db.Create(&entity.Tenant{ID: "t1", LLMID: "gpt-4", Status: ptr("1")})
	db.Create(&entity.Knowledgebase{ID: "kb1", TenantID: "t1", EmbdID: "e1", Status: ptr("1"), ParserConfig: entity.JSONMap{}})
	db.Create(&entity.Document{ID: "doc-real", KbID: "kb1", ParserID: "naive", ParserConfig: entity.JSONMap{}})

	// ── 3. Producer: Mirrors document.go:1062-1085 exactly ──
	ingestionTask := &entity.IngestionTask{
		ID:         "ingest-task-1",
		UserID:     "u1",
		DocumentID: "doc-real",
		DatasetID:  "kb1",
		Status:     common.CREATED,
	}
	created, err := dao.NewIngestionTaskDAO().CheckAndCreate(ingestionTask)
	if err != nil {
		t.Fatalf("CheckAndCreate: %v", err)
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

	// Mirrors Start():142-143 — SetRunningByIngestor
	ingestionTaskDAO := dao.NewIngestionTaskDAO()
	task, err := ingestionTaskDAO.SetRunningByIngestor(taskMsg.TaskID)
	if err != nil {
		if errors.Is(err, common.ErrTaskNotFound) {
			t.Logf("Consumer: task %s not found in ingestion_task table — skipped", taskMsg.TaskID)
			taskHandle.Ack()
			return
		}
		t.Fatalf("SetRunningByIngestor: %v", err)
	}
	t.Logf("Consumer: SetRunningByIngestor status=%s", task.Status)

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
	// executeTask needs DB data for TaskHandler
	db.Create(&entity.Task{ID: task.ID, DocID: "doc-real", FromPage: 0, ToPage: 100000})

	tc, err := LoadTaskContext(task.ID)
	if err != nil {
		t.Fatalf("LoadTaskContext: %v", err)
	}
	t.Logf("Consumer: Loaded Doc=%s Parser=%s KB=%s Tenant=%s",
		tc.Doc.ID, tc.Doc.ParserID, tc.KB.ID, tc.Tenant.ID)

	handler := NewTaskHandler(tc)
	if err := handler.Handle(); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	t.Log("Consumer: TaskHandler.Handle() — OK")

	// Mirrors executeTask — mark as completed
	if err := ingestionTaskDAO.UpdateStatus(task.ID, common.COMPLETED); err != nil {
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
