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
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/parser/parser"
	"ragflow/internal/storage"

	"ragflow/internal/utility"
)

// Column probing reads a spreadsheet through the parser ingestion would use and
// reports the columns that parser produced. It is read-only: no document row, no
// index write, no configuration change.
//
// The limits exist because the request hands a whole workbook to a synchronous
// HTTP call: a file that expands enormously, or a flood of concurrent probes,
// would otherwise cost more than the caller is willing to wait for.
const (
	// TableProbeMaxFileBytes caps the file a column probe will read. Uploads
	// keep their own size rules; this one bounds a synchronous request.
	TableProbeMaxFileBytes     = 32 << 20
	tableProbeMaxExpandedBytes = 256 << 20
	tableProbeMaxSheets        = 256
	tableProbeDeadline         = 30 * time.Second
	tableProbeMaxConcurrent    = 4
)

var tableProbeSlots = make(chan struct{}, tableProbeMaxConcurrent)

// Business codes for probe failures. The numeric transport code stays the
// shared common.ErrorCode family; these say which rule rejected the request.
const (
	TableProbeUnsupportedFormat = "UNSUPPORTED_TABLE_FORMAT"
	TableProbeHeaderNotFound    = "TABLE_HEADER_NOT_FOUND"
	TableProbeParseFailed       = "TABLE_PARSE_FAILED"
	TableProbeLimit             = "TABLE_PROBE_LIMIT"
	TableProbeTimeout           = "TABLE_PROBE_TIMEOUT"
	// TableAccessDenied answers a probe request from someone the dataset is not
	// shared with, the same code the dataset endpoints use for it.
	TableAccessDenied = "DATASET_ACCESS_DENIED"
)

// TableProbeError carries a code plus a human-readable reason.
type TableProbeError struct {
	Code    string
	Message string
}

func (e *TableProbeError) Error() string { return e.Message }

func tableProbeFailure(code, format string, args ...any) error {
	return &TableProbeError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// TableProbeSheet is one sheet's columns. Sheet indexes are 1-based, as
// everywhere else in the spreadsheet wire.
type TableProbeSheet struct {
	SheetIndex int                  `json:"sheet_index"`
	Name       string               `json:"name"`
	RowCount   int                  `json:"row_count"`
	Columns    []entity.TableColumn `json:"columns"`
}

// TableProbeResult is the probe response. Source is "file" for both entry
// points: the columns describe the stored file, not what an index currently
// holds.
type TableProbeResult struct {
	Source   string            `json:"source"`
	Sheets   []TableProbeSheet `json:"sheets"`
	Warnings []string          `json:"warnings"`
}

// ProbeTableColumns reads the columns of one candidate CSV or XLSX file.
func (s *DocumentService) ProbeTableColumns(ctx context.Context, filename string, data []byte) (*TableProbeResult, error) {
	return s.probeTableColumns(ctx, filename, strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), ".")), data)
}

func (s *DocumentService) probeTableColumns(ctx context.Context, filename, extension string, data []byte) (*TableProbeResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, tableProbeFailure(TableProbeTimeout, "column probe cancelled: %v", err)
	}
	switch extension {
	case "csv", "xlsx":
	default:
		// TSV is not routed to the CSV parser, and a .xls extension does not
		// prove the bytes went through a real OLE reader. Neither can carry
		// roles today, so say so rather than guess from the filename.
		return nil, tableProbeFailure(TableProbeUnsupportedFormat,
			"%q cannot be probed: column roles apply to csv and xlsx files only", filename)
	}
	if len(data) > TableProbeMaxFileBytes {
		return nil, tableProbeFailure(TableProbeLimit,
			"%q is %d bytes; column probing accepts files up to %d bytes", filename, len(data), TableProbeMaxFileBytes)
	}

	release, err := acquireTableProbeSlot(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if extension == "xlsx" {
		archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, tableProbeFailure(TableProbeParseFailed, "invalid XLSX archive: %v", err)
		}
		var expanded uint64
		for _, entry := range archive.File {
			if entry.UncompressedSize64 > tableProbeMaxExpandedBytes-expanded {
				return nil, tableProbeFailure(TableProbeLimit, "workbook exceeds %d expanded bytes", tableProbeMaxExpandedBytes)
			}
			expanded += entry.UncompressedSize64
		}
	}

	parseCtx, cancel := context.WithTimeout(ctx, tableProbeDeadline)
	defer cancel()

	items, err := parseSpreadsheetItems(parseCtx, extension, filename, data)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, tableProbeFailure(TableProbeTimeout, "probing %q timed out", filename)
		}
		return nil, tableProbeFailure(TableProbeParseFailed, "%q could not be parsed: %v", filename, err)
	}

	sheets, warnings, err := collectProbeSheets(items)
	if err != nil {
		return nil, err
	}
	return &TableProbeResult{Source: "file", Sheets: sheets, Warnings: warnings}, nil
}

