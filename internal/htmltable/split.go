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

package htmltable

import (
	"regexp"
	"strings"
)

var trRegex = regexp.MustCompile(`(?is)<tr\b[^>]*>.*?</tr>`)

// SplitLargeHTMLTable splits an HTML table into sub-tables that fit within
// maxTokens, replicating the caption and header row(s) in each sub-table.
// Token counting is injected (countTokens) so this package stays free of NLP
// dependencies; callers pass their own tokenizer.
//
// Alongside the parts it returns dataRanges: for part i, the half-open range
// [start, end) of DATA rows (rows below the header in the original table)
// the part contains. A caller carrying a per-row position matrix slices
// matrix[1+start : 1+end] (plus its own header tuple) to stay aligned with
// the sub-table markup. Ranges are nil when the text is returned unchanged.
func SplitLargeHTMLTable(text string, maxTokens int, countTokens func(string) int) ([]string, [][2]int) {
	lower := strings.ToLower(text)
	startIdx := strings.Index(lower, "<table")
	endIdx := strings.LastIndex(lower, "</table>")
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		return []string{text}, nil
	}

	prefix := text[:startIdx]
	afterOpen := text[startIdx:]
	tagEnd := strings.Index(afterOpen, ">")
	if tagEnd < 0 {
		return []string{text}, nil
	}
	tableOpenTag := afterOpen[:tagEnd+1]
	inner := text[startIdx+tagEnd+1 : endIdx]
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
		return []string{text}, nil
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
	headerRows := rows[:headerEnd]
	dataRows := rows[headerEnd:]

	baseHeader := tableOpenTag + captionHTML + strings.Join(headerRows, "")
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
		return []string{text}, nil
	}
	if suffix != "" {
		chunks[len(chunks)-1] += suffix
	}
	return chunks, ranges
}
