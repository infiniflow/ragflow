package util

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestIsASCIIPrintable(t *testing.T) {
	english := "hello WORLD 123"
	for _, r := range english {
		if !IsASCIIPrintable(r) {
			t.Errorf("IsASCIIPrintable(%q) = false, want true", r)
		}
	}
	symbols := ",/;:'[]()!@#$%^&*\"?<>._-"
	for _, r := range symbols {
		if !IsASCIIPrintable(r) {
			t.Errorf("IsASCIIPrintable(%q) = false, want true", r)
		}
	}
	nonEnglish := "你好世界"
	for _, r := range nonEnglish {
		if IsASCIIPrintable(r) {
			t.Errorf("IsASCIIPrintable(%q) = true, want false", r)
		}
	}
}

func TestDefaultSampleChars(t *testing.T) {
	chars := []pdf.TextChar{
		{Text: "A"}, {Text: "B"}, {Text: "C"}, {Text: "D"}, {Text: "E"},
	}
	got := DefaultSampleChars(chars, 3)
	if len(got) == 0 {
		t.Error("expected non-empty sample")
	}
	// Every sampled char should be from our set
	for _, r := range got {
		if !strings.ContainsRune("ABCDE", r) {
			t.Errorf("unexpected char %q in sample", r)
		}
	}

	// n <= 0 returns empty
	if DefaultSampleChars(chars, 0) != "" {
		t.Error("n=0 should return empty")
	}
	if DefaultSampleChars(nil, 10) != "" {
		t.Error("nil chars should return empty")
	}
}

func TestDefaultSampleCharsStable(t *testing.T) {
	chars := make([]pdf.TextChar, 40)
	for i := range chars {
		chars[i] = pdf.TextChar{Text: string(rune('a' + i%26))}
	}
	// Evenly spaced indices 0,4,...,36 of a repeating a-z alphabet.
	const want = "aeimquycgk"
	var first string
	for range 30 {
		got := DefaultSampleChars(chars, 10)
		if first == "" {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("DefaultSampleChars changed across calls: %q vs %q", first, got)
		}
	}
	if first != want {
		t.Fatalf("DefaultSampleChars = %q, want %q", first, want)
	}
}

func TestDetectEnglishPageStable(t *testing.T) {
	// 29 ASCII, one CJK, 29 ASCII. In page order the longest ASCII run is 29,
	// so the page is not English. A shuffle of the same characters usually
	// builds a run of 30 and flips the vote.
	chars := make([]pdf.TextChar, 59)
	for i := range chars {
		if i == 29 {
			chars[i] = pdf.TextChar{Text: "你"}
			continue
		}
		chars[i] = pdf.TextChar{Text: "A"}
	}
	first := DetectEnglishPage(chars, nil)
	for range 20 {
		if got := DetectEnglishPage(chars, nil); got != first {
			t.Fatalf("DetectEnglishPage flipped: %v then %v", first, got)
		}
	}
	if first {
		t.Fatal("DetectEnglishPage = true, want false")
	}
}

func TestFullTextFromChars(t *testing.T) {
	chars := map[int][]pdf.TextChar{
		0: {{Text: "Hello"}, {Text: " "}, {Text: "World"}},
		1: {{Text: "Page2"}},
	}
	got := FullTextFromChars(chars)
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") || !strings.Contains(got, "Page2") {
		t.Errorf("FullTextFromChars = %q, missing expected content", got)
	}
	if FullTextFromChars(nil) != "" {
		t.Error("nil should return empty")
	}
}

func TestDetectEnglish(t *testing.T) {
	// Page with enough ASCII chars → English
	englishChars := make([]pdf.TextChar, 40)
	for i := range englishChars {
		englishChars[i] = pdf.TextChar{Text: "A"}
	}
	nonEnglishChars := []pdf.TextChar{{Text: "你好世界你好世界你好世界你好世界"}}

	t.Run("all pages English", func(t *testing.T) {
		chars := map[int][]pdf.TextChar{0: englishChars, 1: englishChars}
		if !DetectEnglish(chars, 2, nil) {
			t.Error("all-English pages should detect English")
		}
	})
	t.Run("majority English with non-English page", func(t *testing.T) {
		chars := map[int][]pdf.TextChar{0: englishChars, 1: nonEnglishChars}
		// totalPages=3: 1 English page out of 3 → not majority
		if DetectEnglish(chars, 3, nil) {
			t.Error("1/3 English pages should NOT detect English")
		}
	})
	t.Run("custom sampler", func(t *testing.T) {
		sampler := func(chars []pdf.TextChar, n int) string {
			return strings.Repeat("A", 40)
		}
		chars := map[int][]pdf.TextChar{0: {{Text: "x"}}}
		if !DetectEnglish(chars, 1, sampler) {
			t.Error("custom sampler returning 40 ASCII chars should detect English")
		}
	})
	t.Run("empty pages", func(t *testing.T) {
		if DetectEnglish(nil, 1, nil) {
			t.Error("nil pageChars should return false")
		}
	})
}
