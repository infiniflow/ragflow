package parser

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/entity"
)

func TestPDFNormalizeResultPositionsAndMetadata(t *testing.T) {
	file := map[string]any{"outline": []string{"Chapter"}, "page_count": 20, "parse_method": "selected-model"}
	input := map[string]any{"text": "caption", "layout": "caption", "doc_type_kwd": "text", "page_number": 2,
		"positions": []any{[]any{2.0, 10.0, 30.0, 20.0, 40.0}, []any{3.0, math.NaN(), 30.0, 20.0, 40.0}, []any{4, 0, 0, 0, 0}}}
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{OutputFormat: "json", File: file, Warnings: []string{"source warning"}, JSON: MapsToItems([]map[string]any{input, {"text": "", "page_number": 4, "positions": []float64{4, 0, 0, 0, 0}}})})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	item := result.JSON[0].ToMap()
	positions, ok := ExtractPDFPositions(item)
	if !ok || len(positions) != 1 || positions[0][0] != 2 || item["doc_type_kwd"] != "text" {
		t.Fatalf("positions/type = %#v", item)
	}
	if len(result.JSON) != 2 || result.JSON[1].ToMap()["page_number"] != 4 {
		t.Fatalf("blank page lost: %#v", result.JSON)
	}
	if _, ok := ExtractPDFPositions(result.JSON[1].ToMap()); ok {
		t.Fatal("zero rectangle exposed for cropping")
	}
	if result.File["parse_method"] != "selected-model" || result.File["page_count"] != 20 || len(result.Warnings) != 1 {
		t.Fatalf("metadata lost: %+v", result)
	}
	if _, ok := input["_pdf_positions"]; ok {
		t.Fatal("input mutated")
	}
}

func TestPDFNormalizeResultMarkdownAndCancellation(t *testing.T) {
	previous := ssrfAllowLoopback
	ssrfAllowLoopback = true
	t.Cleanup(func() { ssrfAllowLoopback = previous })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	result := NewPDFParser().NormalizeResult(t.Context(), "ocr.pdf", ParseResult{OutputFormat: "markdown", Markdown: "| A | B |\n| --- | --- |\n| one | two |\n\n![remote](" + server.URL + "/image.png)"})
	if result.Err != nil || result.OutputFormat != "json" || result.Markdown != "" || requests != 0 {
		t.Fatalf("result=%+v requests=%d", result, requests)
	}
	var table *entity.TableData
	for _, item := range result.JSON {
		if td, ok := item.ToMap()["table"].(*entity.TableData); ok {
			table = td
		}
	}
	if table == nil || len(table.CellTexts()) < 2 || len(table.CellTexts()[1]) == 0 || table.CellTexts()[1][0] != "one" {
		t.Fatalf("markdown table lost: %#v", result.JSON)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result = NewPDFParser().NormalizeResult(ctx, "ocr.pdf", ParseResult{OutputFormat: "markdown", Markdown: "body"})
	if !errors.Is(result.Err, context.Canceled) || result.OutputFormat != "" {
		t.Fatalf("cancellation = %+v", result)
	}
}

func TestPDFMarkdownBackendDoesNotFetchRemoteImages(t *testing.T) {
	withSSRFBypass(t)
	previous := ssrfAllowLoopback
	ssrfAllowLoopback = true
	t.Cleanup(func() { ssrfAllowLoopback = previous })
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusNotFound) }))
	defer server.Close()
	result := pdfMarkdownToResult(t.Context(), "remote.pdf", "![remote]("+server.URL+"/image.png)", "json", 1)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if requests.Load() != 0 {
		t.Fatal("PDF Markdown backend normalization fetched a remote image")
	}
}

func TestPDFNormalizeResultKeepsTCADPPageWithoutCrop(t *testing.T) {
	items := tcadpAnyToItems(map[string]any{"type": "text", "content": "page text", "page_num": 7})
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{OutputFormat: "json", JSON: MapsToItems(items)})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	item := result.JSON[0].ToMap()
	if item["page_number"] != 7 {
		t.Fatalf("page identity lost: %#v", item)
	}
	if _, ok := ExtractPDFPositions(item); ok {
		t.Fatalf("unknown region exposed for cropping: %#v", item)
	}
}

