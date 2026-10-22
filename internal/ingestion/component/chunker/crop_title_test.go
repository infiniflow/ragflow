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

// TestCropTitleChunks_NilEnginePassthrough asserts the helper is a no-op
// when no PDF engine is available (best-effort contract).
func TestCropTitleChunks_NilEnginePassthrough(t *testing.T) {
	in := []map[string]any{{"text": "hello"}}
	got := cropTitleChunks(context.Background(), nil, in)
	if len(got) != 1 || got[0]["text"] != "hello" {
		t.Fatalf("nil engine should pass through unchanged: %v", got)
	}
}
