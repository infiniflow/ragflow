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
	if boxes[0].Text != "a b" || boxes[1].Text != "c d" {
		t.Errorf("grouping = %q / %q, want %q / %q", boxes[0].Text, boxes[1].Text, "a b", "c d")
	}
	for i, box := range boxes {
		if !box.HasPageNumber || box.PageNumber != 0 {
			t.Errorf("box %d page metadata = (%v, %d), want (true, 0)", i, box.HasPageNumber, box.PageNumber)
		}
	}
}

// TestRescueUnmatchedChars_StraddlingGroupSurvives is the "345" case end to
// end: OCR kept the middle digit, so "3" and "5" pass the per-char coverage
// filter, merge into one rescue group whose SPANNING bbox overlaps the kept
// box heavily — and must still be added (the old bbox-level dedup discarded
// the whole group, losing both rescued digits).
func TestRescueUnmatchedChars_StraddlingGroupSurvives(t *testing.T) {
	kept := []pdf.TextBox{{Text: "4", X0: 4, X1: 13, Top: 8, Bottom: 22}}
	out := rescueUnmatchedChars(kept, []pdf.TextChar{rch("3", 0, 5), rch("5", 12, 17)}, 0)
	if len(out) != 2 {
		t.Fatalf("expected kept box + one rescue box, got %d boxes", len(out))
	}
	if out[1].Text != "3 5" {
		t.Errorf("rescued group = %q, want %q", out[1].Text, "3 5")
	}
}

// TestRescueBoxes_CappedThresholdSplitsCrossCellChars pins the P0 shape: a
// line consisting ONLY of isolated chars from far-apart cells has inter-char
// gaps equal to the cell gaps themselves (80pt, 120pt). The uncapped
// median+8pt rule computes a threshold above every gap and never splits,
// gluing whole rows into one giant box; the 12pt cap forces per-cell boxes.
func TestRescueBoxes_CappedThresholdSplitsCrossCellChars(t *testing.T) {
	boxes := rescueBoxes([]pdf.TextChar{
		rch("3", 0, 5), rch("7", 85, 90), rch("9", 210, 215),
	}, 0)
	if len(boxes) != 3 {
		t.Fatalf("expected 3 single-char boxes (gaps 80/120 exceed the 12pt cap), got %d: %q",
			len(boxes), []string{boxes[0].Text, boxes[len(boxes)-1].Text})
	}
	for i, want := range []string{"3", "7", "9"} {
		if boxes[i].Text != want {
			t.Errorf("box %d = %q, want %q", i, boxes[i].Text, want)
		}
	}
}

// TestRescueUnmatchedChars_CoveredCharsNeverRescued: the per-char coverage
// pre-filter (<=30% overlap to count as unmatched) is the real dedup — chars
// sitting inside a retained box never reach rescueBoxes at all.
func TestRescueUnmatchedChars_CoveredCharsNeverRescued(t *testing.T) {
	kept := []pdf.TextBox{{Text: "78", X0: 0, X1: 20, Top: 8, Bottom: 22}}
	out := rescueUnmatchedChars(kept, []pdf.TextChar{rch("7", 1, 6), rch("8", 8, 13)}, 0)
	if len(out) != 1 {
		t.Fatalf("covered chars must be filtered before grouping; got %d boxes", len(out))
	}
}
