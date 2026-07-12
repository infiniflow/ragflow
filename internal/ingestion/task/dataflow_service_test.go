package task

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/service"
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

func setupDataflowServiceTestDB(t *testing.T) func() {
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

func mustNewDataflowService(t *testing.T, taskCtx *TaskContext, dataflowID string, embeddingBatchSize int, docBulkSize int) *PipelineExecutor {
	t.Helper()
	svc, err := NewDataflowService(taskCtx, dataflowID, embeddingBatchSize, docBulkSize)
	if err != nil {
		t.Fatalf("NewDataflowService: %v", err)
	}
	return svc
}

func TestDataflowService_DefaultLoadDSL_UsesUserCanvas(t *testing.T) {
	cleanup := setupDataflowServiceTestDB(t)
	defer cleanup()

	dslMap := entity.JSONMap{"dsl": map[string]any{"graph": map[string]any{"nodes": []any{}, "edges": []any{}}}}
	title := "title 1"
	if err := dao.NewUserCanvasDAO().Create(&entity.UserCanvas{Title: &title, ID: "canvas-1", UserID: "u1", Permission: "me", CanvasCategory: "agent_canvas", DSL: dslMap}); err != nil {
		t.Fatalf("create user canvas: %v", err)
	}

	ctx := makeTaskCtx()
	svc := mustNewDataflowService(t, ctx, "canvas-1", 0, 0)
	gotDSL, correctedID, err := svc.loadDSLFunc(context.Background(), "canvas-1")
	if err != nil {
		t.Fatalf("loadDSLFunc: %v", err)
	}
	if correctedID != "canvas-1" {
		t.Fatalf("correctedID = %q, want canvas-1", correctedID)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(gotDSL), &decoded); err != nil {
		t.Fatalf("unmarshal dsl: %v", err)
	}
	if _, ok := decoded["dsl"].(map[string]any); !ok {
		t.Fatalf("decoded dsl = %v, want top-level dsl map", decoded)
	}
}

// =============================================================================
// NewDataflowService — constructor
// =============================================================================

func TestNewDataflowService_Basic(t *testing.T) {
	svc, err := NewDataflowService(makeTaskCtx(), "flow-1", 0, 0)
	if err != nil {
		t.Fatalf("NewDataflowService: %v", err)
	}
	if svc == nil {
		t.Fatal("NewDataflowService returned nil")
	}
	if svc.taskCtx == nil {
		t.Error("taskCtx should not be nil")
	}
	if svc.docSvc == nil || svc.chunkCounter == nil || svc.insertChunksFunc == nil || svc.logCreateFunc == nil || svc.loadDSLFunc == nil || svc.runPipelineFunc == nil {
		t.Fatal("expected production dependencies to be fully initialized")
	}
}

func TestNewDataflowService_RejectsNilTaskContext(t *testing.T) {
	_, err := NewDataflowService(nil, "flow-1", 0, 0)
	if err == nil {
		t.Fatal("expected error for nil task context")
	}
}

func TestNewDataflowService_RejectsEmptyDataflowID(t *testing.T) {
	_, err := NewDataflowService(makeTaskCtx(), "", 0, 0)
	if err == nil {
		t.Fatal("expected error for empty dataflow id")
	}
}

func TestNewDataflowService_RejectsIncompleteTaskContext(t *testing.T) {
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
			_, err := NewDataflowService(ctx, "flow-1", 0, 0)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestNewDataflowService_CustomBatchSizes(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 64, 128)
	if svc.embeddingBatchSize != 64 {
		t.Errorf("embeddingBatchSize = %d, want 64", svc.embeddingBatchSize)
	}
	if svc.docBulkSize != 128 {
		t.Errorf("docBulkSize = %d, want 128", svc.docBulkSize)
	}
}

func TestNewDataflowService_WithProgressFunc(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithProgressFunc(func(prog float64, msg string) {})
	if svc.progressFunc == nil {
		t.Error("progressFunc should be set via WithProgressFunc")
	}
}

func TestNewDataflowService_DataflowID(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "my-flow-id", 0, 0)
	if svc.dataflowID != "my-flow-id" {
		t.Errorf("dataflowID = %q, want %q", svc.dataflowID, "my-flow-id")
	}
}

func TestKB_Doc_Tenant_Accessors(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0)
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

