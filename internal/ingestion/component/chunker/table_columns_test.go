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

package chunker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

// tableColumnChunks drives TableChunker with a column configuration over the
// canonical spreadsheet wire and returns the emitted chunk maps.
func tableColumnChunks(t *testing.T, params map[string]any, fileType string, items ...map[string]any) []map[string]any {
	t.Helper()
	comp, err := NewTableChunker(params)
	if err != nil {
		t.Fatalf("NewTableChunker: %v", err)
	}
	out, err := comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "orders." + fileType,
		"file_type":     fileType,
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if msg, ok := out["_ERROR"].(string); ok {
		t.Fatalf("_ERROR set: %s", msg)
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks not []map[string]any: %T", out["chunks"])
	}
	return chunks
}

// tableColumnError returns the invocation failure for invalid column routing.
func tableColumnError(t *testing.T, params map[string]any, fileType string, items ...map[string]any) string {
	t.Helper()
	comp, err := NewTableChunker(params)
	if err != nil {
		t.Fatalf("NewTableChunker: %v", err)
	}
	_, err = comp.Invoke(t.Context(), nil, map[string]any{
		"name":          "orders." + fileType,
		"file_type":     fileType,
		"output_format": "json",
		"json":          items,
	})
	if err == nil {
		t.Fatal("invalid column routing returned success")
	}
	return err.Error()
}

func chunkDataRow(t *testing.T, ck map[string]any) map[string]any {
	t.Helper()
	data, ok := ck["chunk_data"].(map[string]any)
	if !ok {
		t.Fatalf("chunk_data is %T, want map", ck["chunk_data"])
	}
	return data
}

func rowSourceMap(t *testing.T, ck map[string]any) map[string]any {
	t.Helper()
	src, ok := ck["table_row_source"].(map[string]any)
	if !ok {
		t.Fatalf("table_row_source is %T, want map", ck["table_row_source"])
	}
	return src
}

var ordersSegment = []string{"ID", "Status"}

// TestTableChunkerAutoWritesChunkDataForEveryColumn: auto puts each column in
// body text and chunk_data, keyed by the JSON-safe data key rather than the
// header text. The body stays the line format the table method has always
// emitted, so an existing document re-parses into the same text.
func TestTableChunkerAutoWritesChunkDataForEveryColumn(t *testing.T) {
	segment := spreadsheetSegmentItem("orders", ordersSegment, [][]string{{"A-1", "paid"}}, 1, 2)
	chunks := tableColumnChunks(t, nil, "xlsx", segment)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	ck := chunks[0]
	if ck["text"] != "- ID: A-1\n- Status: paid" {
		t.Errorf("text = %v", ck["text"])
	}
	data := chunkDataRow(t, ck)
	want := map[string]any{"ID": "A-1", "Status": "paid"}
	for name, value := range want {
		if got := data[entity.TableDataKey(name)]; got != value {
			t.Errorf("chunk_data[%s] = %v, want %v", name, got, value)
		}
	}
	if len(data) != len(want) {
		t.Errorf("chunk_data has %d keys, want %d: %v", len(data), len(want), data)
	}
	if ck["table_row_int"] != float64(1) {
		t.Errorf("table_row_int = %v, want 1", ck["table_row_int"])
	}
	src := rowSourceMap(t, ck)
	if src["mode"] != "auto" || src["sheet_index"] != float64(1) || src["source_row"] != float64(2) {
		t.Errorf("table_row_source = %v", src)
	}
}

// TestTableChunkerManualRoleSplit: a manual role sends each column to exactly
// the places its role names, and a column with no entry behaves as both.
func TestTableChunkerManualRoleSplit(t *testing.T) {
	segment := spreadsheetSegmentItem("orders", ordersSegment, [][]string{{"A-1", "paid"}}, 1, 2)
	chunks := tableColumnChunks(t, map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"ID": "indexing", "Status": "metadata"},
	}, "xlsx", segment)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	ck := chunks[0]
	if ck["text"] != "- ID: A-1" {
		t.Errorf("indexing column must be the only body line, got %q", ck["text"])
	}
	data := chunkDataRow(t, ck)
	if data[entity.TableDataKey("Status")] != "paid" {
		t.Errorf("metadata column missing from chunk_data: %v", data)
	}
	if _, ok := data[entity.TableDataKey("ID")]; ok {
		t.Errorf("indexing column must not enter chunk_data: %v", data)
	}
	src := rowSourceMap(t, ck)
	roles, _ := src["roles"].(map[string]any)
	if roles["ID"] != "indexing" || roles["Status"] != "metadata" {
		t.Errorf("row carries the configured roles, got %v", roles)
	}
}

