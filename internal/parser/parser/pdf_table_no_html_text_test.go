package parser

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// TestPDFTableItemsOmitHTMLText pins the structured TableData contract: a table
// item must carry `table` (parsed rows) and must NOT carry the legacy HTML
// `text`. The HTML string is a redundant, storage-costly duplicate that the
// markdown/search paths derive on demand from `table`. Reducing this catches
// any producer that regresses to emitting both.
func TestPDFTableItemsOmitHTMLText(t *testing.T) {
	const tableHTML = "<table><tr><td>What is RAGFlow?</td><td>A RAG engine.</td></tr><tr><td>Is it open source?</td><td>Yes.</td></tr></table>"
	const docxIR = `{"sections":[{"elements":[
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"cell"}]}]}]}]}
	]}]}`

	cases := []struct {
		name    string
		produce func() []map[string]any
	}{
		{"projectPDFTable (DLA)", func() []map[string]any {
			item := map[string]any{"doc_type_kwd": "table", "text": tableHTML, "page_number": 1}
			projectPDFTable(item)
			return []map[string]any{item}
		}},
		{"opendataloader html", func() []map[string]any {
			return openDataLoaderItems(map[string]any{"type": "table", "html": tableHTML})
		}},
		{"tcadp html content", func() []map[string]any {
			return tcadpAnyToItems(map[string]any{"type": "table", "content": tableHTML})
		}},
		{"somark html", func() []map[string]any {
			return []map[string]any{soMarkBlockToItem(map[string]any{"type": "table", "content": tableHTML}, false)}
		}},
		{"docx table", func() []map[string]any {
			return buildDOCXJSONSections(docxIR, newEmbeddedMediaBudget())
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := tc.produce()
			var tableItem map[string]any
			for _, it := range items {
				if dt, _ := it["doc_type_kwd"].(string); dt == "table" {
					tableItem = it
					break
				}
			}
			if tableItem == nil {
				t.Fatalf("producer %q emitted no table item: %#v", tc.name, items)
			}
			if v, ok := tableItem["text"]; ok && v != "" {
				t.Fatalf("table item still carries legacy HTML text %q", v)
			}
			td, ok := tableItem["table"].(*entity.TableData)
			if !ok || td == nil || len(td.Rows) == 0 {
				t.Fatalf("table item missing structured TableData: %#v", tableItem)
			}
			if got := tableutil.RenderTableText(td); !strings.Contains(got, "What is RAGFlow?") && !strings.Contains(got, "cell") {
				t.Fatalf("structured table missing cell text %q: %#v", got, td)
			}
		})
	}
}

// TestPDFItemsToMarkdownRendersTableFromTableField verifies the markdown
// renderer derives table markup from `table` when the legacy HTML `text` is
// absent. This is the consumer that previously forced producers to keep `text`;
// once producers drop `text`, markdown must still embed the table. The output
// must be byte-equivalent to rendering `table` directly (stable across the
// ParseTableHTML/RenderTableHTML round-trip).
func TestPDFItemsToMarkdownRendersTableFromTableField(t *testing.T) {
	td := &entity.TableData{
		Rows:       [][]string{{"What is RAGFlow?", "A RAG engine."}, {"Is it open source?", "Yes."}},
		HeaderRows: 1,
	}
	// Producer-after-cleanup shape: `table` present, `text` absent.
	item := map[string]any{"doc_type_kwd": "table", "layout": "table", "table": td}
	res := pdfItemsToResult("doc.pdf", []map[string]any{item}, "markdown", 1)
	if res.Err != nil {
		t.Fatalf("pdfItemsToResult markdown: %v", res.Err)
	}
	want := strings.TrimSpace(tableutil.RenderTableHTML(td))
	if got := strings.TrimSpace(res.Markdown); got != want {
		t.Fatalf("markdown table mismatch:\n got: %q\nwant: %q", got, want)
	}
	if !strings.Contains(res.Markdown, "What is RAGFlow?") {
		t.Fatalf("markdown lost table cell text: %q", res.Markdown)
	}
}

// TestPDFItemsToMarkdownSkipsTableWithoutStructuredData guards the
// structured-first contract: a table-typed item that carries no `table`
// (e.g. a mislabeled table region) must be skipped from markdown rather than
// falling back to a raw HTML `text`. This pins "tables are structured, not
// markup" so a future change cannot reintroduce an HTML fallback for tables.
func TestPDFItemsToMarkdownSkipsTableWithoutStructuredData(t *testing.T) {
	// doc_type_kwd "table" but no `table` field and no `text`.
	item := map[string]any{"doc_type_kwd": "table", "layout": "table"}
	res := pdfItemsToResult("doc.pdf", []map[string]any{item}, "markdown", 1)
	if res.Err != nil {
		t.Fatalf("pdfItemsToResult markdown: %v", res.Err)
	}
	if strings.TrimSpace(res.Markdown) != "" {
		t.Fatalf("markdown must skip table-typed item without structured data, got %q", res.Markdown)
	}
}

// TestProjectPDFTableKeepsTextWhenNotProjected guards the safety net: items
// that are not tables, or whose text is not <table> markup, must keep their
// `text` untouched and receive no `table` — we must never delete text for
// non-table items while stripping it from projected tables.
func TestProjectPDFTableKeepsTextWhenNotProjected(t *testing.T) {
	t.Run("non-table doc_type", func(t *testing.T) {
		item := map[string]any{"doc_type_kwd": "text", "text": "plain prose"}
		projectPDFTable(item)
		if item["text"] != "plain prose" {
			t.Fatalf("text mutated for non-table item: %#v", item)
		}
		if _, ok := item["table"]; ok {
			t.Fatalf("non-table item gained table: %#v", item)
		}
	})
	t.Run("table doc_type but non-HTML text", func(t *testing.T) {
		item := map[string]any{"doc_type_kwd": "table", "text": "not a table"}
		projectPDFTable(item)
		if item["text"] != "not a table" {
			t.Fatalf("text mutated for non-HTML table item: %#v", item)
		}
		if _, ok := item["table"]; ok {
			t.Fatalf("non-HTML table item gained table: %#v", item)
		}
	})
	t.Run("nil item", func(t *testing.T) {
		projectPDFTable(nil) // must not panic
	})
}
