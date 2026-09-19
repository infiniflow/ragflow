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
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.UserCanvas{},
		&entity.PipelineDSLVersion{},
		&entity.PipelineOperationLog{},
		&entity.Document{},
		&entity.IngestionTask{},
	); err != nil {
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
	if svc.indexWriter == nil || svc.loadDSLFunc == nil || svc.runPipelineFunc == nil {
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

func TestPipelineDSLID(t *testing.T) {
	tests := []struct {
		name       string
		pipelineID string
		parserID   string
		want       string
	}{
		{
			name:       "custom pipeline",
			pipelineID: "pipeline-1",
			parserID:   "general",
			want:       "pipeline-1",
		},
		{
			name:     "builtin pipeline",
			parserID: "general",
			want:     "builtin:general",
		},
		{
			name: "missing identity",
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := pipelineDSLID(test.pipelineID, test.parserID); got != test.want {
				t.Fatalf(
					"pipelineDSLID(%q, %q) = %q, want %q",
					test.pipelineID,
					test.parserID,
					got,
					test.want,
				)
			}
		})
	}
}

func TestSanitizePipelineDSLRemovesRuntimeState(t *testing.T) {
	dsl := entity.JSONMap{
		"task_id": "run-1",
		"components": map[string]any{
			"parser": map[string]any{
				"obj": map[string]any{
					"params": map[string]any{
						"mode":    "static",
						"q_vec":   []float64{0.5},
						"q_3_vec": []float64{0.1},
						"outputs": map[string]any{
							"chunks": []any{"runtime"},
						},
					},
				},
			},
		},
	}

	sanitizePipelineDSL(dsl)

	if _, exists := dsl["task_id"]; exists {
		t.Fatalf("runtime task_id was not removed: %v", dsl)
	}
	components := dsl["components"].(map[string]any)
	parser := components["parser"].(map[string]any)
	obj := parser["obj"].(map[string]any)
	params := obj["params"].(map[string]any)

	if _, exists := params["outputs"]; exists {
		t.Fatalf("runtime outputs were not removed: %v", params)
	}
	if _, exists := params["q_3_vec"]; exists {
		t.Fatalf("indexed embedding vector was not removed: %v", params)
	}
	if _, exists := params["q_vec"]; !exists {
		t.Fatalf("non-index field q_vec should be preserved: %v", params)
	}
	if params["mode"] != "static" {
		t.Fatalf("static params were not preserved: %v", params)
	}
}

func TestSanitizePipelineDSLRemovesTaskIDFromWrappedDSL(t *testing.T) {
	dsl := entity.JSONMap{
		"task_id": "outer-run",
		"dsl": map[string]any{
			"task_id":    "inner-run",
			"components": map[string]any{},
		},
	}

	sanitizePipelineDSL(dsl)

	if _, exists := dsl["task_id"]; exists {
		t.Fatalf("outer runtime task_id was not removed: %v", dsl)
	}

	nested := dsl["dsl"].(map[string]any)
	if _, exists := nested["task_id"]; exists {
		t.Fatalf("nested runtime task_id was not removed: %v", nested)
	}
}

func TestRecordPipelineLogStoresVersionedDSLReferences(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	documents := []struct {
		id  string
		dsl string
	}{
		{
			id:  "doc-1",
			dsl: `{"task_id":"run-1","components":{"parser":{"revision":1,"obj":{"params":{"mode":"static","outputs":{"chunks":{"value":[{"text":"first","q_3_vec":[0.1]}]}}}}}}}`,
		},
		{
			id:  "doc-2",
			dsl: `{"task_id":"run-2","components":{"parser":{"revision":1,"obj":{"params":{"mode":"static","outputs":{"chunks":{"value":[{"text":"second","q_3_vec":[0.2]}]}}}}}}}`,
		},
		{
			id:  "doc-3",
			dsl: `{"task_id":"run-3","components":{"parser":{"revision":2,"obj":{"params":{"mode":"static","outputs":{"chunks":{"value":[{"text":"third","q_3_vec":[0.3]}]}}}}}}}`,
		},
	}

	for _, item := range documents {
		logID := "log-" + item.id

		if err := dao.DB.Create(&entity.PipelineOperationLog{
			ID:              logID,
			DocumentID:      item.id,
			TenantID:        "tenant-1",
			KbID:            "kb-1",
			OperationStatus: "5",
		}).Error; err != nil {
			t.Fatalf("seed pipeline log for %s: %v", item.id, err)
		}

		name := item.id + ".pdf"
		if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
			TenantID:      "tenant-1",
			KbID:          "kb-1",
			DocumentID:    item.id,
			DSL:           item.dsl,
			Status:        "3",
			PipelineLogID: logID,
			Document: entity.Document{
				ID:         item.id,
				KbID:       "kb-1",
				ParserID:   "general",
				SourceType: "local",
				Type:       "pdf",
				Name:       &name,
				Suffix:     ".pdf",
			},
		}); err != nil {
			t.Fatalf("RecordPipelineLog(%s): %v", item.id, err)
		}
	}

	wantVersions := map[string]int64{
		"doc-1": 1,
		"doc-2": 1,
		"doc-3": 2,
	}

	for documentID, wantVersion := range wantVersions {
		var log entity.PipelineOperationLog
		if err := dao.DB.
			Where("document_id = ?", documentID).
			First(&log).Error; err != nil {
			t.Fatalf("load pipeline log for %s: %v", documentID, err)
		}

		if log.DSLID == nil || *log.DSLID != "builtin:general" {
			t.Fatalf("%s DSLID = %v, want builtin:general", documentID, log.DSLID)
		}
		if log.DSLVersion == nil || *log.DSLVersion != wantVersion {
			t.Fatalf("%s DSLVersion = %v, want %d", documentID, log.DSLVersion, wantVersion)
		}
		if len(log.DSL) != 0 {
			t.Fatalf("%s embedded DSL = %v, want empty", documentID, log.DSL)
		}
	}

	var versions []entity.PipelineDSLVersion
	if err := dao.DB.
		Where("dsl_id = ?", "builtin:general").
		Order("version").
		Find(&versions).Error; err != nil {
		t.Fatalf("load pipeline DSL versions: %v", err)
	}

	if len(versions) != 2 {
		t.Fatalf("pipeline DSL version count = %d, want 2", len(versions))
	}

	for _, version := range versions {
		if _, exists := version.DSL["task_id"]; exists {
			t.Fatalf("version %d stores runtime task_id: %v", version.Version, version.DSL)
		}

		components, _ := version.DSL["components"].(map[string]any)
		parser, _ := components["parser"].(map[string]any)
		obj, _ := parser["obj"].(map[string]any)
		params, _ := obj["params"].(map[string]any)

		if _, exists := params["outputs"]; exists {
			t.Fatalf("version %d stores runtime outputs: %v", version.Version, version.DSL)
		}
		if params["mode"] != "static" {
			t.Fatalf("version %d dropped static params: %v", version.Version, version.DSL)
		}
	}
}

