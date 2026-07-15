//go:build integration
// +build integration

package pipeline

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	componentpkg "ragflow/internal/ingestion/component"
	_ "ragflow/internal/ingestion/component/chunker"
	"ragflow/internal/server"
	"ragflow/internal/storage"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPipelineRun_TemplateGeneral_RealMySQLMinIO_OutputShape(t *testing.T) {
	prepareTokenizerResourceForIntegration(t)
	RequireTokenizerPool(t)

	cfg := mustLoadRealIntegrationConfig(t)
	realDB := mustOpenRealMySQL(t, cfg)
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
	origDocResolver := componentpkg.ResolveDocumentStorageOverride
	dao.DB = realDB
	storage.GetStorageFactory().SetStorage(realStorage)
	componentpkg.ResolveDocumentStorageOverride = nil
	t.Cleanup(func() {
		dao.DB = origDB
		storage.GetStorageFactory().SetStorage(origStorage)
		componentpkg.ResolveDocumentStorageOverride = origDocResolver
	})

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_general.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	templateBytes = disableTokenizerEmbeddingForTemplate(t, templateBytes)
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:LegalReadersDecide" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:LegalReadersDecide]", terminalIDs)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := limit32("it_tenant_" + suffix)
	kbID := limit32("it_kb_" + suffix)
	docID := limit32("it_doc_" + suffix)
	fileID := limit32("it_file_" + suffix)
	bucket := s3SafeBucketName(kbID)
	objectPath := fmt.Sprintf("integration/pipeline/%s/template-general.txt", docID)
	docName := "template-general.txt"
	content := "Alpha paragraph.\n\nBeta paragraph."

	mustSeedRealPipelineDocument(t, realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath, docName, content)
	t.Cleanup(func() {
		cleanupRealPipelineDocument(realDB, realStorage, tenantID, kbID, docID, fileID, bucket, objectPath)
	})

	pipe, err := NewPipelineFromDSL(templateBytes, "template-general-real-mysql-minio")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}

	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	wantChunkTexts := []string{"Alpha paragraph.", "Beta paragraph."}
	if len(chunks) != len(wantChunkTexts) {
		t.Fatalf("len(chunks) = %d, want %d", len(chunks), len(wantChunkTexts))
	}
	for i, wantText := range wantChunkTexts {
		if got := chunks[i]["text"]; got != wantText {
			t.Fatalf("chunks[%d].text = %v, want %q", i, got, wantText)
		}
		if got, ok := chunks[i]["text"].(string); !ok || got == "" {
			t.Fatalf("chunks[%d].text type/value = %T/%v, want non-empty string", i, chunks[i]["text"], chunks[i]["text"])
		}
		if _, hasVec := chunks[i]["q_4_vec"]; hasVec {
			t.Fatalf("chunks[%d] unexpectedly contains vector field q_4_vec after embedding-disabled template", i)
		}
	}
	if _, ok := payload["embedding_token_consumption"]; ok {
		t.Fatalf("embedding_token_consumption should be absent when tokenizer search_method excludes embedding: %v", payload["embedding_token_consumption"])
	}

	state := stateFromRunOutput(t, out)
	fileState, ok := state["File"]
	if !ok {
		t.Fatal("missing File state")
	}
	if got := fileState["name"]; got != docName {
		t.Fatalf("file state name = %v, want %q", got, docName)
	}
	if _, ok := fileState["bucket"]; ok {
		t.Fatalf("file state should not expose bucket on doc_id path: %v", fileState["bucket"])
	}
	if _, ok := fileState["path"]; ok {
		t.Fatalf("file state should not expose path on doc_id path: %v", fileState["path"])
	}

	parserState, ok := state["Parser:HipSignsRhyme"]
	if !ok {
		t.Fatal("missing Parser:HipSignsRhyme state")
	}
	if got := parserState["output_format"]; got != "json" {
		t.Fatalf("parser output_format = %v, want json", got)
	}
	jsonItems, ok := parserState["json"].([]map[string]any)
	if !ok || len(jsonItems) != 2 {
		t.Fatalf("parser json = %T/%v, want 2 items", parserState["json"], parserState["json"])
	}
	for i, wantText := range wantChunkTexts {
		if got := jsonItems[i]["text"]; got != wantText {
			t.Fatalf("parser json[%d].text = %v, want %q", i, got, wantText)
		}
	}

	chunkerState, ok := state["TokenChunker:SixApplesFall"]
	if !ok {
		t.Fatal("missing TokenChunker:SixApplesFall state")
	}
	if got := chunkerState["output_format"]; got != "chunks" {
		t.Fatalf("chunker output_format = %v, want chunks", got)
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != len(wantChunkTexts) {
		t.Fatalf("chunker chunks = %T/%v, want %d items", chunkerState["chunks"], chunkerState["chunks"], len(wantChunkTexts))
	}
	for i, wantText := range wantChunkTexts {
		if got := chunkerChunks[i]["text"]; got != wantText {
			t.Fatalf("chunker chunk[%d].text = %v, want %q", i, got, wantText)
		}
		if got := chunkerChunks[i]["doc_type_kwd"]; got != "text" {
			t.Fatalf("chunker chunk[%d].doc_type_kwd = %v, want text", i, got)
		}
	}
}

