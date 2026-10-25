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
	"encoding/json"
	"reflect"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// TestMergePositionMatrixAcceptsPageLists pins the Title-family parity fix:
// positions produced by layout.SectionsToJSON keep the page as a list until
// normalization, and a source may be a single flat 5-tuple (TCADP). Both must
// survive the merge instead of being dropped, so the on-demand PDF crop can
// still run.
func TestMergePositionMatrixAcceptsPageLists(t *testing.T) {
	listPage := json.RawMessage(`[[[1],10,100,10,100]]`)
	flatRow := json.RawMessage(`[2,5,50,5,50]`)
	duplicate := json.RawMessage(`[[[1],10,100,10,100]]`)

	got := mergePositionMatrix(listPage, flatRow, duplicate)
	want := [][]any{
		{[]any{float64(1)}, float64(10), float64(100), float64(10), float64(100)},
		{float64(2), float64(5), float64(50), float64(5), float64(50)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergePositionMatrix = %#v, want %#v", got, want)
	}
}

// TestMergePositionMatrixPreservesNonNumericRows pins that a malformed row is
// kept verbatim instead of being silently coerced to zeros.
func TestMergePositionMatrixPreservesNonNumericRows(t *testing.T) {
	src := json.RawMessage(`[[1,"bad",100,10,100]]`)
	got := mergePositionMatrix(src)
	if len(got) != 1 {
		t.Fatalf("mergePositionMatrix = %#v, want the raw row", got)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(raw) != `[[1,"bad",100,10,100]]` {
		t.Errorf("raw rows = %s, want the source unchanged", raw)
	}
}

// TestTitleFamilyAcceptsFlatRowPositions covers the TCADP shape: a source may
// carry one flat 5-tuple instead of a list of rows. The title merge must nest
// it so the emitted chunk is still [[page,left,right,top,bottom]].
func TestTitleFamilyAcceptsFlatRowPositions(t *testing.T) {
	pos := []float64{1, 10, 100, 10, 100}
	items := []map[string]any{
		{"text": "before", "doc_type_kwd": "text"},
		{"text": "Figure 1: caption", "doc_type_kwd": "image", "positions": pos, "_pdf_positions": pos},
	}
	title, err := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	out, err := title.Invoke(t.Context(), nil, map[string]any{
		"name": "doc.pdf", "output_format": "json", "json": items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	var image map[string]any
	for _, ck := range chunks {
		if ck["doc_type_kwd"] == "image" {
			image = ck
		}
	}
	if image == nil {
		t.Fatalf("no image chunk emitted: %#v", chunks)
	}
	raw, err := json.Marshal(image["positions"])
	if err != nil {
		t.Fatalf("marshal positions: %v", err)
	}
	if string(raw) != `[[1,10,100,10,100]]` {
		t.Errorf("positions = %s, want [[1,10,100,10,100]]", raw)
	}
}

// TestPositionOrderingAcceptsListPages pins the ordering paths to the same
// shape handling as the merge: a list page cell must be seen as positioned,
// otherwise Manual/General leave records in parser order.
func TestPositionOrderingAcceptsListPages(t *testing.T) {
	first := json.RawMessage(`[[[1],10,100,100,200]]`)  // top 100
	second := json.RawMessage(`[[[1],10,100,300,400]]`) // top 300

	recs := []lineRecord{
		{text: "second", pdfPositions: second},
		{text: "first", pdfPositions: first},
	}
	if !hasPdfPositions(recs) {
		t.Fatal("hasPdfPositions = false for list-page positions")
	}
	if row, ok := firstPositionRow(recs[0]); !ok || row[0] != 1 || row[3] != 300 {
		t.Fatalf("firstPositionRow = %v, %v; want page 1 top 300", row, ok)
	}
	sortRecordsByPosition(recs)
	if recs[0].text != "first" || recs[1].text != "second" {
		t.Fatalf("sorted records = %q, %q; want first, second", recs[0].text, recs[1].text)
	}

	units := []schema.ChunkDoc{
		{Text: "second", PDFPositions: second},
		{Text: "first", PDFPositions: first},
	}
	if !hasPDFPositions(units) {
		t.Fatal("hasPDFPositions = false for list-page positions")
	}
	sorted := sortPDFUnits(units)
	if sorted[0].Text != "first" || sorted[1].Text != "second" {
		t.Fatalf("sortPDFUnits = %q, %q; want first, second", sorted[0].Text, sorted[1].Text)
	}
}

// TestMergePositionMatrixGroupsFlatMultiTuples pins the flat multi-tuple
// handling: a flat array carrying several 5-tuples (TCADP/external form) must
// be grouped by five instead of treated as one row, so no tuple is dropped
// and distinct sources sharing a first tuple are not deduplicated into one.
func TestMergePositionMatrixGroupsFlatMultiTuples(t *testing.T) {
	src := json.RawMessage(`[1,10,100,20,80,2,30,120,40,90]`)
	got := mergePositionMatrix(src)
	if len(got) != 2 {
		t.Fatalf("mergePositionMatrix flat multi-tuple = %#v, want 2 rows", got)
	}
	first, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	second, err := json.Marshal(got[1])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(first) != `[1,10,100,20,80]` || string(second) != `[2,30,120,40,90]` {
		t.Fatalf("mergePositionMatrix = %s %s, want [1,10,100,20,80] [2,30,120,40,90]", first, second)
	}

	a := json.RawMessage(`[1,10,100,20,80,2,30,120,40,90]`)
	b := json.RawMessage(`[1,10,100,20,80,3,30,120,40,90]`)
	got = mergePositionMatrix(a, b)
	if len(got) != 3 {
		t.Fatalf("mergePositionMatrix distinct flat sources = %#v, want 3 rows", got)
	}
}

// TestPositionOrderingAcceptsFlatMultiTuples pins that a flat multi-tuple
// record participates in physical-order sorting by its first tuple, while a
// malformed flat sequence (ambiguous boundaries) reports unpositioned instead
// of sorting by the wrong keys.
func TestPositionOrderingAcceptsFlatMultiTuples(t *testing.T) {
	multi := json.RawMessage(`[2,30,120,40,90,1,10,100,20,80]`)
	single := json.RawMessage(`[[1,10,100,10,100]]`)

	if row, ok := firstPositionRow(lineRecord{pdfPositions: multi}); !ok || row[0] != 2 || row[3] != 40 {
		t.Fatalf("firstPositionRow flat multi-tuple = %v, %v; want page 2 top 40", row, ok)
	}

	recs := []lineRecord{
		{text: "multi", pdfPositions: multi},
		{text: "single", pdfPositions: single},
	}
	sortRecordsByPosition(recs)
	if recs[0].text != "single" || recs[1].text != "multi" {
		t.Fatalf("sorted records = %q, %q; want single, multi", recs[0].text, recs[1].text)
	}

	units := []schema.ChunkDoc{
		{Text: "multi", PDFPositions: multi},
		{Text: "single", PDFPositions: single},
	}
	sorted := sortPDFUnits(units)
	if sorted[0].Text != "single" || sorted[1].Text != "multi" {
		t.Fatalf("sortPDFUnits = %q, %q; want single, multi", sorted[0].Text, sorted[1].Text)
	}

	malformed := json.RawMessage(`[1,10,100,20,80,2,30]`)
	if _, ok := firstPositionRow(lineRecord{pdfPositions: malformed}); ok {
		t.Fatal("firstPositionRow malformed flat = ok, want false for ambiguous boundaries")
	}
}

// TestTitleFamilyKeepsListPagePositions is the end-to-end guard: an image
// record whose page cell is a list must keep its coordinates on the emitted
// chunk, matching General's pass-through behavior.
func TestTitleFamilyKeepsListPagePositions(t *testing.T) {
	pos := [][]any{{[]any{1}, 10, 100, 10, 100}}
	items := []map[string]any{
		{"text": "before", "doc_type_kwd": "text"},
		{"text": "Figure 1: caption", "doc_type_kwd": "image", "positions": pos, "_pdf_positions": pos},
	}
	levels := [][]string{{`^# `}}

	title, err := NewGroupTitleChunker(map[string]any{"levels": levels})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	out, err := title.Invoke(t.Context(), nil, map[string]any{
		"name": "doc.pdf", "output_format": "json", "json": items,
	})
	if err != nil {
		t.Fatalf("Title Invoke: %v", err)
	}
	titleChunks, _ := out["chunks"].([]map[string]any)
	var titleImage map[string]any
	for _, ck := range titleChunks {
		if ck["doc_type_kwd"] == "image" {
			titleImage = ck
		}
	}
	if titleImage == nil {
		t.Fatalf("Title emitted no image chunk: %#v", titleChunks)
	}
	assertListPagePositions(t, "title", titleImage)

	general, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	gout, err := general.Invoke(t.Context(), nil, map[string]any{
		"name": "doc.pdf", "file_type": "pdf", "output_format": "json", "json": items,
	})
	if err != nil {
		t.Fatalf("General Invoke: %v", err)
	}
	generalChunks, _ := gout["chunks"].([]map[string]any)
	var generalImage map[string]any
	for _, ck := range generalChunks {
		if ck["doc_type_kwd"] == "image" {
			generalImage = ck
		}
	}
	if generalImage == nil {
		t.Fatalf("General emitted no image chunk: %#v", generalChunks)
	}
	assertListPagePositions(t, "general", generalImage)
}

func assertListPagePositions(t *testing.T, name string, ck map[string]any) {
	t.Helper()
	want := []any{[]any{[]any{float64(1)}, float64(10), float64(100), float64(10), float64(100)}}
	for _, key := range []string{"positions", "_pdf_positions"} {
		raw, err := json.Marshal(ck[key])
		if err != nil {
			t.Fatalf("%s %s: marshal: %v", name, key, err)
		}
		var got any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%s %s: unmarshal: %v", name, key, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s %s = %s, want [[[1],10,100,10,100]]", name, key, raw)
		}
	}
}
