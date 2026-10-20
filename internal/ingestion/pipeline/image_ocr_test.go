// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pipeline

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/chunker"
)

type imageOCREmbedder struct{ texts []string }

// MaxTokens gives the fixture enough room for the invoice's OCR text.
func (*imageOCREmbedder) MaxTokens() int { return 2048 }

// BatchSize bounds the embedding fixture's request size.
func (*imageOCREmbedder) BatchSize() int { return 16 }

// Encode captures the exact indexed text and supplies deterministic vectors.
func (e *imageOCREmbedder) Encode(_ context.Context, texts []string) ([]component.EmbeddingResult, error) {
	e.texts = append(e.texts, texts...)
	out := make([]component.EmbeddingResult, len(texts))
	for i := range out {
		out[i] = component.EmbeddingResult{Vector: []float64{0.2, 0.8}, TokenCount: 8}
	}
	return out, nil
}

// TestImageOCRSavedConfigToSearchableChunk runs the real config cleaner, SQLite
// persistence, resolver, OCR driver, Parser, decorated OneChunker and Tokenizer.
// Only OCR inference, object storage and embedding inference are test doubles.
func TestImageOCRSavedConfigToSearchableChunk(t *testing.T) {
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 12, 12))); err != nil {
		t.Fatal(err)
	}
	const text = "Invoice INV-21030 total 42."
	seen := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/layout-parsing" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Errorf("wrong selected instance request: %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var request struct {
			File      string `json:"file"`
			FileType  int    `json:"fileType"`
			Algorithm string `json:"algorithm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.FileType != 1 || request.Algorithm != "PP-OCRv6" {
			t.Errorf("not the selected image model: %+v", request)
		}
		data, _ := base64.StdEncoding.DecodeString(request.File)
		seen <- data
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"errorCode":0,"result":{"layoutParsingResults":[{"markdown":{"text":%q}}]}}`, text)
	}))
	defer server.Close()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}); err != nil {
		t.Fatal(err)
	}
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	template, ok := registry.Get("picture")
	if !ok {
		t.Fatal("picture template missing")
	}
	dsl, err := json.Marshal(template.DSL)
	if err != nil {
		t.Fatal(err)
	}
	const parserID = "Parser:ViewsCaptureLight"
	config := BuildParserConfig(dsl, map[string]any{parserID: map[string]any{
		"image":                     map[string]any{"ocr_enabled": true, "ocr_model_id": "ocr-model", "output_format": "json"},
		"enable_vision_enhancement": false,
	}})
	for _, row := range []any{
		&entity.Knowledgebase{ID: "kb-1", TenantID: "tenant-1", Name: "Image OCR fixture", ParserConfig: config},
		&entity.TenantModelProvider{ID: "ocr-provider", TenantID: "tenant-1", ProviderName: "PaddleOCR.local"},
		&entity.TenantModelInstance{ID: "ocr-instance", ProviderID: "ocr-provider", InstanceName: "selected",
			Status: "active", APIKey: "fixture-key", Extra: fmt.Sprintf(`{"base_url":%q}`, server.URL)},
		&entity.TenantModel{ID: "ocr-model", ProviderID: "ocr-provider", InstanceID: "ocr-instance",
			ModelName: "PP-OCRv6", Status: "active", ModelType: int(entity.ModelTypeOCR)},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	var saved entity.Knowledgebase
	if err := db.Where("id = ?", "kb-1").Take(&saved).Error; err != nil {
		t.Fatal(err)
	}
	params := saved.ParserConfig[parserID].(map[string]any)
	if params["image"].(map[string]any)["ocr_model_id"] != "ocr-model" {
		t.Fatal("image model selection did not survive save/readback")
	}
	parserComponent, err := component.NewParserComponent(params)
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{"name": "invoice.png", "file_type": "image", "binary": imageBytes.Bytes(),
		"tenant_id": "tenant-1", "kb_id": "kb-1", "doc_id": "doc-1"}
	parsed, err := parserComponent.Invoke(t.Context(), db, inputs)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-seen:
		if !bytes.Equal(data, imageBytes.Bytes()) {
			t.Fatal("OCR received different image bytes")
		}
	default:
		t.Fatal("OCR model not invoked")
	}
	factory, _, _, ok := runtime.DefaultRegistry.Lookup("OneChunker")
	if !ok {
		t.Fatal("OneChunker not registered")
	}
	chunkComponent, err := factory("OneChunker", nil)
	if err != nil {
		t.Fatal(err)
	}
	originalUploader := chunker.ChunkImageUploader
	var uploaded []byte
	chunker.ChunkImageUploader = func(_ context.Context, kbID, chunkID string, data []byte) (string, error) {
		uploaded = append([]byte(nil), data...)
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { chunker.ChunkImageUploader = originalUploader })
	for key, value := range parsed {
		inputs[key] = value
	}
	chunked, err := chunkComponent.Invoke(t.Context(), db, inputs)
	if err != nil {
		t.Fatal(err)
	}
	embedder := &imageOCREmbedder{}
	tokenizer, err := component.NewTokenizerComponentWithResolver(map[string]any{
		"search_method": []string{"embedding"}, "filename_embd_weight": 0,
	}, func(context.Context, string, string) (component.Embedder, string, error) {
		return embedder, "fixture-embedding", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range chunked {
		inputs[key] = value
	}
	indexable, err := tokenizer.Invoke(t.Context(), db, inputs)
	if err != nil {
		t.Fatal(err)
	}
	chunks := indexable["chunks"].([]map[string]any)
	if len(chunks) != 1 || chunks[0]["text"] != text {
		t.Fatalf("searchable OCR text was lost: %+v", chunks)
	}
	if !reflect.DeepEqual(embedder.texts, []string{"invoice.png", text}) || !bytes.Equal(uploaded, imageBytes.Bytes()) {
		t.Fatalf("embedding input=%q image preserved=%v", embedder.texts, bytes.Equal(uploaded, imageBytes.Bytes()))
	}
	if id, _ := chunks[0]["img_id"].(string); !strings.HasPrefix(id, "kb-1-") {
		t.Fatalf("indexed image reference lost: %+v", chunks[0])
	}
	if _, ok := chunks[0]["q_2_vec"]; !ok {
		t.Fatalf("embedding vector missing: %+v", chunks[0])
	}
}