// TestTableChunkerManualMetadataOnlyRowKeepsLocatorText: when every body
// column is excluded, the row still indexes, with text that locates it without
// repeating any excluded cell value.
func TestTableChunkerManualMetadataOnlyRowKeepsLocatorText(t *testing.T) {
	segment := spreadsheetSegmentItem("orders", []string{"金额"}, [][]string{{"100"}}, 1, 3)
	chunks := tableColumnChunks(t, map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"金额": "metadata"},
	}, "xlsx", segment)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want the row kept for its structured value", len(chunks))
	}
	ck := chunks[0]
	if ck["text"] != "Sheet 1, row 3" {
		t.Errorf("text = %q, want the sheet/row locator", ck["text"])
	}
	if strings.Contains(ck["text"].(string), "100") {
		t.Errorf("locator text leaked the excluded cell value: %q", ck["text"])
	}
	if got := chunkDataRow(t, ck)[entity.TableDataKey("金额")]; got != "100" {
		t.Errorf("chunk_data[金额] = %v, want 100", got)
	}
}

// TestTableChunkerManualEmptyCellWrittenAsEmptyString: the row's field set is
// the sheet's column set, so a missing value is an empty string rather than an
// absent key.
func TestTableChunkerManualEmptyCellWrittenAsEmptyString(t *testing.T) {
	segment := spreadsheetSegmentItem("orders", ordersSegment, [][]string{{"A-2", ""}}, 1, 2)
	chunks := tableColumnChunks(t, map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"Status": "metadata"},
	}, "xlsx", segment)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if got := chunkDataRow(t, chunks[0])[entity.TableDataKey("Status")]; got != "" {
		t.Errorf("chunk_data[Status] = %q, want empty string", got)
	}
}

// TestTableChunkerManualDropsAllEmptyRows: a row with no content at all is
// skipped, not indexed as an empty locator.
func TestTableChunkerManualDropsAllEmptyRows(t *testing.T) {
	segment := spreadsheetSegmentItem("orders", ordersSegment, [][]string{{"", ""}}, 1, 2)
	chunks := tableColumnChunks(t, map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"Status": "metadata"},
	}, "xlsx", segment)
	if len(chunks) != 0 {
		t.Errorf("got %d chunks, want the empty row skipped: %#v", len(chunks), chunks)
	}
}

// TestTableChunkerManualHeaderOnlyEmitsNoWholeTable: a manual configuration
// must never fall back to the unfiltered table markup, which would index every
// excluded column.
func TestTableChunkerManualHeaderOnlyEmitsNoWholeTable(t *testing.T) {
	segment := spreadsheetSegmentItem("orders", ordersSegment, nil, 1, 1)
	chunks := tableColumnChunks(t, map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"Status": "metadata"},
	}, "xlsx", segment)
	if len(chunks) != 0 {
		t.Fatalf("got %d chunks, want none: %#v", len(chunks), chunks)
	}
	for _, ck := range chunks {
		if strings.Contains(strings.ToLower(ck["text"].(string)), "<table") {
			t.Errorf("manual mode fell back to whole-table markup: %v", ck["text"])
		}
	}
}

// TestTableChunkerManualRejectsUnsupportedPaths: column roles are defined
// against the canonical spreadsheet wire, so a file that is not routed through
// the CSV/XLSX parser, or a table item that carries no sheet identity, fails
// loudly instead of quietly indexing every column.
func TestTableChunkerManualRejectsUnsupportedPaths(t *testing.T) {
	manual := map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"Status": "metadata"},
	}
	rowSegment := spreadsheetSegmentItem("orders", ordersSegment, [][]string{{"A-1", "paid"}}, 1, 2)
	pdfTable := map[string]any{
		"text":         "<table><tr><th>ID</th></tr><tr><td>a</td></tr></table>",
		"doc_type_kwd": "table",
		"ck_type":      "table",
	}
	cases := []struct {
		name     string
		fileType string
		item     map[string]any
		contains string
	}{
		{"tsv is not routed to the CSV parser", "tsv", rowSegment, "not supported"},
		{"binary xls has no verified Go wire", "xls", rowSegment, "not supported"},
		{"table without sheet identity", "csv", pdfTable, "spreadsheet table wire"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := tableColumnError(t, manual, c.fileType, c.item)
			if msg == "" {
				t.Fatal("expected _ERROR, chunker accepted the path")
			}
			if !strings.Contains(msg, c.contains) {
				t.Errorf("error %q does not mention %q", msg, c.contains)
			}
		})
	}
}

