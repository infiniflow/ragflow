//go:build cgo

package parser

import (
	"archive/zip"
	"bytes"
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
	if !res.Produced() {
		t.Fatalf("produced = %t, want true", res.Produced())
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
	if td.HeaderRows != 1 {
		t.Fatalf("table.doc HeaderRows = %d, want 1", td.HeaderRows)
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
	if !res.Produced() {
		t.Fatalf("produced = %t, want true", res.Produced())
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
// path. The parser only emits JSON regardless of any produced request.
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
	if !res.Produced() {
		t.Fatalf("DOC must ignore text produced and emit json, got produced=%t", res.Produced())
	}
	if len(res.JSON) == 0 {
		t.Fatalf("DOC json output must contain items, got empty")
	}
}

func buildMinimalDOCXWithDocumentXML(t *testing.T, documentXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
			`</Relationships>`,
		"word/document.xml": documentXML,
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDOCXParser_EndToEndTableHeaderSemantics(t *testing.T) {
	// 1. Real checked-in DOCX fixture (w:tblLook w:firstRow="1").
	fixtureData, err := os.ReadFile(filepath.Join("testdata", "docx_table_image.docx"))
	if err != nil {
		t.Fatalf("read docx_table_image.docx: %v", err)
	}
	res := NewDOCXParser().ParseWithResult(context.Background(), "docx_table_image.docx", fixtureData)
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	foundTable := false
	for _, item := range res.JSON {
		if td, ok := item.ToMap()["table"].(*entity.TableData); ok && td != nil {
			foundTable = true
			if td.HeaderRows != 1 {
				t.Fatalf("docx_table_image.docx HeaderRows = %d, want 1", td.HeaderRows)
			}
		}
	}
	if !foundTable {
		t.Fatal("docx_table_image.docx emitted no structured table")
	}

	// 2. Synthetic real DOCX archives exercising tblLook firstRow=1, firstRow=0, and multi-row tblHeader.
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:tbl><w:tblPr><w:tblLook w:firstRow="1" w:val="04A0"/></w:tblPr>` +
		`<w:tr><w:tc><w:p><w:r><w:t>H1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>H2</w:t></w:r></w:p></w:tc></w:tr>` +
		`<w:tr><w:tc><w:p><w:r><w:t>V1</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>V2</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`<w:tbl><w:tblPr><w:tblLook w:firstRow="0" w:val="0000"/></w:tblPr>` +
		`<w:tr><w:tc><w:p><w:r><w:t>Key</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Val</w:t></w:r></w:p></w:tc></w:tr>` +
		`<w:tr><w:tc><w:p><w:r><w:t>Owner</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>RET</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`<w:tbl><w:tblPr><w:tblLook w:firstRow="1" w:val="04A0"/></w:tblPr>` +
		`<w:tr><w:trPr><w:tblHeader/></w:trPr><w:tc><w:p><w:r><w:t>Group</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Metric</w:t></w:r></w:p></w:tc></w:tr>` +
		`<w:tr><w:trPr><w:tblHeader/></w:trPr><w:tc><w:p><w:r><w:t>Subgroup</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Unit</w:t></w:r></w:p></w:tc></w:tr>` +
		`<w:tr><w:tc><w:p><w:r><w:t>A</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>ms</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`
	parsed := NewDOCXParser().ParseWithResult(context.Background(), "tables.docx", buildMinimalDOCXWithDocumentXML(t, docXML))
	if parsed.Err != nil {
		t.Fatalf("ParseWithResult(tables.docx) error: %v", parsed.Err)
	}
	var gotHeaders []int
	for _, item := range parsed.JSON {
		if td, ok := item.ToMap()["table"].(*entity.TableData); ok && td != nil {
			gotHeaders = append(gotHeaders, td.HeaderRows)
		}
	}
	wantHeaders := []int{1, 0, 2}
	if len(gotHeaders) != len(wantHeaders) {
		t.Fatalf("got %d tables (%v), want %v", len(gotHeaders), gotHeaders, wantHeaders)
	}
	for i, want := range wantHeaders {
		if gotHeaders[i] != want {
			t.Fatalf("table[%d].HeaderRows = %d, want %d", i, gotHeaders[i], want)
		}
	}
}
