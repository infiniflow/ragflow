package parser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func requireTableRows(t *testing.T, item map[string]any, want [][]string) {
	t.Helper()
	td, ok := item["table"].(*entity.TableData)
	if item["doc_type_kwd"] != "table" || !ok || td == nil {
		t.Fatalf("expected structured table, got %#v", item)
	}
	if got := td.CellTexts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("table rows = %#v, want %#v", got, want)
	}
}

func TestMarkdownParserPreservesEmptyTableCells(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	for _, md := range []string{
		"| | |\n|---|---|\n| Document ID | demo |\n| Owner | RET |",
		"| | |\r\n|---|---|\r\n| Document ID | demo |\r\n| Owner | RET |\r\n",
	} {
		res := p.ParseWithResult(t.Context(), "metadata.md", []byte(md))
		if res.Err != nil || len(res.JSON) != 1 {
			t.Fatalf("result = %+v", res)
		}
		requireTableRows(t, res.JSON[0].ToMap(), [][]string{{"", ""}, {"Document ID", "demo"}, {"Owner", "RET"}})
	}
	res := p.ParseWithResult(t.Context(), "blank-row.md", []byte("| A | B |\n|---|---|\n| | |\n| x | y |"))
	if len(res.JSON) != 1 {
		t.Fatalf("items = %d, want 1", len(res.JSON))
	}
	requireTableRows(t, res.JSON[0].ToMap(), [][]string{{"A", "B"}, {"", ""}, {"x", "y"}})
}

func TestMarkdownParserPreservesHTMLTableAtEOF(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	res := p.ParseWithResult(t.Context(), "table.md", []byte("<table><tr><td>Owner</td><td>RET</td></tr></table>"))
	if res.Err != nil || len(res.JSON) != 1 {
		t.Fatalf("result = %+v", res)
	}
	requireTableRows(t, res.JSON[0].ToMap(), [][]string{{"Owner", "RET"}})
}

func TestMarkdownParserPreservesInlineHTMLTable(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	res := p.ParseWithResult(t.Context(), "table.md", []byte("Before. <table><tr><td>Owner</td><td>RET</td></tr></table> After."))
	if res.Err != nil || len(res.JSON) != 3 {
		t.Fatalf("result = %+v", res)
	}
	if res.JSON[0].ToMap()["text"] != "Before." || res.JSON[2].ToMap()["text"] != "After." {
		t.Fatalf("items=%#v", ItemsToMaps(res.JSON))
	}
	requireTableRows(t, res.JSON[1].ToMap(), [][]string{{"Owner", "RET"}})
}

func TestMarkdownParserPreservesWideTableWithInlineHeaderAndReference(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	res := p.ParseWithResult(t.Context(), "table.md", []byte("<b>Field</b> | Value\n---|---\nOwner | [team][ref] | TAIL\n\n[ref]: https://example.org\n"))
	if res.Err != nil || len(res.JSON) != 1 {
		t.Fatalf("result=%+v", res)
	}
	requireTableRows(t, res.JSON[0].ToMap(), [][]string{{"Field", "Value", ""}, {"Owner", "team", "TAIL"}})
}

func TestMarkdownParserPreservesEscapedPipesInHTMLAttributes(t *testing.T) {
	for _, destination := range []string{`x\|y`, `x&#124;y`} {
		p, _ := NewMarkdownParser(GoMarkdown)
		res := p.ParseWithResult(t.Context(), "table.md", []byte("| A | B |\n|---|---|\n| <a href=\""+destination+"\">link</a> | TAIL |\n"))
		if res.Err != nil || len(res.JSON) != 1 {
			t.Fatalf("result=%+v", res)
		}
		requireTableRows(t, res.JSON[0].ToMap(), [][]string{{"A", "B"}, {"link", "TAIL"}})
	}
}

func TestMarkdownParserDoesNotTreatTablePrefixAsTable(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	source := "Before <tableau>value</tableau> after."
	res := p.ParseWithResult(t.Context(), "text.md", []byte(source))
	if res.Err != nil || len(res.JSON) != 1 || res.JSON[0].ToMap()["text"] != source {
		t.Fatalf("non-table HTML changed: %+v", res)
	}
}

func TestMarkdownTableImageTimeoutDoesNotConsumeBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	doc := markdownNew().Parse([]byte("| A | B |\n|---|---|\n| Logo | ![logo](http://127.0.0.1/logo.png) |\n"))
	budget := newEmbeddedMediaBudget()
	var items []map[string]any
	unresolved := walkMarkdownBlocksWithImages(ctx, doc, &items, false, true, &htmlWalkState{mediaBudget: budget})
	if budget.items != 0 || unresolved != 0 {
		t.Fatalf("timed out image consumed budget: attempts=%d unresolved=%d", budget.items, unresolved)
	}
}

