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
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

type XLSXParser struct {
	libType                        string
	ParseMethod                    string
	OutputFormat                   string
	HTML4Excel                     bool
	TCADPAPIServer                 string
	TCADPAPIKey                    string
	TCADPTableResultType           string
	TCADPMarkdownImageResponseType string
	ColumnMode                     string
	ColumnRoles                    map[string]string
}

func NewXLSXParser(libType string) (*XLSXParser, error) {
	if libType == "" {
		libType = "excelize"
	}
	return &XLSXParser{
		libType:                        libType,
		TCADPTableResultType:           "1",
		TCADPMarkdownImageResponseType: "1",
	}, nil
}

func (p *XLSXParser) String() string {
	return "XLSXParser"
}

func (p *XLSXParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["parse_method"].(string); ok && v != "" {
		p.ParseMethod = v
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.OutputFormat = v
	}
	if v, ok := setup["html4excel"].(bool); ok {
		p.HTML4Excel = v
	}
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
	if mode, roles := DecodeTableColumnConfig(setup); mode != "" || roles != nil {
		if mode != "" {
			p.ColumnMode = mode
		}
		if roles != nil {
			p.ColumnRoles = roles
		}
	}
}

func normalizeXLSXParseMethod(raw string) string {
	method := strings.ToLower(strings.TrimSpace(raw))
	if method == "tcadp parser" {
		return "tcadp"
	}
	// "deepdoc" / "deepdoc parser" is the default spreadsheet parse_method
	// (see schema.ParserParam.Defaults and the matching Python ParserParam),
	// and the DSL templates ship "DeepDOC". Normalize to "" so the default
	// Excelize/CSV path is taken by all three spreadsheet parsers, mirroring
	// rag/flow/parser/parser.py:_spreadsheet which only special-cases
	// "tcadp parser" and routes every other value to the default parser.
	if method == "deepdoc" || method == "deepdoc parser" {
		return ""
	}
	return method
}

func (p *XLSXParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	method := normalizeXLSXParseMethod(p.ParseMethod)
	switch method {
	case "tcadp":
		return parseWithTCADP(
			ctx, filename, data, "XLSX",
			p.TCADPAPIServer, p.TCADPAPIKey,
			p.TCADPTableResultType, p.TCADPMarkdownImageResponseType,
			p.OutputFormat,
		)
	case "", "excelize":
		// Continue with the local Excelize parser.
	default:
		// PDF-specific methods like "DeepDOC" / "PaddleOCR" / "MinerU"
		// are meaningless for XLSX; treat them as the default excelize path,
		// matching Python's behaviour where parse_method is irrelevant
		// for spreadsheet processing.
	}

	// Structured JSON row rendering applies only to the JSON output format: an
	// html/markdown canvas setup keeps its legacy rendering. A setup that states
	// no column_mode renders auto — every column lands in text and chunk_data,
	// which is what rag/app/table.py does when table_column_mode is not manual.
	if strings.EqualFold(p.OutputFormat, "json") {
		items, allColumns, warnings, sheets, err := parseXLSXRowsJSON(data, p.ColumnMode, p.ColumnRoles)
		if err == nil {
			return xlsxRowParseResult(filename, items, allColumns, warnings, sheets)
		}

		normalized, normalizeWarnings, changed, normalizeErr := normalizeXLSXForRead(data)
		if normalizeErr != nil {
			return ParseResult{Err: fmt.Errorf("xlsx parse: %w; normalize: %v", err, normalizeErr)}
		}
		if !changed {
			return ParseResult{Err: fmt.Errorf("xlsx parse: %w", err)}
		}
		items, allColumns, retryWarnings, sheets, retryErr := parseXLSXRowsJSON(normalized, p.ColumnMode, p.ColumnRoles)
		if retryErr != nil {
			return ParseResult{Err: fmt.Errorf("xlsx parse: %w; retry after normalization: %v", err, retryErr)}
		}
		warnings = append(normalizeWarnings, warnings...)
		warnings = append(warnings, retryWarnings...)
		return xlsxRowParseResult(filename, items, allColumns, warnings, sheets)
	}

	items, warnings, sheets, err := parseXLSXBytes(data, p.HTML4Excel)
	if err == nil {
		return xlsxParseResult(filename, items, warnings, sheets)
	}

	normalized, normalizeWarnings, changed, normalizeErr := normalizeXLSXForRead(data)
	if normalizeErr != nil {
		return ParseResult{Err: fmt.Errorf("xlsx parse: %w; normalize: %v", err, normalizeErr)}
	}
	if !changed {
		return ParseResult{Err: fmt.Errorf("xlsx parse: %w", err)}
	}
	items, warnings, sheets, retryErr := parseXLSXBytes(normalized, p.HTML4Excel)
	if retryErr != nil {
		return ParseResult{Err: fmt.Errorf("xlsx parse: %w; retry after normalization: %v", err, retryErr)}
	}
	warnings = append(normalizeWarnings, warnings...)
	return xlsxParseResult(filename, items, warnings, sheets)
}

