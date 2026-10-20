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
	// All ASCII, so each span of four gives its first character.
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

func TestDetectEnglishPageAlternatingDoesNotAlias(t *testing.T) {
	// 200 glyphs repeating a short mix. A fixed stride of len/n == 2 samples
	// only A from A/你, and a cycling span offset samples only A from A你你A.
	// Neither page has a 30-character ASCII run, so the vote must stay false.
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
			if DetectEnglishPage(chars, nil) {
				t.Fatal("DetectEnglishPage = true, want false")
			}
		})
	}
}

func TestDetectEnglishPageNeedsRealRun(t *testing.T) {
	// One CJK glyph per period of 2-12 at every phase, across page lengths
	// that give span widths of one to five. No page has a 30-character ASCII
	// run, so no alignment of glyphs and spans may vote English.
	for period := 2; period <= 12; period++ {
		for phase := range period {
			for pageLen := 101; pageLen <= 513; pageLen++ {
				chars := make([]pdf.TextChar, pageLen)
				for i := range chars {
					chars[i].Text = "A"
					if i%period == phase {
						chars[i].Text = "你"
					}
				}
				if DetectEnglishPage(chars, nil) {
					t.Fatalf("period %d, phase %d, length %d: DetectEnglishPage = true, want false", period, phase, pageLen)
				}
			}
		}
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
	// A curly quote every 12 characters, about 8% of the page, so letters
	// between quotes run only 11 long. One clean 40-character stretch is the
	// page's only real 30-character run. Every span is still mostly ASCII,
	// and the page has to stay English.
	const pageLen = 1000
	chars := make([]pdf.TextChar, pageLen)
	placed := 0
	for i := range chars {
		chars[i].Text = "e"
		if i%12 == 0 && (i < 400 || i >= 440) {
			chars[i].Text = "“"
			placed++
		}
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

func TestDetectEnglishPagePhaseAlignedPunctuation(t *testing.T) {
	// Quotes share the first character of a span and are denser than one in
	// twenty. A raw page-order sample is ten copies of a quote and nine
	// letters, so its longest ASCII run stays under 30. Each page still has
	// real runs of 30 ASCII characters, and the vote has to stay English.
	cases := []struct {
		name string
		fill func([]pdf.TextChar)
	}{
		{
			name: "quote clusters",
			// 92% ASCII: four curly quotes, then 46 letters.
			fill: func(chars []pdf.TextChar) {
				for i := range chars {
					chars[i].Text = "e"
					if i%50 < 4 {
						chars[i].Text = "“"
					}
				}
			},
		},
		{
			name: "quote clusters fill half a span",
			// 90% ASCII: five curly quotes, then 45 letters. Each affected
			// span of ten is exactly half quotes. The quotes sit between two
			// real runs and are shorter than each, so they read as text.
			fill: func(chars []pdf.TextChar) {
				for i := range chars {
					chars[i].Text = "e"
					if i%50 < 5 {
						chars[i].Text = "“"
					}
				}
			},
		},
		{
			name: "quotes then ascii",
			// A curly quote every 10 characters through index 790, then
			// ASCII. Sampled quotes sit at positions 0, 10, ..., 70.
			fill: func(chars []pdf.TextChar) {
				for i := range chars {
					chars[i].Text = "e"
					if i <= 790 && i%10 == 0 {
						chars[i].Text = "“"
					}
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chars := make([]pdf.TextChar, 1000)
			tc.fill(chars)
			non := 0
			for i := range chars {
				if breaksASCIIRun(chars[i].Text) {
					non++
				}
			}
			if non*20 <= len(chars) {
				t.Fatalf("non-ASCII = %d/%d, want denser than 1 in 20", non, len(chars))
			}
			if !asciiRunAtLeast(chars, 0, len(chars)-1, 30) {
				t.Fatal("fixture has no 30-character ASCII run")
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
		})
	}
}

func TestDetectEnglishPageQuoteClustersIgnorePhaseAndLength(t *testing.T) {
	// k curly quotes every 50 characters, shifted through every phase, on
	// pages of at least three periods. Span width changes with page length,
	// so neither where the clusters fall nor how long the page is may change
	// the vote. Up to 20 quotes leave real runs of 30 that outlast each
	// cluster, so the page is English; beyond that no real run is left.
	for _, pageLen := range []int{150, 200, 333, 500, 777, 1000, 1500, 2000, 4000} {
		for k := 1; k <= 25; k++ {
			want := k <= 20
			for phase := range 50 {
				chars := make([]pdf.TextChar, pageLen)
				for i := range chars {
					chars[i].Text = "e"
					if (i+phase)%50 < k {
						chars[i].Text = "“"
					}
				}
				if got := DetectEnglishPage(chars, nil); got != want {
					t.Fatalf("length %d, %d quotes, phase %d: DetectEnglishPage = %v, want %v", pageLen, k, phase, got, want)
				}
			}
		}
	}
}

func TestDetectEnglishPageShortEnglishBlockOnCJK(t *testing.T) {
	// A 30-letter heading is the page's only real 30-character run. Letters
	// after it are isolated, so those spans are not English text and the
	// heading alone must not outvote the rest of the page.
	cases := []struct {
		name      string
		wantASCII int
		letter    func(i int) bool
	}{
		{
			// One letter every 10 characters, CJK elsewhere: 127 ASCII out
			// of 1000, about 87% CJK.
			name:      "periodic letters",
			wantASCII: 127,
			letter:    func(i int) bool { return i < 30 || i%10 == 5 },
		},
		{
			// Alternating letter and CJK: each later span is exactly half
			// ASCII.
			name:      "alternating",
			wantASCII: 515,
			letter:    func(i int) bool { return i < 30 || i%2 == 0 },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chars := make([]pdf.TextChar, 1000)
			ascii := 0
			for i := range chars {
				chars[i].Text = "你"
				if tc.letter(i) {
					chars[i].Text = "e"
					ascii++
				}
			}
			if ascii != tc.wantASCII {
				t.Fatalf("ASCII characters = %d, want %d", ascii, tc.wantASCII)
			}
			if !asciiRunAtLeast(chars, 0, len(chars)-1, 30) {
				t.Fatal("fixture has no 30-character ASCII run")
			}
			if DetectEnglishPage(chars, nil) {
				t.Fatalf("DetectEnglishPage = true, want false; sample %q", DefaultSampleChars(chars, 100))
			}
			if DetectEnglishPage(chars, nil) {
				t.Fatal("DetectEnglishPage flipped to true")
			}
		})
	}
}

// asciiRunAtLeast reports whether chars[lo] through chars[hi] contain n
// consecutive ASCII-printable runes.
func asciiRunAtLeast(chars []pdf.TextChar, lo, hi, n int) bool {
	run := 0
	for j := lo; j <= hi; j++ {
		for _, r := range chars[j].Text {
			if !IsASCIIPrintable(r) {
				run = 0
				continue
			}
			run++
			if run >= n {
				return true
			}
		}
	}
	return false
}

func TestFullTextFromChars(t *testing.T) {
	chars := map[int][]pdf.TextChar{
		0: {{Text: "Hello"}, {Text: " "}, {Text: "World"}},
		1: {{Text: "Page2"}},
	}
	const want = "Hello WorldPage2"
	if got := FullTextFromChars(chars); got != want {
		t.Errorf("FullTextFromChars = %q, want %q", got, want)
	}
	if FullTextFromChars(nil) != "" {
		t.Error("nil should return empty")
	}

	// Keys are inserted out of page order. Concatenation follows ascending
	// page number, and a second call returns the same text.
	scrambled := map[int][]pdf.TextChar{
		5: {{Text: "c"}},
		1: {{Text: "a"}},
		3: {{Text: "b"}},
	}
	for range 30 {
		if got := FullTextFromChars(scrambled); got != "abc" {
			t.Fatalf("FullTextFromChars = %q, want abc", got)
		}
	}
}

func TestDetectEnglishPageContiguousThreshold(t *testing.T) {
	// These pages vote English. Python's random.choices draws character
	// texts independently, so the same ASCII share almost never produces 30
	// ASCII characters in a row.
	t.Run("30 percent block on CJK", func(t *testing.T) {
		for _, n := range []int{1000, 2000, 4000, 8000} {
			for _, atEnd := range []bool{false, true} {
				block := n * 30 / 100
				lo := 0
				if atEnd {
					lo = n - block
				}
				chars := cjkPage(n)
				for i := lo; i < lo+block; i++ {
					chars[i].Text = "e"
				}
				if !DetectEnglishPage(chars, nil) || !DetectEnglishPage(chars, nil) {
					t.Fatalf("n=%d atEnd=%v: DetectEnglishPage = false, want true", n, atEnd)
				}
			}
		}
	})

	t.Run("50/50 bilingual", func(t *testing.T) {
		for _, n := range []int{1000, 4000, 8000} {
			half := n / 2
			for _, englishFirst := range []bool{true, false} {
				chars := cjkPage(n)
				for i := range chars {
					if (i < half) == englishFirst {
						chars[i].Text = "e"
					}
				}
				if !DetectEnglishPage(chars, nil) {
					t.Fatalf("n=%d englishFirst=%v: DetectEnglishPage = false, want true", n, englishFirst)
				}
			}
		}
	})

	t.Run("numeric table", func(t *testing.T) {
		// 60% digits in one block, the rest CJK labels.
		for _, n := range []int{1000, 4000, 8000} {
			block := n * 60 / 100
			for _, lo := range []int{0, n - block} {
				chars := cjkPage(n)
				digits := 0
				for i := lo; i < lo+block; i++ {
					chars[i].Text = string(rune('0' + i%10))
					digits++
				}
				if digits*100/n != 60 {
					t.Fatalf("digits = %d/%d, want 60%%", digits, n)
				}
				if !DetectEnglishPage(chars, nil) {
					t.Fatalf("n=%d lo=%d: DetectEnglishPage = false, want true", n, lo)
				}
			}
		}
	})

	t.Run("interleaved with one run", func(t *testing.T) {
		// Three ASCII characters and two CJK, so the page is 60% ASCII and
		// the longest run is 3. One 30-character run then makes each span
		// count as English text.
		for _, n := range []int{1000, 8000} {
			chars := make([]pdf.TextChar, n)
			ascii := 0
			for i := range chars {
				if i%5 < 3 {
					chars[i].Text = "e"
					ascii++
				} else {
					chars[i].Text = "你"
				}
			}
			if ascii*5 != n*3 {
				t.Fatalf("ASCII = %d/%d, want 60%%", ascii, n)
			}
			if DetectEnglishPage(chars, nil) {
				t.Fatal("interleaved page without a 30-character run voted English")
			}
			start := n / 5 * 2
			for i := start; i < start+30; i++ {
				if chars[i].Text != "e" {
					chars[i].Text = "e"
					ascii++
				}
			}
			if ascii*100/n > 62 {
				t.Fatalf("ASCII = %d/%d, want about 60%%", ascii, n)
			}
			if !asciiRunAtLeast(chars, start, start+29, 30) {
				t.Fatal("fixture has no 30-character ASCII run")
			}
			if !DetectEnglishPage(chars, nil) {
				t.Fatalf("n=%d: DetectEnglishPage = false, want true; sample %q", n, DefaultSampleChars(chars, 100))
			}
		}
	})

	t.Run("29 and 30 percent boundary", func(t *testing.T) {
		// Pages of 1000 to 8000 characters. 29% of the page never covers 30
		// spans. 31% does, at every offset. 30% does when the block starts
		// on a span boundary, and a 1000-character block of 300 characters
		// at offset 5 covers only 29.
		for _, n := range []int{1000, 1001, 2000, 3456, 4000, 8000} {
			if offset, bad := slidingBlockDisagrees(n, n*29/100, false); bad {
				t.Fatalf("29%% n=%d offset=%d: DetectEnglishPage = true, want false", n, offset)
			}
			if offset, bad := slidingBlockDisagrees(n, n*31/100, true); bad {
				t.Fatalf("31%% n=%d offset=%d: DetectEnglishPage = false, want true", n, offset)
			}
			chars := cjkPage(n)
			block := n * 30 / 100
			for i := range block {
				chars[i].Text = "e"
			}
			if !DetectEnglishPage(chars, nil) {
				t.Fatalf("30%% n=%d offset=0: DetectEnglishPage = false, want true", n)
			}
		}
		chars := cjkPage(1000)
		for i := 5; i < 305; i++ {
			chars[i].Text = "e"
		}
		if DetectEnglishPage(chars, nil) {
			t.Fatal("30% of 1000 characters at offset 5: DetectEnglishPage = true, want false")
		}
	})
}

// cjkPage returns n CJK characters.
func cjkPage(n int) []pdf.TextChar {
	chars := make([]pdf.TextChar, n)
	for i := range chars {
		chars[i].Text = "你"
	}
	return chars
}

// slidingBlockDisagrees slides one ASCII block across a CJK page and reports
// the first offset whose English vote is not want.
func slidingBlockDisagrees(n, block int, want bool) (int, bool) {
	chars := cjkPage(n)
	for i := range block {
		chars[i].Text = "e"
	}
	for offset := 0; ; offset++ {
		if DetectEnglishPage(chars, nil) != want {
			return offset, true
		}
		if offset == n-block {
			return 0, false
		}
		chars[offset].Text = "你"
		chars[offset+block].Text = "e"
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
