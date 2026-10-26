//go:build cgo

//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package chunker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/tokenizer"
)

// TestCropTitleChunks_CropsPositionedChunk pins residual A: chunks
// produced by the Title/Group/Hierarchy chunkers must be cropped on demand
// (mirroring the TokenChunker JSON path). The chunk shape mirrors the real
// Group/Hierarchy output: it carries doc_type_kwd + positions and no
// block-kind field. cropImageChunks derives the crop type from doc_type_kwd.
// A text chunk with positions gets a rendered preview; a chunk without
// positions is left untouched.
func TestCropTitleChunks_CropsPositionedChunk(t *testing.T) {
	ctx := context.Background()
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	in := []map[string]any{
		{"text": "page text", "doc_type_kwd": "text", "positions": pos},
		{"text": "no coords", "doc_type_kwd": "text"},
	}

	got := cropTitleChunks(ctx, mockCropEngine{}, in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	img, _ := got[0]["image"].(string)
	if !strings.HasPrefix(img, "data:image/png;base64,") {
		t.Errorf("positioned text chunk: image = %q, want data:image/png;base64, prefix", img)
	}
	if got[1]["image"] != nil {
		t.Errorf("chunk without positions should not be cropped, got %v", got[1]["image"])
	}
}

// TestCropTitleChunks_CropsImageWithPageList covers the image variant: a
// title-family image chunk whose page cell is a list must still be cropped.
func TestCropTitleChunks_CropsImageWithPageList(t *testing.T) {
	pos := [][]any{{[]any{1}, 10, 100, 10, 100}}
	in := []map[string]any{
		{"text": "Figure 1: caption", "doc_type_kwd": "image", "positions": pos, "_pdf_positions": pos},
	}
	got := cropTitleChunks(context.Background(), mockCropEngine{}, in)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	img, _ := got[0]["image"].(string)
	if !strings.HasPrefix(img, "data:image/png;base64,") {
		t.Errorf("image = %q, want a cropped data URL", img)
	}
}

// Position-only figures must keep distinct identities even when the root
// heading supplies identical text. Streamed uploads, decorator retries and
// the no-engine fallback must agree on the same per-figure IDs. Changing only
// the heading text must invalidate those IDs for downstream extraction caches.
func TestCropTitleChunks_PositionOnlyImagesWithRootHeading(t *testing.T) {
	for _, tc := range []struct {
		name string
		new  func(map[string]any) (runtime.Component, error)
	}{
		{"group", NewGroupTitleChunker},
		{"hierarchy", NewHierarchyTitleChunker},
		{"manual", NewManualChunker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp, err := tc.new(map[string]any{
				"levels":                [][]string{{`^# `}},
				"hierarchy":             1,
				"root_chunk_as_heading": true,
			})
			if err != nil {
				t.Fatalf("new chunker: %v", err)
			}
			var originalIDs, changedIDs, repeatedIDs [2]string
			t.Run("original-heading", func(t *testing.T) {
				originalIDs = testPositionOnlyImagesWithRootHeading(t, comp, "# Figure context")
			})
			t.Run("changed-heading", func(t *testing.T) {
				changedIDs = testPositionOnlyImagesWithRootHeading(t, comp, "# Updated figure context")
			})
			t.Run("repeated-heading", func(t *testing.T) {
				repeatedIDs = testPositionOnlyImagesWithRootHeading(t, comp, "# Figure context")
			})
			for i := range originalIDs {
				if originalIDs[i] == changedIDs[i] {
					t.Errorf("figure %d ID %q unchanged after heading text changed at the same coordinates", i, originalIDs[i])
				}
				if originalIDs[i] != repeatedIDs[i] {
					t.Errorf("figure %d ID = %q, want repeatable ID %q", i, repeatedIDs[i], originalIDs[i])
				}
			}
		})
	}
}

