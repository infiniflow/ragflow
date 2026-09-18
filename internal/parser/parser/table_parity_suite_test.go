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
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

var standardTableRows = [][]string{
	{"Title", "Content", "Country", "Category", "Year"},
	{"Doc A", "First document text", "Turkey", "Tech", "2024"},
	{"Doc B", "Second document text", "Greece", "Finance", "2023"},
	{"Doc C", "Third document text", "Spain", "Tech", "2024"},
}

// Case: row bookkeeping columns are dropped before rendering, mirroring
// rag/app/table.py, which deletes them from every frame (:539-541).
// Keeping them would index the primary key into the chunk text and into
// chunk_data, and would give the chunk a different id than Python's.
func TestParity_ReservedColumnsDropped(t *testing.T) {
	rows := [][]string{
		{"id", "_id", "index", "idx", "name", "amount"},
		{"1", "2", "3", "4", "alice", "10"},
	}
	items, headers := RenderRowsToJSONChunks(rows, "", "auto", nil, TableHeaderRuleSpreadsheet)

	if !reflect.DeepEqual(headers, []string{"name", "amount"}) {
		t.Fatalf("headers = %#v, want name/amount only", headers)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if text := items[0]["text"]; text != "- name: alice\n- amount: 10" {
		t.Errorf("text = %q", text)
	}
	cd, _ := items[0]["chunk_data"].(map[string]any)
	if len(cd) != 2 || cd["name"] != "alice" || cd["amount"] != "10" {
		t.Errorf("chunk_data = %#v", cd)
	}
}

// Only the exact reserved names are dropped: a similarly named column and an
// empty header (renamed Column_N from its original position) both survive, and
// the surviving columns keep the values of their own source cells.
func TestParity_ReservedColumnsOnlyExactMatch(t *testing.T) {
	rows := [][]string{
		{"id", "", "id_number"},
		{"1", "kept", "42"},
	}
	items, headers := RenderRowsToJSONChunks(rows, "", "auto", nil, TableHeaderRuleSpreadsheet)

	if !reflect.DeepEqual(headers, []string{"Column_2", "id_number"}) {
		t.Fatalf("headers = %#v, want Column_2/id_number", headers)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if text := items[0]["text"]; text != "- Column_2: kept\n- id_number: 42" {
		t.Errorf("text = %q", text)
	}
}

// Case: every header is a reserved name — the rows render to nothing, matching
// Python's empty-DataFrame branch (no text fields, no stored fields).
func TestParity_OnlyReservedColumnsProducesNoItems(t *testing.T) {
	rows := [][]string{
		{"id", "index"},
		{"1", "2"},
	}
	items, headers := RenderRowsToJSONChunks(rows, "", "auto", nil, TableHeaderRuleSpreadsheet)

	if len(headers) != 0 || len(items) != 0 {
		t.Fatalf("headers = %#v, items = %#v; want both empty", headers, items)
	}
}

// Role classification decides both halves of a rendered row: what reaches the
// chunk body and what reaches chunk_data. Every case is checked on every row, so
// a role that leaks on one row cannot pass on the next.
func TestParity_RoleClassification(t *testing.T) {
	all := []string{"Title", "Content", "Country", "Category", "Year"}
	roleOf := func(role string) map[string]string {
		roles := make(map[string]string, len(all))
		for _, col := range all {
			roles[col] = role
		}
		return roles
	}

	cases := []struct {
		name      string
		mode      string
		roles     map[string]string
		inText    []string
		notInText []string
		inData    []string
		notInData []string
	}{
		{"auto keeps every column in both tiers", "auto", nil, all, nil, all, nil},
		{"manual all indexing", "manual", roleOf("indexing"), all, nil, nil, all},
		{"manual all metadata", "manual", roleOf("metadata"), nil, all, all, nil},
		{"manual all both", "manual", roleOf("both"), all, nil, all, nil},
		{
			name:   "manual mixed roles",
			mode:   "manual",
			roles:  map[string]string{"Title": "both", "Content": "indexing", "Country": "metadata", "Category": "both", "Year": "metadata"},
			inText: []string{"Title", "Content", "Category"}, notInText: []string{"Country", "Year"},
			inData: []string{"Title", "Country", "Category", "Year"}, notInData: []string{"Content"},
		},
		{
			name:   "a column the roles map leaves out defaults to both",
			mode:   "manual",
			roles:  map[string]string{"Title": "indexing", "Country": "metadata"},
			inText: []string{"Title", "Content", "Category", "Year"}, notInText: []string{"Country"},
			inData: []string{"Content", "Country", "Category", "Year"}, notInData: []string{"Title"},
		},
		{
			name:   "the legacy vectorize alias indexes",
			mode:   "manual",
			roles:  map[string]string{"Title": "vectorize", "Country": "metadata", "Category": "both"},
			inText: []string{"Title", "Content", "Category", "Year"}, notInText: []string{"Country"},
			inData: []string{"Content", "Country", "Category", "Year"}, notInData: []string{"Title"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items, headers := RenderRowsToJSONChunks(standardTableRows, "", tc.mode, tc.roles, TableHeaderRuleSpreadsheet)
			if !reflect.DeepEqual(headers, all) {
				t.Fatalf("headers = %#v, want %v", headers, all)
			}
			if len(items) != 3 {
				t.Fatalf("items = %d, want 3", len(items))
			}
			colOf := map[string]int{}
			for i, col := range all {
				colOf[col] = i
			}
			for row, item := range items {
				text := item["text"].(string)
				data, _ := item["chunk_data"].(map[string]any)
				cell := func(col string) string { return standardTableRows[row+1][colOf[col]] }
				for _, col := range tc.inText {
					if want := "- " + col + ": " + cell(col); !strings.Contains(text, want) {
						t.Errorf("row %d: text must index %s as %q, got %q", row, col, want, text)
					}
				}
				for _, col := range tc.notInText {
					if strings.Contains(text, col) {
						t.Errorf("row %d: text must not name %s at all, got %q", row, col, text)
					}
				}
				for _, col := range tc.inData {
					if got, ok := data[col]; !ok || got != cell(col) {
						t.Errorf("row %d: chunk_data must store %s = %q, got %#v", row, col, cell(col), data)
					}
				}
				for _, col := range tc.notInData {
					if got, ok := data[col]; ok {
						t.Errorf("row %d: chunk_data must not store %s = %v", row, col, got)
					}
				}
				if len(data) != len(tc.inData) {
					t.Errorf("row %d: chunk_data stores %#v, want exactly %v", row, data, tc.inData)
				}
				if len(tc.inData) == 0 {
					if _, exists := item["chunk_data"]; exists {
						t.Errorf("row %d: a chunk with nothing to store must omit chunk_data, got %#v", row, item["chunk_data"])
					}
				}
			}
		})
	}
}

// Case 8: a role shape Python's membership test rejects is excluded, whatever
// it looks like -- wrong case, wrong padding. The "both" default covers only a
// column the roles map leaves out.
func TestParity_Manual_UnknownRoleShapeIsExcluded(t *testing.T) {
	roles := map[string]string{
		"Title":    "INDEXING",
		"Content":  "  indexing  ",
		"Country":  "MetaData",
		"Category": "both",
		"Year":     "  METADATA  ",
	}
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "manual", roles, TableHeaderRuleSpreadsheet)
	row0 := items[0]
	text0 := row0["text"].(string)
	cd0 := row0["chunk_data"].(map[string]any)

	for _, col := range []string{"Title", "Content", "Country", "Year"} {
		if strings.Contains(text0, "- "+col+": ") {
			t.Errorf("%q (role %q) must not reach the chunk body: Python excludes a role it does not match exactly", col, roles[col])
		}
		if _, ok := cd0[col]; ok {
			t.Errorf("%q (role %q) must not reach chunk_data", col, roles[col])
		}
	}
	if !strings.Contains(text0, "- Category: ") {
		t.Errorf("Category with the exact role \"both\" must stay indexed, got %q", text0)
	}
	if _, ok := cd0["Category"]; !ok {
		t.Errorf("Category with the exact role \"both\" must stay in chunk_data: %v", cd0)
	}
}

// Case 8b: the mode is compared exactly, as Python's
// `parser_config.get("table_column_mode") == "manual"` does (rag/app/table.py:529),
// so a differently cased mode is auto -- and auto ignores the roles entirely.
func TestParity_Manual_MisCasedModeIsAuto(t *testing.T) {
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "Manual", map[string]string{"Title": "metadata"}, TableHeaderRuleSpreadsheet)
	row0 := items[0]
	text0 := row0["text"].(string)
	cd0 := row0["chunk_data"].(map[string]any)

	if !strings.Contains(text0, "- Title: ") {
		t.Errorf("mode %q is not manual, so every column must be indexed, got %q", "Manual", text0)
	}
	for _, col := range []string{"Title", "Content", "Country", "Category", "Year"} {
		if _, ok := cd0[col]; !ok {
			t.Errorf("%q must be in chunk_data under auto mode: %v", col, cd0)
		}
	}
}

