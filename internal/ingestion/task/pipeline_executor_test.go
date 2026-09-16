package task

import (
	"context"
	"encoding/json"
	"errors"
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
	documentpkg "ragflow/internal/service/document"
)

// =============================================================================
// Test helpers
// =============================================================================

func strPtr(s string) *string { return &s }

// TestMarkCompiledProductsHidden verifies the pipeline caller stages ONLY the
// wiki variant's per-document rows as available_int=0 (they are intermediate
// state for the consumer's merged pages). Tree / structure (page_index) /
// mindmap rows are the final per-document products — Python writes them visible
// at compile time — so they keep the index default available_int=1, as do
// ordinary source chunks.
func TestMarkCompiledProductsHidden(t *testing.T) {
	chunks := []map[string]any{
		{"id": "src-1", "content_with_weight": "ordinary source chunk"},
		{"id": "tree-1", "compile_kwd": "tree", "content_with_weight": "tree node"},
		{"id": "struct-1", "compile_kwd": "page_index", "content_with_weight": "entity A"},
		{"id": "wiki-1", "compile_kwd": "wiki_page", "content_with_weight": "page X"},
		{"id": "wiki-2", "compile_kwd": "wiki_section", "content_with_weight": "section X"},
		{"id": "src-2", "content_with_weight": "another source chunk"},
	}
	markCompiledProductsHidden(chunks)

	if v, ok := chunks[0]["available_int"]; ok {
		t.Fatalf("ordinary source chunk should keep default available_int, got %v", v)
	}
	if v, ok := chunks[1]["available_int"]; ok {
		t.Fatalf("tree row should stay visible (Python parity), got available_int=%v", v)
	}
	if v, ok := chunks[2]["available_int"]; ok {
		t.Fatalf("page_index row should stay visible (Python parity), got available_int=%v", v)
	}
	if chunks[3]["available_int"] != 0 {
		t.Fatalf("wiki page staging row should be available_int=0, got %v", chunks[3]["available_int"])
	}
	if chunks[4]["available_int"] != 0 {
		t.Fatalf("wiki section staging row should be available_int=0, got %v", chunks[4]["available_int"])
	}
	if v, ok := chunks[5]["available_int"]; ok {
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
// every row — source chunks AND compiled products — to available_int=0
// (matching Python's doc_id-scoped availability toggle). Wiki staging rows are
// already 0 from markCompiledProductsHidden; the stamp is a no-op for them.
func TestApplyDocumentAvailability(t *testing.T) {
	chunks := []map[string]any{
		{"id": "src-1", "content_with_weight": "ordinary source chunk"},
		{"id": "tree-1", "compile_kwd": "tree", "content_with_weight": "tree node"},
		{"id": "wiki-1", "compile_kwd": "wiki_page", "content_with_weight": "page X", "available_int": 0},
		{"id": "src-2", "content_with_weight": "another source chunk"},
	}
	markCompiledProductsHidden(chunks)
	applyDocumentAvailability(chunks, strPtr("0"))

	if chunks[0]["available_int"] != 0 {
		t.Fatalf("disabled doc source chunk should be available_int=0, got %v", chunks[0]["available_int"])
	}
	if chunks[1]["available_int"] != 0 {
		t.Fatalf("disabled doc compiled row should be available_int=0, got %v", chunks[1]["available_int"])
	}
	if chunks[2]["available_int"] != 0 {
		t.Fatalf("wiki staging row should stay available_int=0, got %v", chunks[2]["available_int"])
	}
	if chunks[3]["available_int"] != 0 {
		t.Fatalf("disabled doc source chunk should be available_int=0, got %v", chunks[3]["available_int"])
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
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.PipelineOperationLog{}, &entity.Document{}, &entity.IngestionTask{}, &entity.Knowledgebase{}); err != nil {
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
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		PipelineID:   strPtr("canvas-1"),
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Name:         &docName,
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

func TestRecordPipelineLog_ReusesOpenPreTerminalRow(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	queuedMsg := "Task is queued..."
	openLog := &entity.PipelineOperationLog{
		ID:              "open-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		DocumentName:    "test-doc.pdf",
		DocumentSuffix:  ".pdf",
		DocumentType:    "pdf",
		SourceFrom:      "local",
		TaskType:        "Parse",
		OperationStatus: "5",
		ProgressMsg:     &queuedMsg,
	}
	if err := dao.DB.Create(openLog).Error; err != nil {
		t.Fatalf("seed open log: %v", err)
	}

	progress := 1.0
	finalMsg := "Parser Done"
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "tenant-1",
		Name:         strPtr("test-doc.pdf"),
		Suffix:       ".pdf",
		Progress:     progress,
		ProgressMsg:  &finalMsg,
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
		TenantID:      "tenant-1",
		KbID:          "kb-1",
		DocumentID:    "doc-1",
		Status:        "3",
		PipelineLogID: "open-log",
	}); err != nil {
		t.Fatalf("RecordPipelineLog: %v", err)
	}

	var count int64
	if err := dao.DB.Model(&entity.PipelineOperationLog{}).Where("document_id = ?", "doc-1").Count(&count).Error; err != nil {
		t.Fatalf("count pipeline logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("pipeline log rows = %d, want 1 (terminal must reuse the bound queued row)", count)
	}
	var log entity.PipelineOperationLog
	if err := dao.DB.First(&log, "id = ?", "open-log").Error; err != nil {
		t.Fatalf("load open log: %v", err)
	}
	if log.OperationStatus != "3" {
		t.Fatalf("OperationStatus = %q, want terminal status", log.OperationStatus)
	}
	if log.Progress != progress {
		t.Fatalf("Progress = %v, want %v", log.Progress, progress)
	}
	if log.ProgressMsg == nil || *log.ProgressMsg != finalMsg {
		t.Fatalf("ProgressMsg = %v, want final message", log.ProgressMsg)
	}
	if len(log.DSL) != 0 {
		t.Fatalf("DSL = %v, want empty object for terminal writer without DSL", log.DSL)
	}
}

// TestRecordPipelineLog_KeepsEarlyTimestampWhenDocumentHasNone locks the
// timestamp handover: the terminal snapshot must not blank the start time the
// queued row was opened with when the document carries none (a run that never
// reached the progress sink).
func TestRecordPipelineLog_KeepsEarlyTimestampWhenDocumentHasNone(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	openedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	queuedMsg := "Task is queued..."
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              "open-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: string(entity.TaskStatusRunning),
		ProgressMsg:     &queuedMsg,
		ProcessBeginAt:  &openedAt,
	}).Error; err != nil {
		t.Fatalf("seed open log: %v", err)
	}

	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "tenant-1",
		Suffix:       ".pdf",
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
		TenantID:      "tenant-1",
		KbID:          "kb-1",
		DocumentID:    "doc-1",
		Status:        "4",
		PipelineLogID: "open-log",
	}); err != nil {
		t.Fatalf("RecordPipelineLog: %v", err)
	}

	var log entity.PipelineOperationLog
	if err := dao.DB.First(&log, "id = ?", "open-log").Error; err != nil {
		t.Fatalf("load open log: %v", err)
	}
	if log.OperationStatus != "4" {
		t.Fatalf("OperationStatus = %q, want terminal status", log.OperationStatus)
	}
	if log.ProcessBeginAt == nil || !log.ProcessBeginAt.Equal(openedAt) {
		t.Fatalf("ProcessBeginAt = %v, want the queued row's %v", log.ProcessBeginAt, openedAt)
	}
	if log.SourceFrom != "local" || log.DocumentSuffix != ".pdf" || log.DocumentType != "pdf" {
		t.Fatalf("terminal write did not refresh document metadata: source_from=%q suffix=%q type=%q",
			log.SourceFrom, log.DocumentSuffix, log.DocumentType)
	}
}

