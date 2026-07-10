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

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/entity"
)

func findAgentTemplatesDir() string {
	candidates := []string{
		"agent/templates",
		filepath.Join("..", "..", "agent", "templates"),
		filepath.Join("..", "..", "..", "agent", "templates"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func readTestTemplate(filename string) ([]byte, error) {
	dir := findAgentTemplatesDir()
	return os.ReadFile(filepath.Join(dir, filename))
}

func TestProcessTemplateDSL_NaiveTokenChunker(t *testing.T) {
	raw, err := readTestTemplate("ingestion_pipeline_general.json")
	if err != nil {
		t.Skipf("template file not found: %v", err)
	}

	parserConfig := entity.JSONMap{
		"chunk_token_num":    256,
		"overlapped_percent": float64(0.2),
		"delimiters":         []any{"\n", "。"},
	}

	dslBytes, err := processTemplateDSL(raw, parserConfig)
	if err != nil {
		t.Fatalf("processTemplateDSL: %v", err)
	}

	var dsl map[string]any
	if err := json.Unmarshal(dslBytes, &dsl); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	comps, ok := dsl["components"].(map[string]any)
	if !ok {
		t.Fatal("components is not a map")
	}
	for _, name := range []string{"File", "Parser:HipSignsRhyme", "TokenChunker:SixApplesFall", "Tokenizer:LegalReadersDecide"} {
		if _, ok := comps[name]; !ok {
			t.Errorf("missing component %q", name)
		}
	}

	chunker := comps["TokenChunker:SixApplesFall"].(map[string]any)
	chunkerObj := chunker["obj"].(map[string]any)
	chunkerParams := chunkerObj["params"].(map[string]any)
	if cts, _ := chunkerParams["chunk_token_size"].(float64); int(cts) != 256 {
		t.Errorf("chunk_token_size = %v, want 256", cts)
	}
	if op, _ := chunkerParams["overlapped_percent"].(float64); op != 0.2 {
		t.Errorf("overlapped_percent = %v, want 0.2", op)
	}

	graph := dsl["graph"].(map[string]any)
	nodes := graph["nodes"].([]any)
	var found bool
	for _, n := range nodes {
		nm := n.(map[string]any)
		if nm["id"] == "TokenChunker:SixApplesFall" {
			found = true
			data := nm["data"].(map[string]any)
			form := data["form"].(map[string]any)
			if cts, _ := form["chunk_token_size"].(float64); int(cts) != 256 {
				t.Errorf("form chunk_token_size = %v, want 256", cts)
			}
			if op, _ := form["overlapped_percent"].(float64); op != 0.2 {
				t.Errorf("form overlapped_percent = %v, want 0.2", op)
			}
		}
	}
	if !found {
		t.Error("TokenChunker:SixApplesFall not found in graph nodes")
	}
}

func TestProcessTemplateDSL_BookTitleChunker(t *testing.T) {
	raw, err := readTestTemplate("ingestion_pipeline_book.json")
	if err != nil {
		t.Skipf("template file not found: %v", err)
	}

	dslBytes, err := processTemplateDSL(raw, entity.JSONMap{})
	if err != nil {
		t.Fatalf("processTemplateDSL: %v", err)
	}

	var dsl map[string]any
	if err := json.Unmarshal(dslBytes, &dsl); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	comps := dsl["components"].(map[string]any)
	chunker, ok := comps["TitleChunker:GrumpyGarlicsBake"]
	if !ok {
		t.Fatal("expected TitleChunker:GrumpyGarlicsBake in book template")
	}
	chunkerMap := chunker.(map[string]any)
	chunkerObj := chunkerMap["obj"].(map[string]any)
	if cn, _ := chunkerObj["component_name"].(string); cn != "TitleChunker" {
		t.Errorf("component_name = %q, want TitleChunker", cn)
	}

	chunkerParams := chunkerObj["params"].(map[string]any)
	if method, _ := chunkerParams["method"].(string); method != "hierarchy" {
		t.Errorf("method = %q, want hierarchy", method)
	}
	if hierarchy, _ := chunkerParams["hierarchy"].(float64); int(hierarchy) != 5 {
		t.Errorf("hierarchy = %v, want 5", hierarchy)
	}
}

func TestProcessTemplateDSL_OneTokenChunker(t *testing.T) {
	raw, err := readTestTemplate("ingestion_pipeline_one.json")
	if err != nil {
		t.Skipf("template file not found: %v", err)
	}

	dslBytes, err := processTemplateDSL(raw, entity.JSONMap{})
	if err != nil {
		t.Fatalf("processTemplateDSL: %v", err)
	}

	var dsl map[string]any
	if err := json.Unmarshal(dslBytes, &dsl); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	comps := dsl["components"].(map[string]any)
	chunker := comps["OneChunker:DryDrinksVisit"].(map[string]any)
	chunkerObj := chunker["obj"].(map[string]any)
	if cn, _ := chunkerObj["component_name"].(string); cn != "OneChunker" {
		t.Errorf("component_name = %q, want OneChunker", cn)
	}
}

func TestProcessTemplateDSL_MissingDSLKey(t *testing.T) {
	raw := []byte(`{"id": 99, "title": {"en": "Test"}}`)
	dslBytes, err := processTemplateDSL(raw, entity.JSONMap{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dslBytes != nil {
		t.Errorf("expected nil bytes for template without DSL key, got %d bytes", len(dslBytes))
	}
}
