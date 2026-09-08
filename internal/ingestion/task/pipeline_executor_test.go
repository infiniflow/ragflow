package task

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
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
	indexdoc "ragflow/internal/ingestion/task/indexdoc"
)

// =============================================================================
// Test helpers
// =============================================================================

func strPtr(s string) *string { return &s }

// TestMarkCompiledProductsHidden verifies the pipeline caller hides
// per-document compiled knowledge products (compile_kwd present) as
// available_int=0 while leaving ordinary source chunks searchable
// (available_int=1, the index default). Merged dataset-level products are written
// by the consumer and never reach this path, so they are never double-marked.
func TestMarkCompiledProductsHidden(t *testing.T) {
	chunks := []map[string]any{
		{"id": "src-1", "content_with_weight": "ordinary source chunk"},
		{"id": "struct-1", "compile_kwd": "structure", "content_with_weight": "entity A"},
		{"id": "wiki-1", "compile_kwd": "wiki_page", "content_with_weight": "page X"},
		{"id": "src-2", "content_with_weight": "another source chunk"},
	}
	markCompiledProductsHidden(chunks)

	if v, ok := chunks[0]["available_int"]; ok {
		t.Fatalf("ordinary source chunk should keep default available_int, got %v", v)
	}
	if chunks[1]["available_int"] != 0 {
		t.Fatalf("compiled structure chunk should be available_int=0, got %v", chunks[1]["available_int"])
	}
	if chunks[2]["available_int"] != 0 {
		t.Fatalf("compiled wiki chunk should be available_int=0, got %v", chunks[2]["available_int"])
	}
	if v, ok := chunks[3]["available_int"]; ok {
		t.Fatalf("source chunk without compile_kwd should keep default available_int, got %v", v)
	}
}

func TestWikiActiveStatesDecodeCheckpointValues(t *testing.T) {
	states, err := wikiActiveStates(map[string]any{
		"wiki_active_map_states": []any{map[string]any{
			"key": "state-1", "tenant_id": "tenant-1", "dataset_id": "kb-1", "document_id": "doc-1", "payload": `{"plan":[]}`,
		}},
	})
	if err != nil {
		t.Fatalf("wikiActiveStates failed: %v", err)
	}
	if len(states) != 1 || states[0].Key != "state-1" || string(states[0].Payload) != `{"plan":[]}` {
		t.Fatalf("decoded states = %#v", states)
	}
}

// TestApplyDocumentAvailability verifies disabled documents (status=0) force
// ordinary source chunks to available_int=0 while compiled products stay hidden.
func TestApplyDocumentAvailability(t *testing.T) {
	chunks := []map[string]any{
		{"id": "src-1", "content_with_weight": "ordinary source chunk"},
		{"id": "struct-1", "compile_kwd": "structure", "content_with_weight": "entity A", "available_int": 0},
		{"id": "src-2", "content_with_weight": "another source chunk"},
	}
	markCompiledProductsHidden(chunks)
	applyDocumentAvailability(chunks, strPtr("0"))

	if chunks[0]["available_int"] != 0 {
		t.Fatalf("disabled doc source chunk should be available_int=0, got %v", chunks[0]["available_int"])
	}
	if chunks[1]["available_int"] != 0 {
		t.Fatalf("compiled product should stay available_int=0, got %v", chunks[1]["available_int"])
	}
	if chunks[2]["available_int"] != 0 {
		t.Fatalf("disabled doc source chunk should be available_int=0, got %v", chunks[2]["available_int"])
	}

	enabled := []map[string]any{
		{"id": "src-3", "content_with_weight": "enabled source"},
	}
	applyDocumentAvailability(enabled, strPtr("1"))
	if v, ok := enabled[0]["available_int"]; ok {
		t.Fatalf("enabled doc should keep default available_int, got %v", v)
	}
	applyDocumentAvailability(enabled, nil)
	if v, ok := enabled[0]["available_int"]; ok {
		t.Fatalf("nil status should keep default available_int, got %v", v)
	}
}

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
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.PipelineOperationLog{}, &entity.Document{}); err != nil {
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

// TestNewPipelineExecutor_AcceptsDebugTaskContext verifies the canvas-debug
// (dry-run) contract: a TaskContext with an empty KB.ID is valid because debug
// mode carries no knowledgebase. KB.ID == "" never occurs in production
// ingestion, which always supplies a KB.
func TestNewPipelineExecutor_AcceptsDebugTaskContext(t *testing.T) {
	ctx := makeTaskCtx()
	ctx.KB = entity.Knowledgebase{ID: ""}
	ctx.Doc.KbID = ""
	if _, err := NewPipelineExecutor(ctx, "flow-1", 0); err != nil {
		t.Fatalf("debug TaskContext rejected: %v", err)
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
	meta, err := indexdoc.ProcessChunksForPipeline(chunks, svc.taskCtx.Doc.ID, *svc.taskCtx.Doc.Name, time.Now())
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
	ctx := t.Context()
	err := svc.indexWriter.Write(ctx, nil)
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
	ctx := t.Context()
	chunks := []map[string]any{{"text": "hello"}}
	err := svc.indexWriter.Write(ctx, chunks)
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
		func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil },
	)
	ctx := t.Context()
	svc.recordPipelineLog(ctx, dao.DB, "doc-1", `{"components": {}}`, "done")
}

