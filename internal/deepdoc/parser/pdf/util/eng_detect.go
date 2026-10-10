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
// periodic mix is not stuck on one phase. The same page always yields the
// same sample. Python's random.choices is still unseeded and jitters; do
// not chase that difference in the parity harness.
func DefaultSampleChars(chars []pdf.TextChar, n int) string {
	if n <= 0 || len(chars) == 0 {
		return ""
	}
	m := min(n, len(chars))
	var buf strings.Builder
	for i := range m {
		start := i * len(chars) / m
		width := (i+1)*len(chars)/m - start
		buf.WriteString(chars[start+i%width].Text)
	}
	return buf.String()
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
