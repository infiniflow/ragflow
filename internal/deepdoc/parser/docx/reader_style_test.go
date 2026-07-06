//go:build cgo

package docx

import "testing"

func TestIrElementToBlock_PreservesCustomStyle(t *testing.T) {
	// irElementToBlock should preserve the Word style name from the IR,
	// not hard-code "Normal" for every non-heading paragraph.
	el := irElement{
		Type:  "paragraph",
		Style: "Caption",
		Content: []irRun{
			{Type: "text", Text: "Figure 1: Architecture diagram"},
		},
	}
	block := irElementToBlock(el)

	if block.Style != "Caption" {
		t.Errorf("irElementToBlock with Style=%q:\ngot  Style=%q\nwant Style=%q",
			el.Style, block.Style, el.Style)
	}
}

func TestIrElementToBlock_PreservesHeadingStyle(t *testing.T) {
	// Heading elements should still produce "Heading N" style.
	el := irElement{
		Type:  "heading",
		Level: 2,
		Content: []irRun{
			{Type: "text", Text: "Section 2.1"},
		},
	}
	block := irElementToBlock(el)

	if block.Style != "Heading 2" {
		t.Errorf("heading: got Style=%q, want %q", block.Style, "Heading 2")
	}
}

func TestIrElementToBlock_FallsBackToNormal(t *testing.T) {
	// When Style is empty, defaults to "Normal".
	el := irElement{
		Type: "paragraph",
		Content: []irRun{
			{Type: "text", Text: "plain text"},
		},
	}
	block := irElementToBlock(el)

	if block.Style != "Normal" {
		t.Errorf("empty style: got %q, want %q", block.Style, "Normal")
	}
}
