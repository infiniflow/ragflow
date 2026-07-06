//go:build cgo

//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

type XLSXParser struct {
	libType string
}

func NewXLSXParser(libType string) (*XLSXParser, error) {
	switch libType {
	case OfficeOxide:
		return &XLSXParser{
			libType: OfficeOxide,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported XLSX library type: %s", libType)
	}
}

func (p *XLSXParser) String() string {
	return "XLSXParser"
}

// ParseWithResult renders the spreadsheet as HTML — the python
// ExcelParser shape. Each sheet becomes a <table> with row /
// column structure preserved; sheet names become <h3> headings
// so a downstream title chunker can pick them up. Implementation
// uses excelize (already in go.mod) instead of office_oxide's
// PlainText/ToMarkdown so cell boundaries survive the round-trip.
func (p *XLSXParser) ParseWithResult(filename string, data []byte) ParseResult {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return ParseResult{Err: fmt.Errorf("xlsx open: %w", err)}
	}
	defer f.Close()

	sheets := f.GetSheetList()
	var html strings.Builder
	html.WriteString("<html><body>")
	for _, sheet := range sheets {
		html.WriteString("<h3>")
		html.WriteString(sheet)
		html.WriteString("</h3>")
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		html.WriteString("<table>")
		for _, row := range rows {
			html.WriteString("<tr>")
			for _, cell := range row {
				html.WriteString("<td>")
				html.WriteString(htmlEscape(cell))
				html.WriteString("</td>")
			}
			html.WriteString("</tr>")
		}
		html.WriteString("</table>")
	}
	html.WriteString("</body></html>")

	return ParseResult{
		OutputFormat: "html",
		File:         map[string]any{"name": filename, "format": "xlsx", "sheets": len(sheets)},
		HTML:         html.String(),
	}
}