// TestRecordPipelineLog_DoesNotAdoptAnotherRunsRow locks the run-isolation
// contract: a terminal write is bound to the row its own run opened, so a late
// write from a superseded run (whose row was replaced) can neither finalize nor
// even touch an open row belonging to another run of the same document.
func TestRecordPipelineLog_DoesNotAdoptAnotherRunsRow(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	// An open row for the document that belongs to a different run: either the
	// superseded run's row (cleanup failed) or the replacement run's fresh
	// queued row. It must survive the write below untouched.
	otherMsg := "Task is queued..."
	other := &entity.PipelineOperationLog{
		ID:              "other-run-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: "5",
		ProgressMsg:     &otherMsg,
	}
	if err := dao.DB.Create(other).Error; err != nil {
		t.Fatalf("seed other run's log: %v", err)
	}

	finalMsg := "Parser Done"
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "tenant-1",
		Suffix:       ".pdf",
		ProgressMsg:  &finalMsg,
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	// This run's own row was deleted along with the superseded task.
	if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
		TenantID:      "tenant-1",
		KbID:          "kb-1",
		DocumentID:    "doc-1",
		Status:        "3",
		PipelineLogID: "superseded-log",
	}); err != nil {
		t.Fatalf("RecordPipelineLog: %v", err)
	}

	var reloaded entity.PipelineOperationLog
	if err := dao.DB.First(&reloaded, "id = ?", "other-run-log").Error; err != nil {
		t.Fatalf("reload other run's log: %v", err)
	}
	if reloaded.OperationStatus != "5" {
		t.Fatalf("other run's row OperationStatus = %q, want %q (must not be touched)", reloaded.OperationStatus, "5")
	}
	var count int64
	if err := dao.DB.Model(&entity.PipelineOperationLog{}).Where("document_id = ?", "doc-1").Count(&count).Error; err != nil {
		t.Fatalf("count pipeline logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("pipeline log rows = %d, want 1 (a bound missing row must not create a duplicate)", count)
	}
}

