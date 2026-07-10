package task

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// =============================================================================
// Test helpers
// =============================================================================

func strPtr(s string) *string { return &s }

func makeTaskCtx() *TaskContext {
	return &TaskContext{
		IngestionTask: &entity.IngestionTask{
			ID:         "task-1",
			DocumentID: "doc-1",
		},
		Doc: entity.Document{
			ID:     "doc-1",
			KbID:   "kb-1",
			Name:   strPtr("test-doc.pdf"),
			Suffix: ".pdf",
			Type:   "pdf",
		},
		KB: entity.Knowledgebase{
			ID:       "kb-1",
			TenantID: "tenant-1",
			EmbdID:   "embd-1",
		},
		Tenant: entity.Tenant{
			ID: "tenant-1",
		},
		ProgressFunc: func(prog float64, msg string) {},
	}
}

func setupPipelineExecutorTestDB(t *testing.T) func() {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("auto-migrate sqlite: %v", err)
	}
	origDB := dao.DB
	dao.DB = db
	return func() { dao.DB = origDB }
}

func mustNewPipelineExecutor(t *testing.T, taskCtx *TaskContext, canvasID string, docBulkSize int) *PipelineExecutor {
	t.Helper()
	svc, err := NewPipelineExecutor(taskCtx, canvasID, docBulkSize)
	if err != nil {
		t.Fatalf("NewPipelineExecutor: %v", err)
	}
	return svc
}

// =============================================================================
// NewPipelineExecutor — constructor
// =============================================================================

func TestNewPipelineExecutor_Basic(t *testing.T) {
	svc, err := NewPipelineExecutor(makeTaskCtx(), "flow-1", 0)
	if err != nil {
		t.Fatalf("NewPipelineExecutor: %v", err)
	}
	if svc == nil {
		t.Fatal("NewPipelineExecutor returned nil")
	}
	if svc.taskCtx == nil {
		t.Error("taskCtx should not be nil")
	}
	if svc.insertChunksFunc == nil || svc.logCreateFunc == nil || svc.loadDSLFunc == nil || svc.runPipelineFunc == nil {
		t.Fatal("expected production dependencies to be fully initialized")
	}
}

func TestNewPipelineExecutor_RejectsNilTaskContext(t *testing.T) {
	_, err := NewPipelineExecutor(nil, "flow-1", 0)
	if err == nil {
		t.Fatal("expected error for nil task context")
	}
}

func TestNewPipelineExecutor_RejectsEmptyCanvasID(t *testing.T) {
	_, err := NewPipelineExecutor(makeTaskCtx(), "", 0)
	if err == nil {
		t.Fatal("expected error for empty canvas id")
	}
}