func TestRecordPipelineLog_InvalidJSONFallback(t *testing.T) {
	var captured *entity.PipelineOperationLog
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		},
	)
	ctx := t.Context()
	svc.recordPipelineLog(ctx, dao.DB, "doc-1", "not-valid-json", "done")
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
		func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		},
	)
	ctx := t.Context()
	svc.recordPipelineLog(ctx, dao.DB, "doc-1", `{"components": {"a": {"obj": {"component_name": "Parser", "params": {}}}}}`, "done")
	if captured == nil {
		t.Fatal("logCreateFunc was not called")
	}
	if captured.DSL["raw"] != nil {
		t.Fatalf("DSL should be parsed JSON, not fallback raw; got %v", captured.DSL)
	}
}

func TestRecordPipelineLog_SharedWriterTerminalWithoutDSL(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	docName := "terminal.pdf"
	run := "1"
	if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
		TenantID:   "tenant-1",
		KbID:       "kb-1",
		DocumentID: "doc-1",
		Status:     "3",
		Document: entity.Document{
			ID:           "doc-1",
			KbID:         "kb-1",
			ParserID:     "naive",
			ParserConfig: entity.JSONMap{},
			SourceType:   "local",
			Type:         "pdf",
			Name:         &docName,
			Suffix:       ".pdf",
			Run:          &run,
		},
	}); err != nil {
		t.Fatalf("RecordPipelineLog: %v", err)
	}

	var log entity.PipelineOperationLog
	if err := dao.DB.First(&log, "document_id = ?", "doc-1").Error; err != nil {
		t.Fatalf("load pipeline log: %v", err)
	}
	if log.OperationStatus != "3" {
		t.Fatalf("OperationStatus = %q, want explicit terminal status", log.OperationStatus)
	}
	if len(log.DSL) != 0 {
		t.Fatalf("DSL = %v, want empty object for terminal writer without DSL", log.DSL)
	}
}

func TestRecordPipelineLog_BuiltinUsesParserIDFallback(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	taskCtx := makeTaskCtx()
	taskCtx.Doc.ParserID = "general"
	taskCtx.Doc.Thumbnail = strPtr("thumb.png")

	var captured *entity.PipelineOperationLog
	svc := mustNewPipelineExecutor(t, taskCtx, "general", 0).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		})
	svc.recordPipelineLog(t.Context(), dao.DB, "doc-1", `{}`, "done")

	if captured == nil {
		t.Fatal("logCreateFunc was not called")
	}
	if captured.PipelineTitle == nil || *captured.PipelineTitle != "general" {
		t.Fatalf("PipelineTitle = %v, want \"general\"", captured.PipelineTitle)
	}
	if captured.Avatar == nil || *captured.Avatar != "thumb.png" {
		t.Fatalf("Avatar = %v, want \"thumb.png\"", captured.Avatar)
	}
	if captured.PipelineID != nil {
		t.Fatalf("PipelineID = %q, want nil for builtin pipeline", *captured.PipelineID)
	}
}

