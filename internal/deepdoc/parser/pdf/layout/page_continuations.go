package layout

import (
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

var pageContinuationListStart = regexp.MustCompile(`^(?:[（(]?[0-9一二三四五六七八九十百]+[）).．、]|[a-zA-Z]\)|[a-zA-Z]\.(?:\s|$)|[-*•]\s)`)

// JoinPageContinuations removes physical page breaks within continuing prose.
// It runs on sections after BoxesToSections so the merged section keeps every
// source page's rectangle instead of collapsing them into one page-level box.
func JoinPageContinuations(sections []pdf.Section) []pdf.Section {
	if len(sections) < 2 {
		return sections
	}
	joined := make([]pdf.Section, 0, len(sections))
	for _, section := range sections {
		if len(joined) == 0 {
			joined = append(joined, section)
			continue
		}
		previous := &joined[len(joined)-1]
		if !canJoinPageContinuation(*previous, section) {
			joined = append(joined, section)
			continue
		}
		joinPageContinuation(previous, section)
	}
	return joined
}

func canJoinPageContinuation(previous, next pdf.Section) bool {
	if !isContinuationText(previous) || !isContinuationText(next) {
		return false
	}
	a, ok := lastSectionPosition(previous)
	if !ok {
		return false
	}
	b, ok := firstSectionPosition(next)
	if !ok {
		return false
	}
	if len(a.PageNumbers) == 0 || len(b.PageNumbers) == 0 {
		return false
	}
	if b.PageNumbers[0] != a.PageNumbers[len(a.PageNumbers)-1]+1 {
		return false
	}
	// A continuation starts higher on the next page than the previous page's
	// last region ended; a region at or below that point is unrelated.
	if b.Top >= a.Bottom {
		return false
	}
	height := math.Min(a.Bottom-a.Top, b.Bottom-b.Top)
	if height <= 0 || a.Right <= a.Left || b.Right <= b.Left {
		return false
	}
	tolerance := math.Min(height*0.6, 12)
	if math.Abs(a.Left-b.Left) > tolerance {
		return false
	}

	left := normalizeContinuationText(previous.Text)
	right := normalizeContinuationText(next.Text)
	// A blank line is an explicit paragraph break, including whitespace-only
	// lines.
	leftEnd := len(strings.TrimRightFunc(left, unicode.IsSpace))
	rightStart := len(right) - len(strings.TrimLeftFunc(right, unicode.IsSpace))
	if strings.Count(left[leftEnd:], "\n") > 1 || strings.Count(right[:rightStart], "\n") > 1 {
		return false
	}
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left == "" || right == "" || pageContinuationListStart.MatchString(right) {
		return false
	}
	ending := strings.TrimRight(left, `"'”’）)]】》`)
	if ending == "" || strings.ContainsRune("。！？.!?；;：:", lastContinuationRune(ending)) {
		return false
	}
	first, _ := utf8.DecodeRuneInString(right)
	if unicode.IsUpper(first) || (!unicode.IsLetter(first) && !strings.ContainsRune("，、,", first)) {
		return false
	}
	return true
}

func isContinuationText(section pdf.Section) bool {
	if section.TableItem != nil || section.Image != "" {
		return false
	}
	if section.DocTypeKwd != "" && section.DocTypeKwd != pdf.LayoutTypeText {
		return false
	}
	if section.LayoutType != "" && section.LayoutType != pdf.LayoutTypeText {
		return false
	}
	return strings.TrimSpace(section.Text) != ""
}

func firstSectionPosition(section pdf.Section) (pdf.Position, bool) {
	if len(section.Positions) == 0 {
		return pdf.Position{}, false
	}
	return section.Positions[0], true
}

func lastSectionPosition(section pdf.Section) (pdf.Position, bool) {
	if len(section.Positions) == 0 {
		return pdf.Position{}, false
	}
	return section.Positions[len(section.Positions)-1], true
}

func joinPageContinuation(previous *pdf.Section, next pdf.Section) {
	left := strings.TrimRightFunc(normalizeContinuationText(previous.Text), unicode.IsSpace)
	right := strings.TrimLeftFunc(normalizeContinuationText(next.Text), unicode.IsSpace)
	previous.Text = left + pageContinuationSeparator(left, right) + right
	previous.Positions = append(previous.Positions, next.Positions...)
	if len(previous.Positions) == 0 {
		return
	}
	first := previous.Positions[0]
	last := previous.Positions[len(previous.Positions)-1]
	if len(first.PageNumbers) == 0 || len(last.PageNumbers) == 0 {
		return
	}
	previous.PositionTag = util.FormatPositionTagRange(
		first.PageNumbers[0],
		last.PageNumbers[len(last.PageNumbers)-1],
		first.Left,
		last.Right,
		first.Top,
		last.Bottom,
	)
}

func normalizeContinuationText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

func lastContinuationRune(text string) rune {
	r, _ := utf8.DecodeLastRuneInString(text)
	return r
}

func pageContinuationSeparator(left, right string) string {
	a := lastContinuationRune(left)
	b, _ := utf8.DecodeRuneInString(right)
	if pageContinuationNoSpaceRune(a) && pageContinuationNoSpaceRune(b) ||
		strings.ContainsRune("-\u2010\u2011", a) || unicode.IsPunct(b) {
		return ""
	}
	return " "
}

func pageContinuationNoSpaceRune(r rune) bool {
	return r == '\u30fc' || unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}
