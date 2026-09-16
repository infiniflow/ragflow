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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/ingestion/task/indexdoc"
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

func TestGeneralChunkerRejectsNegativeTokenBudget(t *testing.T) {
	if _, err := NewGeneralChunker(map[string]any{"chunk_token_size": -1}); err == nil {
		t.Fatal("NewGeneralChunker accepted a negative chunk_token_size")
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

func TestGeneralChunkerLogsUnknownFileTypeFallback(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	_, err = component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.bin",
		"file_type":     "application/x-custom",
		"output_format": "json",
		"json":          []map[string]any{{"text": "alpha", "doc_type_kwd": "text"}},
	})
	if err != nil {
		t.Fatalf("Invoke unknown file type: %v", err)
	}
	if !strings.Contains(logs.String(), "unknown file_type") {
		t.Fatalf("logs = %q, want unknown file_type fallback diagnostic", logs.String())
	}

	logs.Reset()
	_, err = component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.txt",
		"file_type":     "txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": "alpha", "doc_type_kwd": "text"}},
	})
	if err != nil {
		t.Fatalf("Invoke canonical text file type: %v", err)
	}
	if strings.Contains(logs.String(), "unknown file_type") {
		t.Fatalf("canonical text fallback emitted unknown-file diagnostic: %q", logs.String())
	}
}

func TestGeneralChunkerInfersFileTypeFromSourceName(t *testing.T) {
	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	_, err = component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": "alpha", "doc_type_kwd": "text"}},
	})
	if err != nil {
		t.Fatalf("Invoke inferred file type: %v", err)
	}
}

func TestGeneralChunkerRequiresFileTypeWhenSourceHasNoExtension(t *testing.T) {
	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	_, err = component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document",
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
	got, ok := mergeMarkdownImagesChecked("data:image/png;base64,"+pixel, "data:image/png;base64,"+secondPixel)
	if !ok {
		t.Fatal("mergeMarkdownImagesChecked rejected valid PNG payloads")
	}
	decoded, ok := decodeMarkdownImage(got)
	if !ok {
		t.Fatalf("merged image is not decodable: %q", got)
	}
	if got := decoded.Bounds(); got.Dx() != 1 || got.Dy() != 2 {
		t.Fatalf("merged image bounds = %v, want 1x2", got)
	}
}

