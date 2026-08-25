package layout

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestLooksLikeWatermarkCandidate(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		// Real watermark tokens from the issue.
		{"Q2qRhNU_P", true},
		{"Wdy_V2iYxTR", true},
		{"ZH1c56f", true},
		{"ce1a60", true},
		{"4326dc", true},
		{"G0", true},
		{"md24", true},
		// Edge cases.
		{"", false},                      // empty
		{"a", false},                     // too short
		{"abc", false},                   // no digit
		{"ABC", false},                   // no digit, no lower
		{"123", false},                   // no letter
		{"abc123", true},                 // lowercase + digit
		{"ABC123", true},                 // uppercase + digit
		{"Abcdef", false},                // no digit
		{"hello world", false},           // whitespace
		{strings.Repeat("A", 65), false}, // too long
	}
	for _, tc := range cases {
		if got := looksLikeWatermarkCandidate(tc.text); got != tc.want {
			t.Errorf("looksLikeWatermarkCandidate(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestFilterWatermarkChars_DropsRepeatedTokens(t *testing.T) {
	// Simulate a page with a resume-watermark pattern: a 6-character token
	// "ce1a60" appears 4 times scattered across the page (the issue
	// reports this exact pattern).
	chars := []pdf.TextChar{
		{Text: "姓", X0: 50, X1: 58, Top: 100, Bottom: 112},
		{Text: "名", X0: 60, X1: 68, Top: 100, Bottom: 112},
		{Text: "ce1a60", X0: 200, X1: 250, Top: 100, Bottom: 112},
		{Text: "Q", X0: 300, X1: 305, Top: 110, Bottom: 112},
		{Text: "2", X0: 310, X1: 315, Top: 110, Bottom: 112},
		{Text: "q", X0: 320, X1: 325, Top: 110, Bottom: 112},
		{Text: "RhNU_P", X0: 330, X1: 360, Top: 110, Bottom: 112},
		{Text: "教育", X0: 50, X1: 68, Top: 200, Bottom: 212},
		{Text: "ce1a60", X0: 200, X1: 250, Top: 200, Bottom: 212},
		{Text: "ce1a60", X0: 200, X1: 250, Top: 300, Bottom: 312},
		{Text: "ce1a60", X0: 200, X1: 250, Top: 400, Bottom: 412},
	}
	filtered := filterWatermarkChars(chars)
	for _, c := range filtered {
		if c.Text == "ce1a60" {
			t.Errorf("watermark token %q survived filtering", c.Text)
		}
	}
	if len(filtered) != len(chars)-4 {
		t.Errorf("expected %d chars after filter, got %d", len(chars)-4, len(filtered))
	}
}

func TestFilterWatermarkChars_KeepsUniqueTokens(t *testing.T) {
	// Single-occurrence tokens must not be stripped even if they look
	// plausible — promotion requires 3+ occurrences on the page.
	chars := []pdf.TextChar{
		{Text: "Abc123", X0: 50, X1: 80, Top: 100, Bottom: 112},
		{Text: "Xyz789", X0: 100, X1: 130, Top: 100, Bottom: 112},
		{Text: "Def456", X0: 150, X1: 180, Top: 100, Bottom: 112},
		{Text: "Ghi012", X0: 200, X1: 230, Top: 100, Bottom: 112},
	}
	filtered := filterWatermarkChars(chars)
	if len(filtered) != len(chars) {
		t.Errorf("expected %d chars after filter (no promotions), got %d", len(chars), len(filtered))
	}
}

func TestCharsToBoxes_DropsWatermarkBeforeGrouping(t *testing.T) {
	// Even when the watermark tokens have unique vertical positions (so
	// they'd each become their own "line" before the fix), the boxes
	// emitted by CharsToBoxes must not contain them.
	chars := []pdf.TextChar{
		{Text: "姓", X0: 50, X1: 58, Top: 100, Bottom: 112},
		{Text: "名", X0: 60, X1: 68, Top: 100, Bottom: 112},
		// Scattered watermark tokens with different vertical positions.
		{Text: "ce1a60", X0: 200, X1: 250, Top: 100, Bottom: 112},
		{Text: "ce1a60", X0: 200, X1: 250, Top: 200, Bottom: 212},
		{Text: "ce1a60", X0: 200, X1: 250, Top: 300, Bottom: 312},
		{Text: "ce1a60", X0: 200, X1: 250, Top: 400, Bottom: 412},
	}
	boxes := CharsToBoxes(chars, 0, false)
	for _, b := range boxes {
		if containsWatermark(b.Text) {
			t.Errorf("box text %q contains watermark token", b.Text)
		}
	}
}

func containsWatermark(s string) bool {
	return strings.Contains(s, "ce1a60")
}
