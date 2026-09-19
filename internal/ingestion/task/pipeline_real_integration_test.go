package task

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"ragflow/internal/server/config"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/pdf/type"
	doctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	_ "ragflow/internal/ingestion/component"
	_ "ragflow/internal/ingestion/component/chunker"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/server"
	documentpkg "ragflow/internal/service/document"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"

	"github.com/glebarez/sqlite"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestPipelineExecutor_Run_RealCanvasDSL_UsesGeneralPipeline(t *testing.T) {
	requireTokenizerPool(t)
	ctx := t.Context()

	mustLoadTaskTestConfig(t)
	origDB := dao.DB
	realDB := mustOpenTaskTestDB(t)
	dao.DB = realDB
	t.Cleanup(func() {
		dao.DB = origDB
	})

	realStorage := storage.NewMemoryStorage()
	origStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(realStorage)
	t.Cleanup(func() {
		storage.GetStorageFactory().SetStorage(origStorage)
	})

	templatePath := filepath.Join(taskRepoRoot(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_general.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	templateBytes = disableTokenizerEmbeddingForTaskTemplate(t, templateBytes)
	var templateDSL entity.JSONMap
	if err = json.Unmarshal(templateBytes, &templateDSL); err != nil {
		t.Fatalf("unmarshal template dsl: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := taskLimit32("it_tenant_" + suffix)
	kbID := taskLimit32("it_kb_" + suffix)
	docID := taskLimit32("it_doc_" + suffix)
	fileID := taskLimit32("it_file_" + suffix)
	canvasID := taskLimit32("it_canvas_" + suffix)
	bucket := taskS3SafeBucketName(kbID)
	objectPath := fmt.Sprintf("integration/task/%s/template-general.txt", docID)
	docName := "template-general.txt"
	content := "Alpha paragraph\n\nBeta paragraph."

	mustSeedTaskRealPipelineDocument(t, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath, docName, content)
	if err = realDB.Model(&entity.Document{}).Where("id = ?", docID).Update("pipeline_id", canvasID).Error; err != nil {
		t.Fatalf("set document pipeline_id: %v", err)
	}
	if err = realDB.Create(&entity.UserCanvas{
		ID:             canvasID,
		UserID:         tenantID,
		Permission:     "me",
		CanvasCategory: "agent_canvas",
		DSL:            templateDSL,
	}).Error; err != nil {
		t.Fatalf("create user canvas: %v", err)
	}
	t.Cleanup(func() {
		cleanUpCtx := context.Background()
		_ = realDB.WithContext(cleanUpCtx).Where("id = ?", canvasID).Delete(&entity.UserCanvas{}).Error
		cleanupTaskRealPipelineDocument(cleanUpCtx, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
	})

	taskCtx := &TaskContext{
		IngestionTask: &entity.IngestionTask{
			ID:         "task-real-canvas-1",
			DocumentID: docID,
			DatasetID:  kbID,
		},
		Doc: entity.Document{
			ID:         docID,
			KbID:       kbID,
			Name:       taskStrPtr(docName),
			PipelineID: taskStrPtr(canvasID),
		},
		KB: entity.Knowledgebase{
			ID:       kbID,
			TenantID: tenantID,
			EmbdID:   "embd-1",
		},
		Tenant: entity.Tenant{ID: tenantID},
	}

	var inserted [][]map[string]any
	svc := mustNewPipelineExecutor(t, taskCtx, canvasID, 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			inserted = append(inserted, deepCopyTaskChunks(chunks))
			return nil, nil
		})

	if _, err = svc.Execute(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(inserted) != 1 {
		t.Fatalf("insert calls = %d, want 1", len(inserted))
	}
	if len(inserted[0]) != 1 {
		t.Fatalf("inserted chunk count = %d, want 1", len(inserted[0]))
	}
	for i, ck := range inserted[0] {
		if got := ck["doc_id"]; got != docID {
			t.Fatalf("chunks[%d].doc_id = %v, want %q", i, got, docID)
		}
		if got := ck["content_with_weight"]; got == nil || got == "" {
			t.Fatalf("chunks[%d].content_with_weight = %v, want non-empty string", i, got)
		}
	}
}

func TestPipelineExecutor_Run_RealPDF_ProducesIndexedChunks(t *testing.T) {
	requireTokenizerPool(t)
	ctx := t.Context()

	// The production parse path must never degrade to a mock; install a
	// test-only MockDocAnalyzer as the in-process DeepDoc backend via the
	// public factory seam so the pipeline runs without a real DeepDoc
	// service or ONNX Runtime models. Reset to nil on cleanup (this test
	// binary registers no real backend).
	t.Cleanup(func() { doctype.SetNativeDocAnalyzerFactory(nil) })
	doctype.SetNativeDocAnalyzerFactory(func() (deepdoctype.DocAnalyzer, bool) {
		return &pdf.MockDocAnalyzer{Healthy: true}, true
	})

	// Loads service config (server.Init side effect) without requiring any
	// external MySQL/MinIO/ES. The pipeline runs against an in-memory sqlite
	// DB and an in-memory storage backend; chunks are captured via WithInsertFunc.
	mustLoadTaskTestConfig(t)
	origDB := dao.DB
	realDB := mustOpenTaskTestDB(t)
	dao.DB = realDB
	t.Cleanup(func() {
		dao.DB = origDB
	})

	realStorage := storage.NewMemoryStorage()
	origStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(realStorage)
	t.Cleanup(func() {
		storage.GetStorageFactory().SetStorage(origStorage)
	})

	templatePath := filepath.Join(taskRepoRoot(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_general.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	templateBytes = disableTokenizerEmbeddingForTaskTemplate(t, templateBytes)
	var templateDSL entity.JSONMap
	if err = json.Unmarshal(templateBytes, &templateDSL); err != nil {
		t.Fatalf("unmarshal template dsl: %v", err)
	}

	pdfPath := filepath.Join(taskRepoRoot(t), "internal", "deepdoc", "parser", "pdf", "testdata", "pdfs", "01_english_simple.pdf")
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Skipf("read pdf fixture %s: %v", pdfPath, err)
		return
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := taskLimit32("it_tenant_" + suffix)
	kbID := taskLimit32("it_kb_" + suffix)
	docID := taskLimit32("it_doc_" + suffix)
	fileID := taskLimit32("it_file_" + suffix)
	canvasID := taskLimit32("it_canvas_" + suffix)
	bucket := taskS3SafeBucketName(kbID)
	docName := "01_english_simple.pdf"
	objectPath := fmt.Sprintf("integration/task/%s/%s", docID, docName)

	mustSeedTaskRealPipelineDocumentBytes(t, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath, docName, ".pdf", "pdf", pdfBytes)
	if err = realDB.Model(&entity.Document{}).Where("id = ?", docID).Update("pipeline_id", canvasID).Error; err != nil {
		t.Fatalf("set document pipeline_id: %v", err)
	}
	if err = realDB.Create(&entity.UserCanvas{
		ID:             canvasID,
		UserID:         tenantID,
		Permission:     "me",
		CanvasCategory: "agent_canvas",
		DSL:            templateDSL,
	}).Error; err != nil {
		t.Fatalf("create user canvas: %v", err)
	}
	t.Cleanup(func() {
		_ = realDB.Where("id = ?", canvasID).Delete(&entity.UserCanvas{}).Error
		cleanupTaskRealPipelineDocument(ctx, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
	})

	taskCtx := &TaskContext{
		IngestionTask: &entity.IngestionTask{
			ID:         "task-real-pdf-1",
			DocumentID: docID,
			DatasetID:  kbID,
		},
		Doc: entity.Document{
			ID:         docID,
			KbID:       kbID,
			Name:       taskStrPtr(docName),
			PipelineID: taskStrPtr(canvasID),
		},
		KB: entity.Knowledgebase{
			ID:       kbID,
			TenantID: tenantID,
			EmbdID:   "embd-1",
		},
		Tenant: entity.Tenant{ID: tenantID},
	}

	var inserted [][]map[string]any
	svc := mustNewPipelineExecutor(t, taskCtx, canvasID, 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			inserted = append(inserted, deepCopyTaskChunks(chunks))
			return nil, nil
		})

	if _, err = svc.Execute(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(inserted) == 0 {
		t.Fatal("no chunks inserted")
	}
	var chunks []map[string]any
	for _, batch := range inserted {
		chunks = append(chunks, batch...)
	}
	if len(chunks) == 0 {
		t.Fatal("inserted 0 chunks")
	}
	sawImage := false
	for i, chunk := range chunks {
		if got := chunk["doc_id"]; got != docID {
			t.Fatalf("chunk[%d].doc_id = %v, want %q", i, got, docID)
		}
		if got := chunk["docnm_kwd"]; got != docName {
			t.Fatalf("chunk[%d].docnm_kwd = %v, want %q", i, got, docName)
		}
		if got := chunk["content_with_weight"]; got == nil || got == "" {
			t.Fatalf("chunk[%d].content_with_weight = %v, want non-empty", i, got)
		}
		if img, ok := chunk["img_id"]; ok && img != nil && img != "" {
			sawImage = true
		}
	}
	if !sawImage {
		t.Fatal("expected at least one chunk with img_id (pdf preview image uploaded to storage)")
	}
}

func TestRunPipeline_RealPipelineOutput_ProducesIndexFields(t *testing.T) {
	requireTokenizerPool(t)
	ctx := t.Context()

	mustLoadTaskTestConfig(t)
	origDB := dao.DB
	realDB := mustOpenTaskTestDB(t)
	dao.DB = realDB
	t.Cleanup(func() {
		dao.DB = origDB
	})

	realStorage := storage.NewMemoryStorage()
	origStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(realStorage)
	t.Cleanup(func() {
		storage.GetStorageFactory().SetStorage(origStorage)
	})

	templatePath := filepath.Join(taskRepoRoot(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_general.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	templateBytes = disableTokenizerEmbeddingForTaskTemplate(t, templateBytes)

	pipe, err := pipelinepkg.NewPipelineFromDSL(templateBytes, "task-real-pipeline")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := taskLimit32("it_tenant_" + suffix)
	kbID := taskLimit32("it_kb_" + suffix)
	docID := taskLimit32("it_doc_" + suffix)
	fileID := taskLimit32("it_file_" + suffix)
	bucket := taskS3SafeBucketName(kbID)
	objectPath := fmt.Sprintf("integration/task/%s/template-general.txt", docID)
	docName := "template-general.txt"
	content := "Alpha paragraph.\n\nBeta paragraph."

	mustSeedTaskRealPipelineDocument(t, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath, docName, content)
	t.Cleanup(func() {
		cleanupTaskRealPipelineDocument(ctx, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
	})

	pipelineOut, err := pipe.Run(ctx, map[string]any{
		"doc_id": docID,
	}, nil)
	if err != nil {
		t.Fatalf("pipeline Run: %v", err)
	}
	pipelineOut = taskTerminalPayloadFromRunOutput(t, pipelineOut, "Tokenizer:LegalReadersDecide")

	taskCtx := &TaskContext{
		IngestionTask: &entity.IngestionTask{
			ID:         "task-real-1",
			DocumentID: docID,
			DatasetID:  kbID,
		},
		Doc: entity.Document{
			ID:   docID,
			KbID: kbID,
			Name: taskStrPtr(docName),
		},
		KB: entity.Knowledgebase{
			ID:       kbID,
			TenantID: tenantID,
			EmbdID:   "embd-1",
		},
		Tenant: entity.Tenant{ID: tenantID},
	}

	var inserted [][]map[string]any
	svc := mustNewPipelineExecutor(t, taskCtx, "flow-real-1", 0).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			inserted = append(inserted, deepCopyTaskChunks(chunks))
			return nil, nil
		})

	if _, err = svc.processOutput(ctx, pipelineOut, time.Now()); err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}

	if len(inserted) != 1 {
		t.Fatalf("insert calls = %d, want 1", len(inserted))
	}
	chunks := inserted[0]
	if len(chunks) != 1 {
		t.Fatalf("inserted chunk count = %d, want 1", len(chunks))
	}
	for i, ck := range chunks {
		if got := ck["doc_id"]; got != docID {
			t.Fatalf("chunks[%d].doc_id = %v, want %q", i, got, docID)
		}
		if got := ck["docnm_kwd"]; got != docName {
			t.Fatalf("chunks[%d].docnm_kwd = %v, want %q", i, got, docName)
		}
		if got := ck["content_with_weight"]; got == nil || got == "" {
			t.Fatalf("chunks[%d].content_with_weight = %v, want non-empty string", i, got)
		}
		if got, ok := ck["content_ltks"].(string); !ok || got == "" {
			t.Fatalf("chunks[%d].content_ltks = %T/%v, want non-empty string", i, ck["content_ltks"], ck["content_ltks"])
		}
		if got, ok := ck["content_sm_ltks"].(string); !ok || got == "" {
			t.Fatalf("chunks[%d].content_sm_ltks = %T/%v, want non-empty string", i, ck["content_sm_ltks"], ck["content_sm_ltks"])
		}
		if _, hasText := ck["text"]; hasText {
			t.Fatalf("chunks[%d] should not keep raw text field after RunPipeline: %v", i, ck["text"])
		}
	}
}

// Who owns the column settings on a real built-in run, checked through the whole
// chain rather than through a resolver: the document's root keys are the
// authoritative intent, the parser component of the run's DSL is the canvas
// author's projection that applies only while the root states nothing, and a
// document nobody configured keeps the auto rendering every column. The built-in
// table template states no column mode, so on the built-in run path nothing but
// the document's own root keys can reach the parser.
func TestPipelineExecutor_Run_BuiltinTableColumnIntent(t *testing.T) {
	requireTokenizerPool(t)
	mustLoadTaskTestConfig(t)

	builtinDSL, err := pipelinepkg.LoadBuiltinDSL("table")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL(table): %v", err)
	}
	if strings.Contains(builtinDSL, "column_mode") {
		t.Fatalf("the built-in table template must not state a column mode, got %s", builtinDSL)
	}
	baseDSL := disableTokenizerEmbeddingForTaskTemplate(t, []byte(builtinDSL))

	const content = "Title,Country,Internal\nDoc A,Turkey,42\n"

	manualRoles := map[string]any{"Title": "indexing", "Country": "metadata"}
	tests := []struct {
		name        string
		docConfig   entity.JSONMap
		canvasSetup map[string]any
		inText      []string
		notInText   []string
		wantRowData map[string]any
	}{
		{
			name: "root column intent reaches the parser",
			docConfig: entity.JSONMap{
				"table_column_mode":  "manual",
				"table_column_roles": manualRoles,
			},
			inText:      []string{"- Title: Doc A", "- Internal: 42"},
			notInText:   []string{"- Country: Turkey"},
			wantRowData: map[string]any{"Country": "Turkey", "Internal": "42"},
		},
		{
			name:      "a canvas projection applies to an unconfigured document",
			docConfig: entity.JSONMap{},
			canvasSetup: map[string]any{
				"column_mode":  "manual",
				"column_roles": manualRoles,
			},
			inText:      []string{"- Title: Doc A", "- Internal: 42"},
			notInText:   []string{"- Country: Turkey"},
			wantRowData: map[string]any{"Country": "Turkey", "Internal": "42"},
		},
		{
			name:      "the document intent shadows the canvas projection",
			docConfig: entity.JSONMap{"table_column_mode": "manual", "table_column_roles": manualRoles},
			canvasSetup: map[string]any{
				"column_mode":  "auto",
				"column_roles": map[string]any{"Country": "both", "Title": "both"},
			},
			inText:      []string{"- Title: Doc A", "- Internal: 42"},
			notInText:   []string{"- Country: Turkey"},
			wantRowData: map[string]any{"Country": "Turkey", "Internal": "42"},
		},
		{
			name:        "an unconfigured document keeps every column",
			docConfig:   entity.JSONMap{},
			inText:      []string{"- Title: Doc A", "- Country: Turkey", "- Internal: 42"},
			wantRowData: map[string]any{"Title": "Doc A", "Country": "Turkey", "Internal": "42"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			dsl := string(taskSetBuiltinParserSpreadsheet(t, baseDSL, tt.canvasSetup))

			origDB := dao.DB
			realDB := mustOpenTaskTestDB(t)
			dao.DB = realDB
			t.Cleanup(func() { dao.DB = origDB })

			realStorage := storage.NewMemoryStorage()
			origStorage := storage.GetStorageFactory().GetStorage()
			storage.GetStorageFactory().SetStorage(realStorage)
			t.Cleanup(func() { storage.GetStorageFactory().SetStorage(origStorage) })

			suffix := fmt.Sprintf("%d", time.Now().UnixNano())
			tenantID := taskLimit32("it_tenant_" + suffix)
			kbID := taskLimit32("it_kb_" + suffix)
			docID := taskLimit32("it_doc_" + suffix)
			fileID := taskLimit32("it_file_" + suffix)
			bucket := taskS3SafeBucketName(kbID)
			objectPath := fmt.Sprintf("integration/task/%s/columns.csv", docID)
			docName := "columns.csv"

			mustSeedTaskRealPipelineDocumentBytes(t, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath, docName, ".csv", "csv", []byte(content))
			t.Cleanup(func() {
				cleanupTaskRealPipelineDocument(context.Background(), realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
			})
			if err = realDB.Model(&entity.Document{}).Where("id = ?", docID).Updates(map[string]any{
				"parser_id":     "table",
				"parser_config": tt.docConfig,
			}).Error; err != nil {
				t.Fatalf("seed table document: %v", err)
			}

			taskCtx := &TaskContext{
				IngestionTask: &entity.IngestionTask{
					ID:         "task-table-" + suffix,
					DocumentID: docID,
					DatasetID:  kbID,
				},
				Doc: entity.Document{
					ID:           docID,
					KbID:         kbID,
					ParserID:     "table",
					ParserConfig: tt.docConfig,
					Name:         taskStrPtr(docName),
				},
				KB:     entity.Knowledgebase{ID: kbID, TenantID: tenantID, EmbdID: "embd-1"},
				Tenant: entity.Tenant{ID: tenantID},
			}

			var inserted [][]map[string]any
			svc := mustNewPipelineExecutor(t, taskCtx, "table", 0).
				WithLoadDSLFunc(func(context.Context, string) (string, string, error) {
					return dsl, "table", nil
				}).
				WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
					inserted = append(inserted, deepCopyTaskChunks(chunks))
					return nil, nil
				})

			if _, err = svc.Execute(ctx); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(inserted) != 1 || len(inserted[0]) != 1 {
				t.Fatalf("inserted = %v, want one chunk", inserted)
			}
			chunk := inserted[0][0]

			text, _ := chunk["content_with_weight"].(string)
			for _, want := range tt.inText {
				if !strings.Contains(text, want) {
					t.Errorf("content_with_weight = %q, want %q", text, want)
				}
			}
			for _, unwanted := range tt.notInText {
				if strings.Contains(text, unwanted) {
					t.Errorf("content_with_weight = %q, must not carry %q", text, unwanted)
				}
			}
			rowData, _ := chunk["chunk_data"].(map[string]any)
			if !reflect.DeepEqual(rowData, tt.wantRowData) {
				t.Errorf("chunk_data = %#v, want %v", chunk["chunk_data"], tt.wantRowData)
			}
		})
	}
}

// Who owns the column settings, checked across two runs of the same document
// row: a run publishes the schema it discovered to the document's root keys, and
// an edit made afterwards must still decide the next parse. Discovered names are
// not intent, so counting them as intent would make the root tier permanent after
// one successful parse and shadow every later change. The publication must also
// leave the document's canvas entry alone.
func TestPipelineExecutor_Run_ReparseHonoursEditedColumnRoles(t *testing.T) {
	requireTokenizerPool(t)
	mustLoadTaskTestConfig(t)

	builtinDSL, err := pipelinepkg.LoadBuiltinDSL("table")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL(table): %v", err)
	}
	dsl := string(disableTokenizerEmbeddingForTaskTemplate(t, []byte(builtinDSL)))
	const content = "Title,Country,Internal\nDoc A,Turkey,42\n"

	origDB := dao.DB
	realDB := mustOpenTaskTestDB(t)
	dao.DB = realDB
	t.Cleanup(func() { dao.DB = origDB })

	realStorage := storage.NewMemoryStorage()
	origStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(realStorage)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(origStorage) })

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := taskLimit32("it_tenant_" + suffix)
	kbID := taskLimit32("it_kb_" + suffix)
	docID := taskLimit32("it_doc_" + suffix)
	fileID := taskLimit32("it_file_" + suffix)
	bucket := taskS3SafeBucketName(kbID)
	objectPath := fmt.Sprintf("integration/task/%s/reparse.csv", docID)

	mustSeedTaskRealPipelineDocumentBytes(t, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath, "reparse.csv", ".csv", "csv", []byte(content))
	t.Cleanup(func() {
		cleanupTaskRealPipelineDocument(context.Background(), realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
	})
	// A document whose dataset runs a canvas carries the parser component in its
	// own configuration. The entry states no column setting, so it is silent for
	// resolution — and it is the shape a run must leave alone.
	if err = realDB.Model(&entity.Document{}).Where("id = ?", docID).Updates(map[string]any{
		"parser_id": "table",
		"parser_config": entity.JSONMap{
			"Parser:HipSignsRhyme": map[string]any{
				"spreadsheet": map[string]any{"output_format": "json"},
			},
		},
	}).Error; err != nil {
		t.Fatalf("seed table document: %v", err)
	}

	docSvc := documentpkg.NewDocumentService()

	// parse reads the configuration the document row actually carries, so the
	// second run resolves what the first run wrote back through the database,
	// and applies the run's publication the way the ingestion service does.
	parse := func(label string) map[string]any {
		t.Helper()
		var doc entity.Document
		if err = realDB.Where("id = ?", docID).First(&doc).Error; err != nil {
			t.Fatalf("%s: load document: %v", label, err)
		}
		var kb entity.Knowledgebase
		if err = realDB.Where("id = ?", kbID).First(&kb).Error; err != nil {
			t.Fatalf("%s: load dataset: %v", label, err)
		}
		taskCtx := &TaskContext{
			IngestionTask: &entity.IngestionTask{
				ID:         "task-reparse-" + label + "-" + suffix,
				DocumentID: docID,
				DatasetID:  kbID,
			},
			Doc:    doc,
			KB:     kb,
			Tenant: entity.Tenant{ID: tenantID},
		}
		var inserted [][]map[string]any
		result, execErr := mustNewPipelineExecutor(t, taskCtx, "table", 0).
			WithLoadDSLFunc(func(context.Context, string) (string, string, error) {
				return dsl, "table", nil
			}).
			WithInsertFunc(func(_ context.Context, chunks []map[string]any, _, _ string) ([]string, error) {
				inserted = append(inserted, deepCopyTaskChunks(chunks))
				return nil, nil
			}).Execute(t.Context())
		if execErr != nil {
			t.Fatalf("%s: Execute: %v", label, execErr)
		}
		if len(inserted) != 1 || len(inserted[0]) != 1 {
			t.Fatalf("%s: inserted = %v, want one chunk", label, inserted)
		}
		if pubErr := docSvc.SaveDocumentTableColumns(t.Context(), docID, result.DiscoveredColumns); pubErr != nil {
			t.Fatalf("%s: publish discovered columns: %v", label, pubErr)
		}
		return inserted[0][0]
	}

	first := parse("first")
	firstText, _ := first["content_with_weight"].(string)
	for _, want := range []string{"- Title: Doc A", "- Country: Turkey", "- Internal: 42"} {
		if !strings.Contains(firstText, want) {
			t.Errorf("first run: content_with_weight = %q, want %q", firstText, want)
		}
	}

	var stored entity.Document
	if err = realDB.Where("id = ?", docID).First(&stored).Error; err != nil {
		t.Fatalf("load published document: %v", err)
	}
	if names := stringSliceFromConfig(stored.ParserConfig["table_column_names"]); !reflect.DeepEqual(names, []string{"Title", "Country", "Internal"}) {
		t.Errorf("published table_column_names = %#v, want [Title Country Internal]", stored.ParserConfig["table_column_names"])
	}
	assertNoProjectedColumnKeys(t, stored.ParserConfig)
	component, _ := stored.ParserConfig["Parser:HipSignsRhyme"].(map[string]any)
	spreadsheet, _ := component["spreadsheet"].(map[string]any)
	if len(spreadsheet) != 1 || spreadsheet["output_format"] != "json" {
		t.Errorf("the first run rewrote the canvas entry: %#v, want the spreadsheet block it started with", spreadsheet)
	}

	// The document dialog's write: mode and roles at the root, next to the
	// names the first run published.
	edited := stored.ParserConfig
	edited["table_column_mode"] = "manual"
	edited["table_column_roles"] = map[string]any{"Title": "indexing", "Country": "metadata"}
	if err = realDB.Model(&entity.Document{}).Where("id = ?", docID).Update("parser_config", edited).Error; err != nil {
		t.Fatalf("edit column roles: %v", err)
	}

	second := parse("second")
	secondText, _ := second["content_with_weight"].(string)
	if strings.Contains(secondText, "- Country: Turkey") {
		t.Errorf("second run: content_with_weight = %q, must not carry the metadata column", secondText)
	}
	for _, want := range []string{"- Title: Doc A", "- Internal: 42"} {
		if !strings.Contains(secondText, want) {
			t.Errorf("second run: content_with_weight = %q, want %q", secondText, want)
		}
	}
	wantRowData := map[string]any{"Country": "Turkey", "Internal": "42"}
	if rowData, _ := second["chunk_data"].(map[string]any); !reflect.DeepEqual(rowData, wantRowData) {
		t.Errorf("second run: chunk_data = %#v, want %v", second["chunk_data"], wantRowData)
	}

	if err = realDB.Where("id = ?", docID).First(&stored).Error; err != nil {
		t.Fatalf("load re-parsed document: %v", err)
	}
	if mode, _ := stored.ParserConfig["table_column_mode"].(string); mode != "manual" {
		t.Errorf("after the second run table_column_mode = %#v, want the edited manual profile to survive publication", stored.ParserConfig["table_column_mode"])
	}
	roles, _ := stored.ParserConfig["table_column_roles"].(map[string]any)
	if role, _ := roles["Country"].(string); role != "metadata" {
		t.Errorf("after the second run Country role = %#v, want metadata", roles["Country"])
	}
	assertNoProjectedColumnKeys(t, stored.ParserConfig)
}

// stringSliceFromConfig reads a JSON round-tripped list of strings.
func stringSliceFromConfig(raw any) []string {
	list, _ := raw.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
}

// assertNoProjectedColumnKeys fails if a run wrote column settings into a
// component entry: the Parser:<id> spreadsheet block belongs to the canvas DSL,
// so only the root keys may carry a document's intent and its discovered schema.
func assertNoProjectedColumnKeys(t *testing.T, config map[string]any) {
	t.Helper()
	var walk func(owner string, node map[string]any)
	walk = func(owner string, node map[string]any) {
		for key, value := range node {
			nested, isMap := value.(map[string]any)
			if !isMap {
				continue
			}
			for _, columnKey := range []string{"column_mode", "column_roles", "column_names"} {
				if _, ok := nested[columnKey]; ok {
					t.Errorf("%s.%s.%s is set: column state belongs to the root keys only", owner, key, columnKey)
				}
			}
			walk(owner+"."+key, nested)
		}
	}
	walk("parser_config", config)
}

// taskSetBuiltinParserSpreadsheet writes a canvas author's spreadsheet setup into
// the Parser component of a built-in DSL, which is the shape a pipeline canvas
func taskSetBuiltinParserSpreadsheet(t *testing.T, raw []byte, setup map[string]any) []byte {
	t.Helper()
	if len(setup) == 0 {
		return raw
	}
	var dsl map[string]any
	if err := json.Unmarshal(raw, &dsl); err != nil {
		t.Fatalf("unmarshal dsl: %v", err)
	}
	components, ok := dsl["components"].(map[string]any)
	if !ok {
		t.Fatalf("dsl components = %T, want map[string]any", dsl["components"])
	}
	patched := 0
	for _, rawComp := range components {
		comp, _ := rawComp.(map[string]any)
		obj, _ := comp["obj"].(map[string]any)
		if obj == nil || obj["component_name"] != "Parser" {
			continue
		}
		params, _ := obj["params"].(map[string]any)
		if params == nil {
			t.Fatal("Parser component carries no params")
		}
		spreadsheet, _ := params["spreadsheet"].(map[string]any)
		if spreadsheet == nil {
			spreadsheet = map[string]any{}
		}
		for k, v := range setup {
			spreadsheet[k] = v
		}
		params["spreadsheet"] = spreadsheet
		patched++
	}
	if patched == 0 {
		t.Fatal("no Parser component found in the built-in DSL")
	}
	out, err := json.Marshal(dsl)
	if err != nil {
		t.Fatalf("marshal patched dsl: %v", err)
	}
	return out
}

func taskRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", "..", ".."))
}

func mustLoadTaskTestConfig(t *testing.T) *config.Config {
	t.Helper()
	if err := common.InitLogger("info", common.FileOutput{}, ""); err != nil {
		t.Fatalf("init common logger: %v", err)
	}
	configPath := filepath.Join(taskRepoRoot(t), "conf", "service_conf.yaml")
	if err := server.Init(configPath); err != nil {
		t.Fatalf("init service config from %s: %v", configPath, err)
	}
	cfg := server.GetConfig()
	if cfg == nil {
		t.Fatal("task test config is nil after server.Init")
	}
	return cfg
}

// mustOpenTaskTestDB opens an isolated in-memory sqlite database and migrates
// the tables the real-pipeline contract tests seed and read. It does not touch
// the filesystem or any external MySQL. The connection pool is pinned to a
// single connection (MaxOpenConns(1)) so the in-memory database is shared
// across all statements — without that, gorm's default pool would hand each
// statement a separate connection, and ":memory:" is per-connection (so seeds
// on one connection would be invisible to reads on another).
func mustOpenTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open in-memory sqlite db: %v", err)
	}
	if err = db.AutoMigrate(
		&entity.Tenant{},
		&entity.Knowledgebase{},
		&entity.Document{},
		&entity.File{},
		&entity.File2Document{},
		&entity.UserCanvas{},
		&entity.PipelineOperationLog{},
	); err != nil {
		t.Fatalf("auto-migrate sqlite tables: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB from gorm: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}

func disableTokenizerEmbeddingForTaskTemplate(t *testing.T, raw []byte) []byte {
	t.Helper()
	var tpl map[string]any
	if err := json.Unmarshal(raw, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	// A canvas template wraps the graph under "dsl"; a builtin DSL from
	// pipeline.LoadBuiltinDSL already is that inner object.
	dsl, ok := tpl["dsl"].(map[string]any)
	if !ok {
		dsl = tpl
	}
	components, ok := dsl["components"].(map[string]any)
	if !ok {
		t.Fatalf("template components = %T, want map[string]any", dsl["components"])
	}
	changed := 0
	for _, rawComp := range components {
		comp, ok := rawComp.(map[string]any)
		if !ok {
			continue
		}
		obj, ok := comp["obj"].(map[string]any)
		if !ok || obj["component_name"] != "Tokenizer" {
			continue
		}
		params, ok := obj["params"].(map[string]any)
		if !ok {
			continue
		}
		params["search_method"] = []string{"full_text"}
		changed++
	}
	if changed == 0 {
		t.Fatal("no Tokenizer component found to disable embedding")
	}
	out, err := json.Marshal(tpl)
	if err != nil {
		t.Fatalf("marshal modified template: %v", err)
	}
	return out
}

func taskMustSymlink(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.Symlink(src, dst); err != nil {
		t.Fatalf("symlink tokenizer resource %s -> %s: %v", src, dst, err)
	}
}

func taskMustWriteTokenizerPOSDef(t *testing.T, dictPath, outPath string) {
	t.Helper()
	data, err := os.ReadFile(dictPath)
	if err != nil {
		t.Fatalf("read tokenizer dict %s: %v", dictPath, err)
	}
	posSet := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 {
			posSet[fields[2]] = struct{}{}
		}
	}
	if len(posSet) == 0 {
		t.Fatalf("no POS tags parsed from tokenizer dict %s", dictPath)
	}
	posList := make([]string, 0, len(posSet))
	for pos := range posSet {
		posList = append(posList, pos)
	}
	sort.Strings(posList)
	content := strings.Join(posList, "\n") + "\n"
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write tokenizer pos file %s: %v", outPath, err)
	}
}

func taskMustPrepareTokenizerWordNet(t *testing.T, root string) {
	t.Helper()
	zipPath := filepath.Join(taskRepoRoot(t), "ragflow_deps", "nltk_data", "corpora", "wordnet.zip")
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Skipf("open wordnet zip %s: %v", zipPath, err)
		return
	}
	defer func() { _ = reader.Close() }()
	for _, f := range reader.File {
		name := strings.TrimPrefix(f.Name, "wordnet/")
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		dst := filepath.Join(root, "wordnet", name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir wordnet dst %s: %v", dst, err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open wordnet entry %s: %v", f.Name, err)
		}
		out, err := os.Create(dst)
		if err != nil {
			_ = rc.Close()
			t.Fatalf("create wordnet dst %s: %v", dst, err)
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = out.Close()
			_ = rc.Close()
			t.Fatalf("copy wordnet entry %s -> %s: %v", f.Name, dst, err)
		}
		if err := out.Close(); err != nil {
			_ = rc.Close()
			t.Fatalf("close wordnet dst %s: %v", dst, err)
		}
		if err := rc.Close(); err != nil {
			t.Fatalf("close wordnet entry %s: %v", f.Name, err)
		}
	}
}

func taskMustPrepareTokenizerOpenCC(t *testing.T, root string) {
	t.Helper()
	const systemOpenCC = "/usr/share/opencc"
	if _, err := os.Stat(systemOpenCC); err != nil {
		t.Skipf("system opencc dir %s not found: %v", systemOpenCC, err)
		return
	}
	taskMustSymlink(t, systemOpenCC, filepath.Join(root, "opencc"))
}

func requireTokenizerPool(t *testing.T) {
	t.Helper()
	if err := tokenizer.Init(&tokenizer.PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        1,
		MaxSize:        2,
		IdleTimeout:    30 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}); err != nil {
		t.Skipf("tokenizer pool init failed: %v", err)
	}
}

