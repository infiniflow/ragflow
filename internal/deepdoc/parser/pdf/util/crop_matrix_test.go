package util

import (
	"testing"
)

// TestPositionsFromMatrix_ConvertsToZeroBased verifies the JSON matrix
// (1-based page numbers, per normalizePDFPageNumber) is decoded to the
// engine's 0-based page indices. This matches Python's crop(), which
// indexes page_images[page_number-1].
func TestPositionsFromMatrix_ConvertsToZeroBased(t *testing.T) {
	m := [][]any{
		{1.0, 10.0, 100.0, 10.0, 100.0},
		{2.0, 0.0, 50.0, 0.0, 50.0},
	}
	got := PositionsFromMatrix(m)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].PageNumbers[0] != 0 {
		t.Errorf("got[0].PageNumbers[0] = %d, want 0 (1-based -> 0-based)", got[0].PageNumbers[0])
	}
	if got[1].PageNumbers[0] != 1 {
		t.Errorf("got[1].PageNumbers[0] = %d, want 1", got[1].PageNumbers[0])
	}
}

// TestPositionsFromMatrix_ListPageNumbers exercises the multi-page list
// form (cross-page sections), confirming each entry is shifted -1.
func TestPositionsFromMatrix_ListPageNumbers(t *testing.T) {
	m := [][]any{
		{[]any{1.0, 2.0}, 10.0, 100.0, 10.0, 100.0},
	}
	got := PositionsFromMatrix(m)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if len(got[0].PageNumbers) != 2 || got[0].PageNumbers[0] != 0 || got[0].PageNumbers[1] != 1 {
		t.Errorf("PageNumbers = %v, want [0 1]", got[0].PageNumbers)
	}
}

// TestPositionsFromMatrix_SkipsMalformedRows confirms rows shorter than 5
// fields and non-numeric pages are dropped rather than producing garbage.
func TestPositionsFromMatrix_SkipsMalformedRows(t *testing.T) {
	m := [][]any{
		{1.0, 10.0, 100.0, 10.0},        // too short
		{"x", 10.0, 100.0, 10.0, 100.0}, // non-numeric page
		{3.0, 10.0, 100.0, 10.0, 100.0}, // valid -> 0-based 2
	}
	got := PositionsFromMatrix(m)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (malformed rows skipped)", len(got))
	}
	if got[0].PageNumbers[0] != 2 {
		t.Errorf("got[0].PageNumbers[0] = %d, want 2", got[0].PageNumbers[0])
	}
}