func mustLoadRealIntegrationConfig(t *testing.T) *server.Config {
	t.Helper()
	server.SetLogger(zap.NewNop())
	configPath := filepath.Join(repoRootFromPipelineTest(t), "conf", "service_conf.yaml")
	if err := server.Init(configPath); err != nil {
		t.Fatalf("init service config from %s: %v", configPath, err)
	}
	cfg := server.GetConfig()
	if cfg == nil || cfg.Database.Host == "" || cfg.StorageEngine.Minio == nil || cfg.StorageEngine.Minio.Host == "" {
		t.Fatal("real integration config is incomplete")
	}
	return cfg
}

func prepareTokenizerResourceForIntegration(t *testing.T) {
	t.Helper()
	if common.GetEnv(common.EnvRAGFlowDictPath) != "" {
		return
	}
	const systemDictPath = "/usr/share/infinity/resource"
	if _, err := os.Stat(filepath.Join(systemDictPath, "rag", "huqie.txt")); err != nil {
		t.Skipf("system tokenizer resource not found at %s: %v", systemDictPath, err)
	}
	if err := os.Setenv(common.EnvRAGFlowDictPath, systemDictPath); err != nil {
		t.Fatalf("set RAGFLOW_DICT_PATH=%s: %v", systemDictPath, err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv(common.EnvRAGFlowDictPath)
	})
}

func mustSymlink(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.Symlink(src, dst); err != nil {
		t.Fatalf("symlink tokenizer resource %s -> %s: %v", src, dst, err)
	}
}

func mustWriteTokenizerPOSDef(t *testing.T, dictPath, outPath string) {
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

func mustPrepareTokenizerWordNet(t *testing.T, root string) {
	t.Helper()
	zipPath := filepath.Join(repoRootFromPipelineTest(t), "ragflow_deps", "nltk_data", "corpora", "wordnet.zip")
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Skipf("open wordnet zip %s: %v", zipPath, err)
		return
	}
	defer func() {
		_ = reader.Close()
	}()
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

func mustPrepareTokenizerOpenCC(t *testing.T, root string) {
	t.Helper()
	const systemOpenCC = "/usr/share/opencc"
	if _, err := os.Stat(systemOpenCC); err != nil {
		t.Skipf("system opencc dir %s not found: %v", systemOpenCC, err)
		return
	}
	mustSymlink(t, systemOpenCC, filepath.Join(root, "opencc"))
}

func mustOpenRealMySQL(t *testing.T, cfg *server.Config) *gorm.DB {
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
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("connect real mysql: %v", err)
	}
	return db
}

func disableTokenizerEmbeddingForTemplate(t *testing.T, raw []byte) []byte {
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
		if !ok {
			continue
		}
		if obj["component_name"] != "Tokenizer" {
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

func mustSeedRealPipelineDocument(
	t *testing.T,
	db *gorm.DB,
	stg storage.Storage,
	tenantID, kbID, docID, fileID, bucket, objectPath, docName, content string,
) {
	t.Helper()
	if err := db.Create(&entity.Tenant{
		ID:     tenantID,
		LLMID:  "gpt-4",
		Status: strPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:           kbID,
		TenantID:     tenantID,
		EmbdID:       "embd-1",
		ParserConfig: entity.JSONMap{},
		Status:       strPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if err := stg.Put(bucket, objectPath, []byte(content)); err != nil {
		t.Fatalf("put real minio object: %v", err)
	}
	if err := db.Create(&entity.File{
		ID:         fileID,
		ParentID:   bucket,
		TenantID:   tenantID,
		CreatedBy:  tenantID,
		Name:       docName,
		Type:       "txt",
		Location:   strPtr(objectPath),
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
		Type:         "txt",
		CreatedBy:    tenantID,
		Name:         strPtr(docName),
		Location:     strPtr(objectPath),
		Suffix:       ".txt",
		Status:       strPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	if err := db.Create(&entity.File2Document{
		ID:         limit32("it_map_" + docID),
		FileID:     strPtr(fileID),
		DocumentID: strPtr(docID),
	}).Error; err != nil {
		t.Fatalf("create file2document: %v", err)
	}
}

func cleanupRealPipelineDocument(db *gorm.DB, stg storage.Storage, tenantID, kbID, docID, fileID, bucket, objectPath string) {
	_ = db.Where("document_id = ?", docID).Delete(&entity.File2Document{}).Error
	_ = db.Where("id = ?", docID).Delete(&entity.Document{}).Error
	_ = db.Where("id = ?", fileID).Delete(&entity.File{}).Error
	_ = db.Where("id = ?", kbID).Delete(&entity.Knowledgebase{}).Error
	_ = db.Where("id = ?", tenantID).Delete(&entity.Tenant{}).Error
	_ = stg.Remove(bucket, objectPath)
}

func strPtr(s string) *string {
	return &s
}

func limit32(s string) string {
	if len(s) <= 32 {
		return s
	}
	return s[:32]
}

func s3SafeBucketName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	return s
}
