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

// Package io — XLSX writer for the Message component's "Excel" export
// format.
//
// It mirrors Python agent/component/message.py's xlsx branch: every
// Markdown table found in the content becomes its own sheet (named
// after the closest preceding title line, sanitized to Excel's 31-char
// sheet-name rules), numeric-looking cells are coerced to native
// numbers, and content without any table falls back to a single
// "Data" sheet holding the whole text.

package io

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// XLSXOptions is the public contract for the XLSX writer. Reserved for
// future header/footer support; the Python xlsx branch has none.
type XLSXOptions struct{}

var (
	thousandSepRe = regexp.MustCompile(`^[+-]?\d{1,3}(,\d{3})+(\.\d+)?$`)
	intRe         = regexp.MustCompile(`^[+-]?\d+$`)
	floatRe       = regexp.MustCompile(`^[+-]?(\d+\.\d+|\d+\.|\.\d+)([eE][+-]?\d+)?$`)
	floatExpRe    = regexp.MustCompile(`^[+-]?\d+[eE][+-]?\d+$`)
	leadingZeroRe = regexp.MustCompile(`^[+-]?0\d+$`)
)

// xlsxCell mirrors Python's _coerce_excel_cell_type: convert cell text
// to a native numeric type when it is safe, so Excel stores real number
// cells instead of text. Values with leading zeros ("00123") stay text
// to avoid losing information, and thousand separators ("1,234.5")
// are stripped before numeric detection.
func xlsxCell(cell string) any {
	value := strings.TrimSpace(cell)
	if value == "" {
		return ""
	}
	if leadingZeroRe.MatchString(value) {
		return cell
	}
	candidate := value
	if thousandSepRe.MatchString(value) {
		candidate = strings.ReplaceAll(value, ",", "")
	}
	if intRe.MatchString(candidate) {
		if n, err := strconv.Atoi(candidate); err == nil {
			return n
		}
		return cell
	}
	if floatRe.MatchString(candidate) || floatExpRe.MatchString(candidate) {
		if f, err := strconv.ParseFloat(candidate, 64); err == nil {
			return f
		}
		return cell
	}
	return cell
}

type xlsxTable struct {
	sheetName string
	header    []string
	rows      [][]any
}

// parseMarkdownTables splits content into markdown tables. Title
// candidates (lines starting with "table", markdown headings, or
// containing ':') directly preceding a table name its sheet.
func parseMarkdownTables(content string) []xlsxTable {
	type rawTable struct {
		title string
		lines []string
	}
	var tables []rawTable
	var current []string
	var pendingTitle string
	inTable := false

	appendTable := func() {
		if len(current) > 0 {
			tables = append(tables, rawTable{title: pendingTitle, lines: current})
		}
		current = nil
	}
	titleCandidate := func(stripped string) string {
		lower := strings.ToLower(stripped)
		if strings.HasPrefix(lower, "table") || strings.HasPrefix(stripped, "#") || strings.Contains(stripped, ":") {
			return strings.TrimSpace(strings.TrimLeft(stripped, "#"))
		}
		return ""
	}

	for _, line := range strings.Split(strings.TrimSpace(content), "\n") {
		stripped := strings.TrimSpace(line)
		if !inTable && stripped != "" && !strings.HasPrefix(stripped, "|") {
			pendingTitle = titleCandidate(stripped)
		}
		if strings.HasPrefix(stripped, "|") && strings.Contains(stripped[1:], "|") {
			cleaned := strings.NewReplacer(" ", "", "|", "", "-", "", ":", "").Replace(stripped)
			if cleaned == "" {
				continue // separator line like |---|---|
			}
			if !inTable {
				inTable = true
				current = nil
			}
			current = append(current, stripped)
			continue
		}
		if inTable {
			appendTable()
			inTable = false
			pendingTitle = ""
			if stripped != "" {
				pendingTitle = titleCandidate(stripped)
			}
		}
	}
	if inTable {
		appendTable()
	}

	out := make([]xlsxTable, 0, len(tables))
	kept := 0
	for _, t := range tables {
		var header []string
		var rows [][]any
		for i, line := range t.lines {
			cells := strings.Split(line, "|")
			// Python drops empty cells ([c for c in cells if c]);
			// mirror that.
			kept := make([]string, 0, len(cells))
			for _, c := range cells {
				c = strings.TrimSpace(c)
				if c != "" {
					kept = append(kept, c)
				}
			}
			if i == 0 {
				header = kept
				continue
			}
			row := make([]any, len(kept))
			for j, c := range kept {
				row[j] = xlsxCell(c)
			}
			rows = append(rows, row)
		}
		// A header-only table with no data rows yields no sheet: the
		// parsed table is empty and is dropped, so the sheet-number
		// fallback below only counts tables that actually land in the
		// workbook.
		if len(rows) == 0 {
			continue
		}
		kept++
		out = append(out, xlsxTable{sheetName: sanitizeSheetName(t.title, kept-1), header: header, rows: rows})
	}
	return out
}