func TestDataflowService_ProcessChunks_WrapsProcessChunksForDataflow(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0)
	chunks := []map[string]any{{"text": "hello world"}}
	meta := svc.processChunks(chunks)

	// Verify the wrapper method works correctly and chunks are processed
	if chunks[0]["doc_id"] != "doc-1" {
		t.Errorf("doc_id = %q, want \"doc-1\"", chunks[0]["doc_id"])
	}
	if meta != nil {
		// No need to verify the detailed content of meta as ProcessChunksForDataflow already has comprehensive tests
	}
}

// =============================================================================
// progress
// =============================================================================

func TestProgress_CallsCallback(t *testing.T) {
	var called bool
	var lastProg float64
	var lastMsg string

	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0)
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
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0)
	svc.progress(0.5, "test")
}

// =============================================================================
// insertChunks
// =============================================================================

func TestInsertChunks_EmptyChunks(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithInsertChunksFunc(
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
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithInsertChunksFunc(
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

// =============================================================================
// updateDocumentMetadata
// =============================================================================

func TestUpdateDocumentMetadata_EmptyMetadata(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithDocService(&stubDocService{})
	err := svc.updateDocumentMetadata("doc-1", nil)
	if err != nil {
		t.Errorf("empty metadata should not error, got %v", err)
	}
}

func TestUpdateDocumentMetadata_MergesNewKeys(t *testing.T) {
	mds := &stubDocService{metaData: map[string]any{"existing": "old"}}
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithDocService(mds)
	err := svc.updateDocumentMetadata("doc-1", map[string]any{"new_key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mds.metaData["existing"] != "old" {
		t.Errorf("existing key should be preserved: got %q", mds.metaData["existing"])
	}
	if mds.metaData["new_key"] != "value" {
		t.Errorf("new_key = %q, want \"value\"", mds.metaData["new_key"])
	}
}

func TestUpdateDocumentMetadata_PreservesExistingKey(t *testing.T) {
	mds := &stubDocService{metaData: map[string]any{"author": "Alice"}}
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithDocService(mds)
	err := svc.updateDocumentMetadata("doc-1", map[string]any{"author": "Bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mds.metaData["author"] != "Alice" {
		t.Errorf("existing key should NOT be overwritten: got %q", mds.metaData["author"])
	}
}

// =============================================================================
// recordPipelineLog
// =============================================================================

func TestRecordPipelineLog(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error { return nil },
	)
	svc.recordPipelineLog("doc-1", `{"components": {}}`, "done")
}

// =============================================================================
// updateDocumentMetadata
// =============================================================================

// incrementChunkNum
// =============================================================================

func TestIncrementChunkNum(t *testing.T) {
	counter := &stubChunkCounter{}
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).
		WithChunkCounter(counter)
	err := svc.incrementChunkNum("doc-1", "kb-1", 10, 100, 0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if counter.lastDocID != "doc-1" {
		t.Errorf("docID = %q, want \"doc-1\"", counter.lastDocID)
	}
	if counter.lastKbID != "kb-1" {
		t.Errorf("kbID = %q, want \"kb-1\"", counter.lastKbID)
	}
	if counter.lastChunkNum != 10 {
		t.Errorf("chunkNum = %d, want 10", counter.lastChunkNum)
	}
	if counter.lastTokenNum != 100 {
		t.Errorf("tokenNum = %d, want 100", counter.lastTokenNum)
	}
}

func TestIncrementChunkNum_ProcessDuration(t *testing.T) {
	counter := &stubChunkCounter{}
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).
		WithChunkCounter(counter)
	err := svc.incrementChunkNum("doc-1", "kb-1", 5, 50, 12.5)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if counter.lastDuration != 12.5 {
		t.Errorf("duration = %f, want 12.5", counter.lastDuration)
	}
	if counter.lastChunkNum != 5 {
		t.Errorf("chunkNum = %d, want 5", counter.lastChunkNum)
	}
	if counter.lastTokenNum != 50 {
		t.Errorf("tokenNum = %d, want 50", counter.lastTokenNum)
	}
}

// =============================================================================
// RunDataflow — Python: run_dataflow (line 94)
// =============================================================================

func TestRunDataflow_NilOutput(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0)
	err := svc.processOutput(context.Background(), nil)
	if err != nil {
		t.Errorf("expected nil error for nil output, got %v", err)
	}
}

func TestRunDataflow_EmptyOutput(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error { return nil },
	)
	err := svc.processOutput(context.Background(), map[string]any{})
	if err != nil {
		t.Errorf("expected nil error for empty output, got %v", err)
	}
}

func TestRunDataflow_NormalizedEmpty(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).WithLogCreateFunc(
		func(log *entity.PipelineOperationLog) error { return nil },
	)
	err := svc.processOutput(context.Background(), map[string]any{"markdown": ""})
	if err != nil {
		t.Errorf("expected nil error for empty normalized output, got %v", err)
	}
}

