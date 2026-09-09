package tool

import (
	"testing"
)

func TestStripMeta(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no meta", "hello world", "hello world"},
		{"with meta", "hello\n#@meta\n{\"key\":\"val\"}", "hello"},
		{"multiple meta lines", "line1\nline2\n#@meta\n{}", "line1\nline2"},
		{"empty", "", ""},
		{"only meta no newline", "#@meta\n{}", "#@meta\n{}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripMeta(tt.input); got != tt.want {
				t.Errorf("StripMeta = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCharSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want float64
	}{
		{"identical", "hello", "hello", 100.0},
		{"completely different", "abc", "xyz", 0.0},
		{"partial overlap", "abc", "bcd", 66.66}, // returns percentage
		{"empty both", "", "", 100.0},
		{"empty a", "", "abc", 0.0},
		{"empty b", "abc", "", 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CharSimilarity(tt.a, tt.b)
			if got < tt.want-1 || got > tt.want+1 {
				t.Errorf("CharSimilarity(%q, %q) = %v, want ~%v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLcsSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want float64
	}{
		{"identical", "hello", "hello", 100.0},
		{"completely different", "abc", "xyz", 0.0},
		{"partial", "abc", "abcd", 75.0}, // LCS "abc" len=3, max len=4 = 75%
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LcsSimilarity(tt.a, tt.b)
			if got < tt.want-1 || got > tt.want+1 {
				t.Errorf("LcsSimilarity(%q, %q) = %v, want ~%v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSectionAlignedScore(t *testing.T) {
	tests := []struct {
		name   string
		goText string
		pyText string
		want   float64
	}{
		{"identical", "hello world", "hello world", 100.0},
		{"different", "hello", "world", 20.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SectionAlignedScore(tt.goText, tt.pyText)
			if got < tt.want-1 || got > tt.want+1 {
				t.Errorf("SectionAlignedScore(%q, %q) = %v, want ~%v",
					tt.goText, tt.pyText, got, tt.want)
			}
		})
	}
}
