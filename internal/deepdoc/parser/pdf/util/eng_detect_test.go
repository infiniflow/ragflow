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
	// Span offsets cycle: indexes 0,5,10,15,16,21,26,31,32,37.
	const want = "afkpqvafgl"
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

func TestDetectEnglishPageAlternatingDoesNotAlias(t *testing.T) {
	// 200 glyphs repeating a short mix. A fixed stride of len/n == 2 samples
	// only A from A/你. The cycling span offset samples only A from A你你A
	// (indexes 0, 3, 4, 7, ...). Neither page has a 30-character ASCII run,
	// so the vote must stay false.
	cycles := []struct {
		name  string
		cycle []string
	}{
		{name: "alternating", cycle: []string{"A", "你"}},
		{name: "period four", cycle: []string{"A", "你", "你", "A"}},
	}
	for _, tc := range cycles {
		t.Run(tc.name, func(t *testing.T) {
			chars := make([]pdf.TextChar, 200)
			for i := range chars {
				chars[i] = pdf.TextChar{Text: tc.cycle[i%len(tc.cycle)]}
			}
			sample := DefaultSampleChars(chars, 100)
			allA := true
			for _, r := range sample {
				if r != 'A' {
					allA = false
					break
				}
			}
			if allA {
				t.Fatalf("DefaultSampleChars = %q, want a sample that is not all A", sample)
			}
			if !strings.Contains(sample, "A") || !strings.Contains(sample, "你") {
				t.Fatalf("DefaultSampleChars = %q, want both A and 你", sample)
			}
			if DetectEnglishPage(chars, nil) {
				t.Fatal("DetectEnglishPage = true, want false")
			}
		})
	}
}

func TestDetectEnglishPageLongEnglish(t *testing.T) {
	// Longer than the 100-character sample, so detection takes the span path.
	// A letter, with a curly quote or em dash every 67th character (about 30
	// per 1000). Those marks are not ASCII-printable, and the page is still
	// English: the sample has to keep a 30-character letter run.
	const pageLen = 4000
	chars := make([]pdf.TextChar, pageLen)
	for i := range chars {
		switch (i + 1) % 67 {
		case 0:
			chars[i] = pdf.TextChar{Text: "—"}
		case 34:
			chars[i] = pdf.TextChar{Text: "“"}
		default:
			chars[i] = pdf.TextChar{Text: "e"}
		}
	}
	if !DetectEnglishPage(chars, nil) {
		t.Fatal("DetectEnglishPage = false, want true")
	}
	again := DetectEnglishPage(chars, nil)
	if !again {
		t.Fatal("DetectEnglishPage flipped to false")
	}
}

func TestReviewLongEnglishOnePercentQuotes(t *testing.T) {
	// 99% ASCII, one curly quote every 100 characters, including index 0.
	// With 100 samples the span offset is i%10, so those quotes land on
	// every tenth sample and used to break the run before it reached 30.
	chars := make([]pdf.TextChar, 1000)
	for i := range chars {
		chars[i].Text = "e"
		if i%100 == 0 {
			chars[i].Text = "“"
		}
	}
	sample := DefaultSampleChars(chars, 100)
	run := 0
	for _, r := range sample {
		if IsASCIIPrintable(r) {
			run++
			if run >= 30 {
				break
			}
			continue
		}
		run = 0
	}
	if run < 30 {
		t.Fatalf("DefaultSampleChars = %q, want a 30-character ASCII run", sample)
	}
	if !DetectEnglishPage(chars, nil) {
		t.Fatal("DetectEnglishPage = false, want true")
	}
	if !DetectEnglishPage(chars, nil) {
		t.Fatal("DetectEnglishPage flipped to false")
	}
}

func TestDetectEnglishPageSixToTenPercentPunctuation(t *testing.T) {
	// Punctuation is 8% of the page and sits off the sampled indexes, with a
	// real 40-character ASCII run left intact. The sample already contains
	// 30 letters; the dense-stretch repair must not punch that run out.
	const pageLen = 1000
	const samples = 100
	chars := make([]pdf.TextChar, pageLen)
	for i := range chars {
		chars[i].Text = "e"
	}
	sampled := make([]bool, pageLen)
	for i := range samples {
		start := i * pageLen / samples
		width := (i+1)*pageLen/samples - start
		sampled[start+i%width] = true
	}
	placed := 0
	for i := range chars {
		if sampled[i] || (i >= 400 && i < 440) {
			continue
		}
		if placed == 80 {
			break
		}
		chars[i].Text = "“"
		placed++
	}
	if placed*100 < 6*pageLen || placed*100 > 10*pageLen {
		t.Fatalf("non-ASCII punctuation = %d/%d, want 6-10%%", placed, pageLen)
	}
	if !asciiRunAtLeast(chars, 0, pageLen-1, 30) {
		t.Fatal("fixture has no 30-character ASCII run")
	}
	if !DetectEnglishPage(chars, nil) {
		t.Fatal("DetectEnglishPage = false, want true")
	}
	if !DetectEnglishPage(chars, nil) {
		t.Fatal("DetectEnglishPage flipped to false")
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
