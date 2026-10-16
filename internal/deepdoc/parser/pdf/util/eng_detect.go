package util

import (
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// IsASCIIPrintable returns true for characters that match Python's
// is_english regex: [ a-zA-Z0-9,/¸;:'\[\]\(\)!@#$%^&*\"?<>._-]
func IsASCIIPrintable(r rune) bool {
	if r == ' ' {
		return true
	}
	if r >= 'a' && r <= 'z' {
		return true
	}
	if r >= 'A' && r <= 'Z' {
		return true
	}
	if r >= '0' && r <= '9' {
		return true
	}
	// Additional ASCII symbols from the Python regex
	switch r {
	case ',', '/', '¸', ';', ':', '\'', '[', ']', '(', ')',
		'!', '@', '#', '$', '%', '^', '&', '*', '"', '?',
		'<', '>', '.', '_', '-':
		return true
	}
	return false
}

// DefaultSampleChars returns up to n character texts, concatenated.
// The page is split into that many equal spans, in page order, and one
// character is taken from each span. The offset inside a span cycles so a
// periodic mix is not stuck on one phase. Punctuation that lands on a
// sample point is moved to an ASCII character in that span when the covered
// stretch has a real run of 30 ASCII characters and the span itself is
// already mostly ASCII, so a quote that shares the sampling phase does not
// hide an English page. A span that is still mostly CJK keeps its sample, so
// a short English block does not reclassify the page. When the points are
// still ASCII across a stretch that is not mostly ASCII, and that stretch
// has no such run, non-ASCII characters from it are kept so a skipped mix
// is not read as an English run. The same page always yields the same sample.
func DefaultSampleChars(chars []pdf.TextChar, n int) string {
	if n <= 0 || len(chars) == 0 {
		return ""
	}
	m := min(n, len(chars))
	idx := make([]int, m)
	for i := range m {
		start := i * len(chars) / m
		width := (i+1)*len(chars)/m - start
		idx[i] = start + i%width
	}
	// A quote on the sampling phase breaks every sample run before a 30-long
	// ASCII run can form. When the stretch really contains 30 printable
	// characters, take a letter from the same span, but only if that span is
	// already mostly ASCII. A mostly CJK span keeps its sample: one letter
	// beside a short English block must not turn the page English. A page
	// with no 30-character run keeps the later repair.
	if asciiRunAtLeast(chars, idx[0], idx[m-1], 30) {
		for i := range m {
			if !breaksASCIIRun(chars[idx[i]].Text) {
				continue
			}
			start := i * len(chars) / m
			end := (i + 1) * len(chars) / m
			ascii := 0
			firstASCII := -1
			for j := start; j < end; j++ {
				if breaksASCIIRun(chars[j].Text) {
					continue
				}
				if firstASCII < 0 {
					firstASCII = j
				}
				ascii++
			}
			if ascii*2 <= end-start {
				continue
			}
			idx[i] = firstASCII
		}
	}
	// A run of ASCII sample points is trusted when the page stretch it covers
	// has at most one non-ASCII character per 20 (an English page with
	// typographic marks stays under that). A denser mix was skipped by the
	// stride; keep characters from it so the run stays under 30. Skip that
	// repair when the stretch already contains a real run of 30 ASCII
	// characters, so typographic punctuation at 6-10% does not hide it.
	for a := 0; a < m; {
		if breaksASCIIRun(chars[idx[a]].Text) {
			a++
			continue
		}
		b := a
		for b+1 < m && !breaksASCIIRun(chars[idx[b+1]].Text) {
			b++
		}
		if b-a+1 >= 30 {
			lo, hi := idx[a], idx[b]
			span := hi - lo + 1
			non := 0
			for j := lo; j <= hi; j++ {
				if breaksASCIIRun(chars[j].Text) {
					non++
				}
			}
			if non*20 > span && !asciiRunAtLeast(chars, lo, hi, 30) {
				next := lo
				for off := 29; off < b-a+1; off += 29 {
					for next <= hi && !breaksASCIIRun(chars[next].Text) {
						next++
					}
					if next > hi {
						break
					}
					idx[a+off] = next
					next++
				}
			}
		}
		a = b + 1
	}
	var buf strings.Builder
	for _, i := range idx {
		buf.WriteString(chars[i].Text)
	}
	return buf.String()
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

// breaksASCIIRun reports whether s contains a rune outside an English run.
func breaksASCIIRun(s string) bool {
	for _, r := range s {
		if !IsASCIIPrintable(r) {
			return true
		}
	}
	return false
}

// FullTextFromChars concatenates all chars text across pages for scan noise detection.
func FullTextFromChars(pageChars map[int][]pdf.TextChar) string {
	var sb strings.Builder
	for _, chars := range pageChars {
		for _, c := range chars {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}

// DetectEnglishPage reports whether one page contains a run of 30+
// consecutive ASCII-printable characters in a sample of up to 100
// extracted character texts.
func DetectEnglishPage(chars []pdf.TextChar, sample pdf.SampleFunc) bool {
	if len(chars) == 0 {
		return false
	}
	if sample == nil {
		sample = DefaultSampleChars
	}

	sampleText := sample(chars, 100)
	run := 0
	for _, r := range sampleText {
		if IsASCIIPrintable(r) {
			run++
			if run >= 30 {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

// DetectEnglish detects whether a PDF is primarily English by per-page
// majority vote, matching Python's is_english logic in __images__
// (pdf_parser.py:1519-1526).
//
// Each page votes via DetectEnglishPage. Returns true when a strict majority
// of pages vote yes. totalPages remains the denominator so image-only pages
// still dilute the majority, matching Python behavior.
func DetectEnglish(pageChars map[int][]pdf.TextChar, totalPages int, sample pdf.SampleFunc) bool {
	if totalPages == 0 || len(pageChars) == 0 {
		return false
	}
	pagesWithSeq := 0
	for _, chars := range pageChars {
		if DetectEnglishPage(chars, sample) {
			pagesWithSeq++
		}
	}
	return pagesWithSeq > totalPages/2
}
