package task

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	_ "ragflow/internal/ingestion/component"
	componentpkg "ragflow/internal/ingestion/component"
	_ "ragflow/internal/ingestion/component/chunker"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/server"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestPipelineExecutor_Run_RealCanvasDSL_UsesGeneralPipeline(t *testing.T) {
	requireTokenizerPool(t)

	cfg := mustLoadTaskRealIntegrationConfig(t)
	realDB := mustOpenTaskRealMySQL(t, cfg)
	if err := realDB.AutoMigrate(
		&entity.Tenant{},
		&entity.Knowledgebase{},
		&entity.Document{},
		&entity.File{},
		&entity.File2Document{},
		&entity.UserCanvas{},
	); err != nil {
		t.Fatalf("auto-migrate real mysql tables: %v", err)
	}

	realStorage, err := storage.NewMinioStorage(cfg.StorageEngine.Minio)
	if err != nil {
		t.Fatalf("connect real minio: %v", err)
	}

	origDB := dao.DB
	origStorage := storage.GetStorageFactory().GetStorage()
	dao.DB = realDB
	storage.GetStorageFactory().SetStorage(realStorage)
	t.Cleanup(func() {
		dao.DB = origDB
		storage.GetStorageFactory().SetStorage(origStorage)
	})

	templatePath := filepath.Join(taskRepoRoot(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_general.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	templateBytes = disableTokenizerEmbeddingForTaskTemplate(t, templateBytes)
	var templateDSL entity.JSONMap
	if err := json.Unmarshal(templateBytes, &templateDSL); err != nil {
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
	if err := realDB.Model(&entity.Document{}).Where("id = ?", docID).Update("pipeline_id", canvasID).Error; err != nil {
		t.Fatalf("set document pipeline_id: %v", err)
	}
	if err := realDB.Create(&entity.UserCanvas{
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
		cleanupTaskRealPipelineDocument(realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
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

	if _, err := svc.Execute(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(inserted) != 1 {
		t.Fatalf("insert calls = %d, want 1", len(inserted))
	}
	if len(inserted[0]) != 2 {
		t.Fatalf("inserted chunk count = %d, want 2", len(inserted[0]))
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

func TestPipelineExecutor_Run_RealPDF_WritesAndReadsBackFromElasticsearch(t *testing.T) {
	requireTokenizerPool(t)

	cfg := mustLoadTaskRealIntegrationConfig(t)
	realDB := mustOpenTaskRealMySQL(t, cfg)
	if err := realDB.AutoMigrate(
		&entity.Tenant{},
		&entity.Knowledgebase{},
		&entity.Document{},
		&entity.File{},
		&entity.File2Document{},
		&entity.UserCanvas{},
	); err != nil {
		t.Fatalf("auto-migrate real mysql tables: %v", err)
	}

	realStorage, err := storage.NewMinioStorage(cfg.StorageEngine.Minio)
	if err != nil {
		t.Fatalf("connect real minio: %v", err)
	}
	if err := engine.Init(&cfg.DocEngine); err != nil {
		t.Fatalf("init real doc engine: %v", err)
	}
	if engine.Get() == nil {
		t.Fatal("doc engine is nil after init")
	}
	if engine.GetEngineType() != engine.EngineElasticsearch {
		t.Fatalf("doc engine type = %s, want %s", engine.GetEngineType(), engine.EngineElasticsearch)
	}

	origDB := dao.DB
	origStorage := storage.GetStorageFactory().GetStorage()
	origDocResolver := componentpkg.ResolveDocumentStorageOverride
	dao.DB = realDB
	storage.GetStorageFactory().SetStorage(realStorage)
	componentpkg.ResolveDocumentStorageOverride = nil
	t.Cleanup(func() {
		dao.DB = origDB
		storage.GetStorageFactory().SetStorage(origStorage)
		componentpkg.ResolveDocumentStorageOverride = origDocResolver
	})

	templatePath := filepath.Join(taskRepoRoot(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_general.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	templateBytes = disableTokenizerEmbeddingForTaskTemplate(t, templateBytes)
	var templateDSL entity.JSONMap
	if err := json.Unmarshal(templateBytes, &templateDSL); err != nil {
		t.Fatalf("unmarshal template dsl: %v", err)
	}

	pdfPath := filepath.Join(taskRepoRoot(t), "internal", "deepdoc", "parser", "pdf", "testdata", "pdfs", "01_english_simple.pdf")
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read pdf fixture: %v", err)
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
	baseName := fmt.Sprintf("ragflow_%s", tenantID)

	mustSeedTaskRealPipelineDocumentBytes(t, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath, docName, ".pdf", "pdf", pdfBytes)
	if err := realDB.Model(&entity.Document{}).Where("id = ?", docID).Update("pipeline_id", canvasID).Error; err != nil {
		t.Fatalf("set document pipeline_id: %v", err)
	}
	if err := realDB.Create(&entity.UserCanvas{
		ID:             canvasID,
		UserID:         tenantID,
		Permission:     "me",
		CanvasCategory: "agent_canvas",
		DSL:            templateDSL,
	}).Error; err != nil {
		t.Fatalf("create user canvas: %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Get().DropChunkStore(context.Background(), baseName, kbID)
		_ = realDB.Where("id = ?", canvasID).Delete(&entity.UserCanvas{}).Error
		cleanupTaskRealPipelineDocument(realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
	})

	taskCtx := &TaskContext{
		IngestionTask: &entity.IngestionTask{
			ID:         "task-real-pdf-es-1",
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

	svc := mustNewPipelineExecutor(t, taskCtx, canvasID, 0)

	if _, err := svc.Execute(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	result, err := engine.Get().Search(context.Background(), &enginetypes.SearchRequest{
		IndexNames: []string{baseName},
		KbIDs:      []string{kbID},
		Limit:      20,
	})
	if err != nil {
		t.Fatalf("search indexed chunks: %v", err)
	}
	if result == nil {
		t.Fatal("search result is nil")
	}
	if len(result.Chunks) == 0 {
		t.Fatal("expected indexed chunks in Elasticsearch, got 0")
	}
	for i, chunk := range result.Chunks {
		if got := chunk["doc_id"]; got != docID {
			t.Fatalf("result chunk[%d].doc_id = %v, want %q", i, got, docID)
		}
		if got := chunk["kb_id"]; got != kbID {
			t.Fatalf("result chunk[%d].kb_id = %v, want %q", i, got, kbID)
		}
		if got := chunk["docnm_kwd"]; got != docName {
			t.Fatalf("result chunk[%d].docnm_kwd = %v, want %q", i, got, docName)
		}
		if got := chunk["content_with_weight"]; got == nil || got == "" {
			t.Fatalf("result chunk[%d].content_with_weight = %v, want non-empty", i, got)
		}
	}
}

func TestRunPipeline_RealPipelineOutput_ProducesIndexFields(t *testing.T) {
	requireTokenizerPool(t)

	cfg := mustLoadTaskRealIntegrationConfig(t)
	realDB := mustOpenTaskRealMySQL(t, cfg)
	if err := realDB.AutoMigrate(
		&entity.Tenant{},
		&entity.Knowledgebase{},
		&entity.Document{},
		&entity.File{},
		&entity.File2Document{},
	); err != nil {
		t.Fatalf("auto-migrate real mysql tables: %v", err)
	}

	realStorage, err := storage.NewMinioStorage(cfg.StorageEngine.Minio)
	if err != nil {
		t.Fatalf("connect real minio: %v", err)
	}

	origDB := dao.DB
	origStorage := storage.GetStorageFactory().GetStorage()
	dao.DB = realDB
	storage.GetStorageFactory().SetStorage(realStorage)
	t.Cleanup(func() {
		dao.DB = origDB
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
		cleanupTaskRealPipelineDocument(realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
	})

	pipelineOut, err := pipe.Run(context.Background(), map[string]any{
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

	if _, err := svc.processOutput(context.Background(), pipelineOut, time.Now()); err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}

	if len(inserted) != 1 {
		t.Fatalf("insert calls = %d, want 1", len(inserted))
	}
	chunks := inserted[0]
	if len(chunks) != 2 {
		t.Fatalf("inserted chunk count = %d, want 2", len(chunks))
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

func taskRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", "..", ".."))
}

func mustLoadTaskRealIntegrationConfig(t *testing.T) *server.Config {
	t.Helper()
	if err := common.Init("info", common.FileOutput{}, ""); err != nil {
		t.Fatalf("init common logger: %v", err)
	}
	server.SetLogger(zap.NewNop())
	configPath := filepath.Join(taskRepoRoot(t), "conf", "service_conf.yaml")
	if err := server.Init(configPath); err != nil {
		t.Fatalf("init service config from %s: %v", configPath, err)
	}
	cfg := server.GetConfig()
	if cfg == nil || cfg.Database.Host == "" || cfg.StorageEngine.Minio == nil || cfg.StorageEngine.Minio.Host == "" {
		t.Fatal("real integration config is incomplete")
	}
	return cfg
}

func mustOpenTaskRealMySQL(t *testing.T, cfg *server.Config) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		cfg.Database.Username,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Database,
		cfg.Database.Charset,
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("connect real mysql: %v", err)
	}
	return db
}

func disableTokenizerEmbeddingForTaskTemplate(t *testing.T, raw []byte) []byte {
	t.Helper()
	var tpl map[string]any
	if err := json.Unmarshal(raw, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	dsl, ok := tpl["dsl"].(map[string]any)
	if !ok {
		t.Fatalf("template dsl = %T, want map[string]any", tpl["dsl"])
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
	if err := db.Create(&entity.Tenant{
		ID:     tenantID,
		LLMID:  "gpt-4",
		Status: taskStrPtr("1"),
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
	if err := stg.Put(bucket, objectPath, content); err != nil {
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

func cleanupTaskRealPipelineDocument(db *gorm.DB, stg storage.Storage, tenantID, kbID, docID, fileID, bucket, objectPath string) {
	_ = db.Where("document_id = ?", docID).Delete(&entity.File2Document{}).Error
	_ = db.Where("id = ?", docID).Delete(&entity.Document{}).Error
	_ = db.Where("id = ?", fileID).Delete(&entity.File{}).Error
	_ = db.Where("id = ?", kbID).Delete(&entity.Knowledgebase{}).Error
	_ = db.Where("id = ?", tenantID).Delete(&entity.Tenant{}).Error
	_ = stg.Remove(bucket, objectPath)
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