// TestTableChunkerManualRejectsMisalignedPositions: without a tuple per
// <tr> there is no reliable source row, so the row cannot be identified.
func TestTableChunkerManualRejectsMisalignedPositions(t *testing.T) {
	item := map[string]any{
		"text":         "<table><tr><th>ID</th></tr><tr><td>a</td></tr><tr><td>b</td></tr></table>",
		"doc_type_kwd": "table",
		"sheet_index":  1,
		"positions":    [][]float64{{1, 1, 2, 1, 1}},
	}
	msg := tableColumnError(t, map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"ID": "metadata"},
	}, "xlsx", item)
	if !strings.Contains(msg, "row-aligned") {
		t.Errorf("error %q does not explain the alignment requirement", msg)
	}
}

// TestTableChunkerManualRejectsRowWiderThanHeader: under manual the roles name
// columns, so a trailing cell with no column identity has no role to route by.
// The refusal reports how many rows are affected, where the first one is, and
// both ways out, rather than reading as a defect in the file.
func TestTableChunkerManualRejectsRowWiderThanHeader(t *testing.T) {
	item := map[string]any{
		"text":         "<table><tr><th>ID</th></tr><tr><td>a</td><td>extra</td></tr><tr><td>b</td><td>extra</td></tr></table>",
		"doc_type_kwd": "table",
		"sheet_index":  1,
		"positions":    [][]float64{{1, 1, 1, 1, 1}, {1, 2, 2, 1, 2}, {1, 3, 3, 1, 3}},
	}
	for _, tc := range []struct {
		fileType string
		hint     string // the repair for that wire: a CSV quotes the cell, a sheet needs a header
	}{
		{"csv", "quote the value holding a comma"},
		{"xlsx", "give those cells header columns"},
	} {
		msg := tableColumnError(t, map[string]any{
			"column_mode":  "manual",
			"column_roles": map[string]any{"ID": "both"},
		}, tc.fileType, item)
		for _, want := range []string{
			"2 of 2 data row(s)",      // the extent, not only the first offending row
			"1-column header",         // the width the cells overrun
			"sheet 1, first at row 2", // where the first one is
			`column mode "manual"`,    // which setting refused it
			tc.hint,                   // what to change in the file
			`column mode "auto"`,      // the way out that keeps the cells
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: error %q does not report %q", tc.fileType, msg, want)
			}
		}
	}
}

// TestTableChunkerAutoKeepsCellsPastTheHeader: auto indexes every column, so a
// row with more cells than its header keeps their text the way this path did
// before column modes existed. Refusing the document over a ragged row would
// cost every other row with it, and chunk_data must not grow a field the
// header never named.
func TestTableChunkerAutoKeepsCellsPastTheHeader(t *testing.T) {
	// Both wires the review named: a dirty CSV or XLSX must not cost the whole
	// document just because one row is wider than its header.
	for _, fileType := range []string{"csv", "xlsx"} {
		segment := spreadsheetSegmentItem("orders", []string{"ID"}, [][]string{{"A-1", "extra"}}, 1, 2)
		chunks := tableColumnChunks(t, nil, fileType, segment)
		if len(chunks) != 1 {
			t.Fatalf("%s: got %d chunks, want 1", fileType, len(chunks))
		}
		ck := chunks[0]
		if got := ck["text"]; got != "- ID: A-1\n- extra" {
			t.Errorf("%s: text = %v, want the extra cell kept in the body", fileType, got)
		}
		data := chunkDataRow(t, ck)
		if _, ok := data[entity.TableDataKey("ID")]; !ok {
			t.Errorf("%s: chunk_data lost the header's column: %v", fileType, data)
		}
		if len(data) != 1 {
			t.Errorf("%s: chunk_data = %v, want only the columns the header named", fileType, data)
		}
	}
}

// TestTableChunkerRowIdentitySurvivesRoleChange: the chunk id follows the
// source row, so re-parsing with different roles keeps the same identity, and
// two rows that render identical text stay distinct.
func TestTableChunkerRowIdentitySurvivesRoleChange(t *testing.T) {
	segment := spreadsheetSegmentItem("orders", ordersSegment, [][]string{{"A-1", "paid"}}, 1, 2)
	auto := tableColumnChunks(t, nil, "xlsx", segment)
	manual := tableColumnChunks(t, map[string]any{
		"column_mode":  "manual",
		"column_roles": map[string]any{"ID": "metadata"},
	}, "xlsx", segment)

	autoID, ok := schema.TableRowIdentity(auto[0])
	if !ok {
		t.Fatalf("auto row has no identity: %#v", auto[0])
	}
	manualID, ok := schema.TableRowIdentity(manual[0])
	if !ok {
		t.Fatalf("manual row has no identity: %#v", manual[0])
	}
	if autoID != manualID {
		t.Errorf("identity changed with roles: %q vs %q", autoID, manualID)
	}
	if autoID != "table-row:v1:1:2" {
		t.Errorf("identity = %q, want table-row:v1:1:2", autoID)
	}

	// Identical body text from different source rows.
	dup := spreadsheetSegmentItem("orders", ordersSegment, [][]string{{"A-1", "paid"}, {"A-1", "paid"}}, 1, 2)
	chunks := tableColumnChunks(t, nil, "xlsx", dup)
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want both rows", len(chunks))
	}
	first, _ := schema.TableRowIdentity(chunks[0])
	second, _ := schema.TableRowIdentity(chunks[1])
	if first == second {
		t.Errorf("two source rows with the same text share an identity: %q", first)
	}
	if tableRowInt := chunks[1]["table_row_int"]; tableRowInt != float64(1) {
		t.Errorf("table_row_int = %v, want 1", tableRowInt)
	}
}

