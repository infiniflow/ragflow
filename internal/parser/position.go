package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// @@ page position tag regex patterns.
//
// Python: pdf_parser.py:1868 remove_tag, 1872 extract_positions

// posTagPattern matches the full @@...## tag including coordinates.
// Format: @@{page_range}\t{left}\t{right}\t{top}\t{bottom}##
var posTagPattern = regexp.MustCompile(`@@[0-9-]+\t[0-9.\t]+##`)

// removeTagPattern matches @@...## fragments for removal.
var removeTagPattern = regexp.MustCompile(`@@[\t0-9.-]+?##`)

// offsetPagePattern matches page numbers in @@ tags for offset adjustment.
var offsetPagePattern = regexp.MustCompile(`@@([0-9-]+)\t`)

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
//	// poss[0] = Position{PageNumbers: [-1, 0], Left: 50.0, Right: 300.0, Top: 200.0, Bottom: 400.0}
func ExtractPositions(text string) []Position {
	var poss []Position
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
				continue
			}
			pageNums = append(pageNums, n-1) // 0-index
		}

		left, _ := strconv.ParseFloat(parts[1], 64)
		right, _ := strconv.ParseFloat(parts[2], 64)
		top, _ := strconv.ParseFloat(parts[3], 64)
		bottom, _ := strconv.ParseFloat(parts[4], 64)

		poss = append(poss, Position{
			PageNumbers: pageNums,
			Left:        left,
			Right:       right,
			Top:         top,
			Bottom:      bottom,
		})
	}
	return poss
}

// RemoveTag strips @@ position tags from text.
//
// Python: pdf_parser.py:1868 remove_tag()
//
// Example:
//
//	text := "Q3 results @@0-1\t50.0\t300.0\t200.0\t400.0##"
//	clean := RemoveTag(text)  // "Q3 results"
func RemoveTag(text string) string {
	return strings.TrimSpace(removeTagPattern.ReplaceAllString(text, ""))
}

// OffsetPositionTag adjusts page numbers in @@ tags by pageOffset.
// Used when parsing only a range of pages (from_page > 0) to renumber
// pages back to global numbering.
//
// Python: pdf_parser.py:1840 _offset_position_tag()
//
// Example:
//
//	tag := "@@0-1\t50.0\t300.0\t200.0\t400.0##"
//	// If we started from page 5, offset by 5:
//	result := OffsetPositionTag(tag, 5)
//	// "@@5-6\t50.0\t300.0\t200.0\t400.0##"
func OffsetPositionTag(text string, pageOffset int) string {
	if text == "" || pageOffset <= 0 {
		return text
	}
	return offsetPagePattern.ReplaceAllStringFunc(text, func(match string) string {
		// Extract page numbers from "@@{pages}\t"
		pagesStr := offsetPagePattern.FindStringSubmatch(match)[1]
		var newPages []string
		for _, p := range strings.Split(pagesStr, "-") {
			n, err := strconv.Atoi(p)
			if err != nil {
				newPages = append(newPages, p)
				continue
			}
			newPages = append(newPages, strconv.Itoa(n+pageOffset))
		}
		return fmt.Sprintf("@@%s\t", strings.Join(newPages, "-"))
	})
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

// OffsetBoxes adjusts page numbers in boxes to global numbering.
//
// Python: pdf_parser.py:1850 _to_global_boxes()
func OffsetBoxes(boxes []TextBox, pageOffset int) []TextBox {
	if pageOffset <= 0 {
		return boxes
	}
	for i := range boxes {
		boxes[i].PageNumber += pageOffset
	}
	return boxes
}
