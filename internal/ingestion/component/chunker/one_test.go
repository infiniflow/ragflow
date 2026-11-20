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
	"encoding/json"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// oneChunksOf drives OneChunker.Invoke and returns the emitted chunk maps.
func oneChunksOf(t *testing.T, inputs map[string]any) []map[string]any {
	t.Helper()
	comp, err := NewOneChunker(nil)
	if err != nil {
		t.Fatalf("NewOneChunker: %v", err)
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("OneChunker.Invoke: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok {
		t.Fatalf("chunks not []map[string]any: %T", out["chunks"])
	}
	return chunks
}

// TestNewChunkerByName_TokenChunkerOneMode pins the DSL-contract
// translation: a TokenChunker component whose delimiter_mode is "one"
// (what the web UI and the Python runtime emit) must build a
// OneChunker, not fail schema validation.
func TestNewChunkerByName_TokenChunkerOneMode(t *testing.T) {
	comp, err := newChunkerByName(ComponentNameTokenChunker, map[string]any{"delimiter_mode": "one"})
	if err != nil {
		t.Fatalf("newChunkerByName: %v", err)
	}
	if _, ok := comp.(*OneChunkerComponent); !ok {
		t.Fatalf("component type = %T, want *OneChunkerComponent", comp)
	}
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

// TestOneChunker_PreservesPositions verifies that when a single upstream
// item carries PDF coordinates, the OneChunker preserves both Positions
// and PDFPositions (with their coordinate values) on the output chunk.
func TestOneChunker_PreservesPositions(t *testing.T) {
	chunks := oneChunksOf(t, map[string]any{
		"name":          "page.pdf",
		"output_format": "json",
		"json": []map[string]any{
			{
				"text":           "page text",
				"positions":      []any{[]any{10.0, 20.0, 30.0, 40.0}},
				"_pdf_positions": []any{[]any{1.0, 2.0, 3.0, 4.0, 5.0}},
			},
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	assertCoordTuple(t, "positions", chunks[0]["positions"], []float64{10.0, 20.0, 30.0, 40.0})
	assertCoordTuple(t, "_pdf_positions", chunks[0]["_pdf_positions"], []float64{1.0, 2.0, 3.0, 4.0, 5.0})
}

// assertCoordTuple verifies a positions/_pdf_positions field round-tripped
// as a [][]float64 with the expected single-row coordinate tuple.
func assertCoordTuple(t *testing.T, key string, got any, want []float64) {
	t.Helper()
	rows, ok := got.([][]float64)
	if !ok {
		t.Fatalf("%s = %v, want [][]float64 (got %T)", key, got, got)
	}
	if len(rows) != 1 {
		t.Fatalf("%s has %d rows, want 1", key, len(rows))
	}
	if len(rows[0]) != len(want) {
		t.Fatalf("%s[0] = %v, want %v (len %d vs %d)", key, rows[0], want, len(rows[0]), len(want))
	}
	for i, w := range want {
		if rows[0][i] != w {
			t.Errorf("%s[0][%d] = %v, want %v", key, i, rows[0][i], w)
		}
	}
}

// TestOneChunkerKeepsImgIDForTextlessItem covers the single-item guard: an
// item kept alive by ImgID must carry that ID into the output chunk.
func TestOneChunkerKeepsImgIDForTextlessItem(t *testing.T) {
	out := emitOneFromItems([]schema.ChunkDoc{{DocType: "image", ImgID: "kb-1-chunk-1"}}, nil)
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	if chunks[0]["img_id"] != "kb-1-chunk-1" {
		t.Fatalf("img_id = %v, want kb-1-chunk-1", chunks[0]["img_id"])
	}
}

func TestOneChunker_MultiItemMergesPDFPositions(t *testing.T) {
	chunks := oneChunksOf(t, map[string]any{
		"name":          "resume.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"json": []map[string]any{
			{
				"text":           "Page 2 experience",
				"_pdf_positions": [][]float64{{2, 20, 200, 40, 80}},
				"positions":      [][]float64{{2, 20, 200, 40, 80}},
			},
			{
				"text":           "Page 1 header",
				"_pdf_positions": [][]float64{{1, 10, 200, 10, 30}, {2, 20, 200, 40, 80}},
				"positions":      [][]float64{{1, 10, 200, 10, 30}, {3, 0, 0, 0, 0}},
			},
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	pdfRows, ok := chunks[0]["_pdf_positions"].([][]float64)
	if !ok {
		t.Fatalf("_pdf_positions type = %T, want [][]float64", chunks[0]["_pdf_positions"])
	}
	if len(pdfRows) != 2 || pdfRows[0][0] != 1 || pdfRows[1][0] != 2 {
		t.Fatalf("_pdf_positions = %v, want sorted deduplicated page 1 then page 2", pdfRows)
	}
	posRows, ok := chunks[0]["positions"].([][]float64)
	if !ok {
		t.Fatalf("positions type = %T, want [][]float64", chunks[0]["positions"])
	}
	if len(posRows) != 3 {
		t.Fatalf("positions rows = %v, want 3 rows", posRows)
	}
}

func TestOneChunker_MultiItemSpreadsheetSkipsPDFPositions(t *testing.T) {
	chunks := oneChunksOf(t, map[string]any{
		"name":          "sheet.xlsx",
		"file_type":     "xlsx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "row 1", "sheet": "Sheet1", "positions": [][]float64{{1, 0, 1, 0, 2}}},
			{"text": "row 2", "sheet": "Sheet1", "positions": [][]float64{{1, 1, 2, 0, 2}}},
		},
	})
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if _, exists := chunks[0]["positions"]; exists {
		t.Fatalf("spreadsheet positions should not be merged as PDF positions: %v", chunks[0]["positions"])
	}
}

func TestOneChunker_NonPDFSkipsPDFPositionsAndRejectsMalformedFlatPositions(t *testing.T) {
	nonPDF := oneChunksOf(t, map[string]any{
		"name":          "notes.docx",
		"file_type":     "docx",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "para 1", "_pdf_positions": [][]float64{{1, 10, 100, 10, 30}}},
			{"text": "para 2", "_pdf_positions": [][]float64{{2, 10, 100, 40, 60}}},
		},
	})
	if len(nonPDF) != 1 {
		t.Fatalf("want 1 non-PDF chunk, got %d", len(nonPDF))
	}
	if _, exists := nonPDF[0]["_pdf_positions"]; exists {
		t.Fatalf("non-PDF chunk should not merge _pdf_positions: %v", nonPDF[0]["_pdf_positions"])
	}

	if got := mergeRawPositionMatrix(json.RawMessage(`[1, 10, 100, 20, 80, 999]`)); got != nil {
		t.Fatalf("6-element flat position array should be rejected, got %s", got)
	}
	gotValid := mergeRawPositionMatrix(
		json.RawMessage(`[[1, 10, 100, 20, 80], [2, 1, 2, 3, 4, 5]]`),
	)
	if string(gotValid) != `[[1,10,100,20,80]]` {
		t.Fatalf("mergeRawPositionMatrix mixed valid/malformed = %s, want [[1,10,100,20,80]]", gotValid)
	}
	gotMalformedFirst := mergeRawPositionMatrix(
		json.RawMessage(`[1, 10, 100, 20, 80, 999]`),
		json.RawMessage(`[1, 10, 100, 20, 80]`),
	)
	if string(gotMalformedFirst) != `[[1,10,100,20,80]]` {
		t.Fatalf("malformed row before valid row = %s, want [[1,10,100,20,80]]", gotMalformedFirst)
	}
}