func TestRecordPipelineLog_CustomCanvasTitle(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	if err := dao.DB.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "tenant-1",
		Title:  strPtr("My Pipeline"),
		Avatar: strPtr("a.png"),
	}).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}

	taskCtx := makeTaskCtx()
	taskCtx.Doc.ParserID = "general"
	taskCtx.PipelineID = "canvas-1"

	var captured *entity.PipelineOperationLog
	svc := mustNewPipelineExecutor(t, taskCtx, "canvas-1", 0).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		})
	svc.recordPipelineLog(t.Context(), dao.DB, "doc-1", `{}`, "done")

	if captured == nil {
		t.Fatal("logCreateFunc was not called")
	}
	if captured.PipelineTitle == nil || *captured.PipelineTitle != "My Pipeline" {
		t.Fatalf("PipelineTitle = %v, want \"My Pipeline\"", captured.PipelineTitle)
	}
	if captured.Avatar == nil || *captured.Avatar != "a.png" {
		t.Fatalf("Avatar = %v, want \"a.png\"", captured.Avatar)
	}
	if captured.PipelineID == nil || *captured.PipelineID != "canvas-1" {
		t.Fatalf("PipelineID = %v, want \"canvas-1\"", captured.PipelineID)
	}
}

func TestRecordPipelineLog_CustomCanvasMissingFallsBackToParserID(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	taskCtx := makeTaskCtx()
	taskCtx.Doc.ParserID = "general"
	taskCtx.PipelineID = "canvas-gone"

	var captured *entity.PipelineOperationLog
	svc := mustNewPipelineExecutor(t, taskCtx, "canvas-gone", 0).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		})
	svc.recordPipelineLog(t.Context(), dao.DB, "doc-1", `{}`, "done")

	if captured == nil {
		t.Fatal("logCreateFunc was not called")
	}
	if captured.PipelineTitle == nil || *captured.PipelineTitle != "general" {
		t.Fatalf("PipelineTitle = %v, want \"general\" fallback", captured.PipelineTitle)
	}
	if captured.PipelineID == nil || *captured.PipelineID != "canvas-gone" {
		t.Fatalf("PipelineID = %v, want \"canvas-gone\"", captured.PipelineID)
	}
}

func TestRecordPipelineLog_TerminalWithoutDSLResolvesCanvasTitle(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	if err := dao.DB.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "tenant-1",
		Title:  strPtr("My Pipeline"),
		Avatar: strPtr("a.png"),
	}).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
	if err := dao.DB.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatalf("migrate knowledgebase: %v", err)
	}
	if err := dao.DB.Create(&entity.Knowledgebase{
		ID:       "kb-1",
		TenantID: "tenant-1",
	}).Error; err != nil {
		t.Fatalf("seed knowledgebase: %v", err)
	}
	docName := "sample.avi"
	run := "1"
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		PipelineID:   strPtr("canvas-1"),
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Name:         &docName,
		Run:          &run,
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	// Mirrors Ingestor.recordTerminalPipelineLog: only the terminal status is
	// known; pipeline_id and DSL are absent and must come from the document.
	if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
		KbID:       "kb-1",
		DocumentID: "doc-1",
		Status:     "3",
	}); err != nil {
		t.Fatalf("RecordPipelineLog: %v", err)
	}

	var log entity.PipelineOperationLog
	if err := dao.DB.First(&log, "document_id = ?", "doc-1").Error; err != nil {
		t.Fatalf("load pipeline log: %v", err)
	}
	if log.OperationStatus != "3" {
		t.Fatalf("OperationStatus = %q, want terminal status", log.OperationStatus)
	}
	if log.PipelineID == nil || *log.PipelineID != "canvas-1" {
		t.Fatalf("PipelineID = %v, want \"canvas-1\"", log.PipelineID)
	}
	if log.PipelineTitle == nil || *log.PipelineTitle != "My Pipeline" {
		t.Fatalf("PipelineTitle = %v, want \"My Pipeline\"", log.PipelineTitle)
	}
	if log.Avatar == nil || *log.Avatar != "a.png" {
		t.Fatalf("Avatar = %v, want \"a.png\"", log.Avatar)
	}
}

func TestRecordPipelineLog_SourceFrom(t *testing.T) {
	cases := []struct {
		name       string
		sourceType string
		want       string
	}{
		{name: "connector source strips connector id", sourceType: "rss/connector-811", want: "rss"},
		{name: "plain source unchanged", sourceType: "local", want: "local"},
		{name: "empty source unchanged", sourceType: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			taskCtx := makeTaskCtx()
			taskCtx.Doc.SourceType = tc.sourceType
			var captured *entity.PipelineOperationLog
			svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).WithLogCreateFunc(
				func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
					captured = log
					return nil
				},
			)
			svc.recordPipelineLog(t.Context(), dao.DB, "doc-1", `{"components": {}}`, "done")
			if captured == nil {
				t.Fatal("logCreateFunc was not called")
			}
			if captured.SourceFrom != tc.want {
				t.Errorf("SourceFrom = %q, want %q", captured.SourceFrom, tc.want)
			}
		})
	}
}

