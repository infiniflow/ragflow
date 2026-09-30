package parser

import (
	"strings"
	"testing"
)

// Consumers of doc_type_kwd "table" parse the item text as <table> markup
// (the QA chunker reads its <tr> rows), so a table item that carries other
// text loses its rows downstream.
func TestPDFRemoteParsers_TableItemsCarryTableMarkup(t *testing.T) {
	cells := []any{
		map[string]any{"row": 0, "content": "What is RAGFlow?"},
		map[string]any{"row": 0, "content": "A RAG engine."},
		map[string]any{"row": 1, "content": "Is it open source?"},
		map[string]any{"row": 1, "content": "Yes."},
	}
	rows := map[string]any{"rows": []any{
		[]any{"What is RAGFlow?", "A RAG engine."},
		[]any{"Is it open source?", "Yes."},
	}}
	const tableHTML = "<table><tr><td>What is RAGFlow?</td><td>A RAG engine.</td></tr></table>"
	const pipeText = "What is RAGFlow? | A RAG engine."
	blankCells := []any{
		map[string]any{"row": 0, "content": ""},
		map[string]any{"row": 0, "content": " "},
	}
	blankRows := map[string]any{"rows": []any{[]any{"", " "}}}
	const markdownTable = "| What is RAGFlow? | A RAG engine. |\n| --- | --- |\n| Is it open source? | Yes. |"

	cases := []struct {
		name        string
		produce     func() []map[string]any
		wantDocType string
	}{
		{"opendataloader html", func() []map[string]any {
			return openDataLoaderItems(map[string]any{"type": "table", "html": tableHTML})
		}, "table"},
		{"opendataloader content only", func() []map[string]any {
			return openDataLoaderItems(map[string]any{"type": "table", "content": pipeText})
		}, "text"},
		{"opendataloader cells only", func() []map[string]any {
			return openDataLoaderItems(map[string]any{"type": "table", "cells": cells})
		}, "table"},
		{"opendataloader blank cells keep content", func() []map[string]any {
			return openDataLoaderItems(map[string]any{"type": "table", "content": pipeText, "cells": blankCells})
		}, "text"},
		{"tcadp html content", func() []map[string]any {
			return tcadpAnyToItems(map[string]any{"type": "table", "content": tableHTML})
		}, "table"},
		{"tcadp content only", func() []map[string]any {
			return tcadpAnyToItems(map[string]any{"type": "table", "content": pipeText})
		}, "text"},
		{"tcadp table_data rows only", func() []map[string]any {
			return tcadpAnyToItems(map[string]any{"type": "table", "table_data": rows})
		}, "table"},
		{"tcadp blank rows keep content", func() []map[string]any {
			return tcadpAnyToItems(map[string]any{"type": "table", "content": pipeText, "table_data": blankRows})
		}, "text"},
		{"somark html", func() []map[string]any {
			return []map[string]any{soMarkBlockToItem(map[string]any{"type": "table", "content": tableHTML}, false)}
		}, "table"},
		{"somark markdown", func() []map[string]any {
			return []map[string]any{soMarkBlockToItem(map[string]any{"type": "table", "content": markdownTable}, false)}
		}, "text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := tc.produce()
			if len(items) != 1 || items[0] == nil {
				t.Fatalf("items = %v, want one item", items)
			}
			item := items[0]
			docType, _ := item["doc_type_kwd"].(string)
			text, _ := item["text"].(string)
			if docType == "table" && !strings.HasPrefix(strings.TrimSpace(text), "<table") {
				t.Fatalf("doc_type_kwd table carries non-<table> text %q", text)
			}
			if docType != tc.wantDocType {
				t.Fatalf("doc_type_kwd = %q, want %q (text %q)", docType, tc.wantDocType, text)
			}
		})
	}
}