func TestMarkdownParserPreservesImageBesideInlineTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()
	old := ssrfAllowLoopback
	ssrfAllowLoopback = true
	defer func() { ssrfAllowLoopback = old }()
	p, _ := NewMarkdownParser(GoMarkdown)
	res := p.ParseWithResult(t.Context(), "table.md", []byte("![logo]("+server.URL+"/logo.png) <table><tr><td>Owner</td><td>RET</td></tr></table>"))
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	images, tables := 0, 0
	for _, item := range res.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] == "table" {
			tables++
		}
		if m["doc_type_kwd"] == "image" && m["image"] == "data:image/octet-stream;base64,aGVsbG8=" {
			images++
		}
	}
	if images != 1 || tables != 1 {
		t.Fatalf("images=%d tables=%d items=%#v", images, tables, ItemsToMaps(res.JSON))
	}
}

func TestMarkdownParserPreservesPipesInsideCells(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	for _, cell := range []string{"a\\|b", "`a|b`", "``a|b``"} {
		res := p.ParseWithResult(t.Context(), "table.md", []byte("| Field | Value |\n|---|---|\n| Owner | "+cell+" |"))
		if res.Err != nil || len(res.JSON) != 1 {
			t.Fatalf("result = %+v", res)
		}
		requireTableRows(t, res.JSON[0].ToMap(), [][]string{{"Field", "Value"}, {"Owner", "a|b"}})
	}
}

func TestMarkdownParserKeepsTableImageAssociation(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	for _, source := range []string{
		"| Field | Value |\n|---|---|\n| Logo | ![logo](data:image/png;base64,aGVsbG8=) |",
		"<table><tr><td>Logo</td><td><img alt=\"logo\" src=\"data:image/png;base64,aGVsbG8=\"></td></tr></table>",
	} {
		res := p.ParseWithResult(t.Context(), "table.md", []byte(source))
		if res.Err != nil || len(res.JSON) != 2 {
			t.Fatalf("result = %+v", res)
		}
		table, img := res.JSON[0].ToMap(), res.JSON[1].ToMap()
		if table["doc_type_kwd"] != "table" || img["doc_type_kwd"] != "image" || table["source_table_id"] == nil || img["parent_table_id"] != table["source_table_id"] {
			t.Fatalf("table/image association lost: %#v", ItemsToMaps(res.JSON))
		}
	}
}

func TestPDFMarkdownPreservesEmptyHeaderTable(t *testing.T) {
	res := pdfMarkdownToResult(t.Context(), "table.pdf", "| | |\n|---|---|\n| Owner | RET |", 2)
	if res.Err != nil || len(res.JSON) != 1 {
		t.Fatalf("result = %+v", res)
	}
	requireTableRows(t, res.JSON[0].ToMap(), [][]string{{"", ""}, {"Owner", "RET"}})
}

func TestSoMarkPreservesMarkdownTableAndNotes(t *testing.T) {
	items, pages, _ := soMarkItems(t.Context(), map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{map[string]any{"blocks": []any{
		map[string]any{"type": "table", "content": "Before.\n\n| | |\n|---|---|\n| Owner | RET |\n\nAfter."},
	}}}}}}, false, nil)
	if pages != 1 || len(items) != 3 {
		t.Fatalf("pages=%d items=%#v", pages, items)
	}
	if items[0]["text"] != "Before." || items[2]["text"] != "After." {
		t.Fatalf("notes = %#v", items)
	}
	requireTableRows(t, items[1], [][]string{{"", ""}, {"Owner", "RET"}})
	if items[1]["layout"] != "table" || items[0]["layout"] == "table" || items[2]["layout"] == "table" {
		t.Fatalf("table layout misplaced: %#v", items)
	}
}

func TestSoMarkPreservesPageOnSplitTableItems(t *testing.T) {
	items, _, _ := soMarkItems(t.Context(), map[string]any{"outputs": map[string]any{"json": map[string]any{"pages": []any{
		map[string]any{"blocks": []any{}},
		map[string]any{"blocks": []any{map[string]any{"type": "table", "content": "Before.\n\n| A | B |\n|---|---|\n| Owner | RET |\n\nAfter."}}},
	}}}}, false, nil)
	if len(items) != 3 {
		t.Fatalf("items=%#v", items)
	}
	for _, item := range items {
		if item["page_number"] != 2 {
			t.Fatalf("page lost: %#v", item)
		}
	}
}