// Case: a column configured with an EMPTY role is excluded, while a column the
// map leaves out keeps the "both" default. Python's `column_roles.get(col, "both")`
// (rag/app/table.py:626) tells these apart -- a present-but-empty value matches
// neither membership test, an omitted key returns the default.
func TestParity_Manual_PresentEmptyRoleIsExcluded(t *testing.T) {
	roles := map[string]string{"Title": "", "Content": "metadata"}
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "manual", roles, TableHeaderRuleSpreadsheet)
	text0 := items[0]["text"].(string)
	cd0 := items[0]["chunk_data"].(map[string]any)

	if strings.Contains(text0, "- Title: ") {
		t.Errorf(`role "" must exclude Title from the chunk body, got %q`, text0)
	}
	if _, ok := cd0["Title"]; ok {
		t.Errorf(`role "" must exclude Title from chunk_data: %v`, cd0)
	}
	if !strings.Contains(text0, "- Country: ") {
		t.Errorf("a column the map omits must default to both, got %q", text0)
	}
	if _, ok := cd0["Country"]; !ok {
		t.Errorf("a column the map omits must default to both in chunk_data: %v", cd0)
	}
	if strings.Contains(text0, "- Content: ") {
		t.Errorf("a metadata column must not reach the chunk body: %q", text0)
	}
	if _, ok := cd0["Content"]; !ok {
		t.Errorf("a metadata column must be in chunk_data: %v", cd0)
	}
}

