package pdf

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func rch(txt string, x0, x1 float64) pdf.TextChar {
	return pdf.TextChar{Text: txt, X0: x0, X1: x1, Top: 10, Bottom: 20, PageNumber: 0}
}

// TestRescueBoxes_SplitsWideCellGapsKeepsTightGroups: gaps [2,30,2] → median
// 2, threshold 10 → the 30pt cell gap splits, tight word gaps do not.
func TestRescueBoxes_SplitsWideCellGapsKeepsTightGroups(t *testing.T) {
	boxes := rescueBoxes([]pdf.TextChar{
		rch("a", 0, 5), rch("b", 7, 12), rch("c", 42, 47), rch("d", 49, 54),
	}, 0)
	if len(boxes) != 2 {
		t.Fatalf("expected 2 rescue boxes (split at the 30pt cell gap), got %d", len(boxes))
	}
	if boxes[0].box.Text != "a b" || boxes[1].box.Text != "c d" {
		t.Errorf("grouping = %q / %q, want %q / %q", boxes[0].box.Text, boxes[1].box.Text, "a b", "c d")
	}
}

// TestRescueBoxes_StraddlingGroupSurvives: OCR kept the middle digit of
// "345"; the rescued "3" and "5" merge into one box whose SPANNING bbox
// overlaps the retained box heavily. The old bbox-level filter discarded the
// whole group (losing both digits); constituents must decide instead.
func TestRescueBoxes_StraddlingGroupSurvives(t *testing.T) {
	kept := []pdf.TextBox{{Text: "4", X0: 4, X1: 13, Top: 8, Bottom: 22}}
	groups := rescueBoxes([]pdf.TextChar{rch("3", 0, 5), rch("5", 12, 17)}, 0)
	if len(groups) != 1 || groups[0].box.Text != "3 5" {
		t.Fatalf("expected one merged rescue box '3 5', got %d groups", len(groups))
	}
	if majorityCovered(groups[0].chars, kept) {
		t.Error("straddling rescue group must survive: neither constituent is covered")
	}
}

func TestMajorityCovered_DropsTrueDuplicate(t *testing.T) {
	kept := []pdf.TextBox{{Text: "78", X0: 0, X1: 20, Top: 8, Bottom: 22}}
	groups := rescueBoxes([]pdf.TextChar{rch("7", 1, 6), rch("8", 8, 13), rch("9", 60, 65)}, 0)
	for _, g := range groups {
		switch g.box.Text {
		case "7 8":
			if !majorityCovered(g.chars, kept) {
				t.Error(`rescued "7 8" sits inside a retained box; must be dropped`)
			}
		case "9":
			if majorityCovered(g.chars, kept) {
				t.Error(`rescued "9" is far from any box; must survive`)
			}
		}
	}
}
