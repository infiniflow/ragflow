package parser

import (
	"context"
	"errors"
	"fmt"
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
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{File: file, Warnings: []string{"source warning"}, JSON: MapsToItems([]map[string]any{input, {"text": "", "page_number": 4, "positions": []float64{4, 0, 0, 0, 0}}})})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	item := result.JSON[0].ToMap()
	positions, ok := ExtractPDFPositions(item)
	if !ok || len(positions) != 2 || positions[0][0] != 2 || item["doc_type_kwd"] != "text" {
		t.Fatalf("positions/type = %#v", item)
	}
	// The NaN row is dropped; the page-only row survives as page identity.
	if positions[1][0] != 4 {
		t.Fatalf("page-only position lost: %#v", positions)
	}
	if len(result.JSON) != 2 || result.JSON[1].ToMap()["page_number"] != 4 {
		t.Fatalf("blank page lost: %#v", result.JSON)
	}
	if !HasPDFCropRegion(item) {
		t.Fatal("real crop region dropped")
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
	result := NewPDFParser().NormalizeMarkdown(t.Context(), "ocr.pdf", "| A | B |\n| --- | --- |\n| one | two |\n\n![remote]("+server.URL+"/image.png)")
	if result.Err != nil || len(result.JSON) == 0 || requests != 0 {
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
	result = NewPDFParser().NormalizeMarkdown(ctx, "ocr.pdf", "body")
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("cancellation = %+v", result)
	}
	if result.File["name"] != "ocr.pdf" {
		t.Fatalf("cancellation lost file metadata: %+v", result)
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
	result := pdfMarkdownToResult(t.Context(), "remote.pdf", "![remote]("+server.URL+"/image.png)", 1)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if requests.Load() != 0 {
		t.Fatal("PDF Markdown backend normalization fetched a remote image")
	}
}

func TestPDFNormalizeResultKeepsTCADPPageWithoutCrop(t *testing.T) {
	items := tcadpAnyToItems(map[string]any{"type": "text", "content": "page text", "page_num": 7})
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{JSON: MapsToItems(items)})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	item := result.JSON[0].ToMap()
	if item["page_number"] != 7 {
		t.Fatalf("page identity lost: %#v", item)
	}
	// The page-only tuple survives as page identity but is not a crop region.
	positions, ok := ExtractPDFPositions(item)
	if !ok || len(positions) != 1 || positions[0][0] != 7 {
		t.Fatalf("page-only position lost: %#v", item)
	}
	if HasPDFCropRegion(item) {
		t.Fatalf("unknown region exposed for cropping: %#v", item)
	}
}

func TestPDFNormalizeResultInfersStructuredTable(t *testing.T) {
	table := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell"}}}}
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{JSON: MapsToItems([]map[string]any{{"table": table, "page_number": 3}})})
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
	result := p.NormalizeResult(t.Context(), "sample.pdf", ParseResult{JSON: []DocItem{
		TableItem{Table: table, Extra: map[string]any{"text": "Footnote"}},
		ImageItem{Text: "Figure", Image: "image-data", Positions: Positions{AbsoluteBox{PageNumber: 2, Left: 1, Right: 5, Top: 2, Bottom: 6}}},
		TextItem{Text: "Title", Extra: map[string]any{"layout": "heading"}},
	}})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	for i, item := range result.JSON[:2] {
		fields := item.ToMap()
		if fields["doc_type_kwd"] != "text" {
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
	if result.JSON[2].ToMap()["layout"] != "heading" {
		t.Fatal("heading layout changed")
	}
}

func TestPDFNormalizeResultDowngradesUnstructuredTableSubtype(t *testing.T) {
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{JSON: MapsToItems([]map[string]any{{
		"doc_type_kwd": "table", "layout": "table", "text": "Recognized text without cells",
	}})})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	item := result.JSON[0].ToMap()
	if item["doc_type_kwd"] != "text" || item["text"] != "Recognized text without cells" {
		t.Fatalf("unstructured table kept an inconsistent subtype: %#v", item)
	}
}

