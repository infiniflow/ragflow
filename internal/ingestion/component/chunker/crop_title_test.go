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
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
)

// TestCropTitleChunks_CropsPositionedChunk pins residual A: chunks
// produced by the Title/Group/Hierarchy chunkers must be cropped on demand
// (mirroring the TokenChunker JSON path). The chunk shape mirrors the real
// Group/Hierarchy output: it carries doc_type_kwd + positions but NO ck_type
// (buildChunksFromRecordGroups never sets ck_type). cropTitleChunks must
// derive ck_type from doc_type_kwd so needsCrop (pdfcrop_cgo.go:151) fires.
// A text chunk with positions gets a rendered preview; a chunk without
// positions is left untouched; and the derived ck_type must not leak into
// the returned chunk shape.
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
	if _, ok := got[0]["ck_type"]; ok {
		t.Errorf("derived ck_type must not leak into output chunk: %v", got[0])
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

// TestTitleMediaContextCropIDProvenance exercises the real crop/upload bridge:
// only Title's finalized media IDs survive; unrelated upstream IDs are rebuilt.
func TestTitleMediaContextCropIDProvenance(t *testing.T) {
	ctx := withIngestionGlobals(t, "kb1", "doc1")
	rec := &recordingUploader{}
	orig := ChunkImageUploader
	ChunkImageUploader = rec.upload
	t.Cleanup(func() { ChunkImageUploader = orig })
	pos := jsonPositions(t, []float64{1, 10, 100, 10, 100})
	titleID := "source-scoped-media-id"
	title := cropTitleChunks(ctx, mockCropEngine{}, []map[string]any{
		{"text": "Context-only body.", "doc_type_kwd": "image", "id": titleID, "positions": pos},
	})
	if len(title) != 1 || title[0]["id"] != titleID || title[0]["img_id"] != "kb1-"+titleID {
		t.Fatalf("title crop lost its finalized identity: %#v", title)
	}
	if _, leaked := title[0]["_title_media_context_id"]; leaked {
		t.Fatal("internal crop provenance leaked into the output")
	}

	upstream := schema.ChunkDoc{
		ID: "stale-upstream-id", CKType: "image", Text: "Caption.@@1\t2##",
		ContextAbove: "Source explanation.\n", PDFPositions: pos,
	}
	want := common.ChunkID("doc1", chunkIDSource(upstream))
	other := cropImageChunks(ctx, mockCropEngine{}, []schema.ChunkDoc{upstream})
	if len(other) != 1 || other[0].ID != want || other[0].ImgID != "kb1-"+want {
		t.Fatalf("non-title crop reused upstream identity: %#v", other)
	}
	if len(rec.calls) != 2 || rec.calls[0].chunkID != titleID || rec.calls[1].chunkID != want {
		t.Fatalf("crop upload keys differ from output IDs: %#v", rec.calls)
	}
}