func TestNewPipelineExecutor_RejectsIncompleteTaskContext(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TaskContext)
	}{
		{name: "missing doc id", mutate: func(ctx *TaskContext) { ctx.Doc.ID = "" }},
		{name: "missing kb id", mutate: func(ctx *TaskContext) { ctx.Doc.KbID = "" }},
		{name: "missing doc name", mutate: func(ctx *TaskContext) { ctx.Doc.Name = nil }},
		{name: "missing knowledgebase id", mutate: func(ctx *TaskContext) { ctx.KB.ID = "" }},
		{name: "missing tenant id", mutate: func(ctx *TaskContext) { ctx.Tenant.ID = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := makeTaskCtx()
			tt.mutate(ctx)
			_, err := NewPipelineExecutor(ctx, "flow-1", 0)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNewPipelineExecutor_DocBulkSize(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 128)
	if svc.docBulkSize != 128 {
		t.Errorf("docBulkSize = %d, want 128", svc.docBulkSize)
	}
}

func TestNewPipelineExecutor_WithProgressFunc(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithProgressFunc(func(prog float64, msg string) {})
	if svc.progressFunc == nil {
		t.Error("progressFunc should be set via WithProgressFunc")
	}
}

func TestNewPipelineExecutor_CanvasID(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "my-flow-id", 0)
	if svc.canvasID != "my-flow-id" {
		t.Errorf("canvasID = %q, want %q", svc.canvasID, "my-flow-id")
	}
}

func TestKB_Doc_Tenant_Accessors(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	if svc.KB().ID != "kb-1" {
		t.Errorf("KB().ID = %q, want \"kb-1\"", svc.KB().ID)
	}
	if svc.Doc().ID != "doc-1" {
		t.Errorf("Doc().ID = %q, want \"doc-1\"", svc.Doc().ID)
	}
	if svc.Tenant().ID != "tenant-1" {
		t.Errorf("Tenant().ID = %q, want \"tenant-1\"", svc.Tenant().ID)
	}
}

// =============================================================================
// processChunks
// =============================================================================

func TestPipelineExecutor_ProcessChunks_WrapsProcessChunksForPipeline(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	chunks := []map[string]any{{"text": "hello world"}}
	meta := svc.processChunks(chunks)

	// Verify the wrapper method works correctly and chunks are processed
	if chunks[0]["doc_id"] != "doc-1" {
		t.Errorf("doc_id = %q, want \"doc-1\"", chunks[0]["doc_id"])
	}
	if meta != nil {
		// No need to verify the detailed content of meta as ProcessChunksForPipeline already has comprehensive tests
	}
}

// =============================================================================
// progress
// =============================================================================

func TestProgress_CallsCallback(t *testing.T) {
	var called bool
	var lastProg float64
	var lastMsg string

	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	svc.WithProgressFunc(func(prog float64, msg string) {
		called = true
		lastProg = prog
		lastMsg = msg
	})
	svc.progress(0.82, "Start to embedding...")

	if !called {
		t.Error("progress callback was not called")
	}
	if lastProg != 0.82 {
		t.Errorf("prog = %f, want 0.82", lastProg)
	}
	if lastMsg != "Start to embedding..." {
		t.Errorf("msg = %q, want \"Start to embedding...\"", lastMsg)
	}
}

func TestProgress_NilCallbackDoesNotPanic(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	svc.progress(0.5, "test")
}

// =============================================================================
// insertChunks
// =============================================================================

func TestInsertChunks_EmptyChunks(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithInsertChunksFunc(
		func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		},
	)
	err := svc.insertChunks(context.Background(), nil)
	if err != nil {
		t.Errorf("expected no error for nil chunks, got %v", err)
	}
}

func TestInsertChunks_BaseNameAndDatasetID(t *testing.T) {
	var capturedBaseName, capturedDatasetID string
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithInsertChunksFunc(
		func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			capturedBaseName = baseName
			capturedDatasetID = datasetID
			return nil, nil
		},
	)
	chunks := []map[string]any{{"text": "hello"}}
	err := svc.insertChunks(context.Background(), chunks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedBaseName != "ragflow_tenant-1" {
		t.Errorf("baseName = %q, want \"ragflow_tenant-1\"", capturedBaseName)
	}
	if capturedDatasetID != "kb-1" {
		t.Errorf("datasetID = %q, want \"kb-1\"", capturedDatasetID)
	}
}

func TestRecordPipelineLog(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error { return nil },
	)
	svc.recordPipelineLog("doc-1", `{"components": {}}`, "done")
}

// =============================================================================
// updateDocumentMetadata
// =============================================================================

func TestRunPipeline_NilOutput(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	_, err := svc.processOutput(context.Background(), nil)
	if err != nil {
		t.Errorf("expected nil error for nil output, got %v", err)
	}
}

func TestRunPipeline_EmptyOutput(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error { return nil },
	)
	_, err := svc.processOutput(context.Background(), map[string]any{})
	if err != nil {
		t.Errorf("expected nil error for empty output, got %v", err)
	}
}

