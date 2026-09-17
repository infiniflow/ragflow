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

package document

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"ragflow/internal/parser/parser"
)

// probeXLSXMaxBytes bounds the workbook stream the probe reads, mirroring the
// Python endpoint's max_excel_probe_bytes. A larger upload cannot be opened
// from a truncated archive, so the probe reports an error and the client falls
// back to local extraction instead of the server reading an unbounded stream.
const probeXLSXMaxBytes = 32 * 1024 * 1024

// ProbeTable extracts the column names from a table file (CSV, TSV, XLSX) from
// its leading rows only, delegating the header rules to the parser that indexes
// the file: parser.ProbeDelimitedColumnNames reads a delimited header through
// the CSV parser's own reader, and parser.ProbeSpreadsheetColumnNames applies
// the spreadsheet rule. The columns offered for configuration are therefore
// exactly the columns ingestion creates.
// Binary XLS (BIFF8) is not supported for streaming probe and returns an error,
// enabling client-side extraction fallback.
func (s *DocumentService) ProbeTable(r io.Reader, filename string) ([]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv", ".tsv", ".txt":
		// Bound header probing to the first 1MB of text stream.
		return parser.ProbeDelimitedColumnNames(io.LimitReader(r, 1024*1024), filename)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return parser.ProbeSpreadsheetColumnNames(io.LimitReader(r, probeXLSXMaxBytes))
	case ".xls":
		return nil, fmt.Errorf("server probe does not support binary xls format: fallback to client probe")
	default:
		return nil, fmt.Errorf("unsupported table format: %s", ext)
	}
}
