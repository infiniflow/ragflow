//go:build cgo

package parser

import "testing"

// TestPDFOutlineNativeExtractionFeedsRemoteResult is the native-path proof for
// #21221: a bookmarked PDF parsed by an engine that reads no bookmarks itself
// (plain_text) must come back with the outline extracted from the source file.
func TestPDFOutlineNativeExtractionFeedsRemoteResult(t *testing.T) {
	data := bookmarkedPDF(t)

	p := &PDFParser{ParseMethod: "plain_text"}
	res := p.ParseWithResult(t.Context(), "bookmarked.pdf", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	outline, ok := res.File["outline"].([]map[string]any)
	if !ok {
		t.Fatalf("File[outline] type = %T, want []map[string]any", res.File["outline"])
	}
	want := []struct {
		title string
		level int
		page  int
	}{
		{"Chapter 1", 0, 1},
		{"Section 1.1", 1, 2},
		{"Chapter 2", 0, 2},
	}
	if len(outline) != len(want) {
		t.Fatalf("outline = %#v, want %d entries", outline, len(want))
	}
	for i, w := range want {
		got := outline[i]
		if got["title"] != w.title || got["level"] != w.level || got["page_number"] != w.page {
			t.Fatalf("outline[%d] = %#v, want title=%q level=%d page=%d", i, got, w.title, w.level, w.page)
		}
	}
}
