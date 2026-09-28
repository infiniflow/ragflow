//go:build cgo

package parser

import "testing"

// TestSelectDocTextViewPrefersStructuredIR locks the fix for the fragile
// "longest view" heuristic in extractDocText: when the office_oxide IR carries
// table (or list) structure, that content is unique to the IR view and must
// win even if PlainText/ToMarkdown produce a longer (but flatter) string.
// Before the fix, a prose-heavy .doc could make PlainText longer than the IR
// and silently drop the recovered table.
func TestSelectDocTextViewPrefersStructuredIR(t *testing.T) {
	// IR JSON that carries a table element (the structure only the IR view
	// surfaces). irText is its flattened form with the " | " separator.
	irJSON := `{"sections":[{"elements":[{"type":"table","rows":[]}]}]}`
	irText := "Name | Value\nAlice | 1"

	// PlainText is deliberately longer than the IR flatten — the old
	// length-only heuristic would have shadowed the table here.
	plainText := "This is a long block of prose that is clearly longer " +
		"than the few table cells above, so a naive longest-view selection " +
		"would pick this string and drop the recovered table structure."

	got := selectDocTextView(irJSON, irText, "", plainText)
	if got != irText {
		t.Errorf("structured IR not preferred:\n got=%q\nwant=%q", got, irText)
	}
	if !contains(got, " | ") {
		t.Errorf("table separator lost in selected view: %q", got)
	}
}

// TestSelectDocTextViewFallsBackToLongest verifies that, absent any structured
// element, the selection still prefers the longest non-empty view (the
// original behavior, preserved so a sparser view never shadows a more
// complete one).
func TestSelectDocTextViewFallsBackToLongest(t *testing.T) {
	// No table/list in the IR.
	irJSON := `{"sections":[{"elements":[{"type":"paragraph"}]}]}`

	irText := "short paragraph"
	mdText := "a medium length markdown rendering of the document body"
	plainText := "the longest plain text among the three available views here"

	got := selectDocTextView(irJSON, irText, mdText, plainText)
	if got != plainText {
		t.Errorf("longest view not selected:\n got=%q\nwant=%q", got, plainText)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
