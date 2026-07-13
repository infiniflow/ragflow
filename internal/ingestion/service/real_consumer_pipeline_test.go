//go:build integration

package service

import (
	"context"
	"encoding/json"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

func TestRealConsumer_PipelineMessageRoutesToExecuteTask(t *testing.T) {
	natsEngine := testutil.SetupNatsEngine(t)
	if err := natsEngine.InitConsumer("tasks.>"); err != nil {
		t.Fatalf("InitConsumer: %v", err)
	}

	for {
		handles, _ := natsEngine.GetMessages(1)
		if len(handles) == 0 {
			break
		}
		_ = handles[0].Ack()
	}

	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, _ := testutil.SeedTestData(t, db,
		testutil.WithTenantID("tenant-q-1"),
		testutil.WithKBID("kb-q-1"),
		testutil.WithDocID("doc-q-1"),
		testutil.WithTaskID("ingest-q-1"),
		testutil.WithPipelineID("flow-queue-1"),
		testutil.WithDocName("queue-pipeline.pdf"),
	)

	// testutil.SeedTestData already created an IngestionTask with status RUNNING.
	// Reset it to CREATED so the consumer path can transition it to RUNNING.
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "ingest-q-1").Update("status", common.CREATED).Error; err != nil {
		t.Fatalf("reset ingestion task status: %v", err)
	}
	ingestionTask := &entity.IngestionTask{
		ID:         "ingest-q-1",
		UserID:     "u1",
		DocumentID: docID,
		DatasetID:  "kb-q-1",
		Status:     common.CREATED,
	}

	payload, _ := json.Marshal(common.TaskMessage{
		TaskID:   ingestionTask.ID,
		TaskType: common.TaskTypeIngestionTask,
	})
	if err := natsEngine.PublishTask("tasks.RAGFLOW", payload); err != nil {
		t.Fatalf("PublishTask: %v", err)
	}

	handles, err := natsEngine.GetMessages(1)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected 1 message, got %d", len(handles))
	}
	taskHandle := handles[0]
	taskMsg := taskHandle.GetMessage()
	if taskMsg.TaskID != ingestionTask.ID {
		t.Fatalf("message task ID = %s, want %s", taskMsg.TaskID, ingestionTask.ID)
	}
	if taskMsg.TaskType != common.TaskTypeIngestionTask {
		t.Fatalf("message task type = %s, want %s", taskMsg.TaskType, common.TaskTypeIngestionTask)
	}

	ingestionTaskDAO := dao.NewIngestionTaskDAO()
	_, err = ingestionTaskDAO.UpdateStatusIfCurrent(taskMsg.TaskID, common.CREATED, common.RUNNING)
	if err != nil {
		t.Fatalf("SetRunningByIngestor: %v", err)
	}
	task, err := ingestionTaskDAO.GetByID(taskMsg.TaskID)
	if err != nil || task == nil {
		t.Fatalf("task not found after publish: %s", taskMsg.TaskID)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("task status after SetRunningByIngestor = %s, want %s", task.Status, common.RUNNING)
	}

	ingestor := NewIngestor("queue-test", 1, []string{"pdf"})
	var routedToPipeline bool
	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		task,
	)
	ingestor.runDocumentTask = func(ctx context.Context, ingestionTask *entity.IngestionTask) error {
		routedToPipeline = true
		return nil
	}

	ingestor.executeTask(taskCtx)

	if !routedToPipeline {
		t.Fatal("expected executeTask to route queue-consumed pipeline task to runDocumentTask")
	}
	if err := taskHandle.Ack(); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	finalTask, err := ingestionTaskDAO.GetByID(task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if finalTask.Status != common.COMPLETED {
		t.Fatalf("final status = %s, want %s", finalTask.Status, common.COMPLETED)
	}
}