// TestRecordPipelineLog_UnboundAdoptsOpenRow keeps the legacy contract for
// callers that carry no bound row (debug-adjacent and non-ingestion paths): the
// document's newest open row is adopted so a stray queued entry is closed
// rather than left open.
func TestRecordPipelineLog_UnboundAdoptsOpenRow(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	openMsg := "Task is queued..."
	open := &entity.PipelineOperationLog{
		ID:              "open-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: "5",
		ProgressMsg:     &openMsg,
	}
	if err := dao.DB.Create(open).Error; err != nil {
		t.Fatalf("seed open log: %v", err)
	}

	finalMsg := "Parser Done"
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "tenant-1",
		Suffix:       ".pdf",
		ProgressMsg:  &finalMsg,
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
		TenantID:   "tenant-1",
		KbID:       "kb-1",
		DocumentID: "doc-1",
		Status:     "3",
	}); err != nil {
		t.Fatalf("RecordPipelineLog: %v", err)
	}

	var reloaded entity.PipelineOperationLog
	if err := dao.DB.First(&reloaded, "id = ?", "open-log").Error; err != nil {
		t.Fatalf("reload open log: %v", err)
	}
	if reloaded.OperationStatus != "3" {
		t.Fatalf("open row OperationStatus = %q, want %q (adopted by the unbound writer)", reloaded.OperationStatus, "3")
	}
	var count int64
	if err := dao.DB.Model(&entity.PipelineOperationLog{}).Where("document_id = ?", "doc-1").Count(&count).Error; err != nil {
		t.Fatalf("count pipeline logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("pipeline log rows = %d, want 1 (adopted, not duplicated)", count)
	}
}

// TestRecordPipelineLog_DoesNotAdoptLiveRunsOpenRow locks the ownership rule on
// the unbound fallback: the document's newest open row belongs to a live run, so
// another run's terminal write must not finalize it. Adopting it would stamp the
// live run's entry with a foreign status and swallow that run's own terminal
// write (its CAS would find the row already closed).
func TestRecordPipelineLog_DoesNotAdoptLiveRunsOpenRow(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	liveMsg := "Task is queued..."
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              "live-run-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: "5",
		ProgressMsg:     &liveMsg,
	}).Error; err != nil {
		t.Fatalf("seed live run's open log: %v", err)
	}
	if err := dao.DB.Create(&entity.IngestionTask{
		ID:         "live-task",
		UserID:     "user-1",
		DocumentID: "doc-1",
		DatasetID:  "kb-1",
		Status:     "RUNNING",
	}).Error; err != nil {
		t.Fatalf("seed live task: %v", err)
	}
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "live-task").
		Update("pipeline_log_id", "live-run-log").Error; err != nil {
		t.Fatalf("bind live task: %v", err)
	}

	finalMsg := "Parser Done"
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "tenant-1",
		Suffix:       ".pdf",
		ProgressMsg:  &finalMsg,
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
		TenantID:   "tenant-1",
		KbID:       "kb-1",
		DocumentID: "doc-1",
		Status:     "4",
	}); err != nil {
		t.Fatalf("RecordPipelineLog: %v", err)
	}

	var live entity.PipelineOperationLog
	if err := dao.DB.First(&live, "id = ?", "live-run-log").Error; err != nil {
		t.Fatalf("load live run's log: %v", err)
	}
	if live.OperationStatus != "5" {
		t.Fatalf("live run's row was adopted: OperationStatus = %q, want it untouched at %q", live.OperationStatus, "5")
	}
	var count int64
	if err := dao.DB.Model(&entity.PipelineOperationLog{}).Where("document_id = ?", "doc-1").Count(&count).Error; err != nil {
		t.Fatalf("count pipeline logs: %v", err)
	}
	if count != 2 {
		t.Fatalf("pipeline log rows = %d, want 2 (the unbound run records its own entry)", count)
	}
}

