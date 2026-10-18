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

package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
)

// mediaContextComponents exercises both Title dispatch modes and standalone variants.
func mediaContextComponents(t *testing.T, windows map[string]any) []titleFamilyCase {
	t.Helper()
	var cases []titleFamilyCase
	for _, tc := range []struct {
		name    string
		method  string
		factory func(map[string]any) (runtime.Component, error)
	}{
		{"title-group", "group", NewTitleChunker},
		{"title-hierarchy", "hierarchy", NewTitleChunker},
		{"group", "group", NewGroupTitleChunker},
		{"hierarchy", "hierarchy", NewHierarchyTitleChunker},
		{"manual", "group", NewManualChunker},
	} {
		conf := map[string]any{"method": tc.method, "hierarchy": 1, "levels": [][]string{{`^# `}}}
		for k, v := range windows {
			conf[k] = v
		}
		comp, err := tc.factory(conf)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, titleFamilyCase{tc.name, comp})
	}
	return cases
}

// titleContextFixture contains source text, an atomic table and a captionless image.
func titleContextFixture(t *testing.T) []map[string]any {
	t.Helper()
	return []map[string]any{
		{"text": "Alpha explains the table.", "doc_type_kwd": "text"},
		tableItem(t, titleTestTableHTML, map[string]any{"img_id": "table-preview", "_pdf_positions": [][]float64{{1, 10, 100, 20, 30}}}),
		{"text": "Beta explains both media.", "doc_type_kwd": "text"},
		{"text": "", "doc_type_kwd": "image", "image": "data:image/png;base64," + pngBase64, "_pdf_positions": [][]float64{{1, 10, 100, 50, 60}}},
		{"text": "Gamma follows the image.", "doc_type_kwd": "text"},
	}
}

// TestTitleMediaContext attaches independent windows without changing payloads or ordering.
func TestTitleMediaContext(t *testing.T) {
	for _, windows := range []map[string]any{
		{"table_context_size": 1, "image_context_size": 1},
		{"table_context_size": 1, "image_context_size": 0},
		{"table_context_size": 0, "image_context_size": 1},
	} {
		for _, tc := range mediaContextComponents(t, windows) {
			t.Run(fmt.Sprintf("%s/table=%v/image=%v", tc.name, windows["table_context_size"], windows["image_context_size"]), func(t *testing.T) {
				fixture := titleContextFixture(t)
				before, _ := json.Marshal(fixture)
				chunks := titleFamilyChunks(t, tc.comp, fixture)
				if len(chunks) != 5 {
					t.Fatalf("chunks=%d: %#v", len(chunks), chunks)
				}
				tableText := renderedChunkText(t, fixture[1])
				if windows["table_context_size"] == 1 {
					tableText = "Alpha explains the table.\n" + tableText + "\nBeta explains both media."
				}
				imageText := ""
				if windows["image_context_size"] == 1 {
					imageText = "Beta explains both media.\n\nGamma follows the image."
				}
				for index, want := range map[int]string{1: tableText, 3: imageText} {
					if got := chunks[index]["text"]; got != want {
						t.Errorf("chunk %d text=%q, want %q", index, got, want)
					}
					for _, key := range []string{"_pdf_positions", "img_id", "image"} {
						if want, exists := fixture[index][key]; exists && !reflect.DeepEqual(chunks[index][key], want) {
							t.Errorf("chunk %d lost %s: %v", index, key, chunks[index][key])
						}
					}
					if _, ok := chunks[index]["context_above"]; ok {
						t.Fatal("context was not materialized")
					}
					if count, ok := chunks[index]["tk_nums"]; ok && count != tokenizeStr(toString(chunks[index]["text"])) {
						t.Errorf("chunk %d has a stale token count: %v", index, count)
					}
				}
				if chunks[0]["text"] != "Alpha explains the table.\n" || chunks[2]["text"] != "Beta explains both media.\n" || chunks[4]["text"] != "Gamma follows the image.\n" {
					t.Fatalf("neighbor text changed: %#v", chunks)
				}
				after, _ := json.Marshal(fixture)
				if string(before) != string(after) {
					t.Fatal("input payload was mutated")
				}
			})
		}
	}
}

// TestTitleMediaContextSheetBoundary stops before a different spreadsheet source.
func TestTitleMediaContextSheetBoundary(t *testing.T) {
	for _, tc := range mediaContextComponents(t, map[string]any{"image_context_size": 128}) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "Sheet one.", "doc_type_kwd": "text", "sheet": "One", "sheet_index": 0},
				{"text": "", "doc_type_kwd": "image", "img_id": "one", "sheet": "One", "sheet_index": 0},
				{"text": "Sheet two.", "doc_type_kwd": "text", "sheet": "Two", "sheet_index": 1},
				{"text": "", "doc_type_kwd": "image", "img_id": "two", "sheet": "Two", "sheet_index": 1},
				{"text": "Sheet two ending.", "doc_type_kwd": "text", "sheet": "Two", "sheet_index": 1},
			})
			if chunks[1]["text"] != "Sheet one.\n" || chunks[3]["text"] != "Sheet two.\n\nSheet two ending." {
				t.Fatalf("context crossed sheet boundary: %#v", chunks)
			}
		})
	}
}

