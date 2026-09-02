//go:build cgo

package docx

import "testing"

func TestJoinElements_MultiParagraphCell(t *testing.T) {
	// When a table cell contains multiple paragraphs, joinElements must
	// insert a newline between them to match python-docx _Cell.text behavior.
	els := []irElement{
		{Type: "paragraph", Content: []irRun{{Type: "text", Text: "first line"}}},
		{Type: "paragraph", Content: []irRun{{Type: "text", Text: "second line"}}},
	}
	got := joinElements(els)
	want := "first line\nsecond line"
	if got != want {
		t.Errorf("joinElements:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestJoinElements_SingleElement(t *testing.T) {
	// Single paragraph cell — no separator expected.
	els := []irElement{
		{Type: "paragraph", Content: []irRun{{Type: "text", Text: "single paragraph"}}},
	}
	got := joinElements(els)
	want := "single paragraph"
	if got != want {
		t.Errorf("joinElements:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestJoinElements_Empty(t *testing.T) {
	got := joinElements(nil)
	if got != "" {
		t.Errorf("joinElements(nil): got %q, want empty", got)
	}
}