func TestRecordPipelineLog_CreatesRowWithoutOpenPreTerminalRow(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	docName := "legacy.pdf"
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "tenant-1",
		Name:         &docName,
		Suffix:       ".pdf",
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	taskCtx := makeTaskCtx()
	taskCtx.Doc.ParserID = "naive"
	var captured *entity.PipelineOperationLog
	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).WithLogCreateFunc(
		func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			captured = log
			return nil
		},
	)
	svc.recordPipelineLog(t.Context(), dao.DB, "doc-1", `{"components": {}}`, "done")
	if captured == nil {
		t.Fatal("logCreateFunc was not called: expected Create fallback without an open row")
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

func TestInjectTableColumnOverride_DoesNotInventDocumentColumns(t *testing.T) {
	docConfig := map[string]interface{}{}
	dsl := []byte(`{"components":{"Parser:Table":{"obj":{"component_name":"Parser","params":{}}}}}`)

	got := injectTableColumnOverride(docConfig, dsl)
	if _, exists := got["Parser:Table"]; exists {
		t.Fatalf("dataset table columns must not be injected into a document: %#v", got)
	}
}

func TestInjectTableColumnOverride_DocumentConfigOverridesPipelineDefaults(t *testing.T) {
	docConfig := map[string]interface{}{
		"table_column_mode":  "manual",
		"table_column_names": []interface{}{"Name"},
		"table_column_roles": map[string]interface{}{"Name": "metadata"},
		"Parser:Table": map[string]interface{}{
			"spreadsheet": map[string]interface{}{
				"column_mode":  "auto",
				"column_names": []interface{}{},
				"column_roles": map[string]interface{}{},
			},
		},
	}
	dsl := []byte(`{"components":{"Parser:Table":{"obj":{"component_name":"Parser","params":{}}}}}`)

	got := injectTableColumnOverride(docConfig, dsl)
	spreadsheet := got["Parser:Table"].(map[string]interface{})["spreadsheet"].(map[string]interface{})
	if spreadsheet["column_mode"] != "manual" {
		t.Fatalf("column_mode = %#v, want manual", spreadsheet["column_mode"])
	}
	wantRoles := map[string]interface{}{"Name": "metadata"}
	if !reflect.DeepEqual(spreadsheet["column_roles"], wantRoles) {
		t.Fatalf("column_roles = %#v, want %#v", spreadsheet["column_roles"], wantRoles)
	}
}

func TestSyncTableColumnNames_PersistsOnlyOnDocument(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()
	if err := dao.DB.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatalf("migrate knowledgebase: %v", err)
	}
	kb := &entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "tenant-1",
		Name:         "kb-1",
		ParserConfig: entity.JSONMap{"dataset_setting": "preserved"},
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("seed knowledgebase: %v", err)
	}
	doc := &entity.Document{
		ID:           "doc-1",
		KbID:         kb.ID,
		ParserID:     "table",
		ParserConfig: entity.JSONMap{"table_column_mode": "manual", "table_column_roles": map[string]interface{}{"Name": "metadata", "Stale": "both"}},
		CreatedBy:    "tenant-1",
		Type:         "csv",
		Suffix:       "csv",
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	if err := documentpkg.NewDocumentService().SaveDocumentTableColumns(t.Context(), doc.ID, []string{"Name", "City"}); err != nil {
		t.Fatalf("sync discovered columns: %v", err)
	}

	persistedDoc, err := dao.NewDocumentDAO().GetByID(t.Context(), dao.DB, doc.ID)
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	wantNames := []interface{}{"Name", "City"}
	if !reflect.DeepEqual(persistedDoc.ParserConfig["table_column_names"], wantNames) {
		t.Fatalf("document column names = %#v, want %#v", persistedDoc.ParserConfig["table_column_names"], wantNames)
	}
	wantRoles := map[string]interface{}{"Name": "metadata"}
	if !reflect.DeepEqual(persistedDoc.ParserConfig["table_column_roles"], wantRoles) {
		t.Fatalf("document column roles = %#v, want %#v", persistedDoc.ParserConfig["table_column_roles"], wantRoles)
	}

	persistedKB, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), dao.DB, kb.ID)
	if err != nil {
		t.Fatalf("load knowledgebase: %v", err)
	}
	if !reflect.DeepEqual(persistedKB.ParserConfig, entity.JSONMap{"dataset_setting": "preserved"}) {
		t.Fatalf("dataset parser config was mutated: %#v", persistedKB.ParserConfig)
	}
}

