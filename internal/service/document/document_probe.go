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
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"ragflow/internal/parser/parser"

	"github.com/xuri/excelize/v2"
)

// ProbeTable extracts the column names from a table file (CSV, TSV, XLSX)
// by reading only the initial rows, skipping leading empty rows, and applying
// the same header rules as ingestion (parser.TableColumnHeaderNames), so the
// columns offered for configuration are exactly the columns the parser indexes.
// Binary XLS (BIFF8) is not supported for streaming probe and returns an error,
// enabling client-side extraction fallback.
func (s *DocumentService) ProbeTable(r io.Reader, filename string) ([]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv", ".tsv", ".txt":
		// Bound header probing to the first 1MB of text stream.
		return probeCSV(io.LimitReader(r, 1024*1024), ext == ".tsv")
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return probeXLSX(r)
	case ".xls":
		return nil, fmt.Errorf("server probe does not support binary xls format: fallback to client probe")
	default:
		return nil, fmt.Errorf("unsupported table format: %s", ext)
	}
}

func probeCSV(r io.Reader, isTSV bool) ([]string, error) {
	// A leading UTF-8 BOM must not become part of the first column name. The
	// Python probe decodes with utf-8-sig (api/apps/restful_apis/document_api.py)
	// and the CSV parser strips the BOM as well, so all three paths agree on the
	// discovered names — otherwise a configured column role stops matching the
	// column the parser actually emits.
	buffered := bufio.NewReader(r)
	if prefix, err := buffered.Peek(3); err == nil && bytes.Equal(prefix, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = buffered.Discard(3)
	}

	reader := csv.NewReader(buffered)
	if isTSV {
		reader.Comma = '\t'
	}
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				return []string{}, nil
			}
			return nil, err
		}
		hasContent := false
		for _, cell := range record {
			if strings.TrimSpace(cell) != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			continue
		}

		names, _ := parser.TableColumnHeaderNames(record)
		return names, nil
	}
}

func probeXLSX(r io.Reader) ([]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return []string{}, nil
	}

	rows, err := f.Rows(sheets[0])
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		cols, err := rows.Columns()
		if err != nil {
			continue
		}
		hasContent := false
		for _, cell := range cols {
			if strings.TrimSpace(cell) != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			continue
		}

		names, _ := parser.TableColumnHeaderNames(cols)
		return names, nil
	}

	return []string{}, nil
}