func tableStructureIR(t *testing.T, container string) string {
	t.Helper()
	table := `{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"Owner"}]}]},{"content":[{"type":"paragraph","content":[{"type":"text","text":"RET"}]}]}]}]}`
	if container == "text_box" {
		table = `{"type":"text_box","content":[` + table + `]}`
	}
	if container == "list" {
		table = `{"type":"list","items":[{"content":[` + table + `]}]}`
	}
	return `{"sections":[{"elements":[{"type":"paragraph","content":[{"type":"text","text":"Before."}]},` + table + `,{"type":"paragraph","content":[{"type":"text","text":"After."}]}]}]}`
}

func TestOfficePreservesNestedTables(t *testing.T) {
	for _, container := range []string{"table", "text_box", "list"} {
		t.Run(container, func(t *testing.T) {
			ir := tableStructureIR(t, container)
			for _, format := range []string{"docx", "pptx"} {
				t.Run(format, func(t *testing.T) {
					var items []map[string]any
					if format == "docx" {
						items = buildDOCXJSONSections(ir, nil)
					} else {
						var err error
						items, err = buildPPTXJSONSections(ir, nil)
						if err != nil {
							t.Fatal(err)
						}
					}
					if len(items) != 3 {
						t.Fatalf("items = %#v, want before/table/after", items)
					}
					requireTableRows(t, items[1], [][]string{{"Owner", "RET"}})
					if items[0]["text"] != "Before." || items[2]["text"] != "After." {
						t.Fatalf("notes = %#v", items)
					}
					if format == "pptx" {
						for _, item := range items {
							if item["slide_number"] != 1 {
								t.Fatalf("slide lost: %#v", item)
							}
						}
					}
				})
			}
		})
	}
}

func TestPPTXTableOnlySlideIsNotEmpty(t *testing.T) {
	if itemsAllEmpty([]map[string]any{{"doc_type_kwd": "table", "table": &entity.TableData{Grid: [][]entity.TableCell{{{Text: "RET"}}}}}}) {
		t.Fatal("table-only slide was considered empty")
	}
}

func TestOfficeTableRetainsDefaultHeaderCount(t *testing.T) {
	ir := `{"sections":[{"elements":[{"type":"table","rows":[{"is_header":true,"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"H1"}]}]}]},{"is_header":true,"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"H2"}]}]}]}]}]}]}`
	items := buildDOCXJSONSections(ir, nil)
	if len(items) != 1 {
		t.Fatalf("items=%#v", items)
	}
	td := items[0]["table"].(*entity.TableData)
	if td.HeaderRows != 1 || len(td.Grid) != 2 {
		t.Fatalf("table=%+v, want two rows and one default header", td)
	}
}

func TestOfficeTablePreservesLiteralCellText(t *testing.T) {
	el := docxIRElement{Type: "table", Rows: []docxIRRow{{Cells: []docxIRCell{{Content: []docxIRElement{{Type: "paragraph", Content: json.RawMessage(`[{"type":"text","text":" <b>A & B</b>\nsecond line "}]`)}}}}}}}
	items := appendOfficeTableSections(nil, el, " A & <B> ", "table-1", newEmbeddedMediaBudget())
	if len(items) != 1 {
		t.Fatalf("items=%#v", items)
	}
	requireTableRows(t, items[0], [][]string{{"<b>A & B</b>\nsecond line"}})
	td := items[0]["table"].(*entity.TableData)
	if td.HeaderRows != 1 || td.Caption != "A & <B>" {
		t.Fatalf("table metadata=%+v", td)
	}
}

func TestEPUBPreservesTablesInChapterOrder(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{
		{name: "one.xhtml", content: "<p>Before.</p><table><tr><td>Owner</td><td>RET</td></tr></table><p>After.</p>"},
		{name: "two.xhtml", content: "<p>Next chapter.</p>"},
	})
	if res.Err != nil || len(res.JSON) != 4 {
		t.Fatalf("result = %+v", res)
	}
	requireTableRows(t, res.JSON[1].ToMap(), [][]string{{"Owner", "RET"}})
	if res.JSON[0].ToMap()["text"] != "Before." || res.JSON[2].ToMap()["text"] != "After." || res.JSON[3].ToMap()["text"] != "Next chapter." {
		t.Fatalf("items = %#v", ItemsToMaps(res.JSON))
	}
	b, err := json.Marshal(res.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(b), "Owner") != 1 {
		t.Fatalf("table duplicated: %s", b)
	}
}

func TestEPUBNormalizesChapterWhitespace(t *testing.T) {
	res := parseTestEPUB(t, []epubTestChapter{{name: "one.xhtml", content: epubTestHTML("<textarea>  first\n    second</textarea>")}})
	if res.Err != nil || len(res.JSON) != 1 {
		t.Fatalf("result=%+v", res)
	}
	if got := res.JSON[0].ToMap()["text"]; got != "Test first second" {
		t.Fatalf("chapter text=%q", got)
	}
}
