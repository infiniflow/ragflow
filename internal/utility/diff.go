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

package utility

import (
	"fmt"
	"strings"
)

const (
	// unifiedDiffContextLines is how many unchanged lines surround a change,
	// matching Python's difflib.unified_diff default.
	unifiedDiffContextLines = 3

	// unifiedDiffMaxCells bounds the LCS table. Chunk content is small by
	// construction, but the same helper serves arbitrary text: past this many
	// cells the diff degrades to one delete block followed by one insert block
	// instead of allocating a quadratic table.
	unifiedDiffMaxCells = 4_000_000
)

type unifiedDiffLine struct {
	prefix  byte
	text    string
	oldLine int
	newLine int
}

// UnifiedDiff returns a standard unified diff (---/+++/@@ headers) between two
// texts. An empty string means the texts are identical, which lets callers use
// the result itself as the "changed" flag.
//
// The output is the format produced by Python's difflib.unified_diff, so the
// frontend's unified-diff renderer parses it without a dedicated code path.
func UnifiedDiff(oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	lines := diffLines(oldLines, newLines)

	changed := make([]int, 0)
	for index, line := range lines {
		if line.prefix != ' ' {
			changed = append(changed, index)
		}
	}
	if len(changed) == 0 {
		return ""
	}

	// Group nearby changes into hunks with up to three context lines.
	type diffHunk struct {
		start int
		end   int
	}
	hunks := make([]diffHunk, 0)
	for _, index := range changed {
		start := index - unifiedDiffContextLines
		if start < 0 {
			start = 0
		}
		end := index + unifiedDiffContextLines
		if end >= len(lines) {
			end = len(lines) - 1
		}
		if len(hunks) > 0 && start <= hunks[len(hunks)-1].end+1 {
			if end > hunks[len(hunks)-1].end {
				hunks[len(hunks)-1].end = end
			}
			continue
		}
		hunks = append(hunks, diffHunk{start: start, end: end})
	}

	var b strings.Builder
	b.WriteString("--- a\n+++ b\n")
	for _, hunk := range hunks {
		oldStart, oldCount := diffRange(lines, hunk.start, hunk.end, false, len(oldLines))
		newStart, newCount := diffRange(lines, hunk.start, hunk.end, true, len(newLines))
		b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))
		for index := hunk.start; index <= hunk.end; index++ {
			b.WriteByte(lines[index].prefix)
			b.WriteString(lines[index].text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// diffLines walks the LCS of the two line slices and renders every line with
// its old/new numbering. Texts whose LCS table would exceed unifiedDiffMaxCells
// are rendered as a single replacement so memory stays linear.
func diffLines(oldLines, newLines []string) []unifiedDiffLine {
	if len(oldLines)*len(newLines) > unifiedDiffMaxCells {
		lines := make([]unifiedDiffLine, 0, len(oldLines)+len(newLines))
		for i, line := range oldLines {
			lines = append(lines, unifiedDiffLine{'-', line, i + 1, 0})
		}
		for j, line := range newLines {
			lines = append(lines, unifiedDiffLine{'+', line, 0, j + 1})
		}
		return lines
	}

	cur := make([][]int, len(oldLines)+1)
	for i := range cur {
		cur[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				cur[i][j] = cur[i+1][j+1] + 1
			} else if cur[i+1][j] >= cur[i][j+1] {
				cur[i][j] = cur[i+1][j]
			} else {
				cur[i][j] = cur[i][j+1]
			}
		}
	}

	lines := make([]unifiedDiffLine, 0, len(oldLines)+len(newLines))
	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		if oldLines[i] == newLines[j] {
			lines = append(lines, unifiedDiffLine{' ', oldLines[i], i + 1, j + 1})
			i++
			j++
		} else if cur[i+1][j] >= cur[i][j+1] {
			lines = append(lines, unifiedDiffLine{'-', oldLines[i], i + 1, 0})
			i++
		} else {
			lines = append(lines, unifiedDiffLine{'+', newLines[j], 0, j + 1})
			j++
		}
	}
	for ; i < len(oldLines); i++ {
		lines = append(lines, unifiedDiffLine{'-', oldLines[i], i + 1, 0})
	}
	for ; j < len(newLines); j++ {
		lines = append(lines, unifiedDiffLine{'+', newLines[j], 0, j + 1})
	}
	return lines
}

// diffRange returns the first line number and line count for one side of a
// unified diff hunk. A zero count is valid for pure insertions or deletions.
func diffRange(lines []unifiedDiffLine, start, end int, newSide bool, totalLines int) (int, int) {
	first := 0
	count := 0
	for index := start; index <= end; index++ {
		lineNumber := lines[index].oldLine
		if newSide {
			lineNumber = lines[index].newLine
		}
		if lineNumber == 0 {
			continue
		}
		if first == 0 {
			first = lineNumber
		}
		count++
	}
	if first == 0 {
		first = totalLines
	}
	return first, count
}
