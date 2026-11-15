package util

import (
	"maps"
	"slices"
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
// characters, or a gap between two such runs that is shorter than each of
// them. An English span gives its first ASCII character; any other span
// gives its first non-ASCII character when it has one.
//
// A page with no real 30-character run therefore never samples one. The
// page votes English when the sample contains 30 ASCII characters in a
// row, so a contiguous English block flips that vote at about 30% of the
// page. On pages of 1000 to 8000 characters, a block of 29% stays within
// 29 spans at every offset. A block of 30% covers 30 spans when it starts
// on a span boundary, and can cover only 29 when it starts in the middle
// of a span (300 characters at offset 5 on a 1000-character page). A block
// of 31% covers 30 spans at every offset. A 30-character heading on a long,
// mostly CJK page still covers only a few spans, so it does not vote
// English.
//
// Python's random.choices draws character texts independently, with
// replacement, in an unseeded order. A reproducible sample is not that
// distribution: at an ASCII share of about 30 to 60 percent, random.choices
// almost never draws 30 ASCII characters in a row, even when those
// characters form one contiguous block. This sampler walks the page in
// order, so the block is a run in the sample. Whether punctuation inside
// English text is bridged depends only on the text around it, not on page
// length or where it falls against the spans. The same page always yields
// the same sample.
func DefaultSampleChars(chars []pdf.TextChar, n int) string {
	if n <= 0 || len(chars) == 0 {
		return ""
	}
	text := make([]bool, len(chars))
	var runs [][2]int
	run, runStart := 0, 0
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
			if run == 30 {
				runs = append(runs, [2]int{runStart, i})
			} else if run > 30 {
				runs[len(runs)-1][1] = i
			}
		}
	}
	for k := 1; k < len(runs); k++ {
		prev, next := runs[k-1], runs[k]
		gap := next[0] - prev[1] - 1
		if gap < prev[1]-prev[0]+1 && gap < next[1]-next[0]+1 {
			for j := prev[1] + 1; j < next[0]; j++ {
				text[j] = true
			}
		}
	}

	m := min(n, len(chars))
	var buf strings.Builder
	for i := range m {
		start := i * len(chars) / m
		end := (i + 1) * len(chars) / m
		english := false
		if len(runs) > 0 {
			k := 0
			for j := start; j < end; j++ {
				if text[j] {
					k++
				}
			}
			english = k*2 > end-start
		}
		// An English span that lies inside a bridged gap has no ASCII
		// character of its own; it gives the first one of the next run.
		limit := end
		if english {
			limit = len(chars)
		}
		pick := start
		for j := start; j < limit; j++ {
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

// FullTextFromChars concatenates character text in ascending page number.
// Map iteration order would otherwise change the string from call to call.
func FullTextFromChars(pageChars map[int][]pdf.TextChar) string {
	var sb strings.Builder
	for _, page := range slices.Sorted(maps.Keys(pageChars)) {
		for _, c := range pageChars[page] {
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
