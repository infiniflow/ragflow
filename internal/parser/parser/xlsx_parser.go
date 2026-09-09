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
	"strings"

	"github.com/xuri/excelize/v2"
)

type XLSXParser struct {
	libType                        string
	ParseMethod                    string
	OutputFormat                   string
	ChunkRows                      int
	TCADPAPIServer                 string
	TCADPAPIKey                    string
	TCADPTableResultType           string
	TCADPMarkdownImageResponseType string
}

func NewXLSXParser(libType string) (*XLSXParser, error) {
	if libType == "" {
		libType = "excelize"
	}
	return &XLSXParser{
		libType:                        libType,
		ChunkRows:                      defaultTableChunkRows,
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
	p.ChunkRows = decodeChunkRows(setup)
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

	chunkRows := p.ChunkRows
	if chunkRows <= 0 {
		chunkRows = defaultTableChunkRows
	}

	items, warnings, sheets, err := parseXLSXBytes(data, chunkRows)
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
	items, warnings, sheets, retryErr := parseXLSXBytes(normalized, chunkRows)
	if retryErr != nil {
		return ParseResult{Err: fmt.Errorf("xlsx parse: %w; retry after normalization: %v", err, retryErr)}
	}
	warnings = append(normalizeWarnings, warnings...)
	return xlsxParseResult(filename, items, warnings, sheets)
}

func parseXLSXBytes(data []byte, chunkRows int) ([]map[string]any, []string, int, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, 0, fmt.Errorf("open XLSX: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	items := make([]map[string]any, 0)
	warnings := make([]string, 0)
	for sheetIdx, sheet := range sheets {
		tables, sheetWarnings, err := renderSheetTableChunks(f, sheet, chunkRows)
		if err != nil {
			return nil, warnings, len(sheets), err
		}
		warnings = append(warnings, sheetWarnings...)
		for _, table := range tables {
			items = append(items, map[string]any{
				"text":         table.HTML,
				"doc_type_kwd": "table",
				"ck_type":      "table",
				"sheet":        sheet,
				"positions": [][]float64{{
					float64(sheetIdx + 1),
					float64(table.RowStart),
					float64(table.RowEnd),
					float64(table.ColStart),
					float64(table.ColEnd),
				}},
			})
		}
		images, imageWarnings := extractXLSXImages(f, sheet)
		items = append(items, images...)
		warnings = append(warnings, imageWarnings...)
	}
	return items, warnings, len(sheets), nil
}

func xlsxParseResult(filename string, items []map[string]any, warnings []string, sheets int) ParseResult {
	return ParseResult{
		OutputFormat: "json",
		File:         map[string]any{"name": filename, "format": "xlsx", "sheets": sheets},
		JSON:         items,
		Warnings:     warnings,
	}
}
