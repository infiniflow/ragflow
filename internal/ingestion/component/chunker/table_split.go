//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package chunker

import (
	"regexp"
	"strings"
)

var trRegex = regexp.MustCompile(`(?is)<tr\b[^>]*>.*?</tr>`)

// tableOpenTagEnd returns the index of the ">" that closes the open tag whose
// name s begins after, skipping ">" inside quoted attribute values. It returns
// -1 when the tag never closes before the next "<": a malformed open tag must
// not be cut at the next ">", which may belong to a closing tag — slicing
// "<table </table>" there would invert the slice bounds.
func tableOpenTagEnd(s string) int {
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i
		case c == '<':
			return -1
		}
	}
	return -1
}

// splitLargeHTMLTable splits an HTML table into sub-tables that fit within
// maxTokens, replicating the caption and header row(s) in each sub-table.
// Token counting is injected (countTokens) so the caller's tokenizer decides
// what "size" means. This is the chunker-side size budget: Python's flow path
// never splits a spreadsheet sheet, but honouring chunk_token_size keeps a
// wide or long table from losing its tail rows to embedding truncation.
//
// The text is expected to hold one outer <table> with flat rows — the shape
// the spreadsheet parsers render. Markup this function cannot cut safely (an
// open tag that never closes, a ">" inside an attribute value, a nested or
// second table) is returned unchanged, so callers fall back to one whole
// chunk instead of cutting at a guessed boundary.
//
// Alongside the parts it returns dataRanges and headerRows: for part i, the
// half-open range [start, end) of DATA rows (rows below the header in the
// original table) the part contains, and the number of leading header rows
// every part replicates. A caller carrying a per-row position matrix slices
// matrix[:headerRows] + matrix[headerRows+start : headerRows+end] to stay
// aligned with the sub-table markup. headerRows and ranges are 0/nil when
// the text is returned unchanged.
func splitLargeHTMLTable(text string, maxTokens int, countTokens func(string) int) (parts []string, dataRanges [][2]int, headerRows int) {
	lower := strings.ToLower(text)
	startIdx := strings.Index(lower, "<table")
	endIdx := strings.LastIndex(lower, "</table>")
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return []string{text}, nil, 0
	}

	prefix := text[:startIdx]
	afterOpen := text[startIdx:]
	const openTag = "<table"
	// The match must be a tag name, not the prefix of a longer one such as
	// "<tableau>".
	if len(afterOpen) <= len(openTag) || !strings.ContainsRune(" \t\n\f\r/>", rune(afterOpen[len(openTag)])) {
		return []string{text}, nil, 0
	}
	tagRel := tableOpenTagEnd(afterOpen[len(openTag):])
	if tagRel < 0 {
		return []string{text}, nil, 0
	}
	tagEnd := len(openTag) + tagRel
	innerStart := startIdx + tagEnd + 1
	if innerStart > endIdx {
		return []string{text}, nil, 0
	}
	tableOpenTag := afterOpen[:tagEnd+1]
	inner := text[innerStart:endIdx]
	// A second table (or a nested one) has no modelled boundary here; leaving
	// the text alone keeps the caller on the single-chunk path.
	if innerLower := strings.ToLower(inner); strings.Contains(innerLower, "<table") || strings.Contains(innerLower, "</table>") {
		return []string{text}, nil, 0
	}
	suffix := text[endIdx+len("</table>"):]

	// Extract caption if present.
	lowerInner := strings.ToLower(inner)
	capStart := strings.Index(lowerInner, "<caption")
	var captionHTML string
	if capStart >= 0 {
		capEndRel := strings.Index(lowerInner[capStart:], "</caption>")
		if capEndRel >= 0 {
			capEnd := capStart + capEndRel + len("</caption>")
			captionHTML = inner[capStart:capEnd]
		}
	}

	// Extract all rows.
	rows := trRegex.FindAllString(inner, -1)
	if len(rows) <= 1 {
		return []string{text}, nil, 0
	}

	// Determine header rows vs data rows.
	headerEnd := 0
	for i, r := range rows {
		if strings.Contains(strings.ToLower(r), "<th") {
			headerEnd = i + 1
		} else {
			break
		}
	}
	if headerEnd == 0 || headerEnd >= len(rows) {
		headerEnd = 1
	}
	headerRowHTML := rows[:headerEnd]
	dataRows := rows[headerEnd:]

	baseHeader := tableOpenTag + captionHTML + strings.Join(headerRowHTML, "")
	prefixTokens := countTokens(prefix)
	baseTokens := countTokens(baseHeader + "</table>")

	// prefix (text before the table, e.g. a short Markdown heading) belongs to
	// the table's position in the document: repeat only the table wrapper,
	// caption and header rows in every chunk, and prepend prefix to the first.
	var chunks []string
	var ranges [][2]int
	var currentDataRows []string
	currentTokens := baseTokens + prefixTokens
	currentStart := 0
	firstChunk := true

	flush := func(end int) {
		if len(currentDataRows) == 0 {
			return
		}
		var b strings.Builder
		if firstChunk {
			b.WriteString(prefix)
			firstChunk = false
		}
		b.WriteString(baseHeader)
		for _, dr := range currentDataRows {
			b.WriteString(dr)
		}
		b.WriteString("</table>")
		chunks = append(chunks, b.String())
		ranges = append(ranges, [2]int{currentStart, end})
		currentDataRows = nil
		currentTokens = baseTokens
		currentStart = end
	}

	for i, row := range dataRows {
		rowTokens := countTokens(row)
		if len(currentDataRows) > 0 && currentTokens+rowTokens > maxTokens {
			flush(i)
		}
		currentDataRows = append(currentDataRows, row)
		currentTokens += rowTokens
	}
	flush(len(dataRows))

	if len(chunks) <= 1 {
		return []string{text}, nil, 0
	}
	if suffix != "" {
		chunks[len(chunks)-1] += suffix
	}
	return chunks, ranges, headerEnd
}
