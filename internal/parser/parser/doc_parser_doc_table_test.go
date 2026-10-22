//go:build cgo

package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// TestDOCParser_TableEmittedAsStructuredItem verifies that the legacy .doc
// path emits a structured doc_type_kwd:"table" item carrying a typed
// *entity.TableData, instead of flattening the whole document (tables
// included) into a single plain-text item with " | " separators.
//
// Before this change the DOC parser flattened every document through
// flattenDocIR, so tables appeared as pipe-delimited plain text in chunks and
// the chunker's HTML-table rendering (#20918) had no structured table to work
// with. office_oxide/go v0.1.9 (#116) recovers the table stream as an IR
// "type":"table" element; buildDOCXJSONSections projects that into the typed
// table contract the chunker reads. The fixture (testdata/table.doc) is a real
// Word binary containing a multi-column table.
func TestDOCParser_TableEmittedAsStructuredItem(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "table.doc"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	res := NewDOCParser().ParseWithResult(context.Background(), "table.doc", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) == 0 {
		t.Fatalf("JSON result is empty: %#v", res)
	}

	var tableItem map[string]any
	for _, it := range res.JSON {
		m := it.ToMap()
		if dt, _ := m[DocTypeKey].(string); dt == DocTypeTable {
			tableItem = m
			break
		}
	}
	if tableItem == nil {
		t.Fatalf("no doc_type_kwd:table item in JSON result; items=%#v", res.JSON)
	}

	// The table must be emitted as a structured TableData — the contract the
	// chunker's HTML-table rendering (#20918) reads — not as pipe-delimited
	// plain text. projectPDFTable moves the HTML into the typed `table` field
	// and deletes the redundant `text`, which is the intended shape.
	td, ok := tableItem["table"].(*entity.TableData)
	if !ok || td == nil {
		t.Fatalf("table item has no structured TableData; item=%#v", tableItem)
	}
	cells := td.CellTexts()
	if len(cells) == 0 {
		t.Fatalf("table TableData has no rows; item=%#v", tableItem)
	}
	var joined strings.Builder
	for _, row := range cells {
		for _, c := range row {
			joined.WriteString(c)
			joined.WriteString(" ")
		}
	}
	if strings.TrimSpace(joined.String()) == "" {
		t.Errorf("table TableData has no cell text; cells=%#v", cells)
	}

	// The legacy pipe-delimited text form must not survive anywhere in the
	// structured JSON: every item's `text` must be free of " | ".
	for _, it := range res.JSON {
		m := it.ToMap()
		if txt, _ := m["text"].(string); strings.Contains(txt, " | ") {
			t.Errorf("item still carries pipe-separated text: %q", txt)
		}
	}
}

// TestDOCParser_NoTableDocStaysTextItem pins the fallback for a .doc whose IR
// yields no structured sections: the parser must still emit the flattened text
// as a single text item (never silently drop content to 0 chunks). The fixture
// is the same table.doc; with the legacy text output path removed the parser
// only emits JSON, so the fallback is exercised through the JSON item below.
func TestDOCParser_NoTableDocStaysTextItem(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "table.doc"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewDOCParser()
	res := p.ParseWithResult(context.Background(), "table.doc", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) == 0 {
		t.Fatalf("doc with no structured sections must still emit a text item, got empty JSON")
	}
	gotText := false
	for _, it := range res.JSON {
		if txt, ok := it.ToMap()["text"].(string); ok && strings.TrimSpace(txt) != "" {
			gotText = true
		}
	}
	if !gotText {
		t.Fatalf("doc fallback produced no non-empty text item: %#v", res.JSON)
	}
}

// TestDOCParserTextOutputRemoved verifies the DOC parser has no "text" output
// path. The parser only emits JSON regardless of any output_format request.
func TestDOCParserTextOutputRemoved(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "table.doc"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	p := NewDOCParser()
	res := p.ParseWithResult(context.Background(), "table.doc", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Fatalf("DOC must ignore text output_format and emit json, got OutputFormat=%q", res.OutputFormat)
	}
	if len(res.JSON) == 0 {
		t.Fatalf("DOC json output must contain items, got empty")
	}
}
