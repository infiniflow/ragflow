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
	"context"
	"testing"
)

// slideChunksOf drives PresentationChunker.Invoke and returns the emitted maps.
func slideChunksOf(t *testing.T, inputs map[string]any) []map[string]any {
	t.Helper()
	comp, err := NewPresentationChunker(nil)
	if err != nil {
		t.Fatalf("NewPresentationChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("PresentationChunker.Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks not []map[string]any: %T", out["chunks"])
	}
	return chunks
}

// TestPresentationChunker_OneChunkPerSlide ports rag/app/presentation.py's
// contract: each slide becomes exactly one chunk, and per-slide metadata
// (page_number, image) survives the chunker round-trip.
func TestPresentationChunker_OneChunkPerSlide(t *testing.T) {
	chunks := slideChunksOf(t, map[string]any{
		"name":          "deck.pptx",
		"output_format": "json",
		"json": []map[string]any{
			{
				"text":         "Slide 1 content",
				"doc_type_kwd": "image",
				"page_number":  float64(1),
				"image":        "base64slide1",
			},
			{
				"text":         "Slide 2 content",
				"doc_type_kwd": "image",
				"page_number":  float64(2),
				"image":        "base64slide2",
			},
			{
				"text":         "Slide 3 content",
				"doc_type_kwd": "image",
				"page_number":  float64(3),
			},
		},
	})

	if len(chunks) != 3 {
		t.Fatalf("expected 3 slides, got %d", len(chunks))
	}

	for i, ck := range chunks {
		wantPage := float64(i + 1)
		if ck["page_number"] != wantPage {
			t.Errorf("slide %d: page_number = %v, want %v", i+1, ck["page_number"], wantPage)
		}
		if ck["doc_type_kwd"] != "image" {
			t.Errorf("slide %d: doc_type_kwd = %v, want image", i+1, ck["doc_type_kwd"])
		}
		if ck["text"] == "" {
			t.Errorf("slide %d: empty text", i+1)
		}
	}

	// Per-slide image attachments must be preserved (the key win over
	// TokenChunker "one", which drops media context).
	if chunks[0]["image"] != "base64slide1" {
		t.Errorf("slide 1: image = %v, want base64slide1", chunks[0]["image"])
	}
	if chunks[1]["image"] != "base64slide2" {
		t.Errorf("slide 2: image = %v, want base64slide2", chunks[1]["image"])
	}
	// A slide with text but no image still yields one chunk.
	if _, ok := chunks[2]["image"]; ok {
		t.Errorf("slide 3: unexpected image key present")
	}
}

// TestPresentationChunker_Empty yields no chunks when there is no data.
func TestPresentationChunker_Empty(t *testing.T) {
	chunks := slideChunksOf(t, map[string]any{
		"name":          "deck.pptx",
		"output_format": "json",
		"json":          []map[string]any{},
	})
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

// TestPresentationChunker_NilInput yields no chunks.
func TestPresentationChunker_NilInput(t *testing.T) {
	chunks := slideChunksOf(t, nil)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for nil input, got %d", len(chunks))
	}
}