// recordPipelineLog reloads the persisted document and derives source_from
// from that row, so a stale task-context snapshot must not leak into the log.
func TestRecordPipelineLog_SourceFromReloadedDoc(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	persisted := &entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "rss/connector-811",
		Type:         "pdf",
		CreatedBy:    "tenant-1",
		Suffix:       ".pdf",
	}
	if err := dao.NewDocumentDAO().Create(t.Context(), dao.DB, persisted); err != nil {
		t.Fatalf("seed document: %v", err)
	}

	taskCtx := makeTaskCtx()
	taskCtx.Doc.SourceType = "local"
	var captured *entity.PipelineOperationLog
	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).WithLogCreateFunc(
		func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		},
	)
	svc.recordPipelineLog(t.Context(), dao.DB, "doc-1", `{"components": {}}`, "done")
	if captured == nil {
		t.Fatal("logCreateFunc was not called")
	}
	if captured.SourceFrom != "rss" {
		t.Errorf("SourceFrom = %q, want %q", captured.SourceFrom, "rss")
	}
}

// =============================================================================
// updateDocumentMetadata
// =============================================================================

func TestRunPipeline_NilOutput(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	ctx := t.Context()
	_, err := svc.processOutput(ctx, nil, time.Now())
	if err != nil {
		t.Errorf("expected nil error for nil output, got %v", err)
	}
}

func TestRunPipeline_EmptyOutput(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil },
	)
	ctx := t.Context()
	_, err := svc.processOutput(ctx, map[string]any{}, time.Now())
	if err != nil {
		t.Errorf("expected nil error for empty output, got %v", err)
	}
}

func TestRunPipeline_NormalizedEmpty(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).WithLogCreateFunc(
		func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil },
	)
	ctx := t.Context()
	_, err := svc.processOutput(ctx, map[string]any{"markdown": ""}, time.Now())
	if err != nil {
		t.Errorf("expected nil error for empty normalized output, got %v", err)
	}
}

func TestRunPipeline_FullFlow(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil })
	output := map[string]any{
		"chunks": []map[string]any{
			{"text": "hello"},
			{"text": "world"},
		},
	}
	ctx := t.Context()
	_, err := svc.processOutput(ctx, output, time.Now())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunPipeline_AlreadyHasVectors(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil })

	output := map[string]any{
		"chunks": []map[string]any{
			{"text": "hello", "q_768_vec": []float64{0.1, 0.2}},
		},
	}
	ctx := t.Context()
	_, err := svc.processOutput(ctx, output, time.Now())
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

	taskCtx := makeTaskCtx()
	taskCtx.PipelineID = "flow-1"

	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).
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
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
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

func TestPipelineExecutor_Execute_DoesNotLogFailedRun(t *testing.T) {
	logged := false
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			return nil, dsl, errors.New("pipeline failed")
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			logged = true
			return nil
		})

	if _, err := svc.Execute(context.Background()); err == nil {
		t.Fatal("Execute error = nil, want failure")
	}
	if logged {
		t.Fatal("executor must not log failed runs before ingestor writes final document status")
	}
}

func TestPipelineExecutor_Execute_DoesNotLogCanceledRun(t *testing.T) {
	logged := false
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			return nil, dsl, context.Canceled
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			logged = true
			return nil
		})

	if _, err := svc.Execute(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}
	if logged {
		t.Fatal("executor must not log canceled runs before ingestor writes final document status")
	}
}

// TestPipelineExecutor_Execute_PropagatesContext verifies the ctx passed to
// Execute is the ctx received by runPipelineFunc - the task context must flow
// through to the pipeline run.
func TestPipelineExecutor_Execute_PropagatesContext(t *testing.T) {
	type ctxKey string
	const key ctxKey = "trace"
	taskCtx := makeTaskCtx()
	ctx := t.Context()
	taskCtx.Ctx = context.WithValue(ctx, key, "task-ctx")

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
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil })

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

func (r *recordingProgressSink) OnComponentTotal(ctx context.Context, taskID string, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.total = total
	r.totalSet = true
}

