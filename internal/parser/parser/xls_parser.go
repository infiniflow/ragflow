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
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	excel "github.com/jloor/go-excel-reader"
)

var compoundDocumentSignature = []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}

type XLSParser struct {
	ParseMethod                    string
	OutputFormat                   string
	TCADPAPIServer                 string
	TCADPAPIKey                    string
	TCADPTableResultType           string
	TCADPMarkdownImageResponseType string
}

func NewXLSParser(_ string) (*XLSParser, error) {
	return &XLSParser{
		TCADPTableResultType:           "1",
		TCADPMarkdownImageResponseType: "1",
	}, nil
}

func (p *XLSParser) String() string {
	return "XLSParser"
}

func (p *XLSParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["parse_method"].(string); ok && v != "" {
		p.ParseMethod = v
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.OutputFormat = v
	}
	deprecatedHTML4Excel(setup, p.String())
	deprecatedChunkRows(setup, p.String())
	if v, ok := setup["tcadp_apiserver"].(string); ok && v != "" {
		p.TCADPAPIServer = v
	}
	if v, ok := setup["tcadp_api_key"].(string); ok {
		p.TCADPAPIKey = v
	}
	if v, ok := setup["table_result_type"].(string); ok && v != "" {
		p.TCADPTableResultType = v
	}
	if v, ok := setup["markdown_image_response_type"].(string); ok && v != "" {
		p.TCADPMarkdownImageResponseType = v
	}
}

func (p *XLSParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	method := normalizeXLSXParseMethod(p.ParseMethod)
	switch method {
	case "tcadp":
		return parseWithTCADP(
			ctx, filename, data, "XLS",
			p.TCADPAPIServer, p.TCADPAPIKey,
			p.TCADPTableResultType, p.TCADPMarkdownImageResponseType,
			p.OutputFormat,
		)
	case "", "excelize":
		// Continue with the local Excelize parser.
	default:
		return ParseResult{
			Err: fmt.Errorf("unsupported XLS parse method: %q", p.ParseMethod),
		}
	}

	var (
		items       []map[string]any
		warnings    []string
		sheetsCount int
		err         error
	)
	if bytes.HasPrefix(data, compoundDocumentSignature) {
		items, sheetsCount, err = parseBIFF8Bytes(ctx, data)
	} else {
		mediaBudget := newEmbeddedMediaBudget()
		items, warnings, sheetsCount, err = parseXLSXBytes(data, mediaBudget)
		warnings = append(warnings, mediaBudget.warnings()...)
	}
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	if err != nil {
		return ParseResult{Err: fmt.Errorf("xls parse: %w", err)}
	}

	return ParseResult{
		OutputFormat: spreadsheetOutputFormat,
		File:         map[string]any{"name": filename, "format": "xls", "sheets": sheetsCount},
		JSON:         items,
		Warnings:     warnings,
	}
}

func parseBIFF8Bytes(ctx context.Context, data []byte) ([]map[string]any, int, error) {
	var reader biffSource
	biff8, err := excel.OpenXLS(bytes.NewReader(data), excel.WithErrorValues(true))
	if err != nil {
		// Excel 95 and older workbooks use BIFF≤5 records that the BIFF8-only
		// reader rejects; xlrd covers the full BIFF2–8 range.
		legacy, legacyErr := newXlrdBIFFSource(data)
		if legacyErr != nil {
			return nil, 0, fmt.Errorf("open BIFF workbook: %w", err)
		}
		reader = legacy
	} else {
		reader = biff8
	}
	defer reader.Close()

	sheetCount := reader.SheetCount()
	items := make([]map[string]any, 0, sheetCount)
	for sheetIndex := 0; reader.NextSheet(); sheetIndex++ {
		if err := ctx.Err(); err != nil {
			return nil, sheetCount, err
		}
		records, dataRows, headerRow := readBIFF8Records(ctx, reader)
		if err := ctx.Err(); err != nil {
			return nil, sheetCount, err
		}
		sheetName := reader.SheetName()
		items = append(items, buildSheetItems(records, sheetName, sheetIndex+1, headerRow, dataRows, nil)...)
		// Same cover-page KV chunk as the xlsx path: the sheet name and
		// label→value pairs must stay searchable for legacy workbooks too.
		if isFormSheet(records) {
			if kvText := renderFormSheetKV(sheetName, records); len(kvText) > 0 {
				kvItem := NewTextJSONItem(kvText)
				kvItem["sheet"] = sheetName
				kvItem["sheet_index"] = sheetIndex + 1
				items = append(items, kvItem)
			}
		}
	}
	if err := reader.Err(); err != nil {
		return nil, sheetCount, fmt.Errorf("read BIFF workbook: %w", err)
	}
	return items, sheetCount, nil
}

func readBIFF8Records(ctx context.Context, reader biffSource) ([][]string, []int, int) {
	type numberedRow struct {
		number int
		cells  []string
	}

	rows := make([]numberedRow, 0)
	for reader.Read() {
		if ctx.Err() != nil {
			return nil, nil, 0
		}
		cells := make([]string, reader.FieldCount())
		for col := range cells {
			cells[col] = spreadsheetCellString(reader.GetValue(col))
		}
		if rowIsEmpty(cells) {
			continue
		}
		rows = append(rows, numberedRow{number: reader.RowIndex() + 1, cells: cells})
	}
	if len(rows) == 0 {
		return nil, nil, 0
	}

	records := make([][]string, 0, len(rows))
	dataRows := make([]int, 0, len(rows)-1)
	records = append(records, rows[0].cells)
	for _, row := range rows[1:] {
		records = append(records, row.cells)
		dataRows = append(dataRows, row.number)
	}
	return cleanIllegalControlChars(records), dataRows, rows[0].number
}

func spreadsheetCellString(value any) string {
	switch value := value.(type) {
	case nil:
		return ""
	case time.Time:
		return value.Format(time.RFC3339)
	case float64:
		// fmt.Sprint falls back to %v scientific notation for large
		// magnitudes (5391273.95 → 5.39127395e+06); keep the decimal form
		// so amounts stay searchable.
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return fmt.Sprint(value)
	}
}

func rowIsEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}
