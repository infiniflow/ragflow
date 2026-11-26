package chunker

import (
	"strings"
	"testing"
)

// TestQAChunker_DetectDelimiter pins the delimiter decision against Python's
// csv_pair_delimiter, the reference this port follows. Each case is one shape
// the heuristic must classify; the expected values were produced by running the
// Python function on the same line sets.
//
// The previous implementation counted raw comma splits, so a quoted row with an
// interior comma counted as neither comma nor tab separated, and rows with three
// fields fell through to the TAB reader.
func TestQAChunker_DetectDelimiter(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{"comma pairs", []string{"q1,a1", "q2,a2"}, ","},
		{"tab pairs", []string{"q1\ta1", "q2\ta2"}, "\t"},
		{"quoted comma no tab", []string{`"what is a, really?","a1"`, `"what is b, really?","b1"`}, ","},
		{"mixed comma and tab rows", []string{"q1,a1", "q2\ta2", "q3,a3"}, "\t"},
		{"first row has tab", []string{"q1\ta1", "q2,a2"}, "\t"},
		{"three field comma rows", []string{"q1,a1,x", "q2,a2,y"}, ","},
		{"tab inside quoted comma field", []string{`"q1","a1"`, `"q2","a2` + "\t" + `X"`, `"q3","a3"`}, ","},
		{"empty lines between tab pairs", []string{"q1\ta1", "", "q2\ta2"}, "\t"},
		{"empty lines between comma pairs", []string{"q1,a1", "", "q2,a2"}, ","},
		{"one tab row among comma rows", []string{"q1,a1", "q2,a2", "q3\ta3"}, "\t"},
		{"single column with tabs", []string{"q1\ta1\textra"}, "\t"},
		{"mostly comma one three field", []string{"q1,a1", "q2,a2", "q3,a3,x"}, ","},
		{"trailing empty fields", []string{"q1,a1,", "q2,a2,"}, ","},
		// A quoted field may span physical lines (RFC 4180). The record shape,
		// not the physical line shape, decides: read line by line, this file
		// fragments into rows the real payload does not have and falls to TAB.
		{"quoted multiline answer with a later tab", []string{`"q1","first line`, `second line"`, `"q2","plain"`, `"q3","has` + "\t" + `tab"`}, ","},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := strings.Join(tc.lines, "\n")
			if got := detectDelimiter(text); got != tc.want {
				t.Errorf("detectDelimiter(%q) = %q, want %q", text, got, tc.want)
			}
		})
	}
}
