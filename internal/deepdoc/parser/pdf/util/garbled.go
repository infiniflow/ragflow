package util

import (
	"regexp"
	"strings"
	"unicode"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// CIDPattern matches pdfminer's CID placeholder like "(cid:123)".
//
// Python: pdf_parser.py:198 _CID_PATTERN
var CIDPattern = regexp.MustCompile(`\(cid\s*:\s*\d+\s*\)`)

// subsetFontPattern matches PDF subset font prefixes like "ABCDEF+".
// PDF subset fonts use a 2-6 uppercase alphanumeric tag followed by '+'.
//
// Python: pdf_parser.py:261 _has_subset_font_prefix()
var subsetFontPattern = regexp.MustCompile(`^[A-Z0-9]{2,6}\+`)

// HasSubsetFontPrefix checks if a font name has a PDF subset prefix.
//
// Example:
//
//	HasSubsetFontPrefix("DY1+ZLQDm1-1") → true
//	HasSubsetFontPrefix("SimSun")        → false
//	HasSubsetFontPrefix("")              → false
//
// Python: pdf_parser.py:253 _has_subset_font_prefix()
func HasSubsetFontPrefix(fontname string) bool {
	if fontname == "" {
		return false
	}
	return subsetFontPattern.MatchString(fontname)
}

// IsGarbledChar checks if a single character is garbled (unmappable from PDF font encoding).
//
// A character is garbled if it falls into:
//   - Private Use Areas (PUA): U+E000-U+F8FF, U+F0000-U+FFFFF, U+100000-U+10FFFF
//   - Replacement character U+FFFD
//   - Control characters (except tab, newline, carriage return)
//   - C1 control range U+0080-U+009F
//   - Unicode categories "Cn" (unassigned) or "Cs" (surrogate)
//
// Python: pdf_parser.py:201 _is_garbled_char()
//
// Example:
//
//	IsGarbledChar("") → true  (PUA)
//	IsGarbledChar("A")       → false
//	IsGarbledChar("�")  → true  (replacement char)
//	IsGarbledChar("")        → false
func IsGarbledChar(ch string) bool {
	if ch == "" {
		return false
	}
	// Always use the actual rune value (handles multi-byte UTF-8 correctly)
	runes := []rune(ch)
	cp := int(runes[0])

	// Private Use Area
	if (cp >= 0xE000 && cp <= 0xF8FF) ||
		(cp >= 0xF0000 && cp <= 0xFFFFF) ||
		(cp >= 0x100000 && cp <= 0x10FFFF) {
		return true
	}
	// Replacement character
	if cp == 0xFFFD {
		return true
	}
	// Control characters (except \t \n \r)
	if cp < 0x20 && ch != "\t" && ch != "\n" && ch != "\r" {
		return true
	}
	// C1 control range
	if cp >= 0x80 && cp <= 0x9F {
		return true
	}

	// Check Unicode category for each rune
	for _, r := range ch {
		cat := catOf(rune(r))
		if cat == "Cn" || cat == "Cs" {
			return true
		}
	}
	return false
}

// IsGarbledText checks if a text string contains too many garbled characters.
// Also detects CID placeholder patterns like "(cid:123)".
//
// Python: pdf_parser.py:229 _is_garbled_text()
//
// Example:
//
//	IsGarbledText("正常文本", 0.5)     → false
//	IsGarbledText("", 0.5) → true
//	IsGarbledText("(cid:123)", 0.5)   → true
//	IsGarbledText("", 0.5)             → false
func IsGarbledText(text string, threshold float64) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if CIDPattern.MatchString(trimmed) {
		return true
	}

	garbledCount := 0
	total := 0
	for _, r := range trimmed {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if IsGarbledChar(string(r)) {
			garbledCount++
		}
	}
	if total == 0 {
		return false
	}
	return float64(garbledCount)/float64(total) >= threshold
}