// ProbeDocumentTableColumns reads the columns of a stored document's own file.
func (s *DocumentService) ProbeDocumentTableColumns(ctx context.Context, datasetID, documentID string) (*TableProbeResult, error) {
	doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(ctx, dao.DB, documentID, datasetID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, fmt.Errorf("the dataset doesn't own the document")
		}
		return nil, err
	}
	if doc.Size > TableProbeMaxFileBytes {
		return nil, tableProbeFailure(TableProbeLimit,
			"the stored file is %d bytes; column probing accepts files up to %d bytes", doc.Size, TableProbeMaxFileBytes)
	}
	bucket, key, err := s.GetDocumentStorageAddress(ctx, doc)
	if err != nil {
		return nil, err
	}
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, errors.New("storage not initialized")
	}
	stream, err := storageImpl.Open(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, TableProbeMaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > TableProbeMaxFileBytes {
		return nil, tableProbeFailure(TableProbeLimit, "the stored file exceeds the column probe size limit")
	}
	name := documentID
	if doc.Name != nil {
		name = *doc.Name
	}

	result, err := s.probeTableColumns(ctx, name, strings.ToLower(strings.TrimPrefix(doc.Suffix, ".")), data)
	if err != nil {
		return nil, err
	}
	// The file answers with its own columns; a role set against an earlier
	// version of it can name a column that is no longer there.
	result.Warnings = append(result.Warnings, staleRoleWarnings(doc, result.Sheets)...)
	return result, nil
}

// parseSpreadsheetItems runs the file through the parser ingestion would use for
// it, so a header the probe reports is a header the chunker will see.
func parseSpreadsheetItems(ctx context.Context, extension, filename string, data []byte) ([]map[string]any, error) {
	var result parser.ParseResult
	switch extension {
	case "csv":
		result = parser.NewCSVParser().ParseWithResult(ctx, filename, data)
	case "xlsx":
		xlsxParser, err := parser.NewXLSXParser("")
		if err != nil {
			return nil, err
		}
		result = xlsxParser.ParseWithResult(ctx, filename, data)
	default:
		return nil, fmt.Errorf("no spreadsheet parser for %q", extension)
	}
	if result.Err != nil {
		return nil, result.Err
	}
	// No items is not a parse failure: a file with nothing to read has no
	// header, which is the more accurate answer and the one the client can act
	// on. A corrupt workbook surfaces through result.Err above.
	return result.JSON, nil
}