func TestRunDataflow_FullFlow(t *testing.T) {
	var progressCalls []float64
	var progressMsgs []string
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).
		WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithDocService(&stubDocService{}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil }).
		WithChunkCounter(&stubChunkCounter{}).
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
	err := svc.processOutput(context.Background(), output)
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

func TestRunDataflow_AlreadyHasVectors(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).
		WithInsertChunksFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithDocService(&stubDocService{}).
		WithLogCreateFunc(func(log *entity.PipelineOperationLog) error { return nil }).
		WithChunkCounter(&stubChunkCounter{}).
		WithProgressFunc(func(prog float64, msg string) {})

	output := map[string]any{
		"chunks": []map[string]any{
			{"text": "hello", "q_768_vec": []float64{0.1, 0.2}},
		},
	}
	err := svc.processOutput(context.Background(), output)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDataflow_ContextCanceled(t *testing.T) {
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.processOutput(ctx, map[string]any{
		"chunks": []map[string]any{{"text": "hello"}},
	})
	if err == nil {
		t.Error("expected context canceled error")
	}
}

func TestExtractDataflowPipelinePayload_UnwrapsSingleTerminalOutput(t *testing.T) {
	dsl := `{"dsl":{"components":{"Parser:A":{"downstream":["Tokenizer:B"]},"Tokenizer:B":{"downstream":[]}}}}`
	out := map[string]any{
		"Tokenizer:B": map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{{"text": "hello world"}},
		},
		"state": map[string]any{"Tokenizer:B": map[string]any{"output_format": "chunks"}},
	}

	payload, err := extractDataflowPipelinePayload(dsl, out)
	if err != nil {
		t.Fatalf("extractDataflowPipelinePayload: %v", err)
	}
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	if len(chunks) != 1 || chunks[0]["text"] != "hello world" {
		t.Fatalf("chunks = %v, want single hello world chunk", chunks)
	}
}

func TestExtractDataflowPipelinePayload_ErrorsOnMultipleTerminals(t *testing.T) {
	dsl := `{"dsl":{"components":{"Tokenizer:A":{"downstream":[]},"Tokenizer:B":{"downstream":[]}}}}`
	_, err := extractDataflowPipelinePayload(dsl, map[string]any{
		"Tokenizer:A": map[string]any{"output_format": "chunks"},
		"Tokenizer:B": map[string]any{"output_format": "chunks"},
	})
	if err == nil {
		t.Fatal("expected error for multiple terminals")
	}
	if !strings.Contains(err.Error(), "exactly 1 terminal") {
		t.Fatalf("err = %v, want exactly 1 terminal", err)
	}
}