func parseXLSXRowsJSON(data []byte, columnMode string, columnRoles map[string]string) ([]map[string]any, []string, []string, int, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, nil, 0, fmt.Errorf("open XLSX: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	allItems := make([]map[string]any, 0)
	warnings := make([]string, 0)
	allColSet := make(map[string]struct{})
	allColumns := make([]string, 0)

	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, nil, warnings, len(sheets), fmt.Errorf("read XLSX sheet %q: %w", sheet, err)
		}
		if len(rows) == 0 {
			continue
		}
		rows = cleanIllegalControlChars(rows)
		items, headers := RenderRowsToJSONChunks(rows, sheet, columnMode, columnRoles, TableHeaderRuleSpreadsheet)
		allItems = append(allItems, items...)
		for _, h := range headers {
			if _, ok := allColSet[h]; !ok {
				allColSet[h] = struct{}{}
				allColumns = append(allColumns, h)
			}
		}
	}
	return allItems, allColumns, warnings, len(sheets), nil
}

func xlsxRowParseResult(filename string, items []map[string]any, columns []string, warnings []string, sheets int) ParseResult {
	return spreadsheetRowParseResult(filename, "xlsx", items, columns, warnings, sheets)
}

// ProbeSpreadsheetColumnNames returns the column names parseXLSXRowsJSON would
// index for the given workbook stream: the first row with content of every
// sheet, unioned in the order the columns are first seen, which is the order
// ingestion writes to table_column_names. The caller bounds the reader; a
// header fits the prefix it reads.
//
// A sheet with merged or multi-level headers is parsed hierarchically, which
// this preview cannot reproduce — the parser stays authoritative.
func ProbeSpreadsheetColumnNames(r io.Reader) ([]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	seen := make(map[string]struct{})
	names := make([]string, 0)
	for _, sheet := range f.GetSheetList() {
		rows, err := f.Rows(sheet)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			cols, err := rows.Columns()
			if err != nil {
				continue
			}
			cols = cleanIllegalControlChars([][]string{cols})[0]
			if !TableRowHasContent(cols) {
				continue
			}

			header, _ := TableColumnHeaderNames(cols, TableHeaderRuleSpreadsheet)
			for _, name := range header {
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				names = append(names, name)
			}
			break
		}
		rows.Close()
	}

	return names, nil
}

func spreadsheetRowParseResult(filename, format string, items []map[string]any, columns []string, warnings []string, sheets int) ParseResult {
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":               filename,
			"format":             format,
			"sheets":             sheets,
			"table_column_names": columns,
		},
		JSON:     items,
		Warnings: warnings,
	}
}

func parseXLSXBytes(data []byte, html4excel bool) ([]map[string]any, []string, int, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, 0, fmt.Errorf("open XLSX: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	items := make([]map[string]any, 0)
	warnings := make([]string, 0)
	for sheetIdx, sheet := range sheets {
		records, dataRows, headerRow, sheetWarnings, err := readSpreadsheetRecords(f, sheet)
		if err != nil {
			return nil, warnings, len(sheets), err
		}
		warnings = append(warnings, sheetWarnings...)
		var sheetItems []map[string]any
		if html4excel {
			if table := recordsToHTMLTableItem(records, sheet, sheetIdx+1, headerRow, dataRows); table != nil {
				sheetItems = []map[string]any{table}
			}
		} else {
			sheetItems = recordsToSpreadsheetItems(records, sheet, sheetIdx+1, headerRow, dataRows)
		}
		images, imageWarnings := extractXLSXImages(f, sheet)
		for _, image := range images {
			row, _ := numericItemInt(image["row_start"])
			col, _ := numericItemInt(image["col_start"])
			image["sheet_index"] = sheetIdx + 1
			image["table_id"] = fmt.Sprintf("sheet-%d", sheetIdx+1)
			image["positions"] = [][]float64{{float64(sheetIdx + 1), float64(row), float64(row), float64(col), float64(col)}}
		}
		sheetItems = append(sheetItems, images...)
		sortSpreadsheetItems(sheetItems)
		items = append(items, sheetItems...)
		warnings = append(warnings, imageWarnings...)
	}
	return items, warnings, len(sheets), nil
}

func xlsxParseResult(filename string, items []map[string]any, warnings []string, sheets int) ParseResult {
	return ParseResult{
		OutputFormat: spreadsheetOutputFormat,
		File:         map[string]any{"name": filename, "format": "xlsx", "sheets": sheets},
		JSON:         items,
		Warnings:     warnings,
	}
}
