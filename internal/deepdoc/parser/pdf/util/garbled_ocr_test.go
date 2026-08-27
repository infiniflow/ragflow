package util

import "testing"

// runeSet builds an alphabet from a string, mirroring how loadOCRAlphabet reads
// ocr.res into a rune set.
func runeSet(s string) map[rune]struct{} {
	m := make(map[rune]struct{})
	for _, r := range s {
		m[r] = struct{}{}
	}
	return m
}

// TestOcrCanRepresent covers the guard that keeps a usable text layer when the
// OCR recogniser's alphabet cannot spell the script. The real ocr.res is
// CJK+Latin (52 Latin / 6 Cyrillic), giving English ~100% coverage and Russian
// ~6% — so a Latin-only alphabet reproduces the split here.
func TestOcrCanRepresent(t *testing.T) {
	latin := runeSet("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 .,-")

	tests := []struct {
		name        string
		text        string
		alphabet    map[rune]struct{}
		minCoverage float64
		want        bool
	}{
		{"english is representable", "Minimum edit distance", latin, 0.8, true},
		{"russian is not representable", "О книге как-то иначе", latin, 0.8, false},
		{"empty text is representable", "", latin, 0.8, true},
		{"whitespace only is representable", "   ", latin, 0.8, true},
		{"unknown alphabet preserves behaviour", "О книге", runeSet(""), 0.8, true},
		{"exactly at threshold", "abcdx", runeSet("abcd"), 0.8, true}, // 4/5 == 0.80
		{"below threshold", "abcxx", runeSet("abcd"), 0.8, false},     // 3/5 == 0.60
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ocrCanRepresent(tt.text, tt.alphabet, tt.minCoverage)
			if got != tt.want {
				t.Errorf("ocrCanRepresent(%q, ...) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
