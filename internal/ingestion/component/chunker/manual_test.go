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
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
)

// manualChunkerInput is a small builder for a structured (output_format=chunks)
// upstream payload, mirroring the shape the Parser emits for PDF/JSON output.
func manualChunkerInput(name string, items ...map[string]any) map[string]any {
	return map[string]any{
		"name":          name,
		"output_format": "chunks",
		"chunks":        items,
	}
}

// posItem builds a chunk item carrying a PDF position matrix
// [page,left,right,top,bottom] under the `_pdf_positions` key (matching
// schema.ChunkDoc.PDFPositions).
func posItem(text string, page, left, right, top, bottom float64) map[string]any {
	return map[string]any{
		"text":           text,
		"doc_type_kwd":   "text",
		"_pdf_positions": [][]float64{{page, left, right, top, bottom}},
	}
}

// mustPosJSON marshals a single 5-tuple position matrix into the
// json.RawMessage form used by lineRecord.pdfPositions / positions.
func mustPosJSON(t *testing.T, page, left, right, top, bottom float64) json.RawMessage {
	t.Helper()
	b, err := json.Marshal([][]float64{{page, left, right, top, bottom}})
	if err != nil {
		t.Fatalf("marshal positions: %v", err)
	}
	return b
}

func plainItem(text string) map[string]any {
	return map[string]any{"text": text, "doc_type_kwd": "text"}
}

func mustManual(t *testing.T, params map[string]any) *ManualChunkerComponent {
	t.Helper()
	c, err := NewManualChunker(params)
	if err != nil {
		t.Fatalf("NewManualChunker: %v", err)
	}
	mc, ok := c.(*ManualChunkerComponent)
	if !ok {
		t.Fatalf("NewManualChunker returned %T, want *ManualChunkerComponent", c)
	}
	return mc
}

// manualChunkTexts pulls the ordered chunk texts out of a chunker output map.
func manualChunkTexts(out map[string]any) []string {
	chunks, _ := out["chunks"].([]map[string]any)
	out2 := make([]string, 0, len(chunks))
	for _, ck := range chunks {
		if s, _ := ck["text"].(string); s != "" {
			out2 = append(out2, s)
		}
	}
	return out2
}

// TestManualChunker_Registered pins that the component is discoverable via the
// runtime registry under the CategoryIngestion category with non-empty
// input/output metadata.
func TestManualChunker_Registered(t *testing.T) {
	factory, cat, meta, ok := runtime.DefaultRegistry.Lookup(ComponentNameManualChunker)
	if !ok {
		t.Fatal("ManualChunker: registry miss")
	}
	if cat != runtime.CategoryIngestion {
		t.Errorf("category = %q, want %q", cat, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Error("factory is nil")
	}
	if len(meta.Inputs) == 0 {
		t.Error("inputs metadata empty")
	}
	if len(meta.Outputs) == 0 {
		t.Error("outputs metadata empty")
	}
}

// TestManualChunker_Empty is the trivial boundary: no records => empty outputs.
func TestManualChunker_Empty(t *testing.T) {
	c := mustManual(t, map[string]any{"levels": [][]string{{`^# `}}})
	out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "doc"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "chunks"; got != want {
		t.Errorf("output_format = %v, want %v", got, want)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Errorf("chunks = %d, want 0", len(chunks))
	}
}

// TestManualChunker_NoPositionsEqualsGroupChunker is the regression lock: when
// the upstream payload carries NO physical coordinates (e.g. docx, which Go
// emits in document-logical order — exactly as Python's manual docx branch,
// which performs no coordinate resort), ManualChunker MUST behave bit-for-bit
// like GroupTitleChunker. Any divergence here is an unintended regression.
func TestManualChunker_NoPositionsEqualsGroupChunker(t *testing.T) {
	var mkLevels = [][]string{{`^# `}, {`^## `}}
	input := manualChunkerInput("doc",
		plainItem("# H1"),
		plainItem("body under h1 a"),
		plainItem("body under h1 b"),
		plainItem("## H2"),
		plainItem("body under h2"),
		plainItem("more body under h2"),
	)

	gc, err := NewGroupTitleChunker(map[string]any{"levels": mkLevels})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	gOut, err := gc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("group Invoke: %v", err)
	}

	mc := mustManual(t, map[string]any{"levels": mkLevels})
	mOut, err := mc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("manual Invoke: %v", err)
	}

	gChunks, _ := gOut["chunks"].([]map[string]any)
	mChunks, _ := mOut["chunks"].([]map[string]any)
	if !reflect.DeepEqual(gChunks, mChunks) {
		t.Fatalf("ManualChunker output diverges from GroupTitleChunker on no-coordinate input\n group: %#v\n manual: %#v", gChunks, mChunks)
	}
}

