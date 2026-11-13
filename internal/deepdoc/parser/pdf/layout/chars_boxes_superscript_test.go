package layout

import (
	"fmt"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Glyph geometry used below: a 12pt baseline row (Top=100, Bottom=112) and a 7pt
// off-baseline glyph — raised to Top=95 (superscript) or lowered to Top=104
// (subscript). Both offsets stay inside verticalOverlap's half-height tolerance,
// so the glyph DOES join the baseline row; what used to go wrong was its position
// inside the assembled line, not its grouping.
func baseChar(x0, x1 float64, text string) pdf.TextChar {
	return pdf.TextChar{X0: x0, X1: x1, Top: 100, Bottom: 112, Text: text}
}

func raisedChar(x0, x1 float64, text string) pdf.TextChar {
	return pdf.TextChar{X0: x0, X1: x1, Top: 95, Bottom: 102, Text: text}
}

func loweredChar(x0, x1 float64, text string) pdf.TextChar {
	return pdf.TextChar{X0: x0, X1: x1, Top: 104, Bottom: 111, Text: text}
}

// lineText joins a grouped line's characters, in the order the caller receives.
func lineText(line []pdf.TextChar) string {
	var b strings.Builder
	for _, c := range line {
		b.WriteString(c.Text)
	}
	return b.String()
}

func boxesText(boxes []pdf.TextBox) string {
	var b strings.Builder
	for _, bx := range boxes {
		b.WriteString(bx.Text)
	}
	return b.String()
}

// sortByTopModes covers both line keys: production defaults to Bottom
// (Config.SortByTop zero value) while the PDF engine path can request Top.
var sortByTopModes = []bool{true, false}

// TestGroupCharsToLines_OffBaselineGlyphKeepsReadingOrder locks the root cause
// of issue #21388: characters were returned in the vertical-key order, so a
// superscript (smaller Top/Bottom) or a subscript (larger Top) sorted ahead of —
// or behind — the whole line instead of staying between its neighbours.
func TestGroupCharsToLines_OffBaselineGlyphKeepsReadingOrder(t *testing.T) {
	for _, off := range []struct {
		name  string
		make  func(x0, x1 float64, text string) pdf.TextChar
		glyph string
	}{
		{"superscript", raisedChar, "2"},
		{"subscript", loweredChar, "2"},
	} {
		for _, sortByTop := range sortByTopModes {
			// Supplied in content-stream order, as the native extractor emits it.
			chars := []pdf.TextChar{
				baseChar(138, 146, "I"),
				off.make(147, 152, off.glyph),
				baseChar(153, 163, "C"),
			}
			lines := GroupCharsToLines(chars, sortByTop)
			if len(lines) != 1 {
				t.Fatalf("%s sortByTop=%v: expected 1 line, got %d", off.name, sortByTop, len(lines))
			}
			if got := lineText(lines[0]); got != "I2C" {
				t.Errorf("%s sortByTop=%v: line order = %q, want %q", off.name, sortByTop, got, "I2C")
			}
		}
	}
}

// TestCharsToBoxes_OffBaselineGlyphStaysInlineInCaption reproduces the reported
// symptom on Holtek application note an0590scv140.pdf page 4, where the caption
// 「图 3. I²C 时序图」 was emitted as 「2图 3. IC 时序图」: the term is no longer
// searchable and the chunk text leads with a stray digit.
func TestCharsToBoxes_OffBaselineGlyphStaysInlineInCaption(t *testing.T) {
	for _, sortByTop := range sortByTopModes {
		chars := []pdf.TextChar{
			baseChar(100, 112, "图"),
			baseChar(118, 124, "3"),
			baseChar(126, 130, "."),
			baseChar(138, 146, "I"),
			raisedChar(147, 152, "2"),
			baseChar(153, 163, "C"),
			baseChar(176, 188, "时"),
			baseChar(190, 202, "序"),
			baseChar(204, 216, "图"),
		}
		text := boxesText(CharsToBoxes(chars, 0, sortByTop))
		if !strings.Contains(text, "I2C") {
			t.Errorf("sortByTop=%v: box text = %q, want it to contain %q", sortByTop, text, "I2C")
		}
		if strings.HasPrefix(text, "2") {
			t.Errorf("sortByTop=%v: box text = %q, but the superscript leads the caption", sortByTop, text)
		}
	}
}

// TestCharsToBoxes_OffBaselineGlyphDoesNotBreakColumnSplit covers the other
// consequence of the same ordering bug: the gap logic that splits two columns
// sharing a visual line reads consecutive characters, so a vertical-first order
// handed it a negative leading gap and welded both columns into one box.
func TestCharsToBoxes_OffBaselineGlyphDoesNotBreakColumnSplit(t *testing.T) {
	for _, sortByTop := range sortByTopModes {
		chars := []pdf.TextChar{
			baseChar(100, 112, "文"),
			baseChar(118, 126, "A"),
			raisedChar(127, 131, "1"),
			baseChar(132, 142, "B"),
			// Second column, same baseline row.
			baseChar(300, 312, "图"),
			baseChar(318, 324, "2"),
		}
		boxes := CharsToBoxes(chars, 0, sortByTop)
		if len(boxes) != 2 {
			t.Fatalf("sortByTop=%v: expected 2 column boxes, got %d: %q",
				sortByTop, len(boxes), boxesText(boxes))
		}
		if !strings.Contains(boxes[0].Text, "A1B") {
			t.Errorf("sortByTop=%v: left column = %q, want it to contain %q", sortByTop, boxes[0].Text, "A1B")
		}
		if boxes[1].Text != "图2" {
			t.Errorf("sortByTop=%v: right column = %q, want %q", sortByTop, boxes[1].Text, "图2")
		}
	}
}

// TestGroupCharsToLines_KeepsSeparateLinesApart guards against fixing the order
// by sorting the whole page horizontally: lines must stay separate groups, in
// vertical order, each with its own off-baseline glyph inlined.
func TestGroupCharsToLines_KeepsSeparateLinesApart(t *testing.T) {
	for _, sortByTop := range sortByTopModes {
		chars := []pdf.TextChar{
			baseChar(100, 108, "A"),
			raisedChar(109, 113, "1"),
			{X0: 100, X1: 108, Top: 120, Bottom: 132, Text: "B"},
			{X0: 109, X1: 113, Top: 115, Bottom: 122, Text: "2"},
		}
		lines := GroupCharsToLines(chars, sortByTop)
		if len(lines) != 2 {
			t.Fatalf("sortByTop=%v: expected 2 lines, got %d", sortByTop, len(lines))
		}
		for i, want := range []string{"A1", "B2"} {
			if got := lineText(lines[i]); got != want {
				t.Errorf("sortByTop=%v: line %d = %q, want %q", sortByTop, i, got, want)
			}
		}
	}
}

// TestGroupCharsToLines_EqualX0KeepsVerticalOrder pins the boundary of the X0
// re-sort: it is stable, so it never reorders a tie — but a tie between two
// glyphs at the SAME X and different heights has no horizontal reading order to
// recover, so the vertical pass is what decides their position. Overlaid
// same-box glyphs (identical key and X0) keep the extractor's order, because the
// vertical pass leaves those in place too.
func TestGroupCharsToLines_EqualX0KeepsVerticalOrder(t *testing.T) {
	for _, sortByTop := range sortByTopModes {
		t.Run(fmt.Sprintf("sortByTop=%v", sortByTop), func(t *testing.T) {
			overlaid := []pdf.TextChar{
				baseChar(50, 58, "a"),
				baseChar(50, 58, "b"),
			}
			lines := GroupCharsToLines(overlaid, sortByTop)
			if len(lines) != 1 {
				t.Fatalf("expected 1 line, got %d", len(lines))
			}
			if got := lineText(lines[0]); got != "ab" {
				t.Errorf("overlaid glyphs = %q, want %q", got, "ab")
			}

			// Same X, different height: the raised glyph sorts first under either
			// vertical key, and the X0 re-sort leaves that tie alone.
			tied := []pdf.TextChar{
				baseChar(50, 58, "I"),
				raisedChar(50, 55, "2"),
			}
			lines = GroupCharsToLines(tied, sortByTop)
			if len(lines) != 1 {
				t.Fatalf("expected 1 line, got %d", len(lines))
			}
			if got := lineText(lines[0]); got != "2I" {
				t.Errorf("equal-X0 glyphs of different height = %q, want %q (vertical pass decides ties)", got, "2I")
			}
		})
	}
}