// TestTitleMediaContextInvalidParameters rejects invalid API input at construction.
func TestTitleMediaContextInvalidParameters(t *testing.T) {
	for _, factory := range []func(map[string]any) (runtime.Component, error){
		NewTitleChunker, NewGroupTitleChunker, NewHierarchyTitleChunker, NewManualChunker,
	} {
		for _, key := range []string{"table_context_size", "image_context_size"} {
			for _, invalid := range []any{-1, -0.5, 0.5, true, nil, "invalid", math.Inf(1), math.NaN(), 1e30} {
				_, err := factory(map[string]any{
					"method": "group", "levels": [][]string{{`^# `}}, "hierarchy": 1, key: invalid,
				})
				if err == nil || !strings.Contains(err.Error(), key) {
					t.Errorf("%s=%v (%T) not rejected: %v", key, invalid, invalid, err)
				}
			}
		}
	}
}

// TestTitleMediaContextRootAndCap keeps media atomic and counts the final prefixed body.
func TestTitleMediaContextRootAndCap(t *testing.T) {
	for _, tc := range mediaContextComponents(t, map[string]any{
		"image_context_size": 1, "chunk_token_cap": 128, "root_chunk_as_heading": true,
	}) {
		t.Run(tc.name, func(t *testing.T) {
			sentence := strings.Repeat("neighbor ", 160) + "ends."
			chunks := titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "Root.", "doc_type_kwd": "text"},
				{"text": "Caption.", "doc_type_kwd": "image", "img_id": "figure"},
				{"text": sentence, "doc_type_kwd": "text"},
			})
			var images []map[string]any
			for _, chunk := range chunks {
				if chunk["doc_type_kwd"] == "image" {
					images = append(images, chunk)
				}
			}
			if len(images) != 1 || !strings.Contains(toString(images[0]["text"]), sentence) {
				t.Fatalf("media body was split or lost context: %#v", chunks)
			}
			if images[0]["tk_nums"] != tokenizeStr(toString(images[0]["text"])) {
				t.Fatalf("stale prefixed token count: %#v", images[0])
			}
		})
	}
}

// TestTitleMediaContextZero preserves byte-for-byte output when windows are disabled.
func TestTitleMediaContextZero(t *testing.T) {
	omitted := mediaContextComponents(t, nil)
	explicit := mediaContextComponents(t, map[string]any{"table_context_size": 0, "image_context_size": 0})
	for i, tc := range omitted {
		t.Run(tc.name, func(t *testing.T) {
			first := titleFamilyChunks(t, tc.comp, titleContextFixture(t))
			second := titleFamilyChunks(t, explicit[i].comp, titleContextFixture(t))
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("zero windows changed output: %#v / %#v", first, second)
			}
		})
	}
}

// TestTitleMediaContextInvocationBoundary proves a reused component keeps no source state.
func TestTitleMediaContextInvocationBoundary(t *testing.T) {
	for _, tc := range mediaContextComponents(t, map[string]any{"image_context_size": 128}) {
		t.Run(tc.name, func(t *testing.T) {
			titleFamilyChunks(t, tc.comp, []map[string]any{{"text": "PRIVATE FIRST DOCUMENT.", "doc_type_kwd": "text"}})
			chunks := titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "", "doc_type_kwd": "image", "img_id": "second-document"},
				{"text": "Only second document.", "doc_type_kwd": "text"},
			})
			if len(chunks) != 2 || chunks[0]["text"] != "\nOnly second document." {
				t.Fatalf("wrong source context: %#v", chunks)
			}
		})
	}
}

// TestTitleMediaContextManualOrder selects neighbors after the physical-position sort.
func TestTitleMediaContextManualOrder(t *testing.T) {
	comp, err := NewManualChunker(map[string]any{"levels": [][]string{{`^# `}}, "image_context_size": 1})
	if err != nil {
		t.Fatal(err)
	}
	chunks := titleFamilyChunks(t, comp, []map[string]any{
		{"text": "Below.", "doc_type_kwd": "text", "_pdf_positions": [][]float64{{1, 10, 100, 30, 40}}},
		{"text": "", "doc_type_kwd": "image", "img_id": "figure", "_pdf_positions": [][]float64{{1, 10, 100, 20, 25}}},
		{"text": "Above.", "doc_type_kwd": "text", "_pdf_positions": [][]float64{{1, 10, 100, 10, 15}}},
	})
	if len(chunks) != 3 || chunks[1]["text"] != "Above.\n\nBelow." {
		t.Fatalf("context did not follow physical order: %#v", chunks)
	}
}

