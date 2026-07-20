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
	"encoding/csv"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// Go port of Python's ILLEGAL_CHARACTERS_RE from
// deepdoc/parser/excel_parser.py.
// Pattern: [\000-\010]|[\013-\014]|[\016-\037]
// Matches all control chars except TAB (\x09), LF (\x0A), CR (\x0D).
var csvIllegalCharsRe = regexp.MustCompile(`[\x00-\x08]|\x0B|\x0C|[\x0E-\x1F]`)

const csvDefaultChunkRows = 256
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
func (p *CSVParser) ParseWithResult(filename string, data []byte) ParseResult {
	method := normalizeXLSXParseMethod(p.ParseMethod)
	switch method {
	case "tcadp":
		return parseSpreadsheetWithTCADP(
			filename, data, "CSV",
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

	text := string(data)
	if strings.TrimSpace(text) == "" {
		return ParseResult{
			OutputFormat: "html",
			File: map[string]any{
				"name":     filename,
				"size":     len(data),
				"encoding": "utf-8",
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
	records = cleanCSVRecords(records)

	chunkRows := p.ChunkRows
	if chunkRows <= 0 {
		chunkRows = csvDefaultChunkRows
	}

	return ParseResult{
		OutputFormat: "html",
		File: map[string]any{
			"name":     filename,
			"size":     len(data),
			"encoding": "utf-8",
		},
		HTML: recordsToHTMLTableChunks(records, chunkRows),
	}
}

// cleanCSVRecords replaces illegal control characters in all cells
// with a single space, matching Python's ILLEGAL_CHARACTERS_RE.
func cleanCSVRecords(records [][]string) [][]string {
	out := make([][]string, len(records))
	for i, row := range records {
		out[i] = make([]string, len(row))
		for j, cell := range row {
			out[i][j] = csvIllegalCharsRe.ReplaceAllString(cell, " ")
		}
	}
	return out
}

// recordsToHTMLTableChunks renders CSV records as one or more
// self-contained HTML <table> chunks. The first row is always the
// header (<th>). Data rows are split into chunks of chunkRows,
// each chunk being a complete <table> with <caption> and a repeated
// header row. Chunks are joined with newlines.
//
// Mirrors Python's RAGFlowExcelParser.html() chunking:
//
//	chunks = (n_data_rows + chunk_rows - 1) // chunk_rows
func recordsToHTMLTableChunks(records [][]string, chunkRows int) string {
	if len(records) == 0 {
		return "<table><caption>" + csvSheetName + "</caption></table>"
	}

	// Build the header row once — repeated in every chunk.
	headerHTML := buildCSVHeaderRow(records[0])
	dataRows := records[1:]
	nData := len(dataRows)

	if nData == 0 {
		// Only a header row exists.
		return "<table><caption>" + csvSheetName + "</caption>\n" + headerHTML + "\n</table>"
	}

	nChunks := (nData + chunkRows - 1) / chunkRows
	var b strings.Builder
	for ci := 0; ci < nChunks; ci++ {
		start := ci * chunkRows
		end := start + chunkRows
		if end > nData {
			end = nData
		}

		b.WriteString("<table><caption>")
		b.WriteString(csvSheetName)
		b.WriteString("</caption>\n")
		b.WriteString(headerHTML)

		for _, row := range dataRows[start:end] {
			b.WriteString("<tr>")
			for _, cell := range row {
				b.WriteString("<td>")
				b.WriteString(html.EscapeString(strings.TrimSpace(cell)))
				b.WriteString("</td>")
			}
			b.WriteString("</tr>\n")
		}
		b.WriteString("</table>\n")
	}
	return b.String()
}

// buildCSVHeaderRow renders the first row as an HTML <th> header row.
func buildCSVHeaderRow(row []string) string {
	var b strings.Builder
	b.WriteString("<tr>")
	for _, cell := range row {
		b.WriteString("<th>")
		b.WriteString(html.EscapeString(strings.TrimSpace(cell)))
		b.WriteString("</th>")
	}
	b.WriteString("</tr>\n")
	return b.String()
}
