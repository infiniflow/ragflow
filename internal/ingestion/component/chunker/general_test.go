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
	"reflect"
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

func TestGeneralChunkerNormalizesLegacyDelimiterString(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"delimiter": "\n!?;。；！？",
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	general := component.(*GeneralChunkerComponent)
	want := []string{"\n", "!", "?", ";", "。", "；", "！", "？"}
	if !reflect.DeepEqual(general.param.Delimiters, want) {
		t.Fatalf("Delimiters = %#v, want %#v", general.param.Delimiters, want)
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

func TestGeneralChunkerMarkdownShortHeadingForcesNextUnit(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 1})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.md",
		"file_type":     "md",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "Title", "doc_type_kwd": "text", "ck_type": "heading"},
			{"text": "body", "doc_type_kwd": "text", "ck_type": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one forced heading/body chunk", chunks)
	}
	if text, _ := chunks[0]["text"].(string); text != "Title\nbody" {
		t.Errorf("text = %q, want %q", text, "Title\nbody")
	}
}

func TestGeneralChunkerMarkdownLongHeadingUsesNormalCap(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 1})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	longHeading := strings.Repeat("heading ", 60)
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.md",
		"file_type":     "md",
		"output_format": "json",
		"json": []map[string]any{
			{"text": longHeading, "doc_type_kwd": "text", "ck_type": "heading"},
			{"text": "body", "doc_type_kwd": "text", "ck_type": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want separate long heading and body chunks", chunks)
	}
	if text, _ := chunks[0]["text"].(string); text != strings.TrimSpace(longHeading) {
		t.Errorf("first text = %q, want long heading", text)
	}
	if text, _ := chunks[1]["text"].(string); text != "body" {
		t.Errorf("second text = %q, want body", text)
	}
}

func TestGeneralChunkerMarkdownImageMergesWithTextAndPreservesPayload(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 1})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.md",
		"file_type":     "md",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "Title", "doc_type_kwd": "text", "ck_type": "heading"},
			{"text": "figure caption", "doc_type_kwd": "image", "image": "data:image/png;base64,AAAA"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one heading/image chunk", chunks)
	}
	if text, _ := chunks[0]["text"].(string); text != "Title\nfigure caption" {
		t.Errorf("text = %q, want %q", text, "Title\nfigure caption")
	}
	if image, _ := chunks[0]["image"].(string); image != "data:image/png;base64,AAAA" {
		t.Errorf("image = %q, want original image payload", image)
	}
}

func TestMergeMarkdownImagesStacksRasterPayloads(t *testing.T) {
	const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=="
	const secondPixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYPj/HwADAgH/5ncLrgAAAABJRU5ErkJggg=="
	got := mergeMarkdownImages("data:image/png;base64,"+pixel, "data:image/png;base64,"+secondPixel)
	decoded, ok := decodeMarkdownImage(got)
	if !ok {
		t.Fatalf("merged image is not decodable: %q", got)
	}
	if got := decoded.Bounds(); got.Dx() != 1 || got.Dy() != 2 {
		t.Fatalf("merged image bounds = %v, want 1x2", got)
	}
}

func TestMergeMarkdownImagesAcceptsBareBase64Payloads(t *testing.T) {
	const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=="
	const secondPixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYPj/HwADAgH/5ncLrgAAAABJRU5ErkJggg=="
	got := mergeMarkdownImages(pixel, secondPixel)
	decoded, ok := decodeMarkdownImage(got)
	if !ok {
		t.Fatalf("merged bare-base64 image is not decodable: %q", got)
	}
	if got := decoded.Bounds(); got.Dx() != 1 || got.Dy() != 2 {
		t.Fatalf("merged image bounds = %v, want 1x2", got)
	}
}

func TestGeneralChunkerMarkdownDoesNotDropUnmergeableImages(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.md",
		"file_type":     "md",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "first", "doc_type_kwd": "image", "image": "s3://bucket/first.png"},
			{"text": "second", "doc_type_kwd": "image", "image": "s3://bucket/second.png"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want two chunks so neither image reference is lost", chunks)
	}
	if chunks[0]["image"] != "s3://bucket/first.png" || chunks[1]["image"] != "s3://bucket/second.png" {
		t.Fatalf("image references = %#v, want both source references", chunks)
	}
}

