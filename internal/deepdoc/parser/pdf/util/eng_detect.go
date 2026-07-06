package util

import (
	"math/rand/v2"
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

// DefaultSampleChars returns a random sample of up to n character texts,
// concatenated.  Matches Python's random.choices([c["text"] for c in
// page_chars], k=min(100, len(page_chars))).
func DefaultSampleChars(chars []pdf.TextChar, n int) string {
	if n <= 0 || len(chars) == 0 {
		return ""
	}
	m := min(n, len(chars))
	// Fisher-Yates shuffle on indices, then take first m.
	indices := make([]int, len(chars))
	for i := range indices {
		indices[i] = i
	}
	rand.Shuffle(len(indices), func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})
	var buf strings.Builder
	for i := 0; i < m; i++ {
		buf.WriteString(chars[indices[i]].Text)
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

// DetectEnglish detects whether a PDF is primarily English by per-page
// majority vote, matching Python's is_english logic in __images__
// (pdf_parser.py:1519-1526).
//
// Each page: sample up to 100 character texts via sampler, join into one
// string, check if there is a run of 30+ consecutive ASCII characters
// (letters, digits, spaces, punctuation).  Pages with such a run vote
// "English".  Returns true when a strict majority of pages vote yes.
//
// totalPages is the denominator (len(self.page_images) in Python), including
// image-only pages that have zero chars.  This matches Python's behavior
// where empty pages dilute the majority.
func DetectEnglish(pageChars map[int][]pdf.TextChar, totalPages int, sample pdf.SampleFunc) bool {
	if totalPages == 0 || len(pageChars) == 0 {
		return false
	}
	if sample == nil {
		sample = DefaultSampleChars
	}
	pagesWithSeq := 0

	for _, chars := range pageChars {
		if len(chars) == 0 {
			continue
		}
		sampleText := sample(chars, 100)
		run := 0
		for _, r := range sampleText {
			if IsASCIIPrintable(r) {
				run++
				if run >= 30 {
					pagesWithSeq++
					break
				}
			} else {
				run = 0
			}
		}
	}

	return pagesWithSeq > totalPages/2
}
