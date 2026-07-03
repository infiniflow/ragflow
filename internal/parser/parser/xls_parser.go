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
	officeOxide "github.com/yfedoseev/office_oxide/go"
)

type XLSParser struct {
	libType string
}

func NewXLSParser(libType string) (*XLSParser, error) {
	switch libType {
	case OfficeOxide:
		return &XLSParser{
			libType: OfficeOxide,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported XLS library type: %s", libType)
	}
}

func (p *XLSParser) Parse(filename string, data []byte) error {
	switch p.libType {
	case OfficeOxide:
		return p.OfficeOxideParse(data)
	default:
		return fmt.Errorf("unsupported XLS library type: %s", p.libType)
	}
}

func (p *XLSParser) OfficeOxideParse(data []byte) error {
	doc, err := officeOxide.OpenFromBytes(data, "xls")
	if err != nil {
		return err
	}
	defer doc.Close()

	docFormat, err := doc.Format()
	if err != nil {
		return err
	}

	fmt.Println("Document format:", docFormat)

	docContext, err := doc.PlainText()
	if err != nil {
		return err
	}
	fmt.Println("Document context:", docContext)

	md, err := doc.ToMarkdown()
	if err != nil {
		return err
	}
	fmt.Println("Document Markdown:", md)
	return nil
}

func (p *XLSParser) String() string {
	return "XLSParser"
}

// ParseWithResult delegates to excelize which handles both .xls
// and .xlsx through the same API. The python ExcelParser falls
// back to a similar delegation; on the Go side excelize is the
// single library for both extensions.
func (p *XLSParser) ParseWithResult(filename string, data []byte) ParseResult {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return ParseResult{Err: fmt.Errorf("xls open: %w", err)}
	}
	defer f.Close()

	var html strings.Builder
	html.WriteString("<html><body>")
	for _, sheet := range f.GetSheetList() {
		html.WriteString("<h3>")
		html.WriteString(sheet)
		html.WriteString("</h3>")
		rows, _ := f.GetRows(sheet)
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
		File:         map[string]any{"name": filename, "format": "xls"},
		HTML:         html.String(),
	}
}