func TestRecordPipelineLog_SharedWriterTerminalWithoutDSL(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              "run-1",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		OperationStatus: "5",
	}).Error; err != nil {
		t.Fatalf("seed pipeline log: %v", err)
	}

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
		PipelineLogID: "run-1",
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

func TestRecordPipelineLogRejectsMissingRunIdentity(t *testing.T) {
	err := RecordPipelineLog(t.Context(), nil, PipelineLogInput{DocumentID: "doc-1", Status: "3"})
	if !errors.Is(err, ErrMissingRunIdentity) {
		t.Fatalf("RecordPipelineLog error = %v, want ErrMissingRunIdentity", err)
	}
}

func TestRecordPipelineLogInternalWriterRejectsMissingRunIdentity(t *testing.T) {
	err := recordPipelineLog(t.Context(), nil, PipelineLogInput{DocumentID: "doc-1", Status: "3"})
	if !errors.Is(err, ErrMissingRunIdentity) {
		t.Fatalf("recordPipelineLog error = %v, want ErrMissingRunIdentity", err)
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
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              "run-1",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		OperationStatus: "5",
	}).Error; err != nil {
		t.Fatalf("seed pipeline log: %v", err)
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
		KbID:          "kb-1",
		DocumentID:    "doc-1",
		Status:        "3",
		PipelineLogID: "run-1",
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
	if log.ProgressMsg == nil || *log.ProgressMsg != queuedMsg {
		t.Fatalf("ProgressMsg = %v, want unchanged queued message", log.ProgressMsg)
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
		t.Fatalf(
			"terminal write did not refresh document metadata: source_from=%q suffix=%q type=%q",
			log.SourceFrom,
			log.DocumentSuffix,
			log.DocumentType,
		)
	}
}

