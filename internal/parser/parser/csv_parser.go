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
	"unicode/utf8"
)

const csvSheetName = "Data"

// csvDelimiters are the separators a spreadsheet actually writes into a file
// named ".csv". Excel writes the list separator of the machine's locale, which
// is a semicolon across most of Europe, and a tab separated export is routinely
// saved as .csv. Mirrors CSV_DELIMITERS in deepdoc/parser/excel_parser.py.
var csvDelimiters = []rune{',', ';', '\t', '|'}

// How much of the file the separator detection looks at. A separator that
// holds for the first rows holds for the file.
const (
	csvSampleBytes = 64 * 1024
	csvSampleRows  = 20
)

// CSVParser reads RFC-4180 CSV data and emits structured table JSON items.
type CSVParser struct {
	ParseMethod                    string
	OutputFormat                   string
	HTML4Excel                     bool
	TCADPAPIServer                 string
	TCADPAPIKey                    string
	TCADPTableResultType           string
	TCADPMarkdownImageResponseType string
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
}

// ParseWithResult implements ParseResultProducer. It reads CSV rows and emits
// a header item followed by ordered data-row items.
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

	records, err := newCSVReader(text, detectCSVDelimiter(text)).ReadAll()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("csv parse: %w", err)}
	}

	// Clean illegal control characters from all cells.
	records = cleanIllegalControlChars(records)

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

// newCSVReader reads text with the given separator, leniently, the same way for
// the separator detection and for the rows themselves.
func newCSVReader(text string, comma rune) *csv.Reader {
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = comma
	reader.LazyQuotes = true
	// TrimLeadingSpace also trims a tab when the tab is the separator, which
	// would swallow the empty field in "Widget\t\t12".
	reader.TrimLeadingSpace = comma != '\t'
	reader.FieldsPerRecord = -1 // Allow variable column counts, matching Python csv.reader behaviour.
	return reader
}

// detectCSVDelimiter returns the separator text was written with, defaulting
// to a comma. encoding/csv reads with a comma by default, and reading a
// semicolon separated export that way does not fail: every row becomes a
// single column holding the whole line, separators included.
func detectCSVDelimiter(text string) rune {
	sample, truncated := text, false
	if len(text) > csvSampleBytes {
		cut := csvSampleBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		sample, truncated = text[:cut], true
	}
	best, bestColumns := ',', 0
	for _, delimiter := range csvDelimiters {
		if columns := consistentColumnCount(sample, delimiter, truncated); columns > bestColumns {
			best, bestColumns = delimiter, columns
		}
	}
	return best
}

// consistentColumnCount returns the columns per row under delimiter, or 0 when
// the rows disagree. A separator the file was not written with either does not
// occur at all (one column) or occurs by accident, and then the rows do not
// line up. Requiring the same count on every row is what keeps a comma inside a
// sentence, or a semicolon inside a quoted field, from being read as a
// separator.
//
// When truncated is set, the sample is a prefix of a longer file, so the row it
// ends in stops wherever the read did, between two fields or inside a quoted
// one. That row is left out rather than counted as having fewer columns, also
// when it is the last of the csvSampleRows rows sampled.
func consistentColumnCount(sample string, delimiter rune, truncated bool) int {
	reader := newCSVReader(sample, delimiter)
	var rows [][]string
	for len(rows) < csvSampleRows {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if !isBlankCSVRow(row) {
			rows = append(rows, row)
		}
	}
	if truncated && len(rows) > 1 && reader.InputOffset() == int64(len(sample)) {
		rows = rows[:len(rows)-1]
	}
	count := 0
	for _, row := range rows {
		if count != 0 && len(row) != count {
			return 0
		}
		count = len(row)
	}
	if count > 1 {
		return count
	}
	return 0
}

// isBlankCSVRow reports whether every cell of row is empty or whitespace; such
// a row says nothing about the separator.
func isBlankCSVRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}
