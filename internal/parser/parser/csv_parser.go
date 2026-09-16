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

// CSVParser emits ordered spreadsheet row items in structured JSON.
// Illegal control characters are replaced with spaces; final chunking is
// owned by GeneralChunker.
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

const csvSheetName = "Data"

// CSVParser reads RFC-4180 CSV data and emits structured table JSON items.
type CSVParser struct {
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

func NewCSVParser() *CSVParser {
	return &CSVParser{
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

// ParseWithResult implements ParseResultProducer. It reads CSV rows and emits
// a header item followed by ordered data-row items.
// When OutputFormat is "json", it renders structured row records
// respecting column_mode and column_roles (one chunk per row).
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
	// Python decodes text with utf-8-sig (rag/nlp/__init__.py decode_text), so a
	// UTF-8 BOM never reaches the first header cell. Column names are the keys
	// of table_column_names and of the column-role map, and the schema probe
	// drops the BOM as well, so the parser must drop it too or every discovered
	// name would carry a leading U+FEFF that no configured role matches.
	text := strings.TrimPrefix(string(decoded), "\uFEFF")
	if strings.TrimSpace(text) == "" {
		if strings.EqualFold(p.OutputFormat, "json") {
			return ParseResult{
				OutputFormat: "json",
				File: map[string]any{
					"name":               filename,
					"size":               len(data),
					"encoding":           encName,
					"table_column_names": []string{},
				},
				JSON: []map[string]any{},
			}
		}
		var emptyJSON []map[string]any
		if p.HTML4Excel {
			emptyJSON = []map[string]any{NewTableJSONItem("<table><caption>Data</caption></table>", csvSheetName, [][]float64{{1, 1, 1, 1, 1}})}
		}
		return ParseResult{
			OutputFormat: spreadsheetOutputFormat,
			File: map[string]any{
				"name":     filename,
				"size":     len(data),
				"encoding": encName,
				"format":   "csv",
				"sheets":   1,
			},
			JSON: emptyJSON,
		}
	}

	reader := csv.NewReader(strings.NewReader(text))
	if strings.HasSuffix(strings.ToLower(filename), ".tsv") || (!strings.Contains(text, ",") && strings.Contains(text, "\t")) {
		reader.Comma = '\t'
	}
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Allow variable column counts, matching Python csv.reader behaviour.

	records, err := reader.ReadAll()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("csv parse: %w", err)}
	}

	// Clean illegal control characters from all cells.
	records = cleanIllegalControlChars(records)

	if strings.EqualFold(p.OutputFormat, "json") && strings.TrimSpace(p.ColumnMode) != "" {
		items, headers := RenderRowsToJSONChunks(records, "", p.ColumnMode, p.ColumnRoles)
		return ParseResult{
			OutputFormat: "json",
			File: map[string]any{
				"name":               filename,
				"size":               len(data),
				"encoding":           encName,
				"table_column_names": headers,
			},
			JSON: items,
		}
	}

	dataRows := make([]int, len(records)-1)
	for i := range dataRows {
		dataRows[i] = i + 2
	}
	var items []map[string]any
	if p.HTML4Excel {
		if table := recordsToHTMLTableItem(records, csvSheetName, 1, 1, dataRows); table != nil {
			items = []map[string]any{table}
		}
	} else {
		items = recordsToSpreadsheetItems(records, csvSheetName, 1, 1, dataRows)
	}
	return ParseResult{
		OutputFormat: spreadsheetOutputFormat,
		File: map[string]any{
			"name":     filename,
			"size":     len(data),
			"encoding": encName,
			"format":   "csv",
			"sheets":   1,
		},
		JSON: items,
	}
}