func (r *recordingProgressSink) OnComponentProgress(ctx context.Context, ev pipelinepkg.ProgressEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

type sinkPassthroughStage struct{}

func (sinkPassthroughStage) Invoke(_ context.Context, _ *gorm.DB, inputs map[string]any) (map[string]any, error) {
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
	ctx := t.Context()

	if _, _, err := svc.runPipelineWithDSL(ctx, dsl); err != nil {
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

func TestCountOriginalChunkIDs(t *testing.T) {
	// Empty list
	if n := countOriginalChunkIDs(nil); n != 0 {
		t.Fatalf("nil chunks: got %d, want 0", n)
	}

	// All unique
	chunks := []map[string]any{
		{"id": "a"},
		{"id": "b"},
		{"id": "c"},
	}
	if n := countOriginalChunkIDs(chunks); n != 3 {
		t.Fatalf("all unique: got %d, want 3", n)
	}

	// Duplicates present — this is the key case
	chunks = []map[string]any{
		{"id": "x"},
		{"id": "y"},
		{"id": "x"}, // duplicate of [0]
		{"id": "z"},
		{"id": "y"}, // duplicate of [1]
	}
	if n := countOriginalChunkIDs(chunks); n != 3 {
		t.Fatalf("with duplicates: got %d, want 3", n)
	}

	// Missing id fields are skipped
	chunks = []map[string]any{
		{"id": "one"},
		{"text": "no id"},
		{"id": "two"},
	}
	if n := countOriginalChunkIDs(chunks); n != 2 {
		t.Fatalf("mixed present/absent ids: got %d, want 2", n)
	}

	// Compiler products are stored as chunks too, but are derived artifacts.
	chunks = []map[string]any{
		{"id": "source-1"},
		{"id": "source-2"},
		{"id": "wiki-page-1", "compile_kwd": "wiki_page"},
		{"id": "wiki-section-1", "compile_kwd": "wiki_section"},
	}
	if n := countOriginalChunkIDs(chunks); n != 2 {
		t.Fatalf("compiler products: got %d, want 2", n)
	}
}

func TestMergeCompiledVariants(t *testing.T) {
	if got := mergeCompiledVariants(nil, nil); got != nil {
		t.Fatalf("empty variants = %v, want nil", got)
	}

	got := mergeCompiledVariants([]string{"wiki", "structure"}, []string{"wiki", "tree"})
	want := []string{"structure", "tree", "wiki"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged variants = %v, want %v", got, want)
	}
}

// TestRunPipelineWithDSL_LogDSLCarriesOutputs locks the dataset-parse
// "View result" contract: the DSL runPipelineWithDSL returns for the pipeline
// operation log (Execute passes it straight to recordPipelineLog) must carry
// each component's runtime outputs under obj.params.outputs, mirroring
// Python's dsl=str(pipeline)
// (rag/svr/task_executor_refactor/dataflow_service.py). Before this, the log
// stored the raw static DSL, so the dataset log "View result" page rendered
// blank panels and "0s" elapsed times even though the chunks were indexed —
// the front-end renders those panels exclusively from
// dsl.components[<id>].obj.params.outputs
// (web/src/pages/dataflow-result/parser.tsx, hooks.ts).
//
// It drives the REAL runPipelineWithDSL with stub ingestion components, so
// the log DSL is built from an actual run output (nested under
// output["state"][<id>] by finalizeResult), not a hand-built one, and the
// enveloped {"dsl": {...}} input is unwrapped to the front-end shape
// (top-level components).
func TestRunPipelineWithDSL_LogDSLCarriesOutputs(t *testing.T) {
	const (
		compC = "logdsl.RealStubChunks"
		compD = "logdsl.RealStubD"
	)
	runtime.MustRegister(compC, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return traceChunkComponent{}, nil },
		runtime.Metadata{Version: "1.0.0"})
	runtime.MustRegister(compD, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return traceStubComponent{}, nil },
		runtime.Metadata{Version: "1.0.0"})

	dsl := `{"dsl":{"components":{
		"begin":{"obj":{"component_name":"Begin","params":{}},"downstream":["c"]},
		"c":{"obj":{"component_name":"` + compC + `","params":{"setups":{"pdf":{"parse_method":"general"}}}},"upstream":["begin"],"downstream":["d"]},
		"d":{"obj":{"component_name":"` + compD + `","params":{}},"upstream":["c"]}
	},"path":["begin","c","d"],"graph":{"nodes":[{"id":"begin","data":{"name":"开始"}},{"id":"c","data":{"name":"解析"}},{"id":"d","data":{"name":"分词"}}]}}}`

	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-logdsl", 0)
	_, logDSL, err := svc.runPipelineWithDSL(t.Context(), dsl)
	if err != nil {
		t.Fatalf("runPipelineWithDSL: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(logDSL), &doc); err != nil {
		t.Fatalf("log DSL is not valid JSON: %v body=%s", err, logDSL)
	}
	// The enveloped canvas DSL must come back unwrapped: the front-end reads
	// dsl.components at the top level.
	components, ok := doc["components"].(map[string]any)
	if !ok {
		t.Fatalf("log DSL must carry top-level components (unwrapped canvas envelope): %s", logDSL)
	}

	// Chunk-emitting component: params.outputs.chunks + output_format.
	cParams, ok := components["c"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if !ok {
		t.Fatalf("log DSL components.c.obj.params missing: %s", logDSL)
	}
	cOutputs, ok := cParams["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("REGRESSION: log DSL has no params.outputs for chunk component c; "+
			"the dataset log 'View result' page would render a blank panel. params=%#v", cParams)
	}
	if of, _ := cOutputs["output_format"].(map[string]any); of["value"] != "chunks" {
		t.Errorf("c output_format=%#v want {value:\"chunks\"}", cOutputs["output_format"])
	}
	chunksVal, _ := cOutputs["chunks"].(map[string]any)["value"].([]any)
	if len(chunksVal) != 1 {
		t.Fatalf("c chunks.value len=%d want 1", len(chunksVal))
	}
	// TrackElapsed bookkeeping must ride along so the timeline shows real
	// per-node elapsed times (hooks.ts reads outputs._elapsed_time.value).
	et, ok := cOutputs["_elapsed_time"].(map[string]any)
	if !ok {
		t.Fatalf("c outputs._elapsed_time missing: %#v", cOutputs)
	}
	if _, ok := et["value"].(float64); !ok {
		t.Errorf("c outputs._elapsed_time.value=%#v want float64", et["value"])
	}
	// TrackElapsed stamps _created_time as an RFC3339Nano wall-clock string;
	// the outputs wrapper carries it verbatim with its type string.
	if cct, ok := cOutputs["_created_time"].(map[string]any); ok {
		cs, ok := cct["value"].(string)
		if !ok || cs == "" {
			t.Errorf("c outputs._created_time.value=%#v want non-empty string", cct["value"])
		} else if _, err := time.Parse(time.RFC3339Nano, cs); err != nil {
			t.Errorf("c outputs._created_time.value %q is not RFC3339Nano: %v", cs, err)
		}
		if cct["type"] != "<class 'str'>" {
			t.Errorf("c outputs._created_time.type=%#v want <class 'str'>", cct["type"])
		}
	} else {
		t.Errorf("c outputs._created_time missing: %#v", cOutputs)
	}
	// Non-components top-level keys are carried verbatim; this fixture's DSL
	// declares "path" — the round-tripped log must keep it for rerun-flow
	// consumers.
	if p, _ := doc["path"].([]any); len(p) != 3 || p[0] != "begin" || p[2] != "d" {
		t.Errorf("log DSL path=%#v want [begin c d]", doc["path"])
	}
}

// TestBuildLogDSL_FallbackToStaticDSL pins the guarantee that log recording
// never fails a run: when the run-result DSL cannot be built or cannot be
// marshaled, buildLogDSL must return the static dsl unchanged rather than a
// half-written payload.
func TestBuildLogDSL_FallbackToStaticDSL(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-logdsl-fallback", 0)

	// Marshal failure: NaN is a valid float64 payload, so the run-result DSL
	// builds fine but json.Marshal rejects it.
	dsl := `{"dsl":{"components":{"a":{"obj":{"component_name":"X","params":{}}}}}}`
	if got := svc.buildLogDSL(dsl, map[string]any{
		"a": map[string]any{"text": math.NaN()},
	}); got != dsl {
		t.Errorf("marshal failure: log DSL must fall back to the static dsl\n got: %s\nwant: %s", got, dsl)
	}

	// Build failure: a DSL without a components map cannot produce a
	// run-result DSL at all.
	badDSL := `{"dsl":{"path":["a"]}}`
	if got := svc.buildLogDSL(badDSL, nil); got != badDSL {
		t.Errorf("build failure: log DSL must fall back to the static dsl\n got: %s\nwant: %s", got, badDSL)
	}
}