// TestManualChunker_SortsByPageTopLeft is the core behavioural difference
// vs. GroupTitleChunker. For a multi-column / manual PDF, the parser emits
// records in READING order (column-by-column), not in PHYSICAL/top-down
// order. ManualChunker must resort records by (page, top, left) before
// grouping so that, e.g., the left column's content precedes the right
// column's when they share a page band.
func TestManualChunker_SortsByPageTopLeft(t *testing.T) {
	// page 1, all body (no heading markers). Physical layout:
	//   top=100, left=10  -> "left col top"
	//   top=100, left=400 -> "right col"
	//   top=300, left=10  -> "left col bottom"
	//   top=400, left=200 -> "footer title"
	// Parser reading order is scrambled relative to physical order.
	input := manualChunkerInput("doc",
		posItem("footer title", 1, 200, 400, 400, 500),
		posItem("left col bottom", 1, 10, 200, 300, 400),
		posItem("right col", 1, 400, 600, 100, 400),
		posItem("left col top", 1, 10, 200, 100, 200),
	)

	mc := mustManual(t, map[string]any{"levels": [][]string{{`^# `}}})
	out, err := mc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// Whether merged into one chunk or several, the concatenated text MUST
	// follow physical order: left col top, right col, left col bottom,
	// footer title.
	var full strings.Builder
	for _, s := range manualChunkTexts(out) {
		full.WriteString(s)
		full.WriteString("\n")
	}
	joined := full.String()
	wantOrder := []string{"left col top", "right col", "left col bottom", "footer title"}
	last := -1
	for _, w := range wantOrder {
		idx := strings.Index(joined, w)
		if idx < 0 {
			t.Fatalf("expected %q to appear in manual output %q", w, joined)
		}
		if idx <= last {
			t.Fatalf("physical order violated: %q appears after a later element in %q", w, joined)
		}
		last = idx
	}
}

// TestManualChunker_SortDiffersFromGroupWhenPositionsPresent proves the resort
// actually fires (and is not a no-op): the same scrambled input must produce a
// DIFFERENT text order from GroupTitleChunker, which groups in input order.
func TestManualChunker_SortDiffersFromGroupWhenPositionsPresent(t *testing.T) {
	input := manualChunkerInput("doc",
		posItem("parser-reads-2nd-col-first", 1, 400, 600, 100, 400),
		posItem("parser-reads-1st-col-first", 1, 10, 200, 100, 200),
	)

	gc, _ := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	gOut, err := gc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("group Invoke: %v", err)
	}
	mc := mustManual(t, map[string]any{"levels": [][]string{{`^# `}}})
	mOut, err := mc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("manual Invoke: %v", err)
	}
	if reflect.DeepEqual(gOut["chunks"], mOut["chunks"]) {
		t.Fatalf("ManualChunker did not reorder relative to GroupTitleChunker; both = %#v", mOut["chunks"])
	}
	// ManualChunker must place the 1st-column item first.
	if texts := manualChunkTexts(mOut); len(texts) == 0 || !strings.Contains(texts[0], "1st-col") {
		t.Fatalf("expected 1st-column text first in manual output, got %#v", texts)
	}
}