func TestProcessOutput_PersistsDiscoveredColumnsFromPayload(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()
	if err := dao.DB.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatalf("migrate knowledgebase: %v", err)
	}
	if err := dao.DB.Create(&entity.Knowledgebase{ID: "kb-1", TenantID: "tenant-1", Name: "kb-1"}).Error; err != nil {
		t.Fatalf("seed knowledgebase: %v", err)
	}
	name := "table.csv"
	doc := &entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "table",
		ParserConfig: entity.JSONMap{"table_column_mode": "auto"},
		CreatedBy:    "tenant-1",
		Type:         "csv",
		Suffix:       "csv",
		Name:         &name,
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}
	taskCtx := makeTaskCtx()
	taskCtx.Doc = *doc
	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil })
	output := map[string]any{
		"chunks": []map[string]any{{"text": "- Name: Alice"}},
		"file":   map[string]any{"name": "table.csv", "table_column_names": []string{"Name", "City"}},
	}
	res, err := svc.processOutput(t.Context(), output, time.Now())
	if err != nil {
		t.Fatalf("processOutput: %v", err)
	}
	wantNames := []string{"Name", "City"}
	if !reflect.DeepEqual(res.DiscoveredColumns, wantNames) {
		t.Fatalf("res.DiscoveredColumns = %#v, want %#v", res.DiscoveredColumns, wantNames)
	}
	if err := documentpkg.NewDocumentService().SaveDocumentTableColumns(t.Context(), doc.ID, res.DiscoveredColumns); err != nil {
		t.Fatalf("saveDocumentTableColumns: %v", err)
	}
	persisted, err := dao.NewDocumentDAO().GetByID(t.Context(), dao.DB, doc.ID)
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	gotNames, _ := persisted.ParserConfig["table_column_names"].([]interface{})
	if len(gotNames) != 2 {
		t.Fatalf("document column names = %#v, want [Name City]", persisted.ParserConfig["table_column_names"])
	}
}

func TestProcessOutput_StripsStaleTableMetadataOnReparse(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()
	name := "table.csv"
	doc := &entity.Document{
		ID:           "doc-1",
		KbID:         "kb-1",
		ParserID:     "table",
		ParserConfig: entity.JSONMap{"table_column_mode": "manual", "table_column_names": []interface{}{"Name", "Age"}},
		CreatedBy:    "tenant-1",
		Type:         "csv",
		Suffix:       "csv",
		Name:         &name,
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}
	taskCtx := makeTaskCtx()
	taskCtx.Doc = *doc
	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil })
	// Seed pre-existing metadata carrying a stale table column ("Age") that
	// the fresh chunks no longer emit. The strip pass must remove it even
	// though AggregateTableDocMetadata returns non-nil here (Name aggregates).
	output := map[string]any{
		"chunks": []map[string]any{{
			"text":       "- Name: Alice",
			"metadata":   map[string]any{"Age": []string{"30"}, "keep": "yes"},
			"chunk_data": map[string]interface{}{"Name": "Alice"},
		}},
	}
	res, err := svc.processOutput(t.Context(), output, time.Now())
	if err != nil {
		t.Fatalf("processOutput: %v", err)
	}
	if res == nil || res.Metadata == nil {
		t.Fatalf("expected metadata, got %+v", res)
	}
	if _, hasAge := res.Metadata["Age"]; hasAge {
		t.Errorf("stale Age key must be stripped when chunks no longer emit it: %v", res.Metadata)
	}
	if res.Metadata["keep"] != "yes" {
		t.Errorf("non-table metadata must survive the strip pass: %v", res.Metadata)
	}
	if len(res.StripKeys) == 0 {
		t.Errorf("expected StripKeys to be populated on reparse: %+v", res.StripKeys)
	}
}

