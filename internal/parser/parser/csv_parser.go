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

// CSVParser renders CSV data as HTML tables, matching the spreadsheet
// family output_format == "html" convention from ParserParam.Defaults().
//
// Mirrors Python's deepdoc/parser/excel_parser.py:RAGFlowExcelParser.html():
//   - CSV data is rendered as an HTML <table> with <caption> "Data".
//   - The first row is treated as the header (<th>).
//   - Illegal control characters are replaced with spaces.
//   - Large sheets are split into chunks of chunk_rows data rows, each
//     chunk being a self-contained <table> with its own <caption> and
//     repeated header row.
//
// It implements the ParseResultProducer contract so the dispatch seam in
// parser_dispatch.go routes .csv files through the structured path.

package parser

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"
)

const csvDefaultChunkRows = defaultTableChunkRows
const csvSheetName = "Data"

// CSVParser reads RFC-4180 CSV data and emits HTML <table> payloads.
type CSVParser struct {
	ParseMethod                    string
	OutputFormat                   string
	ChunkRows                      int
	TCADPAPIServer                 string
	TCADPAPIKey                    string
	TCADPTableResultType           string
	TCADPMarkdownImageResponseType string
}

func NewCSVParser() *CSVParser {
	return &CSVParser{
		ChunkRows:                      csvDefaultChunkRows,
		TCADPTableResultType:           "1",
		TCADPMarkdownImageResponseType: "1",
	}
}

func (p *CSVParser) String() string {
	return "CSVParser"
}

func (p *CSVParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["parse_method"].(string); ok && v != "" {
		p.ParseMethod = v
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.OutputFormat = v
	}
	if v, ok := setup["chunk_rows"]; ok {
		switch n := v.(type) {
		case float64:
			p.ChunkRows = int(n)
		case int:
			p.ChunkRows = n
		case int64:
			p.ChunkRows = int(n)
		}
		if p.ChunkRows <= 0 {
			p.ChunkRows = csvDefaultChunkRows
		}
	}
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

// ParseWithResult implements ParseResultProducer. It reads CSV rows
// and renders them as HTML <table> chunks with <caption>, header row
// repeated per chunk, and illegal-character filtering — mirroring
// Python's RAGFlowExcelParser.html().
// When TCADP parse_method is configured, the file is dispatched to
// the Tencent Cloud Document Parsing API.
func (p *CSVParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	method := normalizeXLSXParseMethod(p.ParseMethod)
	switch method {
	case "tcadp":
		return parseWithTCADP(
			ctx, filename, data, "CSV",
			p.TCADPAPIServer, p.TCADPAPIKey,
			p.TCADPTableResultType, p.TCADPMarkdownImageResponseType,
			p.OutputFormat,
		)
	case "", "csv":
		// Continue with the local CSV parser.
	default:
		// PDF-specific methods like "DeepDOC" / "PaddleOCR" / "MinerU"
		// are meaningless for CSV; treat them as the default CSV path,
		// matching Python's behaviour where parse_method is irrelevant
		// for CSV processing.
	}

	decoded, encName := DecodeToUTF8(data, "text/csv")
	text := string(decoded)
	if strings.TrimSpace(text) == "" {
		return ParseResult{
			OutputFormat: "html",
			File: map[string]any{
				"name":     filename,
				"size":     len(data),
				"encoding": encName,
			},
			HTML: "<table><caption>" + csvSheetName + "</caption><tr><td></td></tr></table>",
		}
	}

	reader := csv.NewReader(strings.NewReader(text))
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Allow variable column counts, matching Python csv.reader behaviour.

	records, err := reader.ReadAll()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("csv parse: %w", err)}
	}

	// Clean illegal control characters from all cells.
	records = cleanIllegalControlChars(records)

	chunkRows := p.ChunkRows
	if chunkRows <= 0 {
		chunkRows = csvDefaultChunkRows
	}

	return ParseResult{
		OutputFormat: "html",
		File: map[string]any{
			"name":     filename,
			"size":     len(data),
			"encoding": encName,
		},
		HTML: recordsToHTMLTableChunks(records, chunkRows, csvSheetName),
	}
}
