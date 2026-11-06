package chunker

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"ragflow/internal/ingestion/component/schema"
)

type pdfSourceSpan struct {
	start, end              int
	pdfPositions, positions json.RawMessage
}

func pdfUnitSources(units []schema.ChunkDoc) [][]pdfSourceSpan {
	sources := make([][]pdfSourceSpan, len(units))
	for index, unit := range units {
		text := removeTag(normalizeGeneralNewlines(unit.Text))
		start := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace))
		sources[index] = []pdfSourceSpan{{start: start, end: start + len(strings.TrimSpace(text)), pdfPositions: unit.PDFPositions, positions: unit.Positions}}
	}
	return sources
}

func splitPDFUnits(units []schema.ChunkDoc, sources [][]pdfSourceSpan, pattern *regexp.Regexp, keepDelimiter bool) ([]schema.ChunkDoc, [][]pdfSourceSpan) {
	if sources == nil {
		return splitGeneralUnits(units, pattern, keepDelimiter), nil
	}
	var result []schema.ChunkDoc
	var resultSources [][]pdfSourceSpan
	for index, unit := range units {
		pieces := splitGeneralUnits([]schema.ChunkDoc{unit}, pattern, keepDelimiter)
		rawText := normalizeGeneralNewlines(unit.Text)
		text := removeTag(rawText)
		var ranges [][2]int
		if !keepDelimiter && pattern != nil && itemDocType(unit) == "text" && pattern.MatchString(rawText) {
			for _, part := range splitDroppedDelimiterRanges(len(rawText), pattern.FindAllStringIndex(rawText, -1)) {
				if strings.TrimSpace(rawText[part[0]:part[1]]) != "" {
					ranges = append(ranges, part)
				}
			}
		}
		cursor := 0
		for pieceIndex, piece := range pieces {
			pieceText := removeTag(piece.Text)
			var spans []pdfSourceSpan
			if ranges != nil && len(ranges) == len(pieces) && sources[index] != nil {
				part := ranges[pieceIndex]
				prefix, selected, suffix := removeTag(rawText[:part[0]]), removeTag(rawText[part[0]:part[1]]), removeTag(rawText[part[1]:])
				if selected == pieceText && prefix+selected+suffix == text {
					spans = slicePDFSourceSpans(sources[index], text, len(prefix), len(prefix)+len(selected))
				}
			} else if ranges == nil {
				if offset := strings.Index(text[cursor:], pieceText); offset >= 0 && sources[index] != nil {
					start := cursor + offset
					end := start + len(pieceText)
					spans = slicePDFSourceSpans(sources[index], text, start, end)
					cursor = end
				}
			}
			if pdfPositions, positions := pdfSourcePositions(spans); len(pdfPositions) > 0 || len(positions) > 0 {
				piece.PDFPositions, piece.Positions = pdfPositions, positions
			}
			result = append(result, piece)
			resultSources = append(resultSources, spans)
		}
	}
	return result, resultSources
}

func slicePDFSourceSpans(spans []pdfSourceSpan, text string, start, end int) []pdfSourceSpan {
	result := make([]pdfSourceSpan, 0, len(spans))
	for _, span := range spans {
		if span.start < 0 || span.end > len(text) || span.start >= span.end {
			continue
		}
		left, right := max(start, span.start), min(end, span.end)
		if left >= right {
			continue
		}
		if left != span.start || right != span.end {
			total := utf8.RuneCountInString(text[span.start:span.end])
			if total > 0 {
				begin := float64(utf8.RuneCountInString(text[span.start:left])) / float64(total)
				finish := float64(utf8.RuneCountInString(text[span.start:right])) / float64(total)
				if sliced := slicePositionsByTextRatio(span.pdfPositions, begin, finish); len(sliced) > 0 {
					span.pdfPositions = sliced
				}
				if sliced := slicePositionsByTextRatio(span.positions, begin, finish); len(sliced) > 0 {
					span.positions = sliced
				}
			}
		}
		span.start = left - start
		span.end = right - start
		result = append(result, span)
	}
	return result
}

func pdfSourcePositions(spans []pdfSourceSpan) (json.RawMessage, json.RawMessage) {
	pdf := make([]json.RawMessage, 0, len(spans))
	positions := make([]json.RawMessage, 0, len(spans))
	for _, span := range spans {
		pdf = append(pdf, span.pdfPositions)
		positions = append(positions, span.positions)
	}
	return mergePDFSourcePositions(pdf), mergePDFSourcePositions(positions)
}

func mergePDFSourcePositions(sources []json.RawMessage) json.RawMessage {
	if rows := mergePositionMatrix(sources...); rows != nil {
		if encoded, err := json.Marshal(rows); err == nil {
			return encoded
		}
	}
	var positions json.RawMessage
	for _, source := range sources {
		positions = extendRawJSONArray(positions, source)
	}
	return positions
}