func TestProcessOutput_SyncsFieldMapToKB(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	status := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "tenant-1",
		Name:         "test-kb",
		Status:       &status,
		CreatedBy:    "tenant-1",
		ParserConfig: entity.JSONMap{},
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("seed kb: %v", err)
	}

	name := "table.csv"
	doc := &entity.Document{
		ID:       "doc-1",
		KbID:     "kb-1",
		ParserID: "table",
		ParserConfig: entity.JSONMap{
			"table_column_mode": "manual",
			"table_column_roles": map[string]interface{}{
				"order_id":     "metadata",
				"product_name": "both",
				"internal_seq": "indexing",
			},
		},
		CreatedBy: "tenant-1",
		Type:      "csv",
		Suffix:    "csv",
		Name:      &name,
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	taskCtx := makeTaskCtx()
	taskCtx.Doc = *doc
	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error { return nil })

	output := map[string]any{
		"chunks": []map[string]any{{"text": "- product_name: Widget"}},
		"file": map[string]any{
			"name":               "table.csv",
			"table_column_names": []string{"order_id", "product_name", "internal_seq"},
		},
	}
	res, err := svc.processOutput(t.Context(), output, time.Now())
	if err != nil {
		t.Fatalf("processOutput: %v", err)
	}
	if len(res.StripKeys) == 0 {
		t.Errorf("expected StripKeys to be populated, got empty")
	}
	if res.FieldMapUpdates["order_id"] != "order id" {
		t.Errorf("expected order_id -> 'order id', got %v", res.FieldMapUpdates["order_id"])
	}
	if res.FieldMapUpdates["product_name"] != "product name" {
		t.Errorf("expected product_name -> 'product name', got %v", res.FieldMapUpdates["product_name"])
	}
	if _, ok := res.FieldMapUpdates["internal_seq"]; ok {
		t.Errorf("indexing-only column internal_seq must NOT be in FieldMapUpdates, got %v", res.FieldMapUpdates["internal_seq"])
	}

	if err := documentpkg.NewDocumentService().SaveKBTableFieldMap(t.Context(), kb.ID, res.FieldMapUpdates); err != nil {
		t.Fatalf("saveKBTableFieldMap: %v", err)
	}

	persistedKB, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), dao.DB, "kb-1")
	if err != nil {
		t.Fatalf("load kb: %v", err)
	}
	fm, ok := persistedKB.ParserConfig["field_map"].(map[string]interface{})
	if !ok {
		t.Fatalf("field_map not found in kb.ParserConfig: %#v", persistedKB.ParserConfig)
	}
	if fm["order_id"] != "order id" {
		t.Errorf("expected order_id -> 'order id', got %v", fm["order_id"])
	}
	if fm["product_name"] != "product name" {
		t.Errorf("expected product_name -> 'product name', got %v", fm["product_name"])
	}
	if _, ok := fm["internal_seq"]; ok {
		t.Errorf("indexing-only column internal_seq must NOT be in field_map, got %v", fm["internal_seq"])
	}
}

func TestMergeKBTableColumnFallback(t *testing.T) {
	doc := map[string]interface{}{"table_column_mode": "manual"}
	kb := map[string]interface{}{
		"table_column_mode":  "auto",
		"table_column_roles": map[string]interface{}{"Age": "metadata"},
		"table_column_names": []interface{}{"Name", "Age"},
	}
	got := mergeKBTableColumnFallback(doc, kb)
	if got["table_column_mode"] != "manual" {
		t.Errorf("doc mode must win, got %v", got["table_column_mode"])
	}
	if _, ok := got["table_column_roles"]; !ok {
		t.Errorf("absent roles must fall back to KB, got %v", got)
	}
	if _, ok := got["table_column_names"]; !ok {
		t.Errorf("absent names must fall back to KB, got %v", got)
	}
	if out := mergeKBTableColumnFallback(nil, nil); out != nil {
		t.Errorf("nil KB must return doc unchanged, got %v", out)
	}
}