// collectProbeSheets groups the parsed items by sheet and derives each sheet's
// columns from its header row. Segments of one sheet repeat the header, so the
// widest header seen for a sheet wins and every segment contributes its rows.
func collectProbeSheets(items []map[string]any) ([]TableProbeSheet, []string, error) {
	type sheetState struct {
		name    string
		columns []entity.TableColumn
		rows    int
	}
	bySheet := map[int]*sheetState{}
	// Data rows carrying more cells than their header, per sheet. Manual mode
	// routes cells by column, and a cell past the header has no column to route
	// by, so the chunker refuses such a sheet: the client has to hear it here,
	// before it commits roles to a file that cannot honour them.
	ragged := map[int]int{}
	for _, item := range items {
		if kind, _ := item[parser.DocTypeKey].(string); kind != parser.DocTypeTable {
			continue
		}
		sheetIndex, ok := intField(item["sheet_index"])
		if !ok {
			continue // no sheet identity: not the spreadsheet wire
		}
		markup, _ := item["text"].(string)
		rows, headerCount := utility.HTMLTableRowsWithHeader(markup)
		if headerCount != 1 || len(rows) == 0 {
			continue
		}
		columns := entity.DeriveTableColumns(rows[0])
		state, seen := bySheet[sheetIndex]
		if !seen {
			if len(bySheet) >= tableProbeMaxSheets {
				return nil, nil, tableProbeFailure(TableProbeLimit, "the file has more than %d sheets", tableProbeMaxSheets)
			}
			state = &sheetState{columns: columns}
			bySheet[sheetIndex] = state
		} else if len(columns) > len(state.columns) {
			state.columns = columns
		}
		if name, isName := item["sheet"].(string); isName && state.name == "" {
			state.name = name
		}
		state.rows += len(rows) - headerCount
		if wide := dataRowsWiderThan(rows[0], rows[1:]); wide > 0 {
			ragged[sheetIndex] += wide
		}
	}

	if len(bySheet) == 0 {
		return nil, nil, tableProbeFailure(TableProbeHeaderNotFound, "no column header was found in the file")
	}

	indexes := make([]int, 0, len(bySheet))
	for index := range bySheet {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	sheets := make([]TableProbeSheet, 0, len(indexes))
	warnings := make([]string, 0, len(indexes))
	for _, index := range indexes {
		state := bySheet[index]
		sheet := TableProbeSheet{
			SheetIndex: index,
			Name:       state.name,
			RowCount:   state.rows,
			Columns:    state.columns,
		}
		sheets = append(sheets, sheet)
		if count := ragged[index]; count > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"sheet %s has %d data row(s) with more cells than its %d-column header; column roles cannot route those cells, and a manual-mode parse refuses the sheet",
				probeSheetLabel(index, state.name), count, len(state.columns)))
		}
	}
	return sheets, warnings, nil
}

// dataRowsWiderThan counts the data rows carrying more cells than the header of
// their own sheet segment.
func dataRowsWiderThan(header []string, dataRows [][]string) int {
	width := len(header)
	count := 0
	for _, row := range dataRows {
		if len(row) > width {
			count++
		}
	}
	return count
}

// probeSheetLabel names a sheet for a warning, falling back to its index when
// the wire carries no sheet name.
func probeSheetLabel(index int, name string) string {
	if name == "" {
		return fmt.Sprintf("%d", index)
	}
	return fmt.Sprintf("%d (%s)", index, name)
}

// staleRoleWarnings names configured roles this file has no column for. A
// duplicate header takes its suffix from its position in the header row, so
// reordering a sheet can strand a role under an old key. The backend does not
// migrate roles by itself; the client can use this to ask the user to re-check.
func staleRoleWarnings(doc *entity.Document, sheets []TableProbeSheet) []string {
	if doc == nil || doc.ParserConfig == nil {
		return nil
	}
	known := map[string]struct{}{}
	for _, sheet := range sheets {
		for _, column := range sheet.Columns {
			known[column.Key] = struct{}{}
		}
	}
	var warnings []string
	for key, raw := range doc.ParserConfig {
		if !pipeline.IsTableChunkerNodeKey(key) {
			continue
		}
		params, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		roles, ok := params["column_roles"].(map[string]any)
		if !ok {
			continue
		}
		for column := range roles {
			if _, found := known[column]; !found {
				warnings = append(warnings, fmt.Sprintf(
					"%s sets a role for %q, which this file has no column for", key, column))
			}
		}
	}
	sort.Strings(warnings)
	return warnings
}

func acquireTableProbeSlot(ctx context.Context) (func(), error) {
	select {
	case tableProbeSlots <- struct{}{}:
		return func() { <-tableProbeSlots }, nil
	default:
	}
	// Busy rather than overcommitted: wait for a free slot up to the same
	// deadline a probe would get, then tell the client to retry instead of
	// queueing unbounded work inside a request.
	timer := time.NewTimer(tableProbeDeadline)
	defer timer.Stop()
	select {
	case tableProbeSlots <- struct{}{}:
		return func() { <-tableProbeSlots }, nil
	case <-ctx.Done():
		return nil, tableProbeFailure(TableProbeTimeout, "column probe cancelled: %v", ctx.Err())
	case <-timer.C:
		return nil, tableProbeFailure(TableProbeLimit, "too many column probes are running; try again shortly")
	}
}

func intField(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
