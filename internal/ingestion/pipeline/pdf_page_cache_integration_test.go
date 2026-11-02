//go:build cgo && integration

package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	doctype "ragflow/internal/deepdoc/parser/type"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	nativeanalyzer "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	"ragflow/internal/engine/kvrocks"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/ingestion/mediacache"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
)

func TestPDFPageCacheRealServices(t *testing.T) {
	cfg := loadMediaIntegrationConfig(t)
	if err := kvrocks.Init(t.Context()); err != nil {
		t.Fatal(err)
	}
	kv := kvrocks.Get()
	stg, err := storage.NewMinioStorage(cfg.GetMinioConfig())
	if err != nil {
		t.Fatal(err)
	}
	oldStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(stg)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(oldStorage) })
	prepareTokenizerResourceForIntegration(t)
	if err := tokenizer.Init(&tokenizer.PoolConfig{DictPath: common.GetEnv(common.EnvRAGFlowDictPath), MinSize: 1, MaxSize: 2, IdleTimeout: time.Minute, AcquireTimeout: 5 * time.Second}); err != nil {
		t.Fatal(err)
	}
	scope := fmt.Sprintf("it-pdf-pages-%d", time.Now().UnixNano())
	bucket := fmt.Sprintf("it-pdf-pages-%d", time.Now().UnixNano())
	ctx := runtime.WithState(t.Context(), runtime.NewCanvasState("", scope))
	globals.SetTaskID(ctx, scope)
	globals.SeedIngestionGlobals(ctx, map[string]any{"kb_id": bucket, "doc_id": scope})
	cache := mediacache.NewPageStore(kv, scope)
	ctx = deepdocpdf.WithPageCache(ctx, cache)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := mediacache.PurgeTask(cleanupCtx, kv, scope); err != nil {
			t.Error(err)
		}
	})
	source, err := os.ReadFile(filepath.Join(repoRootFromPipelineTest(t), "test/benchmark/test_docs/Doc1.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	modelDir := os.Getenv("DEEPDOC_MODEL_DIR")
	if modelDir == "" {
		modelDir = filepath.Join(repoRootFromPipelineTest(t), "internal/rag/res/deepdoc")
	}
	if err := nativeanalyzer.Register(modelDir, 0.5); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RAGFLOW_PDF_PAGE_CACHE", "1")
	dsl := []byte(`{"dsl":{"components":{
  "begin":{"obj":{"component_name":"Begin","params":{}},"downstream":["Parser:PDF"]},
  "Parser:PDF":{"obj":{"component_name":"Parser","params":{"pdf":{"parse_method":"deepdoc","pages":[[1,1]]}}},"upstream":["begin"],"downstream":["GeneralChunker:PDF"]},
  "GeneralChunker:PDF":{"obj":{"component_name":"GeneralChunker","params":{"chunk_token_size":256}},"upstream":["Parser:PDF"]}
 },"path":["begin","Parser:PDF","GeneralChunker:PDF"],"graph":{"nodes":[]}}}`)
	pipe, err := NewPipelineFromDSL(dsl, scope, WithKV(kv))
	if err != nil {
		t.Fatal(err)
	}
	run, err := pipe.Run(t.Context(), map[string]any{"binary": source, "name": "cached.pdf", "kb_id": bucket, "doc_id": scope}, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := stateFromRunOutput(t, run)
	parsed := state["Parser:PDF"]
	for _, stage := range state {
		if chunks, ok := schema.ChunkCarrierToMaps(stage["chunks"]); ok {
			for _, chunk := range chunks {
				if id, ok := chunk["id"].(string); ok {
					t.Cleanup(func() { _ = stg.Remove(context.Background(), bucket, id) })
				}
			}
		}
	}

	document, ok := parsed["pdf_pages"].(*doctype.PageDocument)
	if !ok || len(document.Pages) != 1 {
		t.Fatalf("page metadata: %#v", parsed["pdf_pages"])
	}
	ref, ok := document.Page(0)
	if !ok {
		t.Fatal("first page is absent")
	}
	page, err := cache.Get(ctx, ref)
	if err != nil || page == nil {
		t.Fatalf("cached page: %v", err)
	}
	factory, _, _, ok := runtime.DefaultRegistry.Lookup("GeneralChunker")
	if !ok {
		t.Fatal("GeneralChunker is not registered")
	}
	chunker, err := factory("page-cache-test", map[string]any{"chunk_token_size": 256})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint map[string]any
	if err := json.Unmarshal(encoded, &checkpoint); err != nil {
		t.Fatal(err)
	}
	checkpoint["bucket"] = "must-not-fetch-source"
	checkpoint["path"] = "missing.pdf"
	out, err := chunker.Invoke(ctx, nil, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok || len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	images := 0
	cachedImages := make(map[string][]byte)
	for _, chunk := range chunks {
		id, _ := chunk["id"].(string)
		imgID, _ := chunk["img_id"].(string)
		if imgID == "" {
			continue
		}
		if imgID != bucket+"-"+id {
			t.Fatalf("invalid img_id: %q", imgID)
		}
		data, err := stg.Get(ctx, bucket, id)
		if err != nil || !bytes.HasPrefix(data, []byte{137, 80, 78, 71}) {
			t.Fatalf("stored PNG: %v", err)
		}
		images++
		cachedImages[id] = data
		t.Cleanup(func() { _ = stg.Remove(context.Background(), bucket, id) })
	}
	if images == 0 {
		t.Fatal("no uploaded previews")
	}
	if err := stg.Put(ctx, bucket, "source.pdf", source); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stg.Remove(context.Background(), bucket, "source.pdf") })
	baselineCtx := runtime.WithState(t.Context(), runtime.NewCanvasState("", scope))
	globals.SetTaskID(baselineCtx, scope)
	globals.SeedIngestionGlobals(baselineCtx, map[string]any{"kb_id": bucket, "doc_id": scope})
	baselineInput := make(map[string]any, len(checkpoint))
	for key, value := range checkpoint {
		baselineInput[key] = value
	}
	delete(baselineInput, "pdf_pages")
	baselineInput["bucket"] = bucket
	baselineInput["path"] = "source.pdf"
	for id := range cachedImages {
		if err := stg.Remove(ctx, bucket, id); err != nil {
			t.Fatal(err)
		}
	}
	baseline, err := chunker.Invoke(baselineCtx, nil, baselineInput)
	if err != nil {
		t.Fatal(err)
	}
	baselineChunks, ok := schema.ChunkCarrierToMaps(baseline["chunks"])
	if !ok || len(baselineChunks) != len(chunks) {
		t.Fatal("cached and native chunk counts differ")
	}
	for id, expected := range cachedImages {
		got, err := stg.Get(ctx, bucket, id)
		if err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("cached and native preview differ: %s: %v", id, err)
		}
	}
	if err := mediacache.PurgeTask(ctx, kv, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(ctx, ref); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("purge did not remove page: %v", err)
	}
	if _, err := chunker.Invoke(ctx, nil, checkpoint); !errors.Is(err, deepdocpdf.ErrPageCache) {
		t.Fatalf("cache miss was not fatal: %v", err)
	}
	t.Logf("real Kvrocks page handoff and MinIO previews verified: pages=%d chunks=%d images=%d", len(document.Pages), len(chunks), images)
}