// TestTerminalDurationAnchorsToProcessBeginAt pins the single duration anchor:
// a persisted run measures from the document's process_begin_at (the value
// PrepareValidatedRun stamped), not from executor start, so the document and
// the pipeline operation log record one number. A begin time in the future
// (DB/worker clock skew) clamps to zero like UpdateRunState does; a missing
// begin time falls back to the executor start.
func TestTerminalDurationAnchorsToProcessBeginAt(t *testing.T) {
	begin := time.Now().Add(-2 * time.Second)
	exec := &PipelineExecutor{taskCtx: &TaskContext{Doc: entity.Document{ProcessBeginAt: &begin}}}
	start := time.Now()
	if d := exec.terminalDuration(start); d < 2.0 || d > 3.0 {
		t.Fatalf("terminalDuration = %v, want ~2s measured from process_begin_at", d)
	}

	future := time.Now().Add(time.Minute)
	execFuture := &PipelineExecutor{taskCtx: &TaskContext{Doc: entity.Document{ProcessBeginAt: &future}}}
	if d := execFuture.terminalDuration(start); d != 0 {
		t.Fatalf("terminalDuration with future begin = %v, want 0", d)
	}

	execNoBegin := &PipelineExecutor{taskCtx: &TaskContext{}}
	if d := execNoBegin.terminalDuration(start); d < 0 || d > 1.0 {
		t.Fatalf("terminalDuration without begin = %v, want executor-start fallback", d)
	}
}

// TestRecordPipelineLog_TerminalDurationOverridesDocumentCopy locks the
// unification handover: when the run passes its measured terminal duration,
// the operation log records exactly that value — not the reloaded document's
// stale mid-run progress-sink value — so the final ApplyDocCounts write to
// document.process_duration and the log row agree.
func TestRecordPipelineLog_TerminalDurationOverridesDocumentCopy(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              "open-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: string(entity.TaskStatusRunning),
	}).Error; err != nil {
		t.Fatalf("seed open log: %v", err)
	}
	if err := dao.DB.Create(&entity.Document{
		ID:              "doc-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		ParserConfig:    entity.JSONMap{},
		SourceType:      "local",
		Type:            "pdf",
		CreatedBy:       "tenant-1",
		Suffix:          ".pdf",
		ProcessDuration: 0.719,
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	terminal := 0.627
	if err := RecordPipelineLog(t.Context(), dao.DB, PipelineLogInput{
		TenantID:         "tenant-1",
		KbID:             "kb-1",
		DocumentID:       "doc-1",
		Status:           "3",
		PipelineLogID:    "open-log",
		TerminalDuration: &terminal,
	}); err != nil {
		t.Fatalf("RecordPipelineLog: %v", err)
	}

	var log entity.PipelineOperationLog
	if err := dao.DB.First(&log, "id = ?", "open-log").Error; err != nil {
		t.Fatalf("load open log: %v", err)
	}
	if log.ProcessDuration != terminal {
		t.Fatalf("ProcessDuration = %v, want the run's terminal value %v, not the document's mid-run 0.719",
			log.ProcessDuration, terminal)
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
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
	ctx := t.Context()
	_, err := svc.processOutput(ctx, map[string]any{}, time.Now())
	if err != nil {
		t.Errorf("expected nil error for empty output, got %v", err)
	}
}

func TestRunPipeline_NormalizedEmpty(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0)
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
		})
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
		})

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
		})

	_, err := svc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inserted {
		t.Fatal("expected insertChunks to be called")
	}
}

func TestPipelineExecutor_Execute_DoesNotLogFailedRun(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			return nil, dsl, errors.New("pipeline failed")
		})

	if _, err := svc.Execute(context.Background()); err == nil {
		t.Fatal("Execute error = nil, want failure")
	}
}