// TestTitleMediaContextSentences uses whole adjacent sentences even over a one-token budget.
func TestTitleMediaContextSentences(t *testing.T) {
	for _, tc := range mediaContextComponents(t, map[string]any{"image_context_size": 1}) {
		t.Run(tc.name, func(t *testing.T) {
			chunks := titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "Distant above. Nearest complete sentence.", "doc_type_kwd": "text"},
				{"text": "Caption.", "doc_type_kwd": "image", "img_id": "figure"},
				{"text": "Closest below. Distant below.", "doc_type_kwd": "text"},
			})
			if got, want := chunks[1]["text"], "Nearest complete sentence.\nCaption.\nClosest below. "; got != want {
				t.Fatalf("text=%q, want whole boundary sentences %q", got, want)
			}
		})
	}
}

// TestTitleMediaContextDecoratorID hashes the final context-bearing body used by upload.
func TestTitleMediaContextDecoratorID(t *testing.T) {
	prev := ChunkImageUploader
	var uploaded string
	ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
		uploaded = chunkID
		return kbID + "-" + chunkID, nil
	}
	t.Cleanup(func() { ChunkImageUploader = prev })
	comp := mediaContextComponents(t, map[string]any{"image_context_size": 1})[0].comp
	decorated := &imageUploadDecorator{inner: comp}
	out, err := decorated.Invoke(t.Context(), nil, map[string]any{
		"name": "book.docx", "kb_id": testKBID, "output_format": "json",
		"json": []map[string]any{
			{"text": "Source explanation.", "doc_type_kwd": "text"},
			{"text": "Caption.", "doc_type_kwd": "image", "image": "data:image/png;base64," + pngBase64},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks := out["chunks"].([]map[string]any)
	wantText := "Source explanation.\nCaption."
	wantID := common.ChunkID("", wantText)
	if chunks[1]["text"] != wantText || chunks[1]["id"] != wantID || uploaded != wantID || chunks[1]["img_id"] != testKBID+"-"+wantID {
		t.Fatalf("context/id/upload mismatch: %#v, uploaded=%s", chunks[1], uploaded)
	}
	if strings.Contains(toString(chunks[1]["text"]), "@@") {
		t.Fatal("position tags leaked into indexed text")
	}
}

// TestTitleMediaContextCaptionlessIDs prevents different images with identical
// surrounding text from overwriting each other's storage and index entries.
func TestTitleMediaContextCaptionlessIDs(t *testing.T) {
	prev := ChunkImageUploader
	t.Cleanup(func() { ChunkImageUploader = prev })
	for _, tc := range mediaContextComponents(t, map[string]any{"image_context_size": 1}) {
		t.Run(tc.name, func(t *testing.T) {
			uploaded := map[string]int{}
			ChunkImageUploader = func(_ context.Context, kbID, chunkID string, _ []byte) (string, error) {
				uploaded[chunkID]++
				return kbID + "-" + chunkID, nil
			}
			ctx := runtime.WithState(t.Context(), &runtime.CanvasState{Globals: map[string]any{"doc_id": "source-doc"}})
			out, err := (&imageUploadDecorator{inner: tc.comp}).Invoke(ctx, nil, map[string]any{
				"name": "book.docx", "kb_id": testKBID, "output_format": "json",
				"json": []map[string]any{
					{"text": "Same before.", "doc_type_kwd": "text"},
					{"text": "", "doc_type_kwd": "image", "image": "data:image/png;base64," + pngBase64},
					{"text": "", "doc_type_kwd": "image", "image": "data:image/png;base64," + pngBase64Red},
					{"text": "Same after.", "doc_type_kwd": "text"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			chunks := out["chunks"].([]map[string]any)
			if len(chunks) != 4 || chunks[1]["text"] != chunks[2]["text"] {
				t.Fatalf("fixture must produce identical contextual bodies: %#v", chunks)
			}
			if chunks[1]["id"] == chunks[2]["id"] || len(uploaded) != 2 {
				t.Fatalf("distinct images collided: %#v; uploads=%v", chunks, uploaded)
			}
			for i, payload := range []string{pngBase64, pngBase64Red} {
				text := toString(chunks[i+1]["text"])
				source := mediaChunkIdentity("data:image/png;base64,"+payload, "") + "\x00" + text
				if want := common.ChunkID("source-doc", source); chunks[i+1]["id"] != want || uploaded[want] != 1 {
					t.Fatalf("context/media/document identity lost: %#v", chunks[i+1])
				}
			}
		})
	}
}

// TestTitleMediaContextStampedCropID keeps the crop/upload identity already
// derived from a contextual captionless image, matching decorator behavior.
func TestTitleMediaContextStampedCropID(t *testing.T) {
	ck := schema.ChunkDoc{ID: "source-scoped-media-id", Text: "Context-only body.", CKType: "image"}
	if got := canonicalChunkID("doc", ck); got != ck.ID {
		t.Fatalf("crop replaced stamped identity: %s", got)
	}
}
