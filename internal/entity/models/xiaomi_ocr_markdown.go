// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var xiaomiCellTag = regexp.MustCompile(`<(?:fcel|ecel|lcel|ucel|xcel|nl)>`)
var xiaomiDisplayMath = regexp.MustCompile(`(?s)\\\[(.*?)\\\]`)
var xiaomiInlineMath = regexp.MustCompile(`(?s)\\\((.*?)\\\)`)

// Convert the model's OTSL blocks before RAGFlow's Markdown parser reads them.
// Keep fenced code verbatim, including literal OTSL examples.
func xiaomiOCRMarkdown(text string) string {
	lines := strings.Split(text, "\n")
	var out []string
	var fence string
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if fence == "" && (strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")) {
			fence = line[:len(line)-len(strings.TrimLeft(line, line[:1]))]
			out = append(out, lines[i])
			i++
			continue
		}
		if fence != "" {
			if strings.HasPrefix(line, fence) && strings.Trim(line, fence[:1]) == "" {
				fence = ""
			}
			out = append(out, lines[i])
			i++
			continue
		}
		if strings.HasPrefix(line, "<fcel>") || strings.HasPrefix(line, "<ecel>") {
			start := i
			for i++; i < len(lines); i++ {
				s := strings.TrimSpace(lines[i])
				if loc := xiaomiCellTag.FindStringIndex(s); loc == nil || loc[0] != 0 {
					break
				}
			}
			raw := strings.Join(lines[start:i], "\n")
			if table := xiaomiOCRTable(raw); table != "" {
				out = append(out, "", table, "")
			} else {
				out = append(out, raw)
			}
			continue
		}
		start := i
		for i++; i < len(lines); i++ {
			s := strings.TrimSpace(lines[i])
			if strings.HasPrefix(s, "```") || strings.HasPrefix(s, "~~~") || strings.HasPrefix(s, "<fcel>") || strings.HasPrefix(s, "<ecel>") {
				break
			}
		}
		prose := strings.Join(lines[start:i], "\n")
		prose = xiaomiDisplayMath.ReplaceAllString(prose, "$$$$$1$$$$")
		out = append(out, xiaomiInlineMath.ReplaceAllString(prose, "$$${1}$$"))
	}
	return strings.Join(out, "\n")
}

type xiaomiTableCell struct {
	row, column, rowSpan, colSpan int
	text                          string
}

// Continuations name the cell to the left (lcel), above (ucel), or both
// (xcel). Remember the owning cell for each column of the previous row.
func xiaomiOCRTable(raw string) string {
	var rows [][]*xiaomiTableCell
	var previous []*xiaomiTableCell
	for _, line := range strings.Split(raw, "<nl>") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		tags := xiaomiCellTag.FindAllStringIndex(line, -1)
		if len(tags) == 0 || tags[0][0] != 0 {
			return ""
		}
		cells := make([]*xiaomiTableCell, len(tags))
		for col, tag := range tags {
			end := len(line)
			if col+1 < len(tags) {
				end = tags[col+1][0]
			}
			value := strings.TrimSpace(line[tag[1]:end])
			switch line[tag[0]:tag[1]] {
			case "<fcel>", "<ecel>":
				cells[col] = &xiaomiTableCell{row: len(rows), column: col, rowSpan: 1, colSpan: 1, text: value}
			case "<lcel>":
				if col == 0 || cells[col-1].row != len(rows) || value != "" {
					return ""
				}
				cells[col] = cells[col-1]
				cells[col].colSpan = max(cells[col].colSpan, col-cells[col].column+1)
			case "<ucel>", "<xcel>":
				if col >= len(previous) || value != "" {
					return ""
				}
				above := previous[col]
				if line[tag[0]:tag[1]] == "<xcel>" {
					if col == 0 || cells[col-1] != above {
						return ""
					}
				} else if above.column != col {
					return ""
				}
				cells[col] = above
				cells[col].rowSpan = len(rows) - cells[col].row + 1
			}
		}
		rows = append(rows, cells)
		previous = cells
	}
	var table strings.Builder
	table.WriteString("<table>")
	for row, cells := range rows {
		table.WriteString("<tr>")
		for col, cell := range cells {
			if cell.row != row || cell.column != col {
				continue
			}
			table.WriteString("<td")
			if cell.rowSpan > 1 {
				fmt.Fprintf(&table, ` rowspan="%d"`, cell.rowSpan)
			}
			if cell.colSpan > 1 {
				fmt.Fprintf(&table, ` colspan="%d"`, cell.colSpan)
			}
			table.WriteString(">" + html.EscapeString(cell.text) + "</td>")
		}
		table.WriteString("</tr>")
	}
	table.WriteString("</table>")
	return table.String()
}