// Case 9: Blank / whitespace column mode defaults to auto
func TestParity_BlankMode_DefaultsToAuto(t *testing.T) {
	items, headers := RenderRowsToJSONChunks(standardTableRows, "", "  ", nil, TableHeaderRuleSpreadsheet)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	cd0 := items[0]["chunk_data"].(map[string]any)
	if len(cd0) != len(headers) {
		t.Errorf("blank mode should default to auto with all columns in chunk_data, got %d vs %d", len(cd0), len(headers))
	}
}

// Case 10: Empty cells in rows. Python skips a cell only when its string form
// is empty (rag/app/table.py:621) and renders the value as read, so a
// whitespace-only cell is content: column_data_type converts a text column with
// str() (:496), which does not strip it either.
func TestParity_EmptyCells(t *testing.T) {
	rows := [][]string{
		{"A", "B", "C"},
		{"1", "", "3"},    // B is empty
		{"", "", ""},      // all empty row -> skipped, nothing is rendered
		{"  ", "2", "  "}, // whitespace cells -> kept, padded
	}
	items, _ := RenderRowsToJSONChunks(rows, "", "auto", nil, TableHeaderRuleSpreadsheet)
	if len(items) != 2 {
		t.Fatalf("expected 2 items (skipping completely empty row), got %d", len(items))
	}

	// First item should only have A and C
	text0 := items[0]["text"].(string)
	cd0 := items[0]["chunk_data"].(map[string]any)
	if strings.Contains(text0, "- B:") {
		t.Errorf("text0 should skip empty B: %s", text0)
	}
	if _, hasB := cd0["B"]; hasB {
		t.Errorf("cd0 should skip empty B: %v", cd0)
	}

	text1 := items[1]["text"].(string)
	if text1 != "- A:   \n- B: 2\n- C:   " {
		t.Errorf("text1 = %q, want every cell rendered with its own spacing", text1)
	}
	cd1 := items[1]["chunk_data"].(map[string]any)
	if cd1["A"] != "  " || cd1["C"] != "  " {
		t.Errorf("cd1 = %v, want the blank-only cells stored verbatim", cd1)
	}
}

// Case 11: deduplicateColumnNames matching Python
func TestParity_DeduplicateColumnNames_Parity(t *testing.T) {
	cols := []string{"name", "name", "name", "age", "age"}
	dedup := deduplicateColumnNames(cols)
	want := []string{"name", "name_2", "name_3", "age", "age_2"}
	if !reflect.DeepEqual(dedup, want) {
		t.Errorf("deduplicateColumnNames = %v, want %v", dedup, want)
	}

	// Collision with existing suffix
	cols2 := []string{"col", "col_2", "col"}
	dedup2 := deduplicateColumnNames(cols2)
	want2 := []string{"col", "col_2", "col_3"}
	if !reflect.DeepEqual(dedup2, want2) {
		t.Errorf("deduplicateColumnNames = %v, want %v", dedup2, want2)
	}
}

