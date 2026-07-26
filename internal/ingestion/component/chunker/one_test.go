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

// oneChunksOf drives OneChunker.Invoke and returns the emitted chunk maps.
func oneChunksOf(t *testing.T, inputs map[string]any) []map[string]any {
	t.Helper()
	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatalf("NewOneChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("OneChunker.Invoke: %v", err)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks not []map[string]any: %T", out["chunks"])
	}
	return chunks
}

// TestOneChunker_Text emits exactly one chunk for a text payload,
// faithful to rag/app/one.py (whole file = one chunk).
func TestOneChunker_Text(t *testing.T) {
	chunks := oneChunksOf(t, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "first paragraph\n\nsecond paragraph",
	})
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if got := chunks[0]["text"]; got != "first paragraph\n\nsecond paragraph" {
		t.Errorf("text = %q", got)
	}
}

// TestOneChunker_JSONSingleItem carries the image/media context through
// for the picture/audio methods, which TokenChunker in "one" mode drops.
func TestOneChunker_JSONSingleItem(t *testing.T) {
	chunks := oneChunksOf(t, map[string]any{
		"name":          "pic.png",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "a cat sitting on a mat", "image": "data:image/png;base64,AAAA", "doc_type_kwd": "image"},
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if got := chunks[0]["image"]; got != "data:image/png;base64,AAAA" {
		t.Errorf("image = %q, want preserved media context", got)
	}
	if got := chunks[0]["text"]; got != "a cat sitting on a mat" {
		t.Errorf("text = %q", got)
	}
}

// TestOneChunker_JSONMultipleMerges collapses many upstream items into a
// single chunk, preserving the first available image (picture/audio
// one-chunk-per-file behavior).
func TestOneChunker_JSONMultipleMerges(t *testing.T) {
	chunks := oneChunksOf(t, map[string]any{
		"name":          "clip.mp4",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "frame one transcript"},
			{"text": "frame two transcript", "image": "data:image/png;base64,BBBB"},
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if got := chunks[0]["text"]; got != "frame one transcript\nframe two transcript" {
		t.Errorf("merged text = %q", got)
	}
	if got := chunks[0]["image"]; got != "data:image/png;base64,BBBB" {
		t.Errorf("image = %q, want first available media context", got)
	}
}