// TestManualChunker_MergesPositions pins parity with GroupTitleChunker: when
// adjacent text records are merged into one chunk, the PDF position matrices
// are MERGED across the merged records (not dropped), and parser position tags
// are stripped from the text.
func TestManualChunker_MergesPositions(t *testing.T) {
	input := manualChunkerInput("doc",
		posItem("body one", 1, 10, 20, 30, 40),
		posItem("body two", 2, 15, 25, 35, 45),
	)
	mc := mustManual(t, map[string]any{"levels": [][]string{{`^# `}}})
	out, err := mc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	found := false
	for _, ck := range chunks {
		text, _ := ck["text"].(string)
		if !strings.Contains(text, "body one") || !strings.Contains(text, "body two") {
			continue
		}
		found = true
		if strings.Contains(text, "@@") {
			t.Errorf("parser position tags leaked into chunk text: %q", text)
		}
		pos, ok := ck["_pdf_positions"].([][]float64)
		if !ok {
			t.Fatalf("_pdf_positions missing or wrong type %T on merged group chunk", ck["_pdf_positions"])
		}
		if len(pos) != 2 {
			// Hard fail: the page/top/left checks below would index out of
			// range if the merged matrix does not have exactly two rows.
			t.Fatalf("merged _pdf_positions = %d rows, want 2 (both records)", len(pos))
		}
		// Merged matrix must itself be sorted by (page, top, left) — the
		// same key pdfPosRowLess uses when merging.
		if pdfPosRowLess(pos[1], pos[0]) {
			t.Errorf("merged _pdf_positions not sorted by (page, top, left): %v", pos)
		}
	}
	if !found {
		t.Fatal("merged body group chunk not found in output")
	}
}

// TestSortRecordsByPosition_Unit pins the comparator directly: (page, top,
// left) ascending, stable for equal keys, and records without positions keep
// their original relative order (so a no-coordinate payload is untouched).
func TestSortRecordsByPosition_Unit(t *testing.T) {
	recs := []lineRecord{
		{text: "a", pdfPositions: mustPosJSON(t, 1, 200, 300, 400, 500)},
		{text: "b", pdfPositions: mustPosJSON(t, 1, 10, 100, 300, 400)},
		{text: "c", pdfPositions: mustPosJSON(t, 1, 10, 100, 100, 200)},
		{text: "d"}, // no position
	}
	sortRecordsByPosition(recs)
	got := []string{recs[0].text, recs[1].text, recs[2].text, recs[3].text}
	want := []string{"c", "b", "a", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sorted order = %v, want %v", got, want)
	}
}

// TestHasPdfPositions is a tiny white-box check for the gate that decides
// whether a resort is performed at all.
func TestHasPdfPositions(t *testing.T) {
	if hasPdfPositions([]lineRecord{{text: "x"}}) {
		t.Error("plain record should report no positions")
	}
	if !hasPdfPositions([]lineRecord{{text: "x", pdfPositions: mustPosJSON(t, 1, 1, 1, 1, 1)}}) {
		t.Error("record with pdf_positions should report positions")
	}
	if !hasPdfPositions([]lineRecord{{text: "x", positions: mustPosJSON(t, 1, 1, 1, 1, 1)}}) {
		t.Error("record with positions should report positions")
	}
}

// parserShapedItem builds a chunk item that mirrors EXACTLY the JSON shape the
// Go PDF parser emits via pdfParseResultToJSON (see
// TestPDFParseResultToJSON_NormalizesCoreFields): _pdf_positions is a [][]any
// with 1-based page numbers, accompanied by layout / page_number /
// doc_type_kwd. Using [][]any — not [][]float64 — matters: that is the concrete
// in-memory type the parser produces, and recordsFromStructured must consume it
// after the chunksFromInputs JSON round-trip.
func parserShapedItem(text string, page, left, right, top, bottom float64) map[string]any {
	return map[string]any{
		"text":           text,
		"doc_type_kwd":   "text",
		"layout":         "text",
		"page_number":    int(page),
		"_pdf_positions": [][]any{{page, left, right, top, bottom}},
	}
}

// legacyPosItem is like posItem but uses the legacy `positions` key instead of
// the structured `_pdf_positions` key.
func legacyPosItem(text string, page, left, right, top, bottom float64) map[string]any {
	return map[string]any{
		"text":         text,
		"doc_type_kwd": "text",
		"positions":    [][]float64{{page, left, right, top, bottom}},
	}
}

