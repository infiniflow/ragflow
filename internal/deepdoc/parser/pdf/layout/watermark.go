package layout

import (
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// watermarkFilter suppresses tiled watermark glyphs that template PDFs plant in
// the text layer (e.g. the Chinese resume-portal watermark reported in
// #18145). The glyphs are rotated diagonally, so GroupCharsToLines sees each
// glyph as its own line and LineToTextBox emits them as single-character
// boxes interleaved with the real content. Downstream `naive.py::chunk()`
// then copies every contaminated line into a chunk, and the extractor ships
// the garbage to the LLM.
//
// The watermark string is a short, mixed-case alphanumeric token (often with
// underscores) that repeats verbatim across the page — a 66-char random
// string would never OCR identically 6 times. Candidate selection is
// permissive (it only needs to look plausible); promotion to a watermark
// requires the candidate to appear 3+ times on the page, which is the
// actual signal that the PDF is templating it.
type watermarkFilter struct {
	promoted map[string]struct{}
}

const watermarkMinOccurrences = 3

// looksLikeWatermarkCandidate returns true for short, no-space tokens that
// mix letters and digits. The heuristic is intentionally lossy — promotion
// (below) is what actually decides. We deliberately exclude pure-letter or
// pure-digit runs so we don't suppress legitimate short words or ISBNs, and
// we exclude tokens with whitespace because watermark strings never have
// spaces.
func looksLikeWatermarkCandidate(text string) bool {
	if len(text) < 2 || len(text) > 64 {
		return false
	}
	hasLetter, hasDigit := false, false
	hasSpace := false
	for _, r := range text {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			hasSpace = true
		}
	}
	if hasSpace {
		return false
	}
	// Watermark tokens mix letters and digits (e.g. `ce1a60`, `Q2qRhNU_P`,
	// `4326dc`). Pure-letter or pure-digit strings are too ambiguous.
	return hasLetter && hasDigit
}

// newWatermarkFilter pre-scans the page chars for repeated candidate tokens
// and returns a filter that flags any further occurrence of the same string.
func newWatermarkFilter(chars []pdf.TextChar) *watermarkFilter {
	counts := make(map[string]int)
	for _, c := range chars {
		t := strings.TrimSpace(c.Text)
		if t == "" {
			continue
		}
		if looksLikeWatermarkCandidate(t) {
			counts[t]++
		}
	}
	promoted := make(map[string]struct{})
	for token, n := range counts {
		if n >= watermarkMinOccurrences {
			promoted[token] = struct{}{}
		}
	}
	return &watermarkFilter{promoted: promoted}
}

// shouldDrop reports whether a character belongs to a promoted watermark
// token and should be removed from the layout.
func (f *watermarkFilter) shouldDrop(c pdf.TextChar) bool {
	if f == nil || len(f.promoted) == 0 {
		return false
	}
	t := strings.TrimSpace(c.Text)
	if t == "" {
		return false
	}
	_, ok := f.promoted[t]
	return ok
}

// filterWatermarkChars returns a copy of chars with watermark glyphs
// removed. The original slice is left intact for callers that still need
// the unfiltered data (e.g. layout-aware consumers, image annotations).
func filterWatermarkChars(chars []pdf.TextChar) []pdf.TextChar {
	if len(chars) == 0 {
		return chars
	}
	f := newWatermarkFilter(chars)
	// Quick path: nothing promoted → caller can skip the alloc.
	if len(f.promoted) == 0 {
		return chars
	}
	out := make([]pdf.TextChar, 0, len(chars))
	for _, c := range chars {
		if !f.shouldDrop(c) {
			out = append(out, c)
		}
	}
	return out
}
