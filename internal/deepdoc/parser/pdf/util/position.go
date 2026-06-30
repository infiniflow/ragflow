package util

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// @@ page position tag regex patterns.
//
// Python: pdf_parser.py:1868 remove_tag, 1872 extract_positions

// posTagPattern matches the full @@...## tag including coordinates.
// Format: @@{page_range}\t{left}\t{right}\t{top}\t{bottom}##
var posTagPattern = regexp.MustCompile(`@@[0-9-]+\t[0-9.\t]+##`)

// ExtractPositions parses @@ position tags from a text string.
//
// Each tag has format:
//
//	@@{page_range}\t{left}\t{right}\t{top}\t{bottom}##
//
// page_range can be a single page ("3") or a range ("0-2").
// Pages are zero-indexed in the returned values (subtracting 1 from PDF page numbers).
//
// Python: pdf_parser.py:1872 extract_positions()
//
// Example:
//
//	text := "Some text @@0-1\t50.0\t300.0\t200.0\t400.0## more text"
//	poss := ExtractPositions(text)
//	// poss[0] = pdf.Position{PageNumbers: [-1, 0], Left: 50.0, Right: 300.0, Top: 200.0, Bottom: 400.0}
func ExtractPositions(text string) []pdf.Position {
	var poss []pdf.Position
	for _, tag := range posTagPattern.FindAllString(text, -1) {
		cleaned := strings.TrimPrefix(strings.TrimSuffix(tag, "##"), "@@")
		parts := strings.Split(cleaned, "\t")
		if len(parts) != 5 {
			continue
		}

		// Parse page range
		var pageNums []int
		for _, p := range strings.Split(parts[0], "-") {
			n, err := strconv.Atoi(p)
			if err != nil {
				slog.Warn("ExtractPositions: invalid page number in tag", "tag", tag, "part", p, "err", err)
				continue
			}
			pageNums = append(pageNums, n-1) // 0-index
		}

		left, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			slog.Warn("ExtractPositions: invalid left coordinate", "tag", tag, "err", err)
			continue
		}
		right, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			slog.Warn("ExtractPositions: invalid right coordinate", "tag", tag, "err", err)
			continue
		}
		top, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			slog.Warn("ExtractPositions: invalid top coordinate", "tag", tag, "err", err)
			continue
		}
		bottom, err := strconv.ParseFloat(parts[4], 64)
		if err != nil {
			slog.Warn("ExtractPositions: invalid bottom coordinate", "tag", tag, "err", err)
			continue
		}

		poss = append(poss, pdf.Position{
			PageNumbers: pageNums,
			Left:        left,
			Right:       right,
			Top:         top,
			Bottom:      bottom,
		})
	}
	return poss
}

// FormatPositionTag creates a @@ position tag string from page number and bounding box.
//
// Reverse of ExtractPositions. Used when converting PDF engine
// bboxes back to RAGFlow position tag format.
//
// Example:
//
//	tag := FormatPositionTag(0, 50.0, 300.0, 200.0, 400.0)
//	// "@@0-0\t50.0\t300.0\t200.0\t400.0##"
func FormatPositionTag(pageNum int, left, right, top, bottom float64) string {
	return fmt.Sprintf("@@%d\t%.1f\t%.1f\t%.1f\t%.1f##",
		pageNum+1, left, right, top, bottom)
}

// FormatPositionTagRange creates a @@ position tag for multi-page content.
//
// Example:
//
//	tag := FormatPositionTagRange(0, 2, 50.0, 300.0, 200.0, 400.0)
//	// "@@0-2\t50.0\t300.0\t200.0\t400.0##"
func FormatPositionTagRange(fromPage, toPage int, left, right, top, bottom float64) string {
	return fmt.Sprintf("@@%d-%d\t%.1f\t%.1f\t%.1f\t%.1f##",
		fromPage+1, toPage+1, left, right, top, bottom)
}