func TestTableColumnNamesFromPayload(t *testing.T) {
	out := map[string]any{"file": map[string]any{"table_column_names": []interface{}{"A", " B ", 1}}}
	if got := tableColumnNamesFromPayload(out); len(got) != 2 || got[0] != "A" || got[1] != " B " {
		t.Errorf("got %q, want [A \" B \"]", got)
	}
	if got := tableColumnNamesFromPayload(map[string]any{}); len(got) != 0 {
		t.Errorf("missing file must yield nil, got %v", got)
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

func TestPipelineExecutor_Execute_RecordsDoneOperationStatus(t *testing.T) {
	taskCtx := makeTaskCtx()
	taskCtx.Ctx = t.Context()
	var capturedLog *entity.PipelineOperationLog

	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(runCtx context.Context, dsl string) (map[string]any, string, error) {
			return map[string]any{"chunks": []map[string]any{{"text": "hello world"}}}, dsl, nil
		}).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			capturedLog = log
			return nil
		})

	if _, err := svc.Execute(taskCtx.Ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLog == nil {
		t.Fatalf("expected pipeline operation log to be recorded")
	}
	if capturedLog.OperationStatus == "" {
		t.Fatalf("expected OperationStatus to be non-empty")
	}
	if capturedLog.OperationStatus != string(entity.TaskStatusDone) {
		t.Fatalf("expected OperationStatus = %q, got %q", entity.TaskStatusDone, capturedLog.OperationStatus)
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

// TestRunPipelineWithDSL_LogDSLStripsOutputs locks the "no business data in
// the pipeline operation log" contract: the DSL runPipelineWithDSL returns for
// the log (Execute passes it straight to recordPipelineLog) must NOT carry any
// component's runtime outputs under obj.params.outputs. The log keeps the DSL
// DEFINITION only (component structure, static params, downstream, graph,
// path) so a historical run can be reconstructed and the dataset "View result"
// / rerun UI no longer renders chunk business data. Dry-run previews still
// carry outputs via ResultSink; only the persisted log is kept output-free
// (buildLogDSL passes includeOutputs=false before recordPipelineLog).
//
// It drives the REAL runPipelineWithDSL with stub ingestion components, so
// the log DSL is built from an actual run output (nested under
// output["state"][<id>] by finalizeResult), not a hand-built one, and the
// enveloped {"dsl": {...}} input is unwrapped to the front-end shape
// (top-level components).
func TestRunPipelineWithDSL_LogDSLStripsOutputs(t *testing.T) {
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

	// Chunk-emitting component: the persisted log must carry the DSL DEFINITION
	// only — NO runtime outputs under obj.params.outputs (the "no business data
	// in the log" contract, Req 1). The dry-run preview (ResultSink/Redis) still
	// carries full outputs for the dataset "View result" page; the persisted row
	// must not. Static params survive so the rerun/canvas flow can reconstruct
	// the component.
	cParams, ok := components["c"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if !ok {
		t.Fatalf("log DSL components.c.obj.params missing: %s", logDSL)
	}
	if _, ok := cParams["outputs"]; ok {
		t.Errorf("REGRESSION: persisted log DSL must NOT carry business data (obj.params.outputs) "+
			"for chunk component c, got %#v", cParams["outputs"])
	}
	if _, ok := cParams["setups"]; !ok {
		t.Errorf("persisted log DSL must keep static params.setups for c, got %#v", cParams)
	}
	// Non-components top-level keys are carried verbatim; this fixture's DSL
	// declares "path" — the round-tripped log must keep it for rerun-flow
	// consumers.
	if p, _ := doc["path"].([]any); len(p) != 3 || p[0] != "begin" || p[2] != "d" {
		t.Errorf("log DSL path=%#v want [begin c d]", doc["path"])
	}
}

// TestBuildLogDSL_FallbackToStaticDSL pins the guarantee that log recording
// never fails a run: when the run-result DSL cannot be built (a malformed
// canvas), buildLogDSL must return the static dsl unchanged rather than a
// half-written payload. The input dsl is the canvas definition (no runtime
// outputs), so the fallback is not a business-data leak and the log row is
// preserved for observability (recordPipelineLog still receives a valid,
// definition-only DSL).
//
// Note: business-data payloads (e.g. NaN inside a chunk value) no longer reach
// the persisted copy at all — the persist path passes includeOutputs=false, so
// the outputs wrapper is never constructed and cannot break marshaling. The
// realistic fallback trigger is therefore a malformed DSL, tested below.
func TestBuildLogDSL_FallbackToStaticDSL(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-logdsl-fallback", 0)

	// Build failure: a DSL without a components map cannot produce a
	// run-result DSL at all. The static dsl is returned unchanged.
	badDSL := `{"dsl":{"path":["a"]}}`
	if got := svc.buildLogDSL(badDSL, nil); got != badDSL {
		t.Errorf("build failure: log DSL must fall back to the static dsl\n got: %s\nwant: %s", got, badDSL)
	}
}

// persistOnlySink simulates the real-parse (DB-backed) sink: it implements
// pipeline.ProgressSink but NOT task.ResultSink, so buildLogDSL must NOT call
// SetResult and the persisted DSL must carry no business data.
type persistOnlySink struct{}

func (persistOnlySink) OnComponentTotal(context.Context, string, int)                  {}
func (persistOnlySink) OnComponentProgress(context.Context, pipelinepkg.ProgressEvent) {}

// capturingSink simulates the dry-run (DebugLogSink) sink: it implements BOTH
// ProgressSink and ResultSink, so buildLogDSL hands it the full result DSL
// (business data) via SetResult for the Redis END marker.
type capturingSink struct {
	got    map[string]any
	gotOut map[string]any
}

func (c *capturingSink) OnComponentTotal(context.Context, string, int)                  {}
func (c *capturingSink) OnComponentProgress(context.Context, pipelinepkg.ProgressEvent) {}
func (c *capturingSink) SetResult(dsl map[string]any, output map[string]any) {
	c.got = dsl
	c.gotOut = output
}

// TestBuildLogDSL_PersistStripsOutputs locks the core contract of the change:
// the DSL string buildLogDSL returns for a NON-ResultSink (real-parse) sink
// carries the DSL definition only — no component has obj.params.outputs, while
// static params / graph / path survive. This is exactly what recordPipelineLog
// persists to pipeline_operation_log.
func TestBuildLogDSL_PersistStripsOutputs(t *testing.T) {
	const dsl = `{"components": {"a": {"obj": {"component_name": "my.Parser", "params": {"setups": {"pdf": {"parse_method": "general"}}}}, "downstream": ["b"]}}, "graph": {"nodes": [{"id": "a"}]}, "path": ["a"]}`
	output := map[string]any{
		"a": map[string]any{"chunks": []any{map[string]any{"text": "hello"}}, "_elapsed_time": 0.35},
	}

	exec := &PipelineExecutor{progressSink: persistOnlySink{}}
	logDSL := exec.buildLogDSL(dsl, output)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(logDSL), &parsed); err != nil {
		t.Fatalf("persisted log DSL must be valid JSON: %v (raw=%s)", err, logDSL)
	}
	comps, _ := parsed["components"].(map[string]any)
	a, _ := comps["a"].(map[string]any)
	aObj, _ := a["obj"].(map[string]any)
	aParams, _ := aObj["params"].(map[string]any)
	if _, ok := aParams["outputs"]; ok {
		t.Errorf("persisted log DSL must NOT carry business data (obj.params.outputs), got %#v", aParams["outputs"])
	}
	// DSL definition preserved.
	if _, ok := aParams["setups"]; !ok {
		t.Error("persisted log DSL must keep static params.setups")
	}
	if _, ok := parsed["graph"]; !ok {
		t.Error("persisted log DSL must keep graph")
	}
	if _, ok := parsed["path"]; !ok {
		t.Error("persisted log DSL must keep path")
	}
}

// TestBuildLogDSL_PreviewKeepsOutputs locks the dry-run (ResultSink) branch:
// buildLogDSL calls ResultSink.SetResult with the FULL result DSL (business
// data in obj.params.outputs), while the DSL it returns for persistence still
// carries no business data. Dry-run never persists (IsDebug() early-return),
// so the outputs only travel to Redis via SetResult.
func TestBuildLogDSL_PreviewKeepsOutputs(t *testing.T) {
	const dsl = `{"components": {"a": {"obj": {"component_name": "my.Parser", "params": {}}, "downstream": ["b"]}}}`
	output := map[string]any{
		"a": map[string]any{"chunks": []any{map[string]any{"text": "hello"}}, "_elapsed_time": 0.35},
	}

	cap := &capturingSink{}
	exec := &PipelineExecutor{progressSink: cap}
	logDSL := exec.buildLogDSL(dsl, output)

	// The ResultSink preview must carry the full outputs (business data).
	if cap.got == nil {
		t.Fatal("buildLogDSL must call ResultSink.SetResult for a dry-run (ResultSink) sink")
	}
	caps, _ := cap.got["components"].(map[string]any)
	ca, _ := caps["a"].(map[string]any)
	caObj, _ := ca["obj"].(map[string]any)
	caParams, _ := caObj["params"].(map[string]any)
	if _, ok := caParams["outputs"]; !ok {
		t.Error("dry-run preview DSL must carry obj.params.outputs (business data)")
	}

	// Persisted copy returned by buildLogDSL still must NOT carry business data.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(logDSL), &parsed); err != nil {
		t.Fatalf("persisted log DSL must be valid JSON: %v", err)
	}
	pc, _ := parsed["components"].(map[string]any)
	pa, _ := pc["a"].(map[string]any)
	paObj, _ := pa["obj"].(map[string]any)
	paParams, _ := paObj["params"].(map[string]any)
	if _, ok := paParams["outputs"]; ok {
		t.Error("persisted log DSL must NOT carry business data even when a ResultSink preview exists")
	}
}
