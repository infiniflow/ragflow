package service

import (
	"context"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

func TestExecuteTask_DataflowRoutesToTaskHandler(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	var routedToDataflow bool
	var gotTaskID string
	var gotProgress []float64
	var gotMsgs []string
	ingestor.runDocumentTask = func(ctx context.Context, ingestionTask *entity.IngestionTask) error {
		routedToDataflow = true
		gotTaskID = ingestionTask.ID
		wrapped := func(prog float64, msg string) {
			gotProgress = append(gotProgress, prog*100)
			gotMsgs = append(gotMsgs, msg)
		}
		wrapped(0.82, "mock dataflow start")
		wrapped(1.0, "mock dataflow done")
		return nil
	}
	ingestor.storageImpl = testutil.NewMockStorage(map[string][]byte{"/unused": []byte("unused")})
	ingestor.documentDAO = &testutil.MockDocDAO{
		Docs: map[string]*entity.Document{"doc-1": testutil.TestDoc("doc-1", "pdf", ".pdf")},
	}

	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		&entity.IngestionTask{ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING},
		nil,
	)

	ingestor.executeTask(taskCtx)

	if !routedToDataflow {
		t.Fatal("expected executeTask to route dataflow task to runDocumentTask")
	}
	if gotTaskID != taskID {
		t.Fatalf("runDocumentTask got task ID %q, want %q", gotTaskID, taskID)
	}
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final ingestion task: %v", err)
	}
	if finalTask.Status != common.COMPLETED {
		t.Fatalf("final status = %s, want %s", finalTask.Status, common.COMPLETED)
	}
	if len(gotProgress) != 2 || gotProgress[0] != 82 || gotProgress[1] != 100 {
		t.Fatalf("gotProgress = %v, want [82 100]", gotProgress)
	}
	if len(gotMsgs) != 2 || gotMsgs[1] != "mock dataflow done" {
		t.Fatalf("gotMsgs = %v, want final message %q", gotMsgs, "mock dataflow done")
	}
}