func TestPDFNormalizeResultInfersStructuredTable(t *testing.T) {
	table := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell"}}}}
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{OutputFormat: "json", JSON: MapsToItems([]map[string]any{{"table": table, "page_number": 3}})})
	item := result.JSON[0].ToMap()
	if item["doc_type_kwd"] != "table" {
		t.Fatalf("table classified incorrectly: %#v", item)
	}
	if table.Page != 0 {
		t.Fatal("input table mutated")
	}
}

func TestPDFNormalizeResultFlattenPreservesContentAndClearsMediaSubtype(t *testing.T) {
	p := NewPDFParser()
	p.FlattenMediaToText = true
	table := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell"}}}, Caption: "Caption"}
	result := p.NormalizeResult(t.Context(), "sample.pdf", ParseResult{OutputFormat: "json", JSON: []DocItem{
		TableItem{Table: table, CKType: "table", Extra: map[string]any{"text": "Footnote"}},
		ImageItem{Text: "Figure", Image: "image-data", CKType: "image", Positions: Positions{AbsoluteBox{PageNumber: 2, Left: 1, Right: 5, Top: 2, Bottom: 6}}},
		TextItem{Text: "Title", CKType: "heading"},
	}})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	for i, item := range result.JSON[:2] {
		fields := item.ToMap()
		if fields["doc_type_kwd"] != "text" || fields["ck_type"] == "image" || fields["ck_type"] == "table" {
			t.Fatalf("flattened item %d still has media type: %#v", i, fields)
		}
	}
	text, _ := result.JSON[0].ToMap()["text"].(string)
	for _, want := range []string{"Cell", "Caption", "Footnote"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if result.JSON[1].ToMap()["image"] != "image-data" {
		t.Fatal("flatten lost image")
	}
	if _, ok := ExtractPDFPositions(result.JSON[1].ToMap()); !ok {
		t.Fatal("flatten lost positions")
	}
	if result.JSON[2].ToMap()["ck_type"] != "heading" {
		t.Fatal("heading subtype changed")
	}
}

func TestPDFNormalizeResultDowngradesUnstructuredTableSubtype(t *testing.T) {
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{OutputFormat: "json", JSON: MapsToItems([]map[string]any{{
		"doc_type_kwd": "table", "ck_type": "table", "layout": "table", "text": "Recognized text without cells",
	}})})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	item := result.JSON[0].ToMap()
	if item["doc_type_kwd"] != "text" || item["ck_type"] == "table" || item["text"] != "Recognized text without cells" {
		t.Fatalf("unstructured table kept an inconsistent subtype: %#v", item)
	}
}

func TestPDFNormalizeResultKeepsPositionedUnstructuredTables(t *testing.T) {
	for _, flatten := range []bool{false, true} {
		p := NewPDFParser()
		p.FlattenMediaToText = flatten
		result := p.NormalizeResult(t.Context(), "sample.pdf", ParseResult{OutputFormat: "json", JSON: MapsToItems([]map[string]any{{
			"doc_type_kwd": "table", "ck_type": "table", "text": "Recognized table without cells",
			"positions": [][]any{{2, 10, 30, 20, 40}},
		}})})
		if result.Err != nil || len(result.JSON) != 1 {
			t.Fatalf("unexpected result: %+v", result)
		}
		item := result.JSON[0].ToMap()
		want := "table"
		if flatten {
			want = "text"
		}
		if item["doc_type_kwd"] != want || item["text"] != "Recognized table without cells" {
			t.Fatalf("flatten=%v: table media identity or text lost: %#v", flatten, item)
		}
		if _, ok := ExtractPDFPositions(item); !ok {
			t.Fatalf("flatten=%v: table crop region lost: %#v", flatten, item)
		}
	}
}

