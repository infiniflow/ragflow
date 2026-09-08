//go:build e2e

package document

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	servicepkg "ragflow/internal/service"
)

// TestRerunDocument_E2E_EnqueuesThroughRealMessageQueue drives the rerun
// through the production MessageQueueTaskPublisher into a real NATS
// JetStream stream and consumes the task message back. The unit tier's
// recording publisher only proves the publish call happened; this tier
// proves the enqueue actually lands on tasks.RAGFLOW with a task id that
// resolves to a SCHEDULED ingestion task for the document.
func TestRerunDocument_E2E_EnqueuesThroughRealMessageQueue(t *testing.T) {
	db := testutil.SetupTestDB(t,
		&entity.IngestionTask{}, &entity.IngestionTaskLog{}, &entity.Task{},
		&entity.Document{}, &entity.Knowledgebase{}, &entity.Tenant{},
		&entity.File{}, &entity.File2Document{}, &entity.PipelineOperationLog{},
	)
	defer testutil.ReplaceDBForTest(t, db)()

	mq := testutil.SetupNatsEngine(t)
	previous := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(mq)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previous) })
	if err := mq.InitConsumer("tasks.>"); err != nil {
		t.Fatalf("InitConsumer: %v", err)
	}

	testutil.SeedTestData(t, db,
		testutil.WithTenantID("tenant-1"),
		testutil.WithKBID("kb-1"),
		testutil.WithDocID("doc-1"),
	)
	// SeedTestData leaves a RUNNING ingestion task behind, which
	// clearDocumentParseResults would (rightly) refuse to clear. The rerun
	// starts from a completed parse, so drop the seeded in-flight rows.
	if err := db.Where("id = ?", "task-1").Delete(&entity.IngestionTask{}).Error; err != nil {
		t.Fatalf("drop seeded ingestion task: %v", err)
	}
	if err := db.Where("id = ?", "task-1").Delete(&entity.Task{}).Error; err != nil {
		t.Fatalf("drop seeded task: %v", err)
	}
	// Prior parse results the rerun must clear, and a finished progress.
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Updates(map[string]interface{}{"token_num": 9, "chunk_num": 4, "progress": 1}).Error; err != nil {
		t.Fatalf("set doc counters: %v", err)
	}
	insertTestPipelineLog(t, "log-1", "doc-1", "kb-1", "tenant-1",
		entity.JSONMap{"components": map[string]interface{}{}})

	svc := testDocumentService(t)
	svc.ingestionTaskSvc.SetTaskPublisher(servicepkg.NewMessageQueueTaskPublisher())

	dsl := map[string]interface{}{
		"components": map[string]interface{}{"c1": map[string]interface{}{"obj": map[string]interface{}{}}},
	}
	if err := svc.RerunDocument(t.Context(), "tenant-1", "log-1", dsl, "c1"); err != nil {
		t.Fatalf("RerunDocument: %v", err)
	}

	// Counters cleared for the rerun.
	doc, err := svc.documentDAO.GetByID(t.Context(), db, "doc-1")
	if err != nil {
		t.Fatalf("reload doc: %v", err)
	}
	if doc.TokenNum != 0 || doc.ChunkNum != 0 {
		t.Fatalf("doc counters after rerun = token %d chunk %d, want zeros", doc.TokenNum, doc.ChunkNum)
	}

	// The edited DSL is persisted on the log row with the rerun entry point.
	updated, err := svc.pipelineLogDAO.GetByID(t.Context(), db, "log-1")
	if err != nil {
		t.Fatalf("reload log: %v", err)
	}
	path, _ := updated.DSL["path"].([]interface{})
	if len(path) != 1 || path[0] != "c1" {
		t.Fatalf("log dsl path = %v, want [c1]", updated.DSL["path"])
	}

	// The enqueue actually landed on tasks.RAGFLOW.
	handles, err := mq.GetMessages(1)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(handles) != 1 {
		t.Fatalf("expected 1 message on tasks.RAGFLOW, got %d", len(handles))
	}
	defer func() { _ = handles[0].Ack() }()
	taskMsg := handles[0].GetMessage()
	if taskMsg.TaskType != common.TaskTypeIngestionTask {
		t.Fatalf("message task type = %s, want %s", taskMsg.TaskType, common.TaskTypeIngestionTask)
	}

	// ...and the referenced task row exists, already SCHEDULED.
	task, err := svc.ingestionTaskDAO.GetByID(t.Context(), db, taskMsg.TaskID)
	if err != nil {
		t.Fatalf("load enqueued ingestion task %s: %v", taskMsg.TaskID, err)
	}
	if task.DocumentID != "doc-1" || task.DatasetID != "kb-1" || task.Status != common.SCHEDULED {
		t.Fatalf("ingestion task = %+v", task)
	}
}