func testPositionOnlyImagesWithRootHeading(t *testing.T, comp runtime.Component, heading string) [2]string {
	t.Helper()
	ctx := withIngestionGlobals(t, "kb1", "doc1")
	var wantIDs [2]string
	for _, mode := range []string{"stream", "retry", "no-engine"} {
		t.Run(mode, func(t *testing.T) {
			out, err := comp.Invoke(ctx, nil, map[string]any{
				"name":          "doc.pdf",
				"output_format": "json",
				"json": []map[string]any{
					{"text": heading, "doc_type_kwd": "text"},
					// _pdf_positions carries only page identity; positions supplies the crop.
					{"text": "", "doc_type_kwd": "image", "_pdf_positions": [][]float64{{2, 0, 0, 0, 0}}, "positions": [][]float64{{1, 10, 100, 10, 100}}},
					{"text": "", "doc_type_kwd": "image", "_pdf_positions": [][]float64{{2, 0, 0, 0, 0}}, "positions": [][]float64{{1, 200, 300, 200, 300}}},
				},
			})
			if err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			chunks, _ := out["chunks"].([]map[string]any)
			if len(chunks) != 2 {
				t.Fatalf("chunks = %#v, want two positioned images", chunks)
			}
			for _, ck := range chunks {
				if ck["doc_type_kwd"] != "image" || !strings.Contains(toString(ck["text"]), heading) {
					t.Fatalf("root heading not applied to image: %#v", ck)
				}
			}
			if chunks[0]["text"] != chunks[1]["text"] {
				t.Fatal("fixture must give both figures identical heading-only text")
			}
			builtIDs := [2]string{toString(chunks[0]["id"]), toString(chunks[1]["id"])}

			rec := &recordingUploader{}
			orig := ChunkImageUploader
			ChunkImageUploader = rec.upload
			t.Cleanup(func() { ChunkImageUploader = orig })
			if mode == "retry" {
				ChunkImageUploader = func(ctx context.Context, kbID, chunkID string, data []byte) (string, error) {
					rec.upload(ctx, kbID, chunkID, data)
					return "", fmt.Errorf("stream upload failed")
				}
			}
			if mode != "no-engine" {
				chunks = cropTitleChunks(ctx, assertZeroPageEngine{}, chunks)
				if len(rec.calls) != 2 {
					t.Fatalf("uploads = %d, want 2", len(rec.calls))
				}
				if rec.calls[0].chunkID == rec.calls[1].chunkID {
					t.Errorf("different figures share upload key %q", rec.calls[0].chunkID)
				}
				for _, ck := range chunks {
					if mode == "retry" && (ck["img_id"] != nil || !strings.HasPrefix(toString(ck["image"]), "data:image/png;base64,")) {
						t.Errorf("failed upload must retain bytes for retry: %#v", ck)
					}
				}
			}

			ChunkImageUploader = rec.upload
			// Ordinary text IDs must retain their existing text-only formula.
			text := map[string]any{"text": "ordinary text", "doc_type_kwd": "text"}
			decorated := &imageUploadDecorator{inner: &stubChunker{chunks: append(chunks, text)}}
			if _, err := decorated.Invoke(ctx, nil, nil); err != nil {
				t.Fatalf("decorator Invoke: %v", err)
			}
			if text["id"] != common.ChunkID("doc1", "ordinary text") {
				t.Errorf("ordinary text ID changed: %v", text["id"])
			}
			ids := [2]string{toString(chunks[0]["id"]), toString(chunks[1]["id"])}
			if ids != builtIDs {
				t.Errorf("IDs changed during crop/upload: built %q, decorated %q", builtIDs, ids)
			}
			if ids[0] == "" || ids[1] == "" || ids[0] == ids[1] {
				t.Fatalf("figure IDs must be nonempty and distinct: %q", ids)
			}
			if mode == "stream" {
				wantIDs = ids
			} else if wantIDs != ids {
				t.Errorf("IDs = %q, want stable IDs %q", ids, wantIDs)
			}
			wantUploads := map[string]int{"stream": 2, "retry": 4, "no-engine": 0}[mode]
			if len(rec.calls) != wantUploads {
				t.Fatalf("uploads = %d, want %d", len(rec.calls), wantUploads)
			}
			uploads := make(map[string]int)
			for _, call := range rec.calls {
				if call.kbID != "kb1" || call.dataLen == 0 || (call.chunkID != ids[0] && call.chunkID != ids[1]) {
					t.Errorf("upload key differs from decorator ID: %#v", call)
				}
				uploads[call.chunkID]++
			}
			for _, id := range ids {
				if uploads[id] != wantUploads/2 {
					t.Errorf("uploads at %q = %d, want %d", id, uploads[id], wantUploads/2)
				}
			}
			if mode != "no-engine" {
				for i, ck := range chunks {
					if ck["img_id"] != "kb1-"+ids[i] || toString(ck["image"]) != "" {
						t.Errorf("figure %d must reference its uploaded preview and drop inline bytes", i)
					}
				}
			}
		})
	}
	return wantIDs
}