func TestGeneralChunkerMarkdownShortHeadingKeepsFollowingTableAtomic(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 1})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.md",
		"file_type":     "md",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "Title", "doc_type_kwd": "text", "ck_type": "heading"},
			{"text": "<table><tr><td>A</td></tr></table>", "doc_type_kwd": "table", "ck_type": "table"},
			{"text": "body", "doc_type_kwd": "text", "ck_type": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want table and body", chunks)
	}
	if text, _ := chunks[0]["text"].(string); text != "Title\n<table><tr><td>A</td></tr></table>" {
		t.Errorf("table text = %q", text)
	}
	if got := chunks[0]["doc_type_kwd"]; got != "table" {
		t.Errorf("table doc_type_kwd = %v, want table", got)
	}
	if got := chunks[0]["ck_type"]; got != "table" {
		t.Errorf("table ck_type = %v, want table", got)
	}
}

func TestGeneralChunkerDOCXMediaDoesNotBreakTextMerge(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 10})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.docx",
		"file_type":     "docx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "before", "doc_type_kwd": "text"},
			{"text": "", "doc_type_kwd": "image", "image": "figure"},
			{"text": "after", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want merged text and image", chunks)
	}
	if chunks[0]["text"] != "beforeafter" {
		t.Errorf("merged text = %q, want beforeafter", chunks[0]["text"])
	}
	if chunks[1]["doc_type_kwd"] != "image" || chunks[1]["image"] != "figure" {
		t.Errorf("image chunk = %+v", chunks[1])
	}
}

func TestGeneralChunkerDOCXAttachesMediaContextBeforeTextMerge(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":   10,
		"table_context_size": 2,
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.docx",
		"file_type":     "docx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "before", "doc_type_kwd": "text"},
			{"text": "<table><tr><td>A</td></tr></table>", "doc_type_kwd": "table"},
			{"text": "after", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want merged text and table", chunks)
	}
	if chunks[1]["doc_type_kwd"] != "table" {
		t.Fatalf("table chunk = %+v", chunks[1])
	}
	if chunks[1]["context_above"] != "before" || chunks[1]["context_below"] != "after" {
		t.Errorf("table context = above:%q below:%q", chunks[1]["context_above"], chunks[1]["context_below"])
	}
}

func TestGeneralChunkerDOCXCustomDelimiterDisablesTextMerge(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 10,
		"delimiters":       []string{"`|`"},
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.docx",
		"file_type":     "docx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "before|after", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"before", "after"}) {
		t.Fatalf("texts = %q, want custom-delimiter units", texts)
	}
}

func TestGeneralChunkerChildrenDelimiterKeepsDelimiter(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"children_delimiters": []string{";"},
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.txt",
		"file_type":     "txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": "part A;part B", "doc_type_kwd": "text"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"part A;", "part B"}) {
		t.Fatalf("children delimiter texts = %q, want [part A; part B]", texts)
	}
}

func TestGeneralChunkerPDFEmitsMediaBeforeMergedBody(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 10})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "before", "doc_type_kwd": "text"},
			{"text": "<table><tr><td>A</td></tr></table>", "doc_type_kwd": "table"},
			{"text": "after", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want media and body", chunks)
	}
	if chunks[0]["doc_type_kwd"] != "table" || chunks[0]["text"] != "<table><tr><td>A</td></tr></table>" {
		t.Errorf("media chunk = %+v", chunks[0])
	}
	if chunks[1]["text"] != "before\nafter" {
		t.Errorf("body chunk = %q, want merged body", chunks[1]["text"])
	}
}

func TestGeneralChunkerPDFUsesPositionOrderForMediaContext(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":   10,
		"table_context_size": 1,
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	position := func(top float64) []any {
		return []any{[]any{1.0, 0.0, 10.0, top, top + 5}}
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "after", "doc_type_kwd": "text", "positions": position(30)},
			{"text": "<table>A</table>", "doc_type_kwd": "table", "positions": position(20)},
			{"text": "before", "doc_type_kwd": "text", "positions": position(10)},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want media and body", chunks)
	}
	if chunks[0]["context_above"] != "before" || chunks[0]["context_below"] != "after" {
		t.Errorf("table context = above:%q below:%q", chunks[0]["context_above"], chunks[0]["context_below"])
	}
	if chunks[1]["text"] != "before\nafter" {
		t.Errorf("position-ordered body = %q", chunks[1]["text"])
	}
}

func TestGeneralChunkerPDFAttachesOutlineOnce(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 10})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"file": map[string]any{
			"outline": []map[string]any{{"title": "Chapter 1", "level": 0}},
		},
		"json": []map[string]any{{"text": "body", "doc_type_kwd": "text"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one chunk", chunks)
	}
	outline, ok := chunks[0]["__outline__"].([]any)
	if !ok || len(outline) != 1 {
		t.Fatalf("outline = %#v, want one entry", chunks[0]["__outline__"])
	}
	entry, _ := outline[0].(map[string]any)
	if entry["title"] != "Chapter 1" || entry["depth"] != float64(0) {
		t.Errorf("outline entry = %#v", entry)
	}
}