func mustSeedTaskRealPipelineDocument(
	t *testing.T,
	db *gorm.DB,
	stg storage.Storage,
	tenantID, kbID, docID, fileID, bucket, objectPath, docName, content string,
) {
	mustSeedTaskRealPipelineDocumentBytes(t, db, stg, tenantID, kbID, docID, fileID, bucket, objectPath, docName, ".txt", "txt", []byte(content))
}

func mustSeedTaskRealPipelineDocumentBytes(
	t *testing.T,
	db *gorm.DB,
	stg storage.Storage,
	tenantID, kbID, docID, fileID, bucket, objectPath, docName, suffix, docType string,
	content []byte,
) {
	t.Helper()
	ocrID := "ocr-default"
	if err := db.Create(&entity.Tenant{
		ID:        tenantID,
		LLMID:     "gpt-4",
		ASRID:     "asr-default",
		Img2TxtID: "img2txt-default",
		RerankID:  "rerank-default",
		ParserIDs: "parser-default",
		OCRID:     &ocrID,
		Status:    taskStrPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:           kbID,
		TenantID:     tenantID,
		EmbdID:       "embd-1",
		ParserConfig: entity.JSONMap{},
		Status:       taskStrPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	ctx := t.Context()
	if err := stg.Put(ctx, bucket, objectPath, content); err != nil {
		t.Fatalf("put real minio object: %v", err)
	}
	if err := db.Create(&entity.File{
		ID:         fileID,
		ParentID:   bucket,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       docName,
		Type:       docType,
		Location:   taskStrPtr(objectPath),
		SourceType: "",
	}).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := db.Create(&entity.Document{
		ID:           docID,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         docType,
		CreatedBy:    tenantID,
		Name:         taskStrPtr(docName),
		Location:     taskStrPtr(objectPath),
		Suffix:       suffix,
		Status:       taskStrPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	if err := db.Create(&entity.File2Document{
		ID:         taskLimit32("it_map_" + docID),
		FileID:     taskStrPtr(fileID),
		DocumentID: taskStrPtr(docID),
	}).Error; err != nil {
		t.Fatalf("create file2document: %v", err)
	}
}

func cleanupTaskRealPipelineDocument(ctx context.Context, db *gorm.DB, storageImpl storage.Storage, tenantID, kbID, docID, fileID, bucket, objectPath string) {
	_ = db.WithContext(ctx).Where("document_id = ?", docID).Delete(&entity.File2Document{}).Error
	_ = db.WithContext(ctx).Where("id = ?", docID).Delete(&entity.Document{}).Error
	_ = db.WithContext(ctx).Where("id = ?", fileID).Delete(&entity.File{}).Error
	_ = db.WithContext(ctx).Where("id = ?", kbID).Delete(&entity.Knowledgebase{}).Error
	_ = db.WithContext(ctx).Where("id = ?", tenantID).Delete(&entity.Tenant{}).Error
	_ = storageImpl.Remove(ctx, bucket, objectPath)
}

func deepCopyTaskChunks(in []map[string]any) []map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make([]map[string]any, len(in))
	for i, ck := range in {
		data, err := json.Marshal(ck)
		if err != nil {
			panic(err)
		}
		var copied map[string]any
		if err := json.Unmarshal(data, &copied); err != nil {
			panic(err)
		}
		out[i] = copied
	}
	return out
}

func taskTerminalPayloadFromRunOutput(t *testing.T, out map[string]any, terminalID string) map[string]any {
	t.Helper()
	if out == nil {
		t.Fatal("Run returned nil output")
	}
	if _, ok := out["output_format"]; ok {
		return out
	}
	nested, ok := out[terminalID].(map[string]any)
	if !ok {
		t.Fatalf("run output missing terminal payload %q in %v", terminalID, out)
	}
	return nested
}

func taskStrPtr(s string) *string { return &s }

func taskLimit32(s string) string {
	if len(s) <= 32 {
		return s
	}
	return s[:32]
}

func taskS3SafeBucketName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	return s
}