// TestTableChunkerDuplicateAndEmptyHeadersGetDistinctKeys: chunk_data keys
// must stay addressable when two columns share a header name or a header is
// blank, and the body shows a readable name for each.
func TestTableChunkerDuplicateAndEmptyHeadersGetDistinctKeys(t *testing.T) {
	header := []string{"金额", "金额", ""}
	segment := spreadsheetSegmentItem("orders", header, [][]string{{"100", "200", "note"}}, 1, 2)
	chunks := tableColumnChunks(t, nil, "xlsx", segment)
	ck := chunks[0]
	if ck["text"] != "- 金额: 100\n- 金额 (2): 200\n- 列 3: note" {
		t.Errorf("text = %q", ck["text"])
	}
	cols := entity.DeriveTableColumns(header)
	data := chunkDataRow(t, ck)
	want := map[string]string{cols[0].DataKey: "100", cols[1].DataKey: "200", cols[2].DataKey: "note"}
	if len(cols) != 3 || cols[1].Key == cols[0].Key || cols[2].Key == "" {
		t.Fatalf("column keys are not distinct: %#v", cols)
	}
	for key, value := range want {
		if data[key] != value {
			t.Errorf("chunk_data[%q] = %v, want %q", key, data[key], value)
		}
	}
}

// TestTableChunkerColumnParamsChangeChunkDataNotPlainText: with no role
// configuration at all a spreadsheet row keeps the body text the table method
// has always produced, so enabling the feature for auto documents cannot alter
// retrieval of existing content.
func TestTableChunkerColumnParamsChangeChunkDataNotPlainText(t *testing.T) {
	segment := spreadsheetSegmentItem("orders", ordersSegment, [][]string{{"A-1", "paid"}}, 1, 2)
	plain := tableColumnChunks(t, nil, "xlsx", segment)
	if plain[0]["text"] != "- ID: A-1\n- Status: paid" {
		t.Errorf("auto body changed: %q", plain[0]["text"])
	}
}

func TestTableChunkerHonorsCancellation(t *testing.T) {
	c, err := NewTableChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	out, err := c.Invoke(ctx, nil, map[string]any{"name": "table.txt", "output_format": "text", "text": "table"})
	if !errors.Is(err, context.Canceled) || out != nil {
		t.Fatalf("cancelled invocation succeeded: %v %v", out, err)
	}
}

func TestChunkerInvalidInputFails(t *testing.T) {
	for _, name := range []string{"QA", "One", "Page"} {
		t.Run(name, func(t *testing.T) {
			var out map[string]any
			var err error
			switch name {
			case "QA":
				c, e := NewQAChunker(nil)
				if e != nil {
					t.Fatal(e)
				}
				out, err = c.Invoke(t.Context(), nil, map[string]any{})
			case "One":
				c, e := NewOneChunker(nil)
				if e != nil {
					t.Fatal(e)
				}
				out, err = c.Invoke(t.Context(), nil, map[string]any{})
			case "Page":
				c, e := NewPageChunker(nil)
				if e != nil {
					t.Fatal(e)
				}
				out, err = c.Invoke(t.Context(), nil, map[string]any{})
			}
			if err == nil || out != nil {
				t.Fatalf("invalid input succeeded: %v %v", out, err)
			}
		})
	}
}

type rowCancelContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *rowCancelContext) Err() error {
	c.checks++
	if c.checks == 5 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestTableChunkerCancelsDuringRowExpansion(t *testing.T) {
	base, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &rowCancelContext{Context: base, cancel: cancel}
	c, err := NewTableChunker(nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([][]string, 20)
	for i := range rows {
		rows[i] = []string{"id", "paid"}
	}
	item := spreadsheetSegmentItem("orders", ordersSegment, rows, 1, 2)
	output, err := c.Invoke(ctx, nil, map[string]any{"name": "orders.csv", "file_type": "csv", "output_format": "json", "json": []map[string]any{item}})
	if !errors.Is(err, context.Canceled) || output != nil || ctx.checks != 5 {
		t.Fatalf("row expansion did not stop: checks=%d output=%v err=%v", ctx.checks, output, err)
	}
}