// Case 12: Empty headers fallback to Column_N
func TestParity_EmptyHeader_ColumnN(t *testing.T) {
	rows := [][]string{
		{"Name", "", "Age", "  "},
		{"Alice", "Engineer", "30", "Beijing"},
	}
	items, headers := RenderRowsToJSONChunks(rows, "", "auto", nil, TableHeaderRuleSpreadsheet)
	wantHeaders := []string{"Name", "Column_2", "Age", "Column_4"}
	if !reflect.DeepEqual(headers, wantHeaders) {
		t.Errorf("headers = %v, want %v", headers, wantHeaders)
	}
	text0 := items[0]["text"].(string)
	if !strings.Contains(text0, "- Column_2: Engineer") || !strings.Contains(text0, "- Column_4: Beijing") {
		t.Errorf("text0 = %s, missing Column_2 or Column_4", text0)
	}
}

// Case 13: Chinese / Non-ASCII column headers
func TestParity_ChineseHeaders(t *testing.T) {
	rows := [][]string{
		{"姓名", "部门", "城市", "职级"},
		{"张三", "技术", "北京", "P7"},
		{"李四", "市场", "上海", "P6"},
	}
	roles := map[string]string{
		"姓名": "both",
		"部门": "metadata",
		"城市": "indexing",
		"职级": "both",
	}
	items, _ := RenderRowsToJSONChunks(rows, "", "manual", roles, TableHeaderRuleSpreadsheet)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	row0 := items[0]
	text0 := row0["text"].(string)
	cd0 := row0["chunk_data"].(map[string]any)

	// In text: 姓名, 城市, 职级 (部门 is metadata only)
	if !strings.Contains(text0, "- 姓名: 张三") || !strings.Contains(text0, "- 城市: 北京") || !strings.Contains(text0, "- 职级: P7") {
		t.Errorf("text0 missing Chinese columns: %s", text0)
	}
	if strings.Contains(text0, "部门") {
		t.Errorf("text0 contains metadata column 部门: %s", text0)
	}

	// In chunk_data: 姓名, 部门, 职级 (城市 is indexing only)
	if _, ok := cd0["城市"]; ok {
		t.Errorf("cd0 contains indexing column 城市: %v", cd0)
	}
	if cd0["姓名"] != "张三" || cd0["部门"] != "技术" || cd0["职级"] != "P7" {
		t.Errorf("cd0 incorrect: %v", cd0)
	}
}

// Case 14: CSV Parser integration with ColumnMode
func TestParity_CSVParser_FullIntegration(t *testing.T) {
	csvData := []byte("Title,Content,Country,Category,Year\nDoc A,First text,Turkey,Tech,2024\nDoc B,Second text,Greece,Finance,2023\n")
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"column_mode":   "manual",
		"column_roles": map[string]any{
			"Title":    "both",
			"Content":  "indexing",
			"Country":  "metadata",
			"Category": "both",
			"Year":     "metadata",
		},
	})

	res := p.ParseWithResult(context.Background(), "test.csv", csvData)
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}

	colNames, ok := res.File["table_column_names"].([]string)
	if !ok || len(colNames) != 5 {
		t.Errorf("table_column_names = %v", res.File["table_column_names"])
	}
	if len(res.JSON) != 2 {
		t.Fatalf("JSON len = %d, want 2", len(res.JSON))
	}
}

// Case 15: XLSX Parser multi-sheet and column discovery
func TestParity_XLSXParser_MultiSheet(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	// Sheet 1
	_ = f.SetSheetName("Sheet1", "Engineering")
	_ = f.SetCellValue("Engineering", "A1", "Name")
	_ = f.SetCellValue("Engineering", "B1", "Team")
	_ = f.SetCellValue("Engineering", "A2", "Alice")
	_ = f.SetCellValue("Engineering", "B2", "Core")

	// Sheet 2
	_, _ = f.NewSheet("Sales")
	_ = f.SetCellValue("Sales", "A1", "Product")
	_ = f.SetCellValue("Sales", "B1", "Revenue")
	_ = f.SetCellValue("Sales", "A2", "SaaS")
	_ = f.SetCellValue("Sales", "B2", "100000")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("excelize Write: %v", err)
	}

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"column_mode":   "auto",
	})

	res := p.ParseWithResult(context.Background(), "multi.xlsx", buf.Bytes())
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("expected 2 chunks across 2 sheets, got %d", len(res.JSON))
	}

	// Check sheets tagged
	if res.JSON[0]["sheet"] != "Engineering" || res.JSON[1]["sheet"] != "Sales" {
		t.Errorf("sheet tagging mismatch: sheet0=%v, sheet1=%v", res.JSON[0]["sheet"], res.JSON[1]["sheet"])
	}

	// Check union of all headers reported
	cols, _ := res.File["table_column_names"].([]string)
	wantCols := []string{"Name", "Team", "Product", "Revenue"}
	if !reflect.DeepEqual(cols, wantCols) {
		t.Errorf("table_column_names = %v, want %v", cols, wantCols)
	}
}
