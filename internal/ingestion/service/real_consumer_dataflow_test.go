//go:build manual
// +build manual

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/nats"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

func TestRealConsumer_DataflowMessageRoutesToExecuteTask(t *testing.T) {
	natsEngine := nats.NewNatsEngine("localhost", 4222)
	if err := natsEngine.Init(); err != nil {
		t.Fatalf("NATS Init: %v", err)
	}
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
		testutil.WithDocName("queue-dataflow.pdf"),
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
	task, err := ingestionTaskDAO.SetRunningByIngestor(taskMsg.TaskID)
	if err != nil {
		if errors.Is(err, common.ErrTaskNotFound) {
			t.Fatalf("task not found after publish: %s", taskMsg.TaskID)
		}
		t.Fatalf("SetRunningByIngestor: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("task status after SetRunningByIngestor = %s, want %s", task.Status, common.RUNNING)
	}

	ingestor := NewIngestor("queue-test", 1, []string{"pdf"})
	var routedToDataflow bool
	var progressEvents []string
	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		task,
	)
	var finalProgress float64
	taskCtx.ProgressFunc = func(prog float64, msg string) {
		finalProgress = prog
		progressEvents = append(progressEvents, msg)
	}
	ingestor.runDocumentTask = func(ctx context.Context, ingestionTask *entity.IngestionTask) error {
		routedToDataflow = true
		taskCtx.ProgressFunc(0.82, "mock queue dataflow start")
		taskCtx.ProgressFunc(1.0, "mock queue dataflow done")
		return nil
	}

	ingestor.executeTask(taskCtx)

	if !routedToDataflow {
		t.Fatal("expected executeTask to route queue-consumed dataflow task to runDocumentTask")
	}
	if finalProgress != 1.0 {
		t.Fatalf("finalProgress = %v, want 1.0", finalProgress)
	}
	if len(progressEvents) != 2 {
		t.Fatalf("progressEvents = %v, want 2 events", progressEvents)
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