// Without a root heading or other text, a position-only figure still separates
// neighboring text records, but is not cropped/uploaded and the tokenizer drops it.
func TestCropTitleChunks_PositionOnlyImageWithoutHeading(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })
	tok, err := component.NewTokenizerComponent(map[string]any{"search_method": []any{"full_text"}})
	if err != nil {
		t.Fatalf("new tokenizer: %v", err)
	}
	for _, tc := range titleFamilyComponents(t) {
		t.Run(tc.name, func(t *testing.T) {
			ctx := withIngestionGlobals(t, "kb1", "doc1")
			rec := &recordingUploader{}
			orig := ChunkImageUploader
			ChunkImageUploader = rec.upload
			t.Cleanup(func() { ChunkImageUploader = orig })
			chunks := titleFamilyChunks(t, tc.comp, []map[string]any{
				{"text": "before figure", "doc_type_kwd": "text"},
				{"text": "", "doc_type_kwd": "image", "positions": [][]float64{{1, 10, 100, 10, 100}}},
				{"text": "after figure", "doc_type_kwd": "text"},
			})
			if len(chunks) != 3 || chunks[1]["doc_type_kwd"] != "image" || chunks[1]["text"] != "" {
				t.Fatalf("chunks = %#v, want text, textless figure, text", chunks)
			}
			chunks = cropTitleChunks(ctx, mockCropEngine{}, chunks)
			decorated := &imageUploadDecorator{inner: &stubChunker{chunks: chunks}}
			out, err := decorated.Invoke(ctx, nil, nil)
			if err != nil {
				t.Fatalf("decorator Invoke: %v", err)
			}
			if len(rec.calls) != 0 || toString(chunks[1]["image"]) != "" || toString(chunks[1]["img_id"]) != "" {
				t.Fatalf("textless figure must not be cropped or uploaded: %#v, uploads=%d", chunks[1], len(rec.calls))
			}
			out["name"] = "doc.pdf"
			out["output_format"] = "chunks"
			final, err := tok.Invoke(ctx, nil, out)
			if err != nil {
				t.Fatalf("tokenizer Invoke: %v", err)
			}
			got, _ := final["chunks"].([]map[string]any)
			if len(got) != 2 || strings.TrimSpace(toString(got[0]["text"])) != "before figure" ||
				strings.TrimSpace(toString(got[1]["text"])) != "after figure" {
				t.Fatalf("tokenizer chunks = %#v, want only the two separate text chunks", got)
			}
		})
	}
}

// TestCropTitleChunks_NilEnginePassthrough asserts the helper is a no-op
// when no PDF engine is available (best-effort contract).
func TestCropTitleChunks_NilEnginePassthrough(t *testing.T) {
	in := []map[string]any{{"text": "hello"}}
	got := cropTitleChunks(context.Background(), nil, in)
	if len(got) != 1 || got[0]["text"] != "hello" {
		t.Fatalf("nil engine should pass through unchanged: %v", got)
	}
}
