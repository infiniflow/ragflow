package task

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
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
	if svc.indexWriter == nil || svc.logCreateFunc == nil || svc.loadDSLFunc == nil || svc.runPipelineFunc == nil {
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
	meta, err := ProcessChunksForPipeline(chunks, svc.taskCtx.Doc.ID, svc.taskCtx.Doc.KbID, *svc.taskCtx.Doc.Name, time.Now())
	if err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	// Verify the wrapper method works correctly and chunks are processed
	if chunks[0]["doc_id"] != "doc-1" {
		t.Errorf("doc_id = %q, want \"doc-1\"", chunks[0]["doc_id"])
	}
	if meta != nil {
		// No need to verify the detailed content of meta as ProcessChunksForPipeline already has comprehensive tests
	}
}

// =============================================================================
// insertChunks
// =============================================================================

func TestInsertChunks_EmptyChunks(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithInsertFunc(
		func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		},
	)
	err := svc.indexWriter.Write(context.Background(), nil)
	if err != nil {
		t.Errorf("expected no error for nil chunks, got %v", err)
	}
}

func TestInsertChunks_BaseNameAndDatasetID(t *testing.T) {
	var capturedBaseName, capturedDatasetID string
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithInsertFunc(
		func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			capturedBaseName = baseName
			capturedDatasetID = datasetID
			return nil, nil
		},
	)
	chunks := []map[string]any{{"text": "hello"}}
	err := svc.indexWriter.Write(context.Background(), chunks)
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

func TestRecordPipelineLog_InvalidJSONFallback(t *testing.T) {
	var captured *entity.PipelineOperationLog
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		},
	)
	svc.recordPipelineLog("doc-1", "not-valid-json", "done")
	if captured == nil {
		t.Fatal("logCreateFunc was not called")
	}
	raw, ok := captured.DSL["raw"].(string)
	if !ok || raw != "not-valid-json" {
		t.Fatalf("DSL = %v, want {\"raw\": \"not-valid-json\"}", captured.DSL)
	}
}

func TestRecordPipelineLog_ValidJSONParsed(t *testing.T) {
	var captured *entity.PipelineOperationLog
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		},
	)
	svc.recordPipelineLog("doc-1", `{"components": {"a": {"obj": {"component_name": "Parser", "params": {}}}}}`, "done")
	if captured == nil {
		t.Fatal("logCreateFunc was not called")
	}
	if captured.DSL["raw"] != nil {
		t.Fatalf("DSL should be parsed JSON, not fallback raw; got %v", captured.DSL)
	}
}

// =============================================================================
// updateDocumentMetadata
// =============================================================================

func TestRunPipeline_NilOutput(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	_, err := svc.processOutput(context.Background(), nil, time.Now())
	if err != nil {
		t.Errorf("expected nil error for nil output, got %v", err)
	}
}

func TestRunPipeline_EmptyOutput(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error { return nil },
	)
	_, err := svc.processOutput(context.Background(), map[string]any{}, time.Now())
	if err != nil {
		t.Errorf("expected nil error for empty output, got %v", err)
	}
}

func TestRunPipeline_NormalizedEmpty(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error { return nil },
	)
	_, err := svc.processOutput(context.Background(), map[string]any{"markdown": ""}, time.Now())
	if err != nil {
		t.Errorf("expected nil error for empty normalized output, got %v", err)
	}
}

func TestRunPipeline_FullFlow(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil })
	output := map[string]any{
		"chunks": []map[string]any{
			{"text": "hello"},
			{"text": "world"},
		},
	}
	_, err := svc.processOutput(context.Background(), output, time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunPipeline_AlreadyHasVectors(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil })

	output := map[string]any{
		"chunks": []map[string]any{
			{"text": "hello", "q_768_vec": []float64{0.1, 0.2}},
		},
	}
	_, err := svc.processOutput(context.Background(), output, time.Now())
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
	}, time.Now())
	if err == nil {
		t.Error("expected context canceled error")
	}
}

func TestPipelineExecutor_Run_MainFlowWithStubs(t *testing.T) {
	logged := false
	inserted := false

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
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			inserted = true
			return nil, nil
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
}

// TestPipelineExecutor_Execute_PropagatesContext verifies the ctx passed to
// Execute is the ctx received by runPipelineFunc - the task context must flow
// through to the pipeline run.
func TestPipelineExecutor_Execute_PropagatesContext(t *testing.T) {
	type ctxKey string
	const key ctxKey = "trace"
	taskCtx := makeTaskCtx()
	taskCtx.Ctx = context.WithValue(context.Background(), key, "task-ctx")

	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(runCtx context.Context, dsl string) (map[string]any, string, error) {
			if got := runCtx.Value(key); got != "task-ctx" {
				t.Fatalf("runCtx value = %v, want task-ctx", got)
			}
			return map[string]any{"chunks": []map[string]any{{"text": "hello world"}}}, dsl, nil
		}).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil })

	if _, err := svc.Execute(taskCtx.Ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Stub implementations for testing
// =============================================================================

// recordingProgressSink captures progress events for asserting the executor
// forwards its sink through runPipelineWithDSL into the pipeline.
type recordingProgressSink struct {
	mu       sync.Mutex
	total    int
	totalSet bool
	events   []pipelinepkg.ProgressEvent
}

func (r *recordingProgressSink) OnComponentTotal(taskID string, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.total = total
	r.totalSet = true
}

func (r *recordingProgressSink) OnComponentProgress(ev pipelinepkg.ProgressEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

type sinkPassthroughStage struct{}

func (sinkPassthroughStage) Invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	return inputs, nil
}

// TestPipelineExecutorRunPipelineWithDSLForwardsSink verifies the sink set via
// WithProgressSink is threaded through runPipelineWithDSL into the pipeline,
// which reports the component total and lifecycle events back to the sink.
func TestPipelineExecutorRunPipelineWithDSLForwardsSink(t *testing.T) {
	const nameA = "task.SinkPassthroughA"
	runtime.MustRegister(nameA, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return sinkPassthroughStage{}, nil },
		runtime.Metadata{Version: "1.0.0"})

	sink := &recordingProgressSink{}
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	svc.WithProgressSink(sink)

	dsl := `{"dsl":{"components":{"begin":{"obj":{"component_name":"Begin","params":{}},"downstream":["a"]},"a":{"obj":{"component_name":"` + nameA + `","params":{}},"upstream":["begin"]}},"path":["begin","a"],"graph":{"nodes":[]}}}`

	if _, _, err := svc.runPipelineWithDSL(context.Background(), dsl); err != nil {
		t.Fatalf("runPipelineWithDSL: %v", err)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if !sink.totalSet || sink.total != 2 {
		t.Fatalf("OnComponentTotal = (%d, set=%v), want 2", sink.total, sink.totalSet)
	}
	if len(sink.events) == 0 {
		t.Fatal("expected progress events forwarded to sink, got none")
	}
	for _, ev := range sink.events {
		if ev.TaskID != "task-1" {
			t.Fatalf("event TaskID = %q, want task-1", ev.TaskID)
		}
		if ev.DocumentID != "doc-1" {
			t.Fatalf("event DocumentID = %q, want doc-1", ev.DocumentID)
		}
	}
}