// IsGarbledByFontEncoding detects if a page's text is garbled due to
// broken font encoding mappings.
//
// Detection: if ≥30% of characters come from subset fonts AND
// <5% are CJK/Hangul/Kana AND >40% are ASCII punctuation/symbols,
// the page is likely garbled.
//
// Python: pdf_parser.py:264 _is_garbled_by_font_encoding()
//
// Example:
//
//	chars := []pdf.TextChar{
//	  {Text: "!", FontName: "DY1+SimSun"},
//	  {Text: "#", FontName: "DY1+SimSun"},
//	  // ... mostly ASCII punctuation with subset font prefix
//	}
//	IsGarbledByFontEncoding(chars, 20) → true  // OCR needed!
func IsGarbledByFontEncoding(chars []pdf.TextChar, minChars int) bool {
	if len(chars) < minChars {
		return false
	}

	subsetFontCount := 0
	totalNonSpace := 0
	asciiPunctSym := 0
	cjkLike := 0

	for _, c := range chars {
		text := strings.TrimSpace(c.Text)
		if text == "" {
			continue
		}
		totalNonSpace++

		if HasSubsetFontPrefix(c.FontName) {
			subsetFontCount++
		}

		// Always use the rune value
		runes := []rune(text)
		cp := int(runes[0])

		// CJK Unified Ideographs, CJK Compatibility, CJK Extension B
		// Hangul syllables, Hiragana, Katakana
		// Fullwidth forms (U+FF00-U+FF5E): legitimate CJK typographic characters
		if (cp >= 0x2E80 && cp <= 0x9FFF) ||
			(cp >= 0xF900 && cp <= 0xFAFF) ||
			(cp >= 0x20000 && cp <= 0x2FA1F) ||
			(cp >= 0xAC00 && cp <= 0xD7AF) ||
			(cp >= 0x3040 && cp <= 0x30FF) ||
			(cp >= 0xFF00 && cp <= 0xFF5E) {
			cjkLike++
		} else if (cp >= 0x21 && cp <= 0x2F) || // !"#$%&'()*+,-./
			(cp >= 0x3A && cp <= 0x40) || // :;<=>?@
			(cp >= 0x5B && cp <= 0x60) || // [\]^_`
			(cp >= 0x7B && cp <= 0x7E) { // {|}~
			asciiPunctSym++
		}
	}

	if totalNonSpace < minChars {
		return false
	}

	subsetRatio := float64(subsetFontCount) / float64(totalNonSpace)
	if subsetRatio < 0.3 {
		return false
	}

	cjkRatio := float64(cjkLike) / float64(totalNonSpace)
	punctRatio := float64(asciiPunctSym) / float64(totalNonSpace)

	return cjkRatio < 0.05 && punctRatio > 0.4
}

// catOf returns "Cs" for surrogates, "Cn" for unassigned code points
// (not in any Unicode category), and "" for everything else.
// Python unicodedata.category() returns "Cc" for control chars, "Cn" only
// for truly unassigned — we match that behavior.
func catOf(r rune) string {
	if r >= 0xD800 && r <= 0xDFFF {
		return "Cs" // surrogate
	}
	// C1 controls (0x80-0x9F): Python returns "Cc", not "Cn".
	if r >= 0x80 && r <= 0x9F {
		return ""
	}
	// A rune is unassigned (Cn) if it's NOT in any recognized category.
	// Python unicodedata.category() returns "Cc" for control chars,
	// "Cn" only for truly unassigned. We match that behavior.
	if !unicode.IsPrint(r) &&
		!unicode.IsSpace(r) &&
		!unicode.IsControl(r) &&
		!unicode.Is(unicode.Cf, r) &&
		!unicode.Is(unicode.Co, r) &&
		r > 0x20 {
		return "Cn"
	}
	return ""
}

// IsGarbledPage returns true if a page is garbled by PUA ratio, font encoding,
// pdf_oxide unmapped glyphs, or scan noise (no real words).
func IsGarbledPage(chars []pdf.TextChar) bool {
	if len(chars) < 20 {
		return false
	}
	// Build full-page text for detection (all O(n) single pass).
	var fullText strings.Builder
	for _, c := range chars {
		fullText.WriteString(c.Text)
	}
	text := fullText.String()
	if IsGarbledText(text, 0.3) {
		return true
	}
	if PdfOxideUnmappedGarbled(text) && IsScanNoise(text) {
		return true
	}
	if IsGarbledByFontEncoding(chars, 20) {
		return true
	}
	if IsScanNoise(text) {
		return true
	}
	return false
}

