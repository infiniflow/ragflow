package chunker

import (
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
)

var pdfListStart = regexp.MustCompile(`^(?:[（(]?[0-9一二三四五六七八九十百]+[）).．、]|[a-zA-Z]\)|[a-zA-Z]\.(?:\s|$)|[-*•]\s)`)

// joinPDFContinuations removes physical page breaks within continuing prose
// before delimiter splitting turns source regions into independent merge units.
// Each source keeps its own text range and page rectangle for later splitting.
func joinPDFContinuations(units []schema.ChunkDoc, sources [][]pdfSourceSpan) ([]schema.ChunkDoc, [][]pdfSourceSpan) {
	first := 1
	for first < len(units) && !canJoinPDFContinuation(units[first-1], units[first]) {
		first++
	}
	if first >= len(units) {
		return units, sources
	}
	if len(sources) != len(units) {
		sources = pdfUnitSources(units)
	}
	joined := append([]schema.ChunkDoc(nil), units[:first]...)
	joinedSources := append([][]pdfSourceSpan(nil), sources[:first]...)
	for i := first; i < len(units); i++ {
		unit := units[i]
		if !canJoinPDFContinuation(joined[len(joined)-1], unit) {
			joined = append(joined, unit)
			joinedSources = append(joinedSources, sources[i])
			continue
		}
		index := len(joined) - 1
		joined[index] = cloneChunkDoc(joined[index])
		previous := &joined[index]
		left := removeTag(normalizeGeneralNewlines(previous.Text))
		right := removeTag(normalizeGeneralNewlines(unit.Text))
		previous.Text = strings.TrimRightFunc(left, unicode.IsSpace)
		unit.Text = strings.TrimLeftFunc(right, unicode.IsSpace)
		leftSources := slicePDFSourceSpans(joinedSources[index], left, 0, len(previous.Text))
		rightSources := slicePDFSourceSpans(sources[i], right, len(right)-len(unit.Text), len(right))
		separator := pdfContinuationSeparator(previous.Text, unit.Text)
		offset := len(previous.Text) + len(separator)
		for j := range rightSources {
			rightSources[j].start += offset
			rightSources[j].end += offset
		}
		joinedSources[index] = append(leftSources, rightSources...)
		previous.Text += separator + unit.Text
		previous.PDFPositions = mergeGeneralPositions(previous.PDFPositions, unit.PDFPositions)
		previous.Positions = mergeGeneralPositions(previous.Positions, unit.Positions)
		mergeGeneralMetadata(previous, unit)
		previous.TKNums = nil // Delimiter splitting counts the resulting units.
	}
	return joined, joinedSources
}

func canJoinPDFContinuation(previous, next schema.ChunkDoc) bool {
	for _, unit := range []schema.ChunkDoc{previous, next} {
		if itemDocType(unit) != "text" || unit.PreserveOrder {
			return false
		}
		for _, layout := range []string{unit.Layout, unit.LayoutType} {
			if layout != "" && layout != "text" {
				return false
			}
		}
	}
	left := normalizeGeneralNewlines(removeTag(previous.Text))
	right := normalizeGeneralNewlines(removeTag(next.Text))
	// A blank line is an explicit paragraph break, including whitespace-only lines.
	leftEnd := len(strings.TrimRightFunc(left, unicode.IsSpace))
	rightStart := len(right) - len(strings.TrimLeftFunc(right, unicode.IsSpace))
	if strings.Count(left[leftEnd:], "\n") > 1 || strings.Count(right[:rightStart], "\n") > 1 {
		return false
	}
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" || right == "" || pdfListStart.MatchString(right) {
		return false
	}
	ending := strings.TrimRight(left, `"'”’）)]】》`)
	if ending == "" || strings.ContainsRune("。！？.!?；;：:", lastPDFRune(ending)) {
		return false
	}
	first, _ := utf8.DecodeRuneInString(right)
	if unicode.IsUpper(first) || (!unicode.IsLetter(first) && !strings.ContainsRune("，、,", first)) {
		return false
	}
	positions := func(unit schema.ChunkDoc) [][]float64 {
		if len(unit.PDFPositions) > 0 {
			return parser.PDFPositionRowsFromJSON(unit.PDFPositions)
		}
		return parser.PDFPositionRowsFromJSON(unit.Positions)
	}
	before, after := positions(previous), positions(next)
	if len(before) == 0 || len(after) == 0 {
		return false
	}
	a, b := before[len(before)-1], after[0]
	// A continuation starts higher on the next page than the previous page's
	// last region ended; a region at or below that point is unrelated.
	if b[0] != a[0]+1 || b[3] >= a[4] {
		return false
	}
	height := math.Min(a[4]-a[3], b[4]-b[3])
	// Regions may cover whole paragraphs: their height must not make a
	// first-line indent look like harmless alignment noise (PDF-point units).
	tolerance := math.Min(height*0.6, 12)
	return height > 0 && a[2] > a[1] && b[2] > b[1] && math.Abs(a[1]-b[1]) <= tolerance
}

func lastPDFRune(text string) rune {
	r, _ := utf8.DecodeLastRuneInString(text)
	return r
}

func pdfContinuationSeparator(left, right string) string {
	a := lastPDFRune(left)
	b, _ := utf8.DecodeRuneInString(right)
	if pdfNoSpaceRune(a) && pdfNoSpaceRune(b) || strings.ContainsRune("-\u2010\u2011", a) || unicode.IsPunct(b) {
		return ""
	}
	return " "
}

// pdfNoSpaceRune reports scripts that write words without separating spaces.
// A boundary keeps the space when only one side is in this set because the
// other side is space-delimited. The prolonged sound mark is Common script, so
// it is listed explicitly; Hangul is excluded because Korean separates words
// with spaces.
func pdfNoSpaceRune(r rune) bool {
	return r == '\u30fc' || unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}