// sanitizeSheetName clamps a title to Excel's sheet-name rules
// (max 31 characters, no / \ * ? [ ] :) and falls back to "Table_N".
func sanitizeSheetName(title string, index int) string {
	name := title
	if name == "" {
		name = fmt.Sprintf("Table_%d", index+1)
	}
	if runes := []rune(name); len(runes) > 31 {
		name = string(runes[:31])
	}
	name = strings.NewReplacer("/", "_", "\\", "_", "*", "", "?", "", "[", "", "]", "", ":", "").Replace(name)
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("Table_%d", index+1)
	}
	return name
}

// WriteXLSX converts Markdown content into an Excel workbook: each
// Markdown table lands on its own sheet; when the content has no
// table at all, a single "Data" sheet holds the whole text.
func WriteXLSX(content string, _ XLSXOptions) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	tables := parseMarkdownTables(content)
	if len(tables) == 0 {
		tables = []xlsxTable{{sheetName: "Data", rows: [][]any{{content}}}}
	}
	used := make(map[string]bool)
	for _, t := range tables {
		name := t.sheetName
		// Ensure unique sheet names: on collision append _1, _2, ...
		// to the original name, truncating it in characters so the
		// final name stays within the 31-character limit. Keys are
		// lowercased because Excel and Excelize resolve sheet names
		// case-insensitively: with a case-sensitive map, "Report"
		// followed by "report" would alias to the same sheet and the
		// second table would overwrite the first.
		key := strings.ToLower(name)
		original := []rune(name)
		for counter := 1; used[key]; counter++ {
			suffix := "_" + strconv.Itoa(counter)
			trim := 31 - len(suffix)
			if trim < 1 {
				trim = 1
			}
			base := original
			if len(base) > trim {
				base = base[:trim]
			}
			name = string(base) + suffix
			key = strings.ToLower(name)
		}
		used[key] = true
		if _, err := f.NewSheet(name); err != nil {
			return nil, fmt.Errorf("xlsx: new sheet %q: %w", name, err)
		}

		row := 1
		if len(t.header) > 0 {
			header := make([]any, len(t.header))
			for i, h := range t.header {
				header[i] = h
			}
			if err := writeXLSXRow(f, name, row, header); err != nil {
				return nil, err
			}
			row++
		}
		for _, r := range t.rows {
			if err := writeXLSXRow(f, name, row, r); err != nil {
				return nil, err
			}
			row++
		}
	}
	// Drop the default sheet; the workbook always has real sheets here.
	if err := f.DeleteSheet("Sheet1"); err != nil {
		return nil, fmt.Errorf("xlsx: delete default sheet: %w", err)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("xlsx: write workbook: %w", err)
	}
	return buf.Bytes(), nil
}

func writeXLSXRow(f *excelize.File, sheet string, row int, cells []any) error {
	for col, v := range cells {
		cell, err := excelize.CoordinatesToCellName(col+1, row)
		if err != nil {
			return fmt.Errorf("xlsx: cell name (%d,%d): %w", col+1, row, err)
		}
		if err := f.SetCellValue(sheet, cell, v); err != nil {
			return fmt.Errorf("xlsx: set %s!%s: %w", sheet, cell, err)
		}
	}
	return nil
}