func TestRunPipeline_NormalizedEmpty(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error { return nil },
	)
	_, err := svc.processOutput(context.Background(), map[string]any{"markdown": ""})
	if err != nil {
		t.Errorf("expected nil error for empty normalized output, got %v", err)
	}
}

func TestRunPipeline_FullFlow(t *testing.T) {
	var progressCalls []float64
	var progressMsgs []string
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil }).
		WithProgressFunc(func(prog float64, msg string) {
			progressCalls = append(progressCalls, prog)
			progressMsgs = append(progressMsgs, msg)
		})

	output := map[string]any{
		"chunks": []map[string]any{
			{"text": "hello"},
			{"text": "world"},
		},
	}
	_, err := svc.processOutput(context.Background(), output)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(progressCalls) < 2 {
		t.Fatalf("expected multiple progress calls, got %v", progressCalls)
	}
	if progressCalls[len(progressCalls)-1] != 1.0 {
		t.Fatalf("final progress = %v, want 1.0", progressCalls[len(progressCalls)-1])
	}
	lastMsg := progressMsgs[len(progressMsgs)-1]
	if !strings.Contains(lastMsg, "Indexing done (") || !strings.Contains(lastMsg, "Task done (") {
		t.Fatalf("final progress msg = %q, want indexing/task timing message", lastMsg)
	}
}

func TestRunPipeline_AlreadyHasVectors(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil }).
		WithProgressFunc(func(prog float64, msg string) {})

	output := map[string]any{
		"chunks": []map[string]any{
			{"text": "hello", "q_768_vec": []float64{0.1, 0.2}},
		},
	}
	_, err := svc.processOutput(context.Background(), output)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunPipeline_ContextCanceled(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.processOutput(ctx, map[string]any{
		"chunks": []map[string]any{{"text": "hello"}},
	})
	if err == nil {
		t.Error("expected context canceled error")
	}
}

func TestPipelineExecutor_Run_MainFlowWithStubs(t *testing.T) {
	logged := false
	inserted := false
	var progressCalls []float64
	var progressMsgs []string

	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, "flow-corrected", nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			return map[string]any{
				"chunks": []map[string]any{
					{"text": "hello world"},
				},
			}, dsl, nil
		}).
		WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			inserted = true
			return nil, nil
		}).
		WithProgressFunc(func(prog float64, msg string) {
			progressCalls = append(progressCalls, prog)
			progressMsgs = append(progressMsgs, msg)
		}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error {
			logged = true
			if log.PipelineID == nil || *log.PipelineID != "flow-corrected" {
				t.Fatalf("PipelineID = %v, want flow-corrected", log.PipelineID)
			}
			return nil
		})

	_, err := svc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inserted {
		t.Fatal("expected insertChunks to be called")
	}
	if !logged {
		t.Fatal("expected pipeline log to be created")
	}
	if len(progressCalls) == 0 || progressCalls[len(progressCalls)-1] != 1.0 {
		t.Fatalf("expected final progress 1.0, got %v", progressCalls)
	}
	lastMsg := progressMsgs[len(progressMsgs)-1]
	if !strings.Contains(lastMsg, "Indexing done (") || !strings.Contains(lastMsg, "Task done (") {
		t.Fatalf("final progress msg = %q, want indexing/task timing message", lastMsg)
	}
}

func TestInsertChunks_ReportsBatchProgress(t *testing.T) {
	var progressCalls []float64
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 1).
		WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			time.Sleep(5 * time.Millisecond)
			return nil, nil
		}).
		WithProgressFunc(func(prog float64, msg string) {
			progressCalls = append(progressCalls, prog)
		})

	chunks := []map[string]any{
		{"text": "a"},
		{"text": "b"},
	}
	if err := svc.insertChunks(context.Background(), chunks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(progressCalls) == 0 {
		t.Fatal("expected indexing progress callbacks")
	}
}

// =============================================================================
// Stub implementations for testing
// =============================================================================