func TestDecodeMarkdownImageSupportsJPEGAndGIF(t *testing.T) {
	var jpegData bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(&jpegData, img, nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	cases := []struct {
		name string
		mime string
		data []byte
	}{
		{
			name: "jpeg",
			mime: "image/jpeg",
			data: jpegData.Bytes(),
		},
		{
			name: "gif",
			mime: "image/gif",
			data: mustDecodeBase64(t, "R0lGODdhAgACAIEAAP8AAAAAAAAAAAAAACwAAAAAAgACAAAIBgABCAQQEAA7"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := "data:" + tc.mime + ";base64," + base64.StdEncoding.EncodeToString(tc.data)
			if _, ok := decodeMarkdownImage(value); !ok {
				t.Fatalf("decodeMarkdownImage rejected %s payload", tc.mime)
			}
		})
	}
}

func mustDecodeBase64(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	return decoded
}

func TestMergeMarkdownImagesAcceptsBareBase64Payloads(t *testing.T) {
	const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=="
	const secondPixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGNgYPj/HwADAgH/5ncLrgAAAABJRU5ErkJggg=="
	got, ok := mergeMarkdownImagesChecked(pixel, secondPixel)
	if !ok {
		t.Fatal("mergeMarkdownImagesChecked rejected bare base64 payloads")
	}
	decoded, ok := decodeMarkdownImage(got)
	if !ok {
		t.Fatalf("merged bare-base64 image is not decodable: %q", got)
	}
	if got := decoded.Bounds(); got.Dx() != 1 || got.Dy() != 2 {
		t.Fatalf("merged image bounds = %v, want 1x2", got)
	}
}

func TestMergeMarkdownImagesRejectsOversizedCanvas(t *testing.T) {
	if markdownImageWithinLimits(maxMarkdownImageDimension, maxMarkdownImageDimension) {
		t.Fatal("maximum-dimension canvas should exceed the pixel safety limit")
	}
	if !markdownImageWithinLimits(1024, 1024) {
		t.Fatal("normal image dimensions should remain mergeable")
	}
	if got, ok := mergeMarkdownImagesChecked("s3://bucket/first.png", "s3://bucket/second.png"); ok || got != "s3://bucket/first.png" {
		t.Fatalf("opaque image references were changed: %q", got)
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
	if chunks[0]["text"] != "before\nafter" {
		t.Errorf("merged text = %q, want before\\nafter", chunks[0]["text"])
	}
	if chunks[1]["doc_type_kwd"] != "image" || chunks[1]["image"] != "figure" {
		t.Errorf("image chunk = %+v", chunks[1])
	}
}

// assertMaterializedMediaContext pins the chunker output contract for media
// chunks: the surrounding context is folded into the chunk body and the
// retrieval-only context fields are gone — the shape Python's chunker emits
// (rag/flow/chunker/token_chunker.py:343-359) and the shape the persisted
// content_with_weight and the chunk id are built from.
func assertMaterializedMediaContext(t *testing.T, chunk map[string]any, wantText string) {
	t.Helper()
	if got := chunk["text"]; got != wantText {
		t.Errorf("media chunk text = %q, want %q", got, wantText)
	}
	for _, key := range []string{"context_above", "context_below"} {
		if _, exists := chunk[key]; exists {
			t.Errorf("media chunk must not carry %s after materialization: %+v", key, chunk)
		}
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
	assertMaterializedMediaContext(t, chunks[1], "before<table><tr><td>A</td></tr></table>after")
}

func TestGeneralMediaContextSeparatesAdjacentSourceUnits(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "before one", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		{Text: "before two", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		{Text: "", DocType: "table", CKType: "table"},
		{Text: "after one", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		{Text: "after two", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}

	if got, want := collectGeneralMediaContext(units, 2, 10, true), "before one\nbefore two"; got != want {
		t.Fatalf("above context = %q, want %q", got, want)
	}
	if got, want := collectGeneralMediaContext(units, 2, 10, false), "after one\nafter two"; got != want {
		t.Fatalf("below context = %q, want %q", got, want)
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

func TestGeneralChunkerDOCXAppliesTextOverlap(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":   2,
		"overlapped_percent": 50,
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.docx",
		"file_type":     "docx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "alpha beta", "doc_type_kwd": "text"},
			{"text": "gamma delta", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	texts := outputTexts(t, out)
	if len(texts) != 2 {
		t.Fatalf("texts = %q, want two overlapped chunks", texts)
	}
	if texts[1] == "gamma delta" || !strings.HasSuffix(texts[1], "\ngamma delta") {
		t.Fatalf("DOCX overlap = %q, want previous tail plus current paragraph", texts[1])
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

func TestGeneralChunkerPDFPreservesPhysicalOrderAroundMedia(t *testing.T) {
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
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want text, media, text in physical order", chunks)
	}
	if chunks[0]["doc_type_kwd"] != "text" || chunks[0]["text"] != "before" {
		t.Errorf("first chunk = %+v", chunks[0])
	}
	if chunks[1]["doc_type_kwd"] != "table" || chunks[1]["text"] != "<table><tr><td>A</td></tr></table>" {
		t.Errorf("media chunk = %+v", chunks[1])
	}
	if chunks[2]["doc_type_kwd"] != "text" || chunks[2]["text"] != "after" {
		t.Errorf("last chunk = %+v", chunks[2])
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
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want text, media, text", chunks)
	}
	assertMaterializedMediaContext(t, chunks[1], "before<table>A</table>after")
	if chunks[0]["text"] != "before" || chunks[2]["text"] != "after" {
		t.Errorf("position-ordered text = [%v, %v]", chunks[0]["text"], chunks[2]["text"])
	}
}

func TestSortPDFUnitsKeepsUnpositionedMediaInInputOrder(t *testing.T) {
	position := func(top float64) json.RawMessage {
		return json.RawMessage(fmt.Sprintf("[[1,0,10,%g,%g]]", top, top+5))
	}
	units := []schema.ChunkDoc{
		{Text: "before", DocType: "text", CKType: "text", Positions: position(10)},
		{Text: "figure", DocType: "image", CKType: "image"},
		{Text: "after", DocType: "text", CKType: "text", Positions: position(30)},
	}

	got := sortPDFUnits(units)
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"before", "figure", "after"}) {
		t.Fatalf("PDF units = %q, want unpositioned media to keep its input position", texts)
	}
}

func TestGeneralChunkerSpreadsheetHeaderOnlyPreservesHeaderChunk(t *testing.T) {
	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "headers.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{{
			"text":         "Name; Amount",
			"doc_type_kwd": "table",
			"ck_type":      "table_header",
			"sheet_index":  1,
			"table_id":     "sheet-1",
			"positions":    []any{[]any{1.0, 1.0, 1.0, 1.0, 2.0}},
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %#v, want one header-only chunk", chunks)
	}
	if chunks[0]["text"] != "Name; Amount" || chunks[0]["ck_type"] != "table_header" {
		t.Fatalf("header-only chunk = %#v", chunks[0])
	}
}

func TestGeneralChunkerSpreadsheetAttachesImageContext(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":   10,
		"image_context_size": 10,
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "figures.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "Revenue", "doc_type_kwd": "text", "ck_type": "table_row", "table_id": "sheet-1", "sheet_index": 1, "tk_nums": 1},
			{"text": "B2", "doc_type_kwd": "image", "ck_type": "image", "image": "figure", "table_id": "sheet-1", "sheet_index": 1},
			{"text": "Growth", "doc_type_kwd": "text", "ck_type": "table_row", "table_id": "sheet-1", "sheet_index": 1, "tk_nums": 1},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want row, image, row", chunks)
	}
	if chunks[1]["ck_type"] != "image" {
		t.Fatalf("image chunk = %#v", chunks[1])
	}
	assertMaterializedMediaContext(t, chunks[1], "RevenueB2Growth")
}

func TestGeneralChunkerSpreadsheetImageContextStopsAtSheetBoundary(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{"image_context_size": 10})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "figures.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "Sheet one", "doc_type_kwd": "text", "ck_type": "table_row", "table_id": "sheet-1", "sheet_index": 1, "tk_nums": 1},
			{"text": "B2", "doc_type_kwd": "image", "ck_type": "image", "image": "figure", "table_id": "sheet-1", "sheet_index": 1},
			{"text": "Sheet two", "doc_type_kwd": "text", "ck_type": "table_row", "table_id": "sheet-2", "sheet_index": 2, "tk_nums": 1},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want row, image, row", chunks)
	}
	assertMaterializedMediaContext(t, chunks[1], "Sheet oneB2")
}

// TestGeneralChunkerMediaContextReachesChunkIDAndIndexContent pins the
// downstream half of the fold: once the context is part of the chunk body, it
// is also part of the persisted content_with_weight and of the chunk id
// (ChunkID hashes the body). Python's chunker emits the merged body, so its
// id and stored content already carry the context; before the fold Go hashed
// and stored the bare media payload instead, which made the configured window
// invisible to both the chunk list and the index.
func TestGeneralChunkerMediaContextReachesChunkIDAndIndexContent(t *testing.T) {
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
	chunks := indexdoc.NormalizeChunks(out)
	if _, err := indexdoc.ProcessChunksForPipeline(chunks, "doc-1", "document.docx", time.Now()); err != nil {
		t.Fatalf("ProcessChunksForPipeline: %v", err)
	}

	const wantText = "before<table><tr><td>A</td></tr></table>after"
	var table map[string]any
	for _, ck := range chunks {
		if ck["ck_type"] == "table" {
			table = ck
			break
		}
	}
	if table == nil {
		t.Fatalf("no table chunk in %#v", chunks)
	}
	if got := table["content_with_weight"]; got != wantText {
		t.Errorf("persisted content_with_weight = %q, want %q", got, wantText)
	}
	if got, want := table["id"], common.ChunkID("doc-1", wantText); got != want {
		t.Errorf("chunk id = %v, want %v (context must drive chunk identity)", got, want)
	}
}

// TestGeneralChunkerMediaContextStripsPositionTags pins the tag-stripping
// order Python uses: remove_tag runs on the merged body, so a position tag
// carried by a neighbouring text unit never reaches the stored media chunk.
// (The parser keeps boxes in the positions field, so this is a guard, not a
// re-strip of the media payload.)
func TestGeneralChunkerMediaContextStripsPositionTags(t *testing.T) {
	position := func(top float64) []any {
		return []any{[]any{1.0, 0.0, 10.0, top, top + 5}}
	}
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":   512,
		"table_context_size": 20,
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "before@@1\t0.0\t10.0\t10.0\t20.0##", "doc_type_kwd": "text", "positions": position(10)},
			{"text": "<table>A</table>", "doc_type_kwd": "table", "positions": position(20)},
			{"text": "after@@1\t0.0\t10.0\t10.0\t30.0\t35.0##", "doc_type_kwd": "text", "positions": position(30)},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want text, media, text", chunks)
	}
	assertMaterializedMediaContext(t, chunks[1], "before<table>A</table>after")
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
