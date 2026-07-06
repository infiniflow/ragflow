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

// Slice 2 tests for port-rag-flow-pipeline-to-go.md Phase 3.
// Pin the chunkFromItem pass-through of python-side fields:
//
//   - mom          — parent-section identifier (title / hierarchy
//                    chunkers populate; TokenChunker forwards it).
//   - img_id       — image attachment identifier.
//   - layout       — layout classification.
//   - _pdf_positions — PDF bbox coordinates.
//
// Each test seeds a JSON-style chunk item with the field set,
// invokes TokenChunker through its public path, and asserts the
// emitted chunk carries the field through unchanged.

package chunker

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

// invokeAsTokenChunker constructs a TokenChunker component and
// drives a single JSON-payload Invoke. Mirrors the production
// token.go:invokeJSONPayload path.
func invokeAsTokenChunker(t *testing.T, items []map[string]any) map[string]any {
	t.Helper()
	comp, err := NewTokenChunker(map[string]any{})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), map[string]any{
		"name":          "test",
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("TokenChunker.Invoke: %v", err)
	}
	return out
}

// TestChunker_PreservesMomField pins the mom pass-through. Title
// and hierarchy chunkers populate mom on each item; the
// TokenChunker pass-through must preserve it on the emitted
// chunk so downstream components can re-attach the parent section.
func TestChunker_PreservesMomField(t *testing.T) {
	items := []map[string]any{
		{"text": "Section body.", "doc_type_kwd": "text", "mom": "sec-1"},
	}
	out := invokeAsTokenChunker(t, items)
	chunks := chunksFromOutput(t, out)
	if len(chunks) == 0 {
		t.Fatal("no chunks emitted")
	}
	if got, want := chunks[0]["mom"], "sec-1"; got != want {
		t.Errorf("chunks[0].mom = %v, want %v", got, want)
	}
}

// TestChunker_PreservesImgIDField pins the img_id pass-through.
// PDF / DOCX parsers attach img_id to image-bearing items; the
// chunker must preserve it so downstream image2id can upload
// the binary to MinIO.
func TestChunker_PreservesImgIDField(t *testing.T) {
	items := []map[string]any{
		{"text": "Caption.", "doc_type_kwd": "image", "img_id": "img-007"},
	}
	out := invokeAsTokenChunker(t, items)
	chunks := chunksFromOutput(t, out)
	if len(chunks) == 0 {
		t.Fatal("no chunks emitted")
	}
	if got, want := chunks[0]["img_id"], "img-007"; got != want {
		t.Errorf("chunks[0].img_id = %v, want %v", got, want)
	}
}

// TestChunker_PreservesLayoutField pins the layout pass-through.
// Layout-aware components (token_chunker._layout) use this field
// to switch processing paths (text vs table vs figure).
func TestChunker_PreservesLayoutField(t *testing.T) {
	items := []map[string]any{
		{"text": "Cell A1 | Cell B1\nCell A2 | Cell B2", "doc_type_kwd": "table", "layout": "table"},
	}
	out := invokeAsTokenChunker(t, items)
	chunks := chunksFromOutput(t, out)
	if len(chunks) == 0 {
		t.Fatal("no chunks emitted")
	}
	if got, want := chunks[0]["layout"], "table"; got != want {
		t.Errorf("chunks[0].layout = %v, want %v", got, want)
	}
}

// TestChunker_PassesPDFPositionsThrough pins the _pdf_positions
// pass-through. The PDF parser path (DeepDOC) attaches bbox
// coordinates under this key; the chunker must forward them
// so downstream layout-aware components can rebuild the page
// geometry.
func TestChunker_PassesPDFPositionsThrough(t *testing.T) {
	positions := [][]float64{{0.1, 0.2, 0.3, 0.4}, {0.5, 0.6, 0.7, 0.8}}
	items := []map[string]any{
		{"text": "PDF paragraph.", "doc_type_kwd": "text", "_pdf_positions": positions},
	}
	out := invokeAsTokenChunker(t, items)
	chunks := chunksFromOutput(t, out)
	if len(chunks) == 0 {
		t.Fatal("no chunks emitted")
	}
	got, ok := chunks[0]["_pdf_positions"]
	if !ok {
		t.Fatalf("chunks[0] missing _pdf_positions; keys=%v", keysOf(chunks[0]))
	}
	if len(got.([][]float64)) != 2 {
		t.Errorf("chunks[0]._pdf_positions len = %d, want 2", len(got.([][]float64)))
	}
}

// TestChunker_FallsBackToContentWithWeight pins the
// content_with_weight fallback on the chunker read side. Python's
// token_chunker.py:111 falls back to content_with_weight when
// the text field is empty.
func TestChunker_FallsBackToContentWithWeight(t *testing.T) {
	items := []map[string]any{
		{"content_with_weight": "fallback text", "doc_type_kwd": "text"},
	}
	out := invokeAsTokenChunker(t, items)
	chunks := chunksFromOutput(t, out)
	if len(chunks) == 0 {
		t.Fatal("no chunks emitted")
	}
	if got, want := chunks[0]["text"], "fallback text"; got != want {
		t.Errorf("chunks[0].text = %v, want %v (content_with_weight fallback)", got, want)
	}
}

// TestChunker_EmptyInputProducesNoChunks pins the empty-input
// behaviour. An empty items slice should produce an empty chunks
// list (mirrors python `_build_json_chunks` for an empty input).
func TestChunker_EmptyInputProducesNoChunks(t *testing.T) {
	out := invokeAsTokenChunker(t, nil)
	chunks := chunksFromOutput(t, out)
	if len(chunks) != 0 {
		t.Errorf("empty input produced %d chunks, want 0", len(chunks))
	}
}

// chunksFromOutput pulls the chunks list out of a TokenChunker
// output. The component emits output_format=chunks with the list
// under "chunks".
func chunksFromOutput(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	raw, ok := out["chunks"]
	if !ok {
		t.Fatalf("output missing 'chunks' key; got keys=%v", keysOf(out))
	}
	switch v := raw.(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		t.Fatalf("chunks: unexpected type %T", raw)
		return nil
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestSlice2_RegisteredContract pins the runtime registry contract
// for TokenChunker. Phase 3 docs reference it as the canonical
// chunker component; this test ensures the registration survived
// the chunkFromItem refactor.
func TestSlice2_RegisteredContract(t *testing.T) {
	factory, category, _, ok := runtime.DefaultRegistry.Lookup("TokenChunker")
	if !ok || factory == nil {
		t.Fatal("TokenChunker not registered in DefaultRegistry")
	}
	if category != runtime.CategoryIngestion {
		t.Errorf("category = %q, want %q", category, runtime.CategoryIngestion)
	}
}

// TestSlice2_NoStringOnlyLayout silently checks that the
// buildChunkMap helper does not panic when the input item has
// layout="string-with-spaces" (a regex error scenario). The
// pass-through uses a free-form map copy, so any string is OK.
func TestSlice2_NoStringOnlyLayout(t *testing.T) {
	items := []map[string]any{
		{"text": "x", "doc_type_kwd": "text", "layout": "figure with caption"},
	}
	out := invokeAsTokenChunker(t, items)
	chunks := chunksFromOutput(t, out)
	if chunks[0]["layout"] != "figure with caption" {
		t.Errorf("layout pass-through broken: got %v", chunks[0]["layout"])
	}
	_ = strings.TrimSpace // avoid unused-import warning when the test list shrinks
}