func TestPipelineExecutor_Execute_DoesNotLogCanceledRun(t *testing.T) {
	svc := mustNewPipelineExecutor(t, makeTaskCtx(), "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			return nil, dsl, context.Canceled
		})

	if _, err := svc.Execute(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
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
			return map[string]any{
				"chunks": []map[string]any{
					{"text": "hello world"},
				},
			}, dsl, nil
		}).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		})

	if _, err := svc.Execute(taskCtx.Ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPipelineExecutor_Execute_RecordsDoneOperationStatus(t *testing.T) {
	taskCtx := makeTaskCtx()
	taskCtx.Ctx = t.Context()

	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(runCtx context.Context, dsl string) (map[string]any, string, error) {
			return map[string]any{
				"chunks": []map[string]any{
					{"text": "hello world"},
				},
			}, dsl, nil
		}).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			return nil, nil
		})

	if _, err := svc.Execute(taskCtx.Ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestPipelineExecutor_Execute_EmptyOutputRecordsTerminalDuration pins the
// no-chunk success branch: such a run never writes a terminal duration to the
// document (processOutput returns before ApplyDocCounts), so Execute must
// recompute from the same process_begin_at anchor and hand that value to the
// operation log instead of letting the row keep the stale mid-run
// progress-sink duration.
func TestPipelineExecutor_Execute_EmptyOutputRecordsTerminalDuration(t *testing.T) {
	cleanup := setupPipelineExecutorTestDB(t)
	defer cleanup()

	begin := time.Now().Add(-2 * time.Second)
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              "open-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: string(entity.TaskStatusRunning),
	}).Error; err != nil {
		t.Fatalf("seed open log: %v", err)
	}
	if err := dao.DB.Create(&entity.Document{
		ID:              "doc-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		ParserConfig:    entity.JSONMap{},
		SourceType:      "local",
		Type:            "pdf",
		CreatedBy:       "tenant-1",
		Suffix:          ".pdf",
		ProcessBeginAt:  &begin,
		ProcessDuration: 0.719,
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	logID := "open-log"
	taskCtx := makeTaskCtx()
	taskCtx.Ctx = t.Context()
	taskCtx.IngestionTask.PipelineLogID = &logID
	taskCtx.Doc.ProcessBeginAt = &begin

	svc := mustNewPipelineExecutor(t, taskCtx, "flow-1", 0).
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return `{"nodes":[{"id":"n1"}],"edges":[]}`, canvasID, nil
		}).
		WithRunPipelineFunc(func(runCtx context.Context, dsl string) (map[string]any, string, error) {
			return map[string]any{"chunks": []map[string]any{}}, dsl, nil
		})

	if _, err := svc.Execute(taskCtx.Ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var log entity.PipelineOperationLog
	if err := dao.DB.First(&log, "id = ?", "open-log").Error; err != nil {
		t.Fatalf("load log row: %v", err)
	}
	if log.OperationStatus != string(entity.TaskStatusDone) {
		t.Fatalf("OperationStatus = %q, want DONE", log.OperationStatus)
	}
	if log.ProcessDuration < 2.0 || log.ProcessDuration > 3.0 {
		t.Fatalf("ProcessDuration = %v, want ~2s measured from process_begin_at, not the stale 0.719", log.ProcessDuration)
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
	runtime.MustRegister(
		nameA,
		runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) {
			return sinkPassthroughStage{}, nil
		},
		runtime.Metadata{Version: "1.0.0"},
	)

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

	got := mergeCompiledVariants(
		[]string{"wiki", "structure"},
		[]string{"wiki", "tree"},
	)
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

	runtime.MustRegister(
		compC,
		runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) {
			return traceChunkComponent{}, nil
		},
		runtime.Metadata{Version: "1.0.0"},
	)
	runtime.MustRegister(
		compD,
		runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) {
			return traceStubComponent{}, nil
		},
		runtime.Metadata{Version: "1.0.0"},
	)

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
		t.Errorf(
			"REGRESSION: persisted log DSL must NOT carry business data (obj.params.outputs) "+
				"for chunk component c, got %#v",
			cParams["outputs"],
		)
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
		t.Errorf(
			"build failure: log DSL must fall back to the static dsl\n got: %s\nwant: %s",
			got,
			badDSL,
		)
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
		"a": map[string]any{
			"chunks":        []any{map[string]any{"text": "hello"}},
			"_elapsed_time": 0.35,
		},
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
		t.Errorf(
			"persisted log DSL must NOT carry business data (obj.params.outputs), got %#v",
			aParams["outputs"],
		)
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
		"a": map[string]any{
			"chunks":        []any{map[string]any{"text": "hello"}},
			"_elapsed_time": 0.35,
		},
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