func TestDataflowService_Run_MainFlowWithStubs(t *testing.T) {
	logged := false
	inserted := false
	var progressCalls []float64
	var progressMsgs []string

	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 0).
		WithLoadDSLFunc(func(ctx context.Context, dataflowID string) (string, string, error) {
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
		WithDocService(&stubDocService{}).
		WithChunkCounter(&stubChunkCounter{}).
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

	err := svc.Execute(context.Background())
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
	svc := mustNewDataflowService(t, makeTaskCtx(), "flow-1", 0, 1).
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

type stubDocService struct {
	err       error
	metaData  map[string]any
	lastReq   *service.UpdateDocumentRequest
	lastDocID string
}

type stubChunkCounter struct {
	err          error
	lastDocID    string
	lastKbID     string
	lastChunkNum int
	lastTokenNum int
	lastDuration float64
	callCount    int
}

func (s *stubDocService) UpdateDocument(id string, req *service.UpdateDocumentRequest) error {
	s.lastDocID = id
	s.lastReq = req
	return s.err
}

func (s *stubDocService) GetDocumentMetadataByID(docID string) (map[string]any, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.metaData == nil {
		return make(map[string]any), nil
	}
	return s.metaData, nil
}

func (s *stubDocService) SetDocumentMetadata(docID string, meta map[string]any) error {
	if s.metaData == nil {
		s.metaData = make(map[string]any)
	}
	for k, v := range meta {
		s.metaData[k] = v
	}
	return s.err
}

func (s *stubChunkCounter) IncrementChunkNum(docID, kbID string, chunkNum, tokenConsumption int, duration float64) error {
	s.lastDocID = docID
	s.lastKbID = kbID
	s.lastChunkNum = chunkNum
	s.lastTokenNum = tokenConsumption
	s.lastDuration = duration
	s.callCount++
	return s.err
}

// Compile-time checks
var (
	_ docService   = (*stubDocService)(nil)
	_ chunkCounter = (*stubChunkCounter)(nil)
)

func makeEmbeddingModelForResolver() *models.EmbeddingModel {
	return models.NewEmbeddingModel(&stubDriver{}, strPtr("embed"), &models.APIConfig{}, 128)
}

func TestEmbedderResolver_ExplicitEmbeddingModelWins(t *testing.T) {
	var gotTenantID, gotEmbdID string
	resolver := newEmbedderResolver(
		func(tenantID, embdID string) (*models.EmbeddingModel, error) {
			gotTenantID, gotEmbdID = tenantID, embdID
			return makeEmbeddingModelForResolver(), nil
		},
		func(string) (*entity.Knowledgebase, error) {
			t.Fatal("kb lookup should not run when embedding_model is set")
			return nil, nil
		},
	)
	emb, err := resolver("tenant-1", "kb-1", "explicit-embd")
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if emb == nil {
		t.Fatal("expected embedder")
	}
	if gotTenantID != "tenant-1" || gotEmbdID != "explicit-embd" {
		t.Fatalf("resolver args = (%q, %q), want (tenant-1, explicit-embd)", gotTenantID, gotEmbdID)
	}
}

func TestEmbedderResolver_FallsBackToDatasetEmbedding(t *testing.T) {
	var gotEmbdID string
	resolver := newEmbedderResolver(
		func(_ string, embdID string) (*models.EmbeddingModel, error) {
			gotEmbdID = embdID
			return makeEmbeddingModelForResolver(), nil
		},
		func(kbID string) (*entity.Knowledgebase, error) {
			if kbID != "kb-1" {
				t.Fatalf("kb lookup id = %q, want kb-1", kbID)
			}
			return &entity.Knowledgebase{ID: "kb-1", EmbdID: "lookup-embd"}, nil
		},
	)
	if _, err := resolver("tenant-1", "kb-1", ""); err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if gotEmbdID != "lookup-embd" {
		t.Fatalf("got embd id %q, want lookup-embd", gotEmbdID)
	}
}

func TestEmbedderResolver_MissingDatasetEmbeddingReturnsError(t *testing.T) {
	resolver := newEmbedderResolver(
		func(string, string) (*models.EmbeddingModel, error) {
			t.Fatal("model resolver should not be called")
			return nil, nil
		},
		func(string) (*entity.Knowledgebase, error) {
			return &entity.Knowledgebase{ID: "kb-1", EmbdID: ""}, nil
		},
	)
	_, err := resolver("tenant-1", "kb-1", "")
	if err == nil {
		t.Fatal("expected error when dataset embd_id is missing, got nil")
	}
	if !strings.Contains(err.Error(), "dataset has no embd_id configured") {
		t.Fatalf("err = %v, want dataset has no embd_id configured", err)
	}
}

func TestEmbedderResolver_MissingEmbeddingModelAndKBReturnsError(t *testing.T) {
	resolver := newEmbedderResolver(
		func(string, string) (*models.EmbeddingModel, error) {
			t.Fatal("model resolver should not be called")
			return nil, nil
		},
		func(string) (*entity.Knowledgebase, error) {
			t.Fatal("kb lookup should not be called without a kb_id")
			return nil, nil
		},
	)
	_, err := resolver("tenant-1", "", "")
	if err == nil {
		t.Fatal("expected error when neither embedding_model nor kb_id provided")
	}
	if !strings.Contains(err.Error(), "neither embedding_model nor kb_id") {
		t.Fatalf("err = %v, want neither embedding_model nor kb_id", err)
	}
}
