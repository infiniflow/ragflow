//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"gopkg.in/yaml.v3"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/entity"
	component "ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/ingestion/mediacache"
	"ragflow/internal/parser/media"
	"ragflow/internal/parser/parser"
	"ragflow/internal/server"
	"ragflow/internal/server/config"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
)

// TestHTMLMediaRealServices exercises the normal ingestion canvas using real
// MySQL, Kvrocks and MinIO. The source HTML contains a one-pixel PNG.
func TestHTMLMediaRealServices(t *testing.T) {
	if os.Getenv("RAGFLOW_REAL_HTML_MEDIA_TEST") != "1" {
		t.Skip("set RAGFLOW_REAL_HTML_MEDIA_TEST=1 with configured MySQL, Kvrocks and MinIO")
	}
	if common.GetEnv(common.EnvRAGFlowDictPath) == "" {
		const dictPath = "/usr/share/infinity/resource"
		if _, err := os.Stat(filepath.Join(dictPath, "rag", "huqie.txt")); err != nil {
			t.Fatalf("tokenizer resource: %v", err)
		}
		t.Setenv(common.EnvRAGFlowDictPath, dictPath)
	}
	if err := tokenizer.Init(&tokenizer.PoolConfig{DictPath: "/usr/share/infinity/resource", MinSize: 1, MaxSize: 2, IdleTimeout: 30 * time.Second, AcquireTimeout: 5 * time.Second}); err != nil {
		t.Fatalf("tokenizer initialization: %v", err)
	}
	cfg := loadHTMLMediaIntegrationConfig(t)
	db := mustOpenRealMySQL(t, cfg)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec("SET SESSION sql_mode = ''").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.UserTenant{}, &entity.Knowledgebase{}, &entity.Document{}, &entity.File{}, &entity.File2Document{}); err != nil {
		t.Fatal(err)
	}
	if err := kvrocks.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	kv := kvrocks.Get()
	stg, err := storage.NewMinioStorage(cfg.GetMinioConfig())
	if err != nil {
		t.Fatal(err)
	}
	readyCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for {
		_, err := stg.BucketExistsWithError(readyCtx, "ragflow-html-media-probe")
		if err == nil {
			break
		}
		select {
		case <-readyCtx.Done():
			t.Fatalf("MinIO S3 not ready: %v", err)
		case <-time.After(250 * time.Millisecond):
		}
	}
	originalDB := dao.DB
	originalStorage := storage.GetStorageFactory().GetStorage()
	originalResolver := component.ResolveDocumentStorageOverride
	dao.DB = db
	storage.GetStorageFactory().SetStorage(stg)
	component.ResolveDocumentStorageOverride = nil
	t.Cleanup(func() {
		dao.DB = originalDB
		storage.GetStorageFactory().SetStorage(originalStorage)
		component.ResolveDocumentStorageOverride = originalResolver
	})
	template, err := os.ReadFile(filepath.Join(repoRootFromPipelineTest(t), "internal/ingestion/pipeline/template/ingestion_pipeline_general.json"))
	if err != nil {
		t.Fatal(err)
	}
	template = disableTokenizerEmbeddingForTemplate(t, template)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID, kbID := limit32("it_tenant_"+suffix), s3SafeBucketName("it-kb-"+suffix)
	docID, fileID := limit32("it_doc_"+suffix), limit32("it_file_"+suffix)
	taskID := "it-html-" + suffix
	bucket := s3SafeBucketName(kbID)
	objectPath := "integration/" + taskID + "/image.html"
	const png64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="
	png, err := base64.StdEncoding.DecodeString(png64)
	if err != nil {
		t.Fatal(err)
	}
	html := `<p>HTML image fixture</p><img alt="sample figure" src="data:image/png;base64,` + png64 + `">`
	mustSeedRealPipelineDocument(t, db, stg, tenantID, kbID, docID, fileID, bucket, objectPath, "image.html", html)
	t.Cleanup(func() {
		_ = mediacache.PurgeTask(context.Background(), kv, taskID)
		cleanupRealPipelineDocument(db, stg, tenantID, kbID, docID, fileID, bucket, objectPath)
	})
	if err := db.Model(&entity.Document{}).Where("id = ?", docID).Updates(map[string]any{"type": "html", "suffix": ".html"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.File{}).Where("id = ?", fileID).Update("type", "html").Error; err != nil {
		t.Fatal(err)
	}
	pipe, err := NewPipelineFromDSL(template, taskID, WithDocumentID(docID), WithKV(kv))
	if err != nil {
		t.Fatal(err)
	}
	out, err := pipe.Run(t.Context(), map[string]any{"doc_id": docID, "kb_id": kbID, "tenant_id": tenantID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := stateFromRunOutput(t, out)
	docs, ok := state["Parser:HipSignsRhyme"]["json"].([]parser.DocItem)
	if !ok {
		t.Fatalf("parser output = %T", state["Parser:HipSignsRhyme"]["json"])
	}
	var ref *entity.MediaRef
	for _, item := range docs {
		m := item.ToMap()
		if m["media_ref"] != nil {
			ref = entity.MediaRefFromAny(m["media_ref"])
			if m["image"] != nil {
				t.Fatal("parser retained inline image")
			}
		}
	}
	if ref == nil {
		t.Fatal("parser did not emit media_ref")
	}
	cached, err := mediacache.New(kv, taskID).Get(t.Context(), *ref)
	if err != nil || !bytes.Equal(cached, png) {
		t.Fatalf("cached bytes differ: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(state["GeneralChunker:SixApplesFall"]["chunks"])
	if !ok {
		t.Fatal("missing chunker output")
	}
	foundImage := false
	for _, ck := range chunks {
		id, _ := ck["id"].(string)
		imgID, _ := ck["img_id"].(string)
		if imgID == "" {
			continue
		}
		if imgID != kbID+"-"+id || ck["media_ref"] != nil {
			t.Fatalf("invalid image chunk: %v", ck)
		}
		got, err := stg.Get(t.Context(), kbID, id)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = stg.Remove(context.Background(), kbID, id) })
		if !bytes.Equal(got, png) {
			t.Fatal("stored image does not match source")
		}
		foundImage = true
	}
	if !foundImage {
		t.Fatalf("chunker produced no uploaded image: chunks=%#v state=%#v", chunks, state["GeneralChunker:SixApplesFall"])
	}
	key := fmt.Sprintf("mc:media:%x:%s", xxhash.Sum64String(taskID), ref.SHA256)
	ttl, err := kv.GetClient().TTL(t.Context(), key).Result()
	if err != nil || ttl <= 23*time.Hour || ttl > mediacache.TTL {
		t.Fatalf("media TTL = %v, %v", ttl, err)
	}
	if err := kv.GetClient().Expire(t.Context(), key, time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err = mediacache.New(kv, taskID).Get(t.Context(), *ref)
		if errors.Is(err, media.ErrMediaNotFound) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatalf("media did not expire: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	retry := mediacache.New(kv, taskID)
	retryRef, err := retry.Put(t.Context(), "html", "image/png", png)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := retry.Get(t.Context(), retryRef); err != nil || !bytes.Equal(got, png) {
		t.Fatalf("retry cache bytes: %v", err)
	}
	if err := mediacache.PurgeTask(t.Context(), kv, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := retry.Get(t.Context(), retryRef); !errors.Is(err, media.ErrMediaNotFound) {
		t.Fatalf("purged cache read: %v", err)
	}
}

// loadHTMLMediaIntegrationConfig uses the CI service addresses when set.
func loadHTMLMediaIntegrationConfig(t *testing.T) *config.Config {
	t.Helper()
	kvAddr := os.Getenv("RAGFLOW_TEST_KVROCKS_ADDR")
	if kvAddr == "" {
		return mustLoadRealIntegrationConfig(t)
	}
	cfg := map[string]any{
		"general": map[string]any{"cache_engine": "kvrocks"},
		"mysql":   map[string]any{"host": os.Getenv("RAGFLOW_MYSQL_HOST"), "port": os.Getenv("RAGFLOW_MYSQL_PORT"), "password": os.Getenv("MYSQL_PASSWORD")},
		"minio":   map[string]any{"host": os.Getenv("RAGFLOW_MINIO_HOST"), "user": os.Getenv("MINIO_USER"), "password": os.Getenv("MINIO_PASSWORD")},
		"kvrocks": map[string]any{"host": kvAddr, "password": os.Getenv("REDIS_PASSWORD"), "db": 1},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "service_conf.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := common.InitLogger("info", common.FileOutput{}, ""); err != nil {
		t.Fatal(err)
	}
	if err := server.Init(path); err != nil {
		t.Fatal(err)
	}
	return server.GetConfig()
}