// IsScanNoise detects scanned pages where pdf_oxide extracts noise glyphs
// instead of real text.  Real text in any language contains word-like runs
// of consecutive letters (L category).  Scan noise consists of random ASCII
// symbols with at most 2-letter fragments.
//
// Three indicators of real (non-noise) text, any one is sufficient:
//   - ≥4 consecutive lowercase Latin letters (e.g. "the", "and")
//   - ≥2 consecutive CJK characters (Han, Hiragana, Katakana, Hangul)
//   - ≥4 consecutive non-ASCII letters (Arabic, Thai, Cyrillic, etc.)
//
// Pure-uppercase fragments like "RASB" are common in pdf_oxide noise but
// never appear as standalone words in real text without lowercase context.
func IsScanNoise(text string) bool {
	nonSpace := 0
	digitCount := 0
	lowerRun := 0
	maxLowerRun := 0
	cjkRun := 0
	maxCJKRun := 0
	nonASCIILetterRun := 0
	maxNonASCIILetterRun := 0

	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			lowerRun = 0
			cjkRun = 0
			nonASCIILetterRun = 0
			continue
		}
		nonSpace++

		// Digit density: real content (tables, dates) has digits;
		// pdf_oxide noise (unmapped glyphs) never produces digits.
		if r >= '0' && r <= '9' {
			digitCount++
		}

		// Lowercase Latin (Ll)
		if unicode.Is(unicode.Ll, r) {
			lowerRun++
			if lowerRun > maxLowerRun {
				maxLowerRun = lowerRun
			}
		} else {
			lowerRun = 0
		}

		// CJK: Han, Hiragana, Katakana, Hangul Syllables & Jamo
		if pdf.IsCJK(r) {
			cjkRun++
			if cjkRun > maxCJKRun {
				maxCJKRun = cjkRun
			}
		} else {
			cjkRun = 0
		}

		// Non-ASCII letter (Arabic U+0600–U+06FF, Thai U+0E00–U+0E7F,
		// Cyrillic U+0400–U+04FF, etc.).  Excludes ASCII so uppercase
		// Latin fragments like "RASB" don't count.
		if unicode.IsLetter(r) && r > unicode.MaxASCII {
			nonASCIILetterRun++
			if nonASCIILetterRun > maxNonASCIILetterRun {
				maxNonASCIILetterRun = nonASCIILetterRun
			}
		} else {
			nonASCIILetterRun = 0
		}
	}

	// Need enough characters to make a meaningful decision.
	if nonSpace < 30 {
		return false
	}

	// Digit density: pdf_oxide never substitutes digits for unmapped
	// glyphs. Real content (tables, dates, page numbers) has ≥10%
	// digits; noise consists of random ASCII punctuation.
	if float64(digitCount)/float64(nonSpace) >= 0.10 {
		return false
	}

	// Real text in any script — any one indicator is sufficient.
	isNoise := maxLowerRun < 4 && maxCJKRun < 2 && maxNonASCIILetterRun < 4

	return isNoise
}

// isCJK reports whether r is a CJK character: Han ideograph, Hiragana,
// Katakana, Hangul syllable, or Hangul Jamo.

// PdfOxideUnmappedGarbled detects pdf_oxide's '#' placeholder glyphs.
// pdf_oxide uses '#' (U+0023) for every glyph it cannot map; consecutive
// unmapped glyphs form "##", "###", "####" sequences.  Three or more
// consecutive '#' is virtually impossible in normal text.
//
// Two conditions (either is sufficient):
//   - ≥ 2 occurrences of "###" (3+ consecutive #)
//   - # density ≥ 5% of non-space characters
func PdfOxideUnmappedGarbled(text string) bool {
	hashCount := 0
	total := 0
	consecutive := 0
	tripleClusters := 0

	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		total++
		if r == '#' {
			hashCount++
			consecutive++
			if consecutive == 3 {
				tripleClusters++
			}
		} else {
			consecutive = 0
		}
	}

	if total == 0 {
		return false
	}

	density := float64(hashCount) / float64(total)

	if tripleClusters >= 1 {
		return true
	}
	// Density check only meaningful with enough chars (matches isGarbledPage's
	// min 20 char guard).  In production the sample is 200 chars.
	if total >= 40 && density >= 0.03 {
		return true
	}
	return false
}

// ocrDetectAndRecognize runs OCR detection + recognition and returns
// recognized pdf.TextBox results. logLabel distinguishes callers in log output
// ("scan page", "garbled page").
