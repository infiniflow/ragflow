package parser

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// The wire tests lock what each remote PDF producer hands to the chunkers
// after PDFParser.NormalizeResult. Every producer fills ExternalBlocks and
// shares one converter, so a change to that converter shows up here as a diff
// against the recorded output of every producer at once.

// externalBlockWire is the recorded form: the normalized items, the page count
// and the warnings.
type externalBlockWire struct {
	PageCount any              `json:"page_count"`
	Items     []map[string]any `json:"items"`
	Warnings  []string         `json:"warnings"`
}

// assertExternalBlockWire normalizes res like PDFParser.ParseWithResult does
// and compares it with testdata/<name>.golden.json. Set
// RAGFLOW_UPDATE_GOLDEN=1 to record the file again after an intended change.
func assertExternalBlockWire(t *testing.T, name string, res ParseResult) {
	t.Helper()
	if res.Err != nil {
		t.Fatalf("producer failed: %v", res.Err)
	}
	normalized := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", res)
	if normalized.Err != nil {
		t.Fatalf("NormalizeResult: %v", normalized.Err)
	}
	wire := externalBlockWire{
		PageCount: normalized.File["page_count"],
		Items:     ItemsToMaps(normalized.JSON),
		Warnings:  normalized.Warnings,
	}
	if wire.Warnings == nil {
		wire.Warnings = []string{}
	}
	got, err := json.MarshalIndent(wire, "", "  ")
	if err != nil {
		t.Fatalf("marshal wire form: %v", err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name+".golden.json")
	if os.Getenv("RAGFLOW_UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("record %s: %v", path, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode wire form: %v", err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if !wireValuesEqual(gotValue, wantValue) {
		t.Fatalf("%s wire form changed.\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// wireValuesEqual compares decoded JSON. Numbers may differ in their last
// bits: scaling a coordinate as x*points/pixels or as x/pixels*points rounds
// differently and names the same point.
func wireValuesEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for key, value := range av {
			other, ok := bv[key]
			if !ok || !wireValuesEqual(value, other) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !wireValuesEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case float64:
		bv, ok := b.(float64)
		return ok && math.Abs(av-bv) <= 1e-9*math.Max(1, math.Max(math.Abs(av), math.Abs(bv)))
	default:
		return a == b
	}
}

func decodeExternalBlockFixture(t *testing.T, raw string) any {
	t.Helper()
	var out any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return out
}

// openDataLoaderWireFixture covers every element type, both table sources, and
// each geometry outcome: a usable box, a bleed inside the tolerance, a box in
// a foreign coordinate space, inverted and zero-area boxes, and elements with
// no box, no page, or a page the geometry does not cover.
const openDataLoaderWireFixture = `{
 "type": "document", "number of pages": 3,
 "kids": [
  {"type": "heading", "content": "Quarterly Report", "page number": 1, "bounding box": [72, 700, 400, 730]},
  {"type": "title", "content": "Overview", "page number": 1, "bounding box": [72, 660, 400, 690]},
  {"type": "paragraph", "content": "Body text on page one.", "page number": 1, "bounding box": [72, 600, 540, 650]},
  {"type": "paragraph", "text": "Uses the text key.", "page_number": 1, "bbox": [72, 560, 540, 590]},
  {"type": "table", "html": "<table><tr><th>Item</th><th>Value</th></tr><tr><td>A</td><td>42</td></tr></table>", "page number": 2, "bounding box": [72, 400, 500, 600]},
  {"type": "table", "cells": [{"row": 0, "content": "k"}, {"row": 0, "content": "v"}, {"row": 1, "content": "a"}, {"row": 1, "content": "1"}], "page number": 2, "bounding_box": [72, 200, 500, 380]},
  {"type": "table", "content": "Item | Value", "page number": 2, "bounding box": [72, 100, 500, 180]},
  {"type": "image", "page number": 2, "bounding box": [72, 20, 300, 90]},
  {"type": "picture", "content": "Figure 1: pipeline", "page number": 3, "bounding box": [72, 500, 300, 700]},
  {"type": "formula", "content": "E = mc^2", "page number": 3, "bounding box": [72, 440, 300, 480]},
  {"type": "paragraph", "content": "bleeds within tolerance", "page number": 3, "bounding box": [-20, 380, 620, 420]},
  {"type": "paragraph", "content": "foreign pixel space", "page number": 3, "bounding box": [150, 1351, 550, 1400]},
  {"type": "paragraph", "content": "inverted box", "page number": 3, "bounding box": [200, 300, 100, 100]},
  {"type": "paragraph", "content": "zero width", "page number": 3, "bounding box": [5, 2, 5, 4]},
  {"type": "paragraph", "content": "no box", "page number": 3},
  {"type": "paragraph", "content": "no page", "bounding box": [1, 2, 3, 4]},
  {"type": "paragraph", "content": "uncovered page", "page number": 9, "bounding box": [72, 100, 300, 140]},
  {"type": "paragraph", "content": "   "},
  {"type": "formula"},
  {"type": "list", "content": "- one"},
  {"content": "typeless is skipped"}
 ]}`

func TestExternalBlockWire_OpenDataLoader(t *testing.T) {
	root := decodeExternalBlockFixture(t, openDataLoaderWireFixture)
	sizes := map[int][2]float64{0: {612, 792}, 1: {612, 792}, 2: {612, 792}}
	items, warnings := openDataLoaderItemsWithGeometry(root, sizes)
	res := pdfItemsToResult("sample.pdf", items, 3)
	res.Warnings = append(res.Warnings, warnings...)
	assertExternalBlockWire(t, "pdf_external_block.opendataloader", res)
}

// Without page geometry (a build without pdfium, or a document it cannot
// load) the items keep their text and page identity but carry no positions.
func TestExternalBlockWire_OpenDataLoaderWithoutGeometry(t *testing.T) {
	root := decodeExternalBlockFixture(t, openDataLoaderWireFixture)
	items, warnings := openDataLoaderItemsWithGeometry(root, nil)
	res := pdfItemsToResult("sample.pdf", items, openDataLoaderReportedPageCount(root))
	res.Warnings = append(res.Warnings, warnings...)
	assertExternalBlockWire(t, "pdf_external_block.opendataloader_nogeometry", res)
}

// soMarkWireFixture covers every block type, the header/footer switch, title
// levels, both table encodings, a block that names its own page, a page index
// outside the document, and a page whose pixel size is unknown.
const soMarkWireFixture = `{"outputs": {"json": {"pages": [
 {"page_num": 0, "page_size": {"w": 1654, "h": 2339}, "blocks": [
  {"type": "header", "content": "Running header", "bbox": [100, 20, 1500, 60]},
  {"type": "title", "title_level": 1, "content": "Annual Report", "bbox": [165.4, 233.9, 827, 300]},
  {"type": "title", "title_level": 3, "content": "Scope", "bbox": [165.4, 320, 827, 360]},
  {"type": "title", "title_level": 9, "content": "Level out of range", "bbox": [165.4, 380, 827, 420]},
  {"type": "text", "content": "Body paragraph.", "bbox": [165.4, 440, 1400, 520]},
  {"type": "cate", "content": "table of contents"},
  {"type": "cate_item", "content": "1. Scope"},
  {"type": "blank", "content": ""},
  {"type": "equation", "content": "a^2 + b^2 = c^2", "bbox": [165.4, 540, 827, 600]},
  {"type": "figure", "content": "", "bbox": [165.4, 620, 827, 900]},
  {"type": "stamp", "content": "Approved", "bbox": [1000, 620, 1300, 900]},
  {"type": "table", "content": "<table><tr><th>Item</th><th>Value</th></tr><tr><td>A</td><td>42</td></tr></table>", "bbox": [165.4, 920, 1400, 1200]},
  {"type": "table", "content": "| K | V |\n| --- | --- |\n| a | 1 |", "bbox": [165.4, 1220, 1400, 1400]},
  {"type": "text", "content": "overflows the page", "bbox": [-50, -10, 1700, 2400]},
  {"type": "text", "content": "swapped edges", "bbox": [827, 1500, 165.4, 1450]},
  {"type": "text", "content": "belongs to page two", "page_idx": 1, "bbox": [413.5, 292.25, 620.25, 438.375]},
  {"type": "text", "content": "page index outside the document", "page_idx": 7, "bbox": [165.4, 1600, 827, 1650]},
  {"type": "text", "content": "no bbox"},
  {"type": "text", "content": "   "},
  {"type": "footer", "content": "Page 1", "bbox": [100, 2280, 1500, 2320]}
 ]},
 {"page_num": 1, "page_size": {"w": 827, "h": 1169}, "blocks": [
  {"type": "text", "content": "Second page body.", "bbox": [82.7, 116.9, 413.5, 233.8]}
 ]},
 {"page_num": 2, "blocks": [
  {"type": "text", "content": "Pixel size unknown.", "bbox": [10, 20, 110, 40]}
 ]}
]}}}`

func soMarkWireResult(t *testing.T, keepHeaderFooter bool, pageSizes [][2]float64) ParseResult {
	t.Helper()
	result, _ := decodeExternalBlockFixture(t, soMarkWireFixture).(map[string]any)
	items, pageCount, warnings := soMarkItems(context.Background(), result, keepHeaderFooter, pageSizes)
	res := pdfItemsToResult("sample.pdf", items, pageCount)
	res.Warnings = warnings
	return res
}

func TestExternalBlockWire_SoMark(t *testing.T) {
	sizes := [][2]float64{{595.28, 841.89}, {300, 400}, {612, 792}}
	assertExternalBlockWire(t, "pdf_external_block.somark", soMarkWireResult(t, false, sizes))
}

func TestExternalBlockWire_SoMarkKeepsHeaderFooter(t *testing.T) {
	sizes := [][2]float64{{595.28, 841.89}, {300, 400}, {612, 792}}
	assertExternalBlockWire(t, "pdf_external_block.somark_header_footer", soMarkWireResult(t, true, sizes))
}

// An unreadable source PDF leaves the page sizes unknown: positions are
// omitted and a warning is reported, per #22578.
func TestExternalBlockWire_SoMarkWithoutPageSizes(t *testing.T) {
	assertExternalBlockWire(t, "pdf_external_block.somark_nosizes", soMarkWireResult(t, false, nil))
}

// doclingWireFixture is a standard Docling response: a body tree in reading
// order with a title, section headers, a list, a captioned table with a
// merged header and a footnote, a captioned picture with text recognized
// inside it, an element that continues on the next page, both coordinate
// origins, and page furniture.
const doclingWireFixture = `{"document": {"md_content": "not the source of the items", "json_content": {
 "pages": {"1": {"page_no": 1, "size": {"width": 612, "height": 792}}, "2": {"page_no": 2, "size": {"width": 612, "height": 792}}},
 "body": {"children": [{"$ref": "#/texts/0"}, {"$ref": "#/texts/1"}, {"$ref": "#/texts/2"}, {"$ref": "#/groups/0"}, {"$ref": "#/tables/0"}, {"$ref": "#/pictures/0"}, {"$ref": "#/texts/9"}, {"$ref": "#/texts/10"}, {"$ref": "#/texts/11"}]},
 "furniture": {"children": [{"$ref": "#/texts/12"}]},
 "groups": [{"label": "list", "children": [{"$ref": "#/texts/3"}, {"$ref": "#/texts/4"}]}],
 "texts": [
  {"label": "title", "text": "Annual Report", "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 740, "r": 400, "b": 710, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "section_header", "level": 1, "text": "Scope", "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 690, "r": 400, "b": 670, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "text", "text": "Body paragraph.", "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 650, "r": 540, "b": 600, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "list_item", "text": "first item", "prov": [{"page_no": 1, "bbox": {"l": 90, "t": 590, "r": 540, "b": 575, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "list_item", "enumerated": true, "marker": "2.", "text": "second item", "prov": [{"page_no": 1, "bbox": {"l": 90, "t": 570, "r": 540, "b": 555, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "caption", "text": "Table 1: totals", "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 545, "r": 540, "b": 530, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "footnote", "text": "a Preliminary figures.", "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 395, "r": 540, "b": 380, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "caption", "text": "Figure 1: pipeline", "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 215, "r": 540, "b": 200, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "text", "text": "600 sec", "prov": [{"page_no": 1, "bbox": {"l": 200, "t": 300, "r": 260, "b": 285, "coord_origin": "BOTTOMLEFT"}}]},
  {"label": "text", "text": "Continues on the next page.", "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 190, "r": 540, "b": 72, "coord_origin": "BOTTOMLEFT"}}, {"page_no": 2, "bbox": {"l": 72, "t": 72, "r": 540, "b": 120, "coord_origin": "TOPLEFT"}}]},
  {"label": "formula", "text": "E = mc^2", "prov": [{"page_no": 2, "bbox": {"l": 72, "t": 140, "r": 300, "b": 170, "coord_origin": "TOPLEFT"}}]},
  {"label": "code", "text": "x := 1", "prov": [{"page_no": 2, "bbox": {"l": 72, "t": 190, "r": 300, "b": 220, "coord_origin": "TOPLEFT"}}]},
  {"label": "page_footer", "content_layer": "furniture", "text": "Page 1", "prov": [{"page_no": 1, "bbox": {"l": 280, "t": 40, "r": 330, "b": 25, "coord_origin": "BOTTOMLEFT"}}]}
 ],
 "tables": [{"label": "table", "captions": [{"$ref": "#/texts/5"}], "footnotes": [{"$ref": "#/texts/6"}], "children": [{"$ref": "#/texts/5"}, {"$ref": "#/texts/6"}],
  "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 525, "r": 540, "b": 400, "coord_origin": "BOTTOMLEFT"}}],
  "data": {"num_rows": 3, "num_cols": 2, "grid": [
   [{"text": "Region totals", "column_header": true, "col_span": 2, "row_span": 1, "start_row_offset_idx": 0, "start_col_offset_idx": 0},
    {"text": "Region totals", "column_header": true, "col_span": 2, "row_span": 1, "start_row_offset_idx": 0, "start_col_offset_idx": 0}],
   [{"text": "North", "start_row_offset_idx": 1, "start_col_offset_idx": 0}, {"text": "42", "start_row_offset_idx": 1, "start_col_offset_idx": 1}],
   [{"text": "South", "start_row_offset_idx": 2, "start_col_offset_idx": 0}, {"text": "17", "start_row_offset_idx": 2, "start_col_offset_idx": 1}]
  ]}}],
 "pictures": [{"label": "picture", "captions": [{"$ref": "#/texts/7"}], "children": [{"$ref": "#/texts/7"}, {"$ref": "#/texts/8"}],
  "prov": [{"page_no": 1, "bbox": {"l": 72, "t": 370, "r": 540, "b": 220, "coord_origin": "BOTTOMLEFT"}}]}]
}}}`

func TestExternalBlockWire_Docling(t *testing.T) {
	res, ok := parseDoclingStandardResult(t.Context(), "sample.pdf", []byte(doclingWireFixture), 0)
	if !ok {
		t.Fatal("standard response was not parsed")
	}
	assertExternalBlockWire(t, "pdf_external_block.docling", res)
}
