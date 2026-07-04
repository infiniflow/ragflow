//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"testing"
	"time"

	_ "ragflow/internal/ingestion/component"
	componentpkg "ragflow/internal/ingestion/component"
	_ "ragflow/internal/ingestion/component/chunker"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
)

type fixedEmbedder struct{}

func (fixedEmbedder) Encode(texts []string) ([][]float64, error) {
	out := make([][]float64, 0, len(texts))
	for _, text := range texts {
		out = append(out, []float64{float64(len(text)), 1, 2, 3})
	}
	return out, nil
}

func TestPipelineRun_TemplateOne_RealComponents(t *testing.T) {
	requireTokenizerPool(t)

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_one.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:FrankWeeksListen" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:FrankWeeksListen]", terminalIDs)
	}

	origStorage := storage.GetStorageFactory().GetStorage()
	mem := storage.NewMemoryStorage()
	storage.GetStorageFactory().SetStorage(mem)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(origStorage) })

	origEncode := componentpkg.EncodeFunc
	componentpkg.EncodeFunc = func(_, _ string) componentpkg.Embedder { return fixedEmbedder{} }
	t.Cleanup(func() { componentpkg.EncodeFunc = origEncode })

	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-one.txt"
		filename = "template-one.txt"
	)
	content := "Alpha paragraph.\n\nBeta paragraph."
	if err := mem.Put(bucket, path, []byte(content)); err != nil {
		t.Fatalf("seed storage: %v", err)
	}

	pipe, err := NewPipelineFromDSL(templateBytes, "template-one-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	out, err := pipe.Run(context.Background(), map[string]any{
		"bucket": bucket,
		"path":   path,
		"files":  []map[string]any{{"name": filename}},
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
	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	wantTexts := []string{"Alpha paragraph.", "Beta paragraph."}
	wantMergedText := "Alpha paragraph.\nBeta paragraph."
	wantTokens := 0
	if got := chunks[0]["text"]; got != wantMergedText {
		t.Fatalf("chunks[0].text = %v, want %q", got, wantMergedText)
	}
	if _, ok := chunks[0]["content_ltks"].(string); !ok || chunks[0]["content_ltks"] == "" {
		t.Fatalf("chunks[0].content_ltks missing or empty: %v", chunks[0]["content_ltks"])
	}
	if _, ok := chunks[0]["content_sm_ltks"].(string); !ok || chunks[0]["content_sm_ltks"] == "" {
		t.Fatalf("chunks[0].content_sm_ltks missing or empty: %v", chunks[0]["content_sm_ltks"])
	}
	vec := floatSliceFromAny(t, chunks[0]["q_4_vec"])
	if len(vec) != 4 {
		t.Fatalf("chunks[0].q_4_vec len = %d, want 4", len(vec))
	}
	if got := vec[0]; got != float64(len(wantMergedText)) {
		t.Fatalf("chunks[0].q_4_vec[0] = %v, want %v", got, float64(len(wantMergedText)))
	}
	wantTokens = tokenizer.NumTokensFromString(wantMergedText)
	if got := payload["embedding_token_consumption"]; got != wantTokens {
		t.Fatalf("embedding_token_consumption = %v, want %d", got, wantTokens)
	}

	state, ok := out["state"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("state = %T, want map[string]map[string]any", out["state"])
	}
	fileState, ok := state["File"]
	if !ok {
		t.Fatal("missing File state")
	}
	if got := fileState["name"]; got != filename {
		t.Fatalf("file state name = %v, want %q", got, filename)
	}
	if got := fileState["bucket"]; got != bucket {
		t.Fatalf("file state bucket = %v, want %q", got, bucket)
	}
	if got := fileState["path"]; got != path {
		t.Fatalf("file state path = %v, want %q", got, path)
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
	for i, wantText := range wantTexts {
		item := jsonItems[i]
		if got := item["text"]; got != wantText {
			t.Fatalf("parser json[%d].text = %v, want %q", i, got, wantText)
		}
	}
	chunkerState, ok := state["TokenChunker:DryDrinksVisit"]
	if !ok {
		t.Fatal("missing TokenChunker:DryDrinksVisit state")
	}
	if got := chunkerState["output_format"]; got != "chunks" {
		t.Fatalf("chunker output_format = %v, want chunks", got)
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != 1 {
		t.Fatalf("chunker chunks = %T/%v, want 1 item", chunkerState["chunks"], chunkerState["chunks"])
	}
	chunkerChunk := chunkerChunks[0]
	if got := chunkerChunk["text"]; got != wantMergedText {
		t.Fatalf("chunker chunk[0].text = %v, want %q", got, wantMergedText)
	}
	if got := chunkerChunk["doc_type_kwd"]; got != "text" {
		t.Fatalf("chunker chunk[0].doc_type_kwd = %v, want text", got)
	}
}

func TestPipelineRun_AllIngestionTemplates_RealComponentsSmoke(t *testing.T) {
	requireTokenizerPool(t)

	origStorage := storage.GetStorageFactory().GetStorage()
	mem := storage.NewMemoryStorage()
	storage.GetStorageFactory().SetStorage(mem)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(origStorage) })

	origEncode := componentpkg.EncodeFunc
	componentpkg.EncodeFunc = func(_, _ string) componentpkg.Embedder { return fixedEmbedder{} }
	t.Cleanup(func() { componentpkg.EncodeFunc = origEncode })

	const (
		bucket = "test-bucket"
		path   = "fixtures/template-smoke.md"
	)
	content := "# Title\n\nIntro paragraph.\n\n## Section\n\nBody paragraph."
	if err := mem.Put(bucket, path, []byte(content)); err != nil {
		t.Fatalf("seed storage: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_*.json"))
	if err != nil {
		t.Fatalf("glob templates: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no ingestion templates found")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			templateBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			if templateUsesComponent(t, templateBytes, "Extractor") {
				t.Skip("template includes real Extractor and requires model credentials; covered separately from File/Parser/Chunker/Tokenizer e2e")
			}
			terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
			if len(terminalIDs) != 1 {
				t.Fatalf("terminal ids = %v, want exactly 1 terminal", terminalIDs)
			}
			pipe, err := NewPipelineFromDSL(templateBytes, filepath.Base(file))
			if err != nil {
				t.Fatalf("NewPipelineFromDSL: %v", err)
			}
			out, err := pipe.Run(context.Background(), map[string]any{
				"bucket": bucket,
				"path":   path,
				"files":  []map[string]any{{"name": "template-smoke.md"}},
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
			if got := payload["output_format"]; got != "chunks" {
				t.Fatalf("output_format = %v, want chunks", got)
			}
			chunks, ok := payload["chunks"].([]map[string]any)
			if !ok || len(chunks) == 0 {
				t.Fatalf("chunks = %T/%v, want non-empty []map[string]any", payload["chunks"], payload["chunks"])
			}
		})
	}
}

func repoRootFromPipelineTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func requireTokenizerPool(t *testing.T) {
	t.Helper()
	if tokenizer.IsInitialized() {
		return
	}
	cfg := &tokenizer.PoolConfig{
		DictPath:       os.Getenv("RAGFLOW_DICT_PATH"),
		MinSize:        1,
		MaxSize:        2,
		IdleTimeout:    30 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}
	if cfg.DictPath == "" {
		cfg.DictPath = "/usr/share/infinity/resource"
	}
	if err := tokenizer.Init(cfg); err != nil {
		t.Skipf("tokenizer pool init failed: %v", err)
	}
}

func floatSliceFromAny(t *testing.T, v any) []float64 {
	t.Helper()
	switch x := v.(type) {
	case []float64:
		return x
	case []any:
		out := make([]float64, 0, len(x))
		for i, item := range x {
			f, ok := item.(float64)
			if !ok {
				t.Fatalf("vector item %d = %T, want float64", i, item)
			}
			out = append(out, f)
		}
		return out
	default:
		t.Fatalf("vector = %T, want []float64 or []any", v)
		return nil
	}
}

func terminalPayloadFromRunOutput(t *testing.T, out map[string]any, terminalID string) map[string]any {
	t.Helper()
	if out == nil {
		t.Fatal("Run returned nil output")
	}
	if _, ok := out["output_format"]; ok {
		return out
	}
	if terminalID == "" {
		t.Fatal("terminalID is empty")
	}
	nested, ok := out[terminalID].(map[string]any)
	if !ok {
		t.Fatalf("run output missing terminal payload %q in %v", terminalID, out)
	}
	return nested
}

func terminalComponentIDsFromTemplate(t *testing.T, raw []byte) []string {
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
	var terminals []string
	for id, rawComp := range components {
		comp, ok := rawComp.(map[string]any)
		if !ok {
			t.Fatalf("component %q = %T, want map[string]any", id, rawComp)
		}
		switch ds := comp["downstream"].(type) {
		case nil:
			terminals = append(terminals, id)
		case []any:
			if len(ds) == 0 {
				terminals = append(terminals, id)
			}
		case []string:
			if len(ds) == 0 {
				terminals = append(terminals, id)
			}
		default:
			t.Fatalf("component %q downstream = %T, want []any/[]string/nil", id, comp["downstream"])
		}
	}
	sort.Strings(terminals)
	return terminals
}

func templateUsesComponent(t *testing.T, raw []byte, componentName string) bool {
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
	for id, rawComp := range components {
		comp, ok := rawComp.(map[string]any)
		if !ok {
			t.Fatalf("component %q = %T, want map[string]any", id, rawComp)
		}
		obj, ok := comp["obj"].(map[string]any)
		if !ok {
			t.Fatalf("component %q obj = %T, want map[string]any", id, comp["obj"])
		}
		name, _ := obj["component_name"].(string)
		if name == componentName {
			return true
		}
	}
	return false
}

func TestTemplateFixtures_AreWrappedTemplates(t *testing.T) {
	path := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_one.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var tpl map[string]any
	if err := json.Unmarshal(raw, &tpl); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if _, ok := tpl["dsl"].(map[string]any); !ok {
		t.Fatalf("fixture dsl = %T, want map[string]any", tpl["dsl"])
	}
}
