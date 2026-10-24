package parser

import (
	"testing"

	"ragflow/internal/entity"
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
			if !ok || td == nil || len(td.CellTexts()) == 0 {
				t.Fatalf("table item missing structured TableData: %#v", tableItem)
			}
			if !cellTextsContain(td, "What is RAGFlow?") && !cellTextsContain(td, "cell") {
				t.Fatalf("structured table missing cell text %q: %#v", "What is RAGFlow?", td)
			}
		})
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
