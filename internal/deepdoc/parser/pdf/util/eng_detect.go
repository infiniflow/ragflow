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

// DefaultSampleChars returns up to n character texts, concatenated. The page
// is split into that many equal spans in page order, and each span gives one
// character. A span is English when the page has a run of 30 consecutive
// ASCII characters and more than half of the span is English text: ASCII
// characters, or a gap shorter than one span between two such runs. An
// English span gives its first ASCII character; any other span gives its
// first non-ASCII character when it has one.
//
// A page with no real 30-character run therefore never samples one, and a
// short English block cannot make a mostly CJK page English. Punctuation
// shorter than a span inside English text never breaks the sample, wherever
// it falls against the spans. The same page always yields the same sample.
func DefaultSampleChars(chars []pdf.TextChar, n int) string {
	if n <= 0 || len(chars) == 0 {
		return ""
	}
	m := min(n, len(chars))
	text := make([]bool, len(chars))
	pageHasRun := false
	run, runStart, lastRunEnd := 0, 0, -1
	for i, c := range chars {
		text[i] = !breaksASCIIRun(c.Text)
		for _, r := range c.Text {
			if !IsASCIIPrintable(r) {
				run = 0
				continue
			}
			if run == 0 {
				runStart = i
			}
			run++
			if run < 30 {
				continue
			}
			// The bound rounds down so a bridged gap never covers a whole
			// span, and every English span still has an ASCII character.
			if run == 30 && lastRunEnd >= 0 && runStart-lastRunEnd-1 < len(chars)/m {
				for j := lastRunEnd + 1; j < runStart; j++ {
					text[j] = true
				}
			}
			pageHasRun = true
			lastRunEnd = i
		}
	}

	var buf strings.Builder
	for i := range m {
		start := i * len(chars) / m
		end := (i + 1) * len(chars) / m
		english := false
		if pageHasRun {
			k := 0
			for j := start; j < end; j++ {
				if text[j] {
					k++
				}
			}
			english = k*2 > end-start
		}
		pick := start
		for j := start; j < end; j++ {
			if breaksASCIIRun(chars[j].Text) != english {
				pick = j
				break
			}
		}
		buf.WriteString(chars[pick].Text)
	}
	return buf.String()
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