func TestPDFNormalizeResultRecoversUnclosedTableCells(t *testing.T) {
	for _, markup := range []string{
		"<table><tr><td>Cell value</td></tr>",
		"<table><tr><td>Cell value",
	} {
		result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{OutputFormat: "json", JSON: MapsToItems([]map[string]any{{
			"doc_type_kwd": "table", "text": markup,
		}})})
		if result.Err != nil || len(result.JSON) != 1 {
			t.Fatalf("unexpected result: %+v", result)
		}
		item := result.JSON[0].ToMap()
		table, _ := item["table"].(*entity.TableData)
		if table == nil || len(table.CellTexts()) != 1 || len(table.CellTexts()[0]) != 1 || table.CellTexts()[0][0] != "Cell value" {
			t.Fatalf("unclosed markup lost recoverable cells: %#v", item)
		}
		if text, _ := item["text"].(string); text != "" {
			t.Fatalf("recovered table still carries raw HTML: %q", text)
		}
	}
}

func TestPDFParserSoMarkFlattensStructuredMedia(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/parse/async" {
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"status":"SUCCESS","result":{"outputs":{"json":{"pages":[{"blocks":[{"type":"figure","content":"Figure caption"},{"type":"table","content":"<table><tr><td>Cell value</td></tr></table>"}]}]}}}}}`))
	}))
	defer server.Close()
	p := NewPDFParser()
	p.ConfigureFromSetup(map[string]any{"parse_method": "somark", "somark_base_url": server.URL, "flatten_media_to_text": true})
	result := p.ParseWithResult(t.Context(), "sample.pdf", []byte("pdf"))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.JSON) != 2 {
		t.Fatalf("items = %#v", result.JSON)
	}
	for i, want := range []string{"Figure caption", "Cell value"} {
		item := result.JSON[i].ToMap()
		text, _ := item["text"].(string)
		if item["doc_type_kwd"] != "text" || !strings.Contains(text, want) {
			t.Fatalf("item %d = %#v; want text containing %q", i, item, want)
		}
	}
}

func TestProjectPDFTablePreservesExistingStructureAndNotes(t *testing.T) {
	table := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Original cell"}}}}
	item := map[string]any{"doc_type_kwd": "table", "table": table, "text": "<table><tr><td>Other</td></tr></table>\nFootnote"}
	projectPDFTable(item)
	text, _ := item["text"].(string)
	if item["table"] != table || !strings.Contains(text, "Footnote") {
		t.Fatalf("existing table or notes overwritten: %#v", item)
	}
	mixed := map[string]any{"doc_type_kwd": "table", "text": "<table><tr><td>Cell</td></tr></table>\nFootnote"}
	projectPDFTable(mixed)
	text, _ = mixed["text"].(string)
	if !strings.Contains(text, "Footnote") {
		t.Fatalf("notes lost: %#v", mixed)
	}
}

func TestPDFTableDescriptionUsesTextItemForBothOutputs(t *testing.T) {
	table := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell value"}}}}
	item := map[string]any{"table": table, "doc_type_kwd": "table", "text": "Footnote", "page_number": 3}
	for _, format := range []string{"json", "markdown"} {
		result := pdfItemsToResult("sample.pdf", []map[string]any{item}, format, 3)
		if result.Err != nil {
			t.Fatal(result.Err)
		}
		if format == "json" {
			if len(result.JSON) != 2 || result.JSON[0].ToMap()["text"] != nil || result.JSON[1].ToMap()["text"] != "Footnote" || result.JSON[1].ToMap()["page_number"] != 3 {
				t.Fatalf("table description not separated: %#v", result.JSON)
			}
		} else if !strings.Contains(result.Markdown, "Cell value") || !strings.Contains(result.Markdown, "Footnote") {
			t.Fatalf("table description lost: %s", result.Markdown)
		}
	}
	if item["text"] != "Footnote" {
		t.Fatal("source item mutated")
	}
}

func TestPDFWhitespaceTableDescriptionCannotHideCells(t *testing.T) {
	item := map[string]any{"table": &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell value"}}}}, "doc_type_kwd": "table", "text": " \n "}
	results := []ParseResult{
		pdfItemsToResult("sample.pdf", []map[string]any{item}, "json", 1),
		NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{OutputFormat: "json", JSON: MapsToItems([]map[string]any{item})}),
	}
	for _, result := range results {
		if result.Err != nil || len(result.JSON) != 1 {
			t.Fatalf("unexpected result: %+v", result)
		}
		if text, _ := result.JSON[0].ToMap()["text"].(string); text != "" {
			t.Fatalf("whitespace still suppresses table materialization: %q", text)
		}
	}
}
