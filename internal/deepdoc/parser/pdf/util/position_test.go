package util

import (
	"testing"
)

func TestExtractPositions(t *testing.T) {
	// Tag uses 1-indexed page numbers (Python convention); ExtractPositions converts to 0-indexed.
	text := "Some text @@1-2\t50.0\t300.0\t200.0\t400.0## more text"
	poss := ExtractPositions(text)
	if len(poss) != 1 {
		t.Fatalf("expected 1 position, got %d", len(poss))
	}
	p := poss[0]
	if len(p.PageNumbers) != 2 {
		t.Errorf("expected 2 page numbers, got %d", len(p.PageNumbers))
	}
	if p.PageNumbers[0] != 0 || p.PageNumbers[1] != 1 {
		t.Errorf("expected page numbers [0, 1], got %v", p.PageNumbers)
	}
	if p.Left != 50.0 || p.Right != 300.0 || p.Top != 200.0 || p.Bottom != 400.0 {
		t.Errorf("unexpected coords: L=%.1f R=%.1f T=%.1f B=%.1f", p.Left, p.Right, p.Top, p.Bottom)
	}
}

func TestExtractPositionsMultiple(t *testing.T) {
	// Single-page format ("@@1") and range format ("@@2-3") both handled.
	text := "@@1\t10.0\t20.0\t30.0\t40.0## middle @@2-3\t50.0\t60.0\t70.0\t80.0## end"
	poss := ExtractPositions(text)
	if len(poss) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(poss))
	}
	if poss[1].Left != 50.0 {
		t.Errorf("second position Left = %v, want 50.0", poss[1].Left)
	}
	// First tag is single-page: 1 element in PageNumbers
	if len(poss[0].PageNumbers) != 1 || poss[0].PageNumbers[0] != 0 {
		t.Errorf("single-page tag: got PageNumbers %v, want [0]", poss[0].PageNumbers)
	}
}

func TestExtractPositionsEmpty(t *testing.T) {
	poss := ExtractPositions("plain text without tags")
	if len(poss) != 0 {
		t.Errorf("expected 0 positions, got %d", len(poss))
	}
}

func TestFormatPositionTag(t *testing.T) {
	tag := FormatPositionTag(0, 50.0, 300.0, 200.0, 400.0)
	// Page 0 → tag uses 1-indexed: page 1. Single page → no dash (Python format).
	if tag != "@@1\t50.0\t300.0\t200.0\t400.0##" {
		t.Errorf("FormatPositionTag = %q, want '@@1\\t50.0\\t300.0\\t200.0\\t400.0##'", tag)
	}
}

func TestFormatPositionTagRoundtrip(t *testing.T) {
	// Format → Extract should recover the same coordinates
	tag := FormatPositionTag(0, 50.0, 300.0, 200.0, 400.0)
	text := "prefix " + tag + " suffix"
	poss := ExtractPositions(text)
	if len(poss) != 1 {
		t.Fatalf("roundtrip failed: got %d positions", len(poss))
	}
	p := poss[0]
	if p.Left != 50.0 || p.Right != 300.0 || p.Top != 200.0 || p.Bottom != 400.0 {
		t.Error("roundtrip mismatch")
	}
	// Page 0 → tag "page 1" → extract → page 0. Single page → 1 element.
	if len(p.PageNumbers) != 1 || p.PageNumbers[0] != 0 {
		t.Errorf("roundtrip page number: got %v, want [0]", p.PageNumbers)
	}
}

func TestFormatPositionTagRange(t *testing.T) {
	tag := FormatPositionTagRange(0, 2, 50.0, 300.0, 200.0, 400.0)
	// Pages 0-2 → tag uses 1-indexed: 1-3
	if tag != "@@1-3\t50.0\t300.0\t200.0\t400.0##" {
		t.Errorf("FormatPositionTagRange = %q", tag)
	}
}