// TestManualChunker_RealParserShapedPayload exercises the EXACT payload shape
// the Go PDF parser delivers to the chunker (pdfParseResultToJSON ->
// recordsFromStructured). It closes the coverage gap where the template
// integration test only feeds a .txt fixture (no coordinates): here a
// coordinate-bearing, parser-shaped multi-column doc proves the (page, top,
// left) resort actually fires and produces a different order from
// GroupTitleChunker (which groups in parser reading order).
func TestManualChunker_RealParserShapedPayload(t *testing.T) {
	input := manualChunkerInput("doc",
		// Parser reading order (column-by-column) is scrambled vs physical.
		parserShapedItem("right column, read first", 1, 400, 600, 100, 400),
		parserShapedItem("left column, read second", 1, 10, 200, 100, 200),
	)

	gc, _ := NewGroupTitleChunker(map[string]any{"levels": [][]string{{`^# `}}})
	gOut, err := gc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("group Invoke: %v", err)
	}
	mc := mustManual(t, map[string]any{"levels": [][]string{{`^# `}}})
	mOut, err := mc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("manual Invoke: %v", err)
	}
	if reflect.DeepEqual(gOut["chunks"], mOut["chunks"]) {
		t.Fatalf("ManualChunker did not reorder parser-shaped payload; both = %#v", mOut["chunks"])
	}
	// Physical (page, top, left) order must put the left column first.
	if texts := manualChunkTexts(mOut); len(texts) == 0 || !strings.Contains(texts[0], "left column") {
		t.Fatalf("expected left-column text first after resort, got %#v", texts)
	}
}

// TestManualChunker_LegacyPositionsKeyTriggersSort pins that the resort fires
// when records carry the LEGACY `positions` key rather than the structured
// `_pdf_positions` key. firstPositionRow (and thus the shared pdfPosRowLess
// comparator) must read both forms, otherwise a payload emitted with the old
// key would silently skip the resort.
func TestManualChunker_LegacyPositionsKeyTriggersSort(t *testing.T) {
	input := manualChunkerInput("doc",
		legacyPosItem("right column, read first", 1, 400, 600, 100, 400),
		legacyPosItem("left column, read second", 1, 10, 200, 100, 200),
	)
	mc := mustManual(t, map[string]any{"levels": [][]string{{`^# `}}})
	out, err := mc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// Without the legacy-key path the output would keep parser reading order
	// (right column before left). The resort must place the left column first.
	if texts := manualChunkTexts(out); len(texts) == 0 || !strings.Contains(texts[0], "left column") {
		t.Fatalf("legacy `positions` key did not trigger resort; got %#v", texts)
	}
}

// TestManualChunker_MixedPositionedAndPlain exercises the boundary where a
// payload mixes coordinate-bearing records with coordinate-free records. A
// single document type will not usually do this (production payloads are
// homogeneously positioned-or-plain), but the sort must not corrupt order when
// it happens: the contiguous positioned run reorders by (page, top, left) and
// coordinate-free records keep their ORIGINAL relative order. (Go's stable
// sort only reorders within a contiguous positioned run — a coordinate-free
// record between two positioned records correctly stops the reorder from
// crossing it, which is why the positioned records here are kept adjacent.)
func TestManualChunker_MixedPositionedAndPlain(t *testing.T) {
	input := manualChunkerInput("doc",
		plainItem("plain A"),
		posItem("right col", 1, 400, 600, 100, 400),
		posItem("left col", 1, 10, 200, 100, 200),
		plainItem("plain B"),
	)
	mc := mustManual(t, map[string]any{"levels": [][]string{{`^# `}}})
	out, err := mc.Invoke(t.Context(), nil, input)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	joined := strings.Join(manualChunkTexts(out), "\n")
	// The contiguous positioned run keeps physical order relative to itself.
	if i, j := strings.Index(joined, "left col"), strings.Index(joined, "right col"); i < 0 || j < 0 || i > j {
		t.Fatalf("positioned records not in physical order in %q", joined)
	}
	// Coordinate-free records keep their original relative order.
	if i, j := strings.Index(joined, "plain A"), strings.Index(joined, "plain B"); i < 0 || j < 0 || i > j {
		t.Fatalf("coordinate-free records lost original order in %q", joined)
	}
}
