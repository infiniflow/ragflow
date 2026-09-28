package parser

import (
	"encoding/json"
	"testing"
)

// cellWithText builds a table cell whose only content is a single paragraph
// run carrying s, so joinCellText renders it as s.
func cellWithText(s string) docxIRCell {
	return docxIRCell{
		Content: []docxIRElement{{
			Type:    "paragraph",
			Content: json.RawMessage(`[{"type":"text","text":"` + s + `"}]`),
		}},
	}
}

// TestDocxElementTextCellSeparator locks the contract introduced when
// docElementText was collapsed into docxElementText: the cellSep argument is
// the within-row separator. docx/pptx pass "\n" (each cell on its own line);
// the .doc flatten path passes " | " so recovered table text stays readable.
func TestDocxElementTextCellSeparator(t *testing.T) {
	table := docxIRElement{
		Type: "table",
		Rows: []docxIRRow{
			{Cells: []docxIRCell{cellWithText("a"), cellWithText("b")}},
			{Cells: []docxIRCell{cellWithText("c"), cellWithText("d")}},
		},
	}

	if got := docxElementText(table, "\n"); got != "a\nb\nc\nd" {
		t.Errorf("docx cellSep=\"\\n\": got %q, want %q", got, "a\nb\nc\nd")
	}

	if got := docxElementText(table, " | "); got != "a | b\nc | d" {
		t.Errorf("doc cellSep=\" | \": got %q, want %q", got, "a | b\nc | d")
	}
}
