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

package chunker

import (
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestGeneralChunkerRegistered(t *testing.T) {
	factory, category, metadata, ok := runtime.DefaultRegistry.Lookup(ComponentNameGeneralChunker)
	if !ok {
		t.Fatal("GeneralChunker is not registered")
	}
	if category != runtime.CategoryIngestion {
		t.Errorf("category = %q, want %q", category, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Fatal("factory is nil")
	}
	if _, ok := metadata.Inputs["file_type"]; !ok {
		t.Error("registered inputs missing file_type")
	}
	component, err := factory(ComponentNameGeneralChunker, nil)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if component == nil {
		t.Fatal("factory returned nil component")
	}
}

func TestGeneralChunkerDefaults(t *testing.T) {
	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	general := component.(*GeneralChunkerComponent)
	if general.param.ChunkTokenSize != 512 {
		t.Errorf("ChunkTokenSize = %d, want 512", general.param.ChunkTokenSize)
	}
	if general.param.OverlappedPercent != 0 {
		t.Errorf("OverlappedPercent = %v, want 0", general.param.OverlappedPercent)
	}
	if general.param.TableContextSize != 0 || general.param.ImageContextSize != 0 {
		t.Errorf("context sizes = (%d, %d), want (0, 0)", general.param.TableContextSize, general.param.ImageContextSize)
	}
	if len(general.param.Delimiters) != 1 || general.param.Delimiters[0] != "\n" {
		t.Errorf("Delimiters = %q, want [newline]", general.param.Delimiters)
	}
	if len(general.param.ChildrenDelimiters) != 0 {
		t.Errorf("ChildrenDelimiters = %q, want empty", general.param.ChildrenDelimiters)
	}
}

func TestGeneralChunkerParamsNormalizeConfiguration(t *testing.T) {
	delimiters := []string{"\n", "`---`"}
	children := []any{". ", "! "}
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":    0,
		"delimiters":          delimiters,
		"overlapped_percent":  0.25,
		"children_delimiters": children,
		"table_context_size":  -2,
		"image_context_size":  7,
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	general := component.(*GeneralChunkerComponent)
	if general.param.ChunkTokenSize != 0 {
		t.Errorf("ChunkTokenSize = %d, want 0", general.param.ChunkTokenSize)
	}
	if general.param.OverlappedPercent != 25 {
		t.Errorf("OverlappedPercent = %v, want 25", general.param.OverlappedPercent)
	}
	if got := strings.Join(general.param.Delimiters, "|"); got != "\n|`---`" {
		t.Errorf("Delimiters = %q", got)
	}
	if got := strings.Join(general.param.ChildrenDelimiters, "|"); got != ". |! " {
		t.Errorf("ChildrenDelimiters = %q", got)
	}
	if general.param.TableContextSize != 0 || general.param.ImageContextSize != 7 {
		t.Errorf("context sizes = (%d, %d), want (0, 7)", general.param.TableContextSize, general.param.ImageContextSize)
	}

	delimiters[0] = "changed"
	children[0] = "changed"
	if general.param.Delimiters[0] != "\n" || general.param.ChildrenDelimiters[0] != ". " {
		t.Fatal("component params retain caller-owned slices")
	}
}

func TestGeneralStrategyForFileType(t *testing.T) {
	tests := []struct {
		fileType string
		want     generalStrategy
	}{
		{"pdf", generalStrategyPDF},
		{"docx", generalStrategyDOCX},
		{"md", generalStrategyMarkdown},
		{"xls", generalStrategySpreadsheet},
		{"xlsx", generalStrategySpreadsheet},
		{"csv", generalStrategySpreadsheet},
		{"txt", generalStrategyText},
		{"other", generalStrategyText},
	}
	for _, test := range tests {
		t.Run(test.fileType, func(t *testing.T) {
			if got := generalStrategyForFileType(test.fileType); got != test.want {
				t.Errorf("generalStrategyForFileType(%q) = %v, want %v", test.fileType, got, test.want)
			}
		})
	}
}

func TestGeneralChunkerRequiresFileType(t *testing.T) {
	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	_, err = component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": "alpha", "doc_type_kwd": "text"}},
	})
	if err == nil || !strings.Contains(err.Error(), "file_type") {
		t.Fatalf("Invoke error = %v, want missing file_type error", err)
	}
}

func TestGeneralChunkerPreservesSingleParserUnit(t *testing.T) {
	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.docx",
		"file_type":     "docx",
		"output_format": "json",
		"json": []map[string]any{{
			"text":          "alpha",
			"doc_type_kwd":  "text",
			"source_order":  3,
			"heading_level": 2,
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one chunk", out["chunks"])
	}
	if chunks[0]["text"] != "alpha" || chunks[0]["source_order"] != float64(3) || chunks[0]["heading_level"] != float64(2) {
		t.Errorf("single parser unit was not preserved: %#v", chunks[0])
	}
}