func TestPDFNormalizeResultKeepsPositionedUnstructuredTables(t *testing.T) {
	for _, flatten := range []bool{false, true} {
		p := NewPDFParser()
		p.FlattenMediaToText = flatten
		result := p.NormalizeResult(t.Context(), "sample.pdf", ParseResult{JSON: MapsToItems([]map[string]any{{
			"doc_type_kwd": "table", "text": "Recognized table without cells",
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
		result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{JSON: MapsToItems([]map[string]any{{
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

func TestPDFTableDescriptionUsesTextItem(t *testing.T) {
	table := &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell value"}}}}
	item := map[string]any{"table": table, "doc_type_kwd": "table", "text": "Footnote", "page_number": 3}
	result := pdfItemsToResult("sample.pdf", []map[string]any{item}, 3)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	result = NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", result)
	if len(result.JSON) != 2 || result.JSON[0].ToMap()["text"] != nil || result.JSON[1].ToMap()["text"] != "Footnote" || result.JSON[1].ToMap()["page_number"] != 3 {
		t.Fatalf("table description not separated: %#v", result.JSON)
	}
	if item["text"] != "Footnote" {
		t.Fatal("source item mutated")
	}
}

func TestPDFWhitespaceTableDescriptionCannotHideCells(t *testing.T) {
	item := map[string]any{"table": &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell value"}}}}, "doc_type_kwd": "table", "text": " \n "}
	results := []ParseResult{
		NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", pdfItemsToResult("sample.pdf", []map[string]any{item}, 1)),
		NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{JSON: MapsToItems([]map[string]any{item})}),
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

func TestPDFNormalizeResultPreservesAdjacentTables(t *testing.T) {
	const markup = "<table><tr><td>First cell</td></tr></table>\n<table><tr><td>Second cell</td></tr></table>\nFootnote"
	for _, projected := range []bool{false, true} {
		for _, flatten := range []bool{false, true} {
			name := fmt.Sprintf("projected=%v/flatten=%v", projected, flatten)
			t.Run(name, func(t *testing.T) {
				item := map[string]any{
					"doc_type_kwd": "table", "text": markup, "page_number": 3,
					"positions": [][]any{{3, 10, 30, 20, 40}}, "_preserve_order": true,
				}
				if projected {
					projectPDFTable(item)
				}
				originalText := item["text"]
				p := NewPDFParser()
				p.FlattenMediaToText = flatten
				result := p.NormalizeResult(t.Context(), "sample.pdf", pdfItemsToResult("sample.pdf", []map[string]any{item}, 3))
				if result.Err != nil {
					t.Fatal(result.Err)
				}
				if flatten {
					var text string
					for _, doc := range result.JSON {
						fields := doc.ToMap()
						if fields["doc_type_kwd"] != "text" || fields["table"] != nil {
							t.Fatalf("media not flattened: %#v", fields)
						}
						value, _ := fields["text"].(string)
						text += value
					}
					for _, want := range []string{"First cell", "Second cell", "Footnote"} {
						if strings.Count(text, want) != 1 {
							t.Fatalf("want %q exactly once in %q", want, text)
						}
					}
				} else {
					if len(result.JSON) != 3 {
						t.Fatalf("want two tables and a footnote, got %#v", result.JSON)
					}
					for i, want := range []string{"First cell", "Second cell"} {
						fields := result.JSON[i].ToMap()
						table, _ := fields["table"].(*entity.TableData)
						if table == nil || table.Page != 3 || len(table.Grid) != 1 || len(table.Grid[0]) != 1 || table.Grid[0][0].Text != want || fields["text"] != nil {
							t.Fatalf("table %d lost cells or metadata: %#v", i, fields)
						}
						if _, ok := ExtractPDFPositions(fields); !ok {
							t.Fatalf("table %d lost crop positions", i)
						}
					}
					note := result.JSON[2].ToMap()
					if note["text"] != "Footnote" || note["doc_type_kwd"] != "text" || note["page_number"] != 3 || note["_preserve_order"] != true {
						t.Fatalf("footnote lost text or metadata: %#v", note)
					}
				}
				if item["text"] != originalText {
					t.Fatal("source item mutated")
				}
			})
		}
	}
}

func TestPDFNormalizeResultIsIdempotent(t *testing.T) {
	const markup = "<table><tr><td>First cell</td></tr></table>\n<table><tr><td>Second cell</td></tr></table>\nFootnote"
	for _, flatten := range []bool{false, true} {
		p := NewPDFParser()
		p.FlattenMediaToText = flatten
		item := map[string]any{
			"doc_type_kwd": "table", "text": markup, "page_number": 3,
			"positions": [][]any{{3, 10, 30, 20, 40}}, "_preserve_order": true,
		}
		once := p.NormalizeResult(t.Context(), "sample.pdf", pdfItemsToResult("sample.pdf", []map[string]any{item}, 3))
		if once.Err != nil {
			t.Fatal(once.Err)
		}
		twice := p.NormalizeResult(t.Context(), "sample.pdf", once)
		if twice.Err != nil {
			t.Fatal(twice.Err)
		}
		if len(once.JSON) != len(twice.JSON) {
			t.Fatalf("flatten=%v: second normalize changed item count %d -> %d", flatten, len(once.JSON), len(twice.JSON))
		}
		for i := range once.JSON {
			a, b := once.JSON[i].ToMap(), twice.JSON[i].ToMap()
			if a["doc_type_kwd"] != b["doc_type_kwd"] {
				t.Fatalf("flatten=%v: item %d type changed %v -> %v", flatten, i, a["doc_type_kwd"], b["doc_type_kwd"])
			}
			ta, _ := a["text"].(string)
			tb, _ := b["text"].(string)
			if ta != tb {
				t.Fatalf("flatten=%v: item %d text changed %q -> %q", flatten, i, ta, tb)
			}
		}
	}
}

// TestPDFNormalizeResultKeepsTCADPPageOnlyPosition covers the TCADP wire
// form: a flat []float64 page-only tuple. Normalization must keep the page
// identity (the indexer derives page_num_int from positions) while the tuple
// stays out of the crop path.
func TestPDFNormalizeResultKeepsTCADPPageOnlyPosition(t *testing.T) {
	items := tcadpAnyToItems(map[string]any{"type": "text", "content": "page text", "page_num": 7})
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	result := NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", ParseResult{JSON: MapsToItems(items)})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	item := result.JSON[0].ToMap()
	if item["page_number"] != 7 {
		t.Fatalf("page identity lost: %#v", item)
	}
	positions, ok := ExtractPDFPositions(item)
	if !ok || len(positions) != 1 {
		t.Fatalf("page-only position dropped: %#v", item)
	}
	if positions[0][0] != 7 {
		t.Fatalf("page-only position lost its page: %#v", positions)
	}
	if HasPDFCropRegion(item) {
		t.Fatalf("page-only position exposed as a crop region: %#v", item)
	}
}

func TestValidPDFPositionsAcceptsFlatTupleAndDropsBadBoxes(t *testing.T) {
	flat := validPDFPositions([]float64{7, 0, 0, 0, 0})
	if len(flat) != 1 || flat[0][0] != 7 {
		t.Fatalf("flat tuple dropped: %#v", flat)
	}
	if HasPDFCropRegion(map[string]any{"positions": flat}) {
		t.Fatal("page-only tuple exposed as a crop region")
	}
	degenerate := validPDFPositions([][]any{{1, 10, 10, 5, 5}})
	if len(degenerate) != 0 {
		t.Fatalf("zero-area box kept: %#v", degenerate)
	}
	region := validPDFPositions([][]any{{1, 10, 30, 5, 25}})
	if len(region) != 1 {
		t.Fatalf("valid region dropped: %#v", region)
	}
	if !HasPDFCropRegion(map[string]any{"positions": region}) {
		t.Fatal("valid region not exposed as a crop region")
	}
}

// TestHasPDFCropRegionReadsEveryRowAndShape covers the crop-region contract:
// every row of the matrix counts, and the legacy flat tuple is understood.
func TestHasPDFCropRegionReadsEveryRowAndShape(t *testing.T) {
	cases := []struct {
		name string
		item map[string]any
		want bool
	}{
		{"page-only row before a real region", map[string]any{"positions": [][]any{{1, 0, 0, 0, 0}, {2, 10, 100, 10, 100}}}, true},
		{"real region before a page-only row", map[string]any{"positions": [][]any{{1, 10, 100, 10, 100}, {2, 0, 0, 0, 0}}}, true},
		{"only page-only rows", map[string]any{"positions": [][]any{{1, 0, 0, 0, 0}}}, false},
		{"flat tuple with a region", map[string]any{"positions": []float64{1, 10, 100, 10, 100}}, true},
		{"flat page-only tuple", map[string]any{"positions": []float64{1, 0, 0, 0, 0}}, false},
		{"no positions", map[string]any{"positions": []any{}}, false},
	}
	for _, tc := range cases {
		if got := HasPDFCropRegion(tc.item); got != tc.want {
			t.Errorf("%s: HasPDFCropRegion = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCountPDFPositionRows(t *testing.T) {
	if got := countPDFPositionRows(nil); got != 0 {
		t.Fatalf("nil = %d, want 0", got)
	}
	if got := countPDFPositionRows([][]any{{1, 0, 1, 0, 1}, {2, 0, 1, 0, 1}}); got != 2 {
		t.Fatalf("[][]any = %d, want 2", got)
	}
	if got := countPDFPositionRows([]any{[]any{1, 0, 1, 0, 1}}); got != 1 {
		t.Fatalf("[]any = %d, want 1", got)
	}
	if got := countPDFPositionRows("bad"); got != 0 {
		t.Fatalf("bad type = %d, want 0", got)
	}
}

func TestPDFNormalizeResultCancellationPreservesFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := NewPDFParser().NormalizeResult(ctx, "cancel.pdf", ParseResult{

		File: map[string]any{"parse_method": "mineru"},
		JSON: MapsToItems([]map[string]any{{"text": "hi", "doc_type_kwd": "text"}}),
	})
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("err = %v", result.Err)
	}
	if result.File["name"] != "cancel.pdf" || result.File["parse_method"] != "mineru" {
		t.Fatalf("file lost: %+v", result.File)
	}
}

func TestPDFMarkdownToResultPreservesFileOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := pdfMarkdownToResult(ctx, "cancel.pdf", "body", 3)
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("err = %v", result.Err)
	}
	if result.File["name"] != "cancel.pdf" || result.File["page_count"] != 3 {
		t.Fatalf("file lost: %+v", result.File)
	}
}
