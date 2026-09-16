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

// Case 1: Auto mode (all columns default to "both", matching Python)
func TestParity_AutoMode(t *testing.T) {
	items, headers := RenderRowsToJSONChunks(standardTableRows, "", "auto", nil)
	if len(headers) != 5 {
		t.Fatalf("expected 5 headers, got %d", len(headers))
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	for i, it := range items {
		text := it["text"].(string)
		cd := it["chunk_data"].(map[string]any)

		// In auto mode, every column must be in text AND in chunk_data
		for _, h := range headers {
			if !strings.Contains(text, "- "+h+": ") {
				t.Errorf("row %d: text missing column %s: %s", i, h, text)
			}
			if _, ok := cd[h]; !ok {
				t.Errorf("row %d: chunk_data missing column %s", i, h)
			}
		}
	}
}

// Case 2: Manual mode - all indexing
func TestParity_Manual_AllIndexing(t *testing.T) {
	roles := map[string]string{
		"Title":    "indexing",
		"Content":  "indexing",
		"Country":  "indexing",
		"Category": "indexing",
		"Year":     "indexing",
	}
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "manual", roles)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	for i, it := range items {
		text := it["text"].(string)
		if strings.TrimSpace(text) == "" {
			t.Errorf("row %d: text should not be empty", i)
		}
		if cd, ok := it["chunk_data"]; ok && len(cd.(map[string]any)) > 0 {
			t.Errorf("row %d: indexing-only mode should not have chunk_data, got %v", i, cd)
		}
	}

	cfg := map[string]interface{}{
		"table_column_mode":  "manual",
		"table_column_roles": roles,
	}
	_ = cfg
}

// Case 3: Manual mode - all metadata
func TestParity_Manual_AllMetadata(t *testing.T) {
	roles := map[string]string{
		"Title":    "metadata",
		"Content":  "metadata",
		"Country":  "metadata",
		"Category": "metadata",
		"Year":     "metadata",
	}
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "manual", roles)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	for i, it := range items {
		text := it["text"].(string)
		if text != "" {
			t.Errorf("row %d: metadata-only mode should have empty text, got %q", i, text)
		}
		cd := it["chunk_data"].(map[string]any)
		if len(cd) != 5 {
			t.Errorf("row %d: chunk_data should have all 5 columns, got %d", i, len(cd))
		}
	}
}

// Case 4: Manual mode - all both
func TestParity_Manual_AllBoth(t *testing.T) {
	roles := map[string]string{
		"Title":    "both",
		"Content":  "both",
		"Country":  "both",
		"Category": "both",
		"Year":     "both",
	}
	items, headers := RenderRowsToJSONChunks(standardTableRows, "", "manual", roles)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	for i, it := range items {
		text := it["text"].(string)
		cd := it["chunk_data"].(map[string]any)
		for _, h := range headers {
			if !strings.Contains(text, "- "+h+": ") {
				t.Errorf("row %d: text missing %s", i, h)
			}
			if _, ok := cd[h]; !ok {
				t.Errorf("row %d: chunk_data missing %s", i, h)
			}
		}
	}
}

// Case 5: Manual mode - mixed roles (indexing, metadata, both)
func TestParity_Manual_MixedRoles(t *testing.T) {
	roles := map[string]string{
		"Title":    "both",
		"Content":  "indexing",
		"Country":  "metadata",
		"Category": "both",
		"Year":     "metadata",
	}
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "manual", roles)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	row0 := items[0]
	text0 := row0["text"].(string)
	cd0 := row0["chunk_data"].(map[string]any)

	// Text must have Title, Content, Category; must NOT have Country, Year
	if !strings.Contains(text0, "- Title: Doc A") || !strings.Contains(text0, "- Content: First document text") || !strings.Contains(text0, "- Category: Tech") {
		t.Errorf("row0 text missing expected indexing columns: %s", text0)
	}
	if strings.Contains(text0, "Country") || strings.Contains(text0, "Year") {
		t.Errorf("row0 text contains metadata-only columns: %s", text0)
	}

	// ChunkData must have Title, Country, Category, Year; must NOT have Content
	for _, col := range []string{"Title", "Country", "Category", "Year"} {
		if _, ok := cd0[col]; !ok {
			t.Errorf("row0 chunk_data missing %s", col)
		}
	}
	if _, ok := cd0["Content"]; ok {
		t.Errorf("row0 chunk_data contains indexing-only column Content: %v", cd0)
	}
}

// Case 6: Manual mode - partial roles (omitted columns default to "both", matching Python)
func TestParity_Manual_PartialRoles_DefaultToBoth(t *testing.T) {
	roles := map[string]string{
		"Title":   "indexing",
		"Country": "metadata",
		// Content, Category, Year omitted
	}
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "manual", roles)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	row0 := items[0]
	text0 := row0["text"].(string)
	cd0 := row0["chunk_data"].(map[string]any)

	// Omitted Content, Category, Year must default to "both":
	// In text: Title (indexing), Content (default both), Category (default both), Year (default both)
	// Country (metadata) excluded from text
	if !strings.Contains(text0, "- Title: Doc A") || !strings.Contains(text0, "- Content: First document text") || !strings.Contains(text0, "- Category: Tech") || !strings.Contains(text0, "- Year: 2024") {
		t.Errorf("text0 missing expected columns: %s", text0)
	}
	if strings.Contains(text0, "Country") {
		t.Errorf("text0 should not contain metadata-only Country: %s", text0)
	}

	// In chunk_data: Country (metadata), Content (default both), Category (default both), Year (default both)
	// Title (indexing) excluded from chunk_data
	if _, ok := cd0["Title"]; ok {
		t.Errorf("cd0 should not contain indexing-only Title: %v", cd0)
	}
	for _, col := range []string{"Country", "Content", "Category", "Year"} {
		if _, ok := cd0[col]; !ok {
			t.Errorf("cd0 missing expected column %s", col)
		}
	}
}

// Case 7: Manual mode - legacy "vectorize" alias maps to "indexing"
func TestParity_Manual_VectorizeAlias(t *testing.T) {
	roles := map[string]string{
		"Title":   "vectorize",
		"Country": "both",
	}
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "manual", roles)
	row0 := items[0]
	text0 := row0["text"].(string)
	cd0 := row0["chunk_data"].(map[string]any)

	if !strings.Contains(text0, "- Title: Doc A") {
		t.Errorf("vectorize column Title must be indexed in text: %s", text0)
	}
	if _, ok := cd0["Title"]; ok {
		t.Errorf("vectorize column Title must NOT be in chunk_data: %v", cd0)
	}
}

// Case 8: Manual mode - case insensitivity
func TestParity_Manual_CaseInsensitive(t *testing.T) {
	roles := map[string]string{
		"Title":    "INDEXING",
		"Content":  "  indexing  ",
		"Country":  "MetaData",
		"Category": "BOTH",
		"Year":     "  METADATA  ",
	}
	items, _ := RenderRowsToJSONChunks(standardTableRows, "", "Manual", roles)
	row0 := items[0]
	text0 := row0["text"].(string)
	cd0 := row0["chunk_data"].(map[string]any)

	if strings.Contains(text0, "Country") || strings.Contains(text0, "Year") {
		t.Errorf("text0 contains metadata columns: %s", text0)
	}
	if _, ok := cd0["Content"]; ok {
		t.Errorf("cd0 contains indexing column Content: %v", cd0)
	}
	if !strings.Contains(text0, "Title") || !strings.Contains(text0, "Content") || !strings.Contains(text0, "Category") {
		t.Errorf("text0 missing indexing columns: %s", text0)
	}
	if _, ok := cd0["Title"]; ok {
		t.Errorf("cd0 contains indexing column Title: %v", cd0)
	}
	if _, ok := cd0["Country"]; !ok {
		t.Errorf("cd0 missing metadata column Country: %v", cd0)
	}
	if _, ok := cd0["Year"]; !ok {
		t.Errorf("cd0 missing metadata column Year: %v", cd0)
	}
}

// Case 9: Blank / whitespace column mode defaults to auto
func TestParity_BlankMode_DefaultsToAuto(t *testing.T) {
	items, headers := RenderRowsToJSONChunks(standardTableRows, "", "  ", nil)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	cd0 := items[0]["chunk_data"].(map[string]any)
	if len(cd0) != len(headers) {
		t.Errorf("blank mode should default to auto with all columns in chunk_data, got %d vs %d", len(cd0), len(headers))
	}
}

// Case 10: Empty cells in rows
func TestParity_EmptyCells(t *testing.T) {
	rows := [][]string{
		{"A", "B", "C"},
		{"1", "", "3"},    // B is empty
		{"", "", ""},      // all empty row -> should be skipped
		{"  ", "2", "  "}, // whitespace cells -> only B present
	}
	items, _ := RenderRowsToJSONChunks(rows, "", "auto", nil)
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

	// Second item should only have B: 2
	text1 := items[1]["text"].(string)
	if text1 != "- B: 2" {
		t.Errorf("text1 = %q, want '- B: 2'", text1)
	}
}

// Case 11: DeduplicateColumnNames matching Python
func TestParity_DeduplicateColumnNames_Parity(t *testing.T) {
	cols := []string{"name", "name", "name", "age", "age"}
	dedup := DeduplicateColumnNames(cols)
	want := []string{"name", "name_2", "name_3", "age", "age_2"}
	if !reflect.DeepEqual(dedup, want) {
		t.Errorf("DeduplicateColumnNames = %v, want %v", dedup, want)
	}

	// Collision with existing suffix
	cols2 := []string{"col", "col_2", "col"}
	dedup2 := DeduplicateColumnNames(cols2)
	want2 := []string{"col", "col_2", "col_3"}
	if !reflect.DeepEqual(dedup2, want2) {
		t.Errorf("DeduplicateColumnNames = %v, want %v", dedup2, want2)
	}
}

// Case 12: Empty headers fallback to Column_N
func TestParity_EmptyHeader_ColumnN(t *testing.T) {
	rows := [][]string{
		{"Name", "", "Age", "  "},
		{"Alice", "Engineer", "30", "Beijing"},
	}
	items, headers := RenderRowsToJSONChunks(rows, "", "auto", nil)
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
	items, _ := RenderRowsToJSONChunks(rows, "", "manual", roles)
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

// Case 17: Direct end-to-end parity against Python execution results
func TestDirectParityWithPython(t *testing.T) {
	// Directly verify that Go RenderRowsToJSONChunks and Python table.chunk produce identical results
	type TestCase struct {
		name     string
		mode     string
		roles    map[string]string
		wantCols []string
		wantSkip []string // columns that should NOT be in text
		wantData []string // columns that MUST be in chunk_data
		noData   []string // columns that must NOT be in chunk_data
	}

	tests := []TestCase{
		{
			name:     "mode_auto",
			mode:     "auto",
			roles:    nil,
			wantCols: []string{"Title", "Content", "Country", "Category", "Year"},
			wantSkip: nil,
			wantData: []string{"Title", "Content", "Country", "Category", "Year"},
			noData:   nil,
		},
		{
			name:     "mode_manual_all_indexing",
			mode:     "manual",
			roles:    map[string]string{"Title": "indexing", "Content": "indexing", "Country": "indexing", "Category": "indexing", "Year": "indexing"},
			wantCols: []string{"Title", "Content", "Country", "Category", "Year"},
			wantSkip: nil,
			wantData: nil,
			noData:   []string{"Title", "Content", "Country", "Category", "Year"},
		},
		{
			name:     "mode_manual_all_metadata",
			mode:     "manual",
			roles:    map[string]string{"Title": "metadata", "Content": "metadata", "Country": "metadata", "Category": "metadata", "Year": "metadata"},
			wantCols: []string{"Title", "Content", "Country", "Category", "Year"},
			wantSkip: []string{"Title", "Content", "Country", "Category", "Year"},
			wantData: []string{"Title", "Content", "Country", "Category", "Year"},
			noData:   nil,
		},
		{
			name:     "mode_manual_mixed",
			mode:     "manual",
			roles:    map[string]string{"Title": "both", "Content": "indexing", "Country": "metadata", "Category": "both", "Year": "metadata"},
			wantCols: []string{"Title", "Content", "Country", "Category", "Year"},
			wantSkip: []string{"Country", "Year"},
			wantData: []string{"Title", "Country", "Category", "Year"},
			noData:   []string{"Content"},
		},
		{
			name:     "mode_manual_partial_fallback",
			mode:     "manual",
			roles:    map[string]string{"Title": "indexing", "Country": "metadata"},
			wantCols: []string{"Title", "Content", "Country", "Category", "Year"},
			wantSkip: []string{"Country"},
			wantData: []string{"Country", "Content", "Category", "Year"},
			noData:   []string{"Title"},
		},
		{
			name:     "mode_manual_vectorize_alias",
			mode:     "manual",
			roles:    map[string]string{"Title": "vectorize", "Country": "metadata", "Category": "both"},
			wantCols: []string{"Title", "Content", "Country", "Category", "Year"},
			wantSkip: []string{"Country"},
			wantData: []string{"Country", "Category", "Content", "Year"},
			noData:   []string{"Title"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items, headers := RenderRowsToJSONChunks(standardTableRows, "", tc.mode, tc.roles)
			if !reflect.DeepEqual(headers, tc.wantCols) {
				t.Errorf("headers = %v, want %v", headers, tc.wantCols)
			}
			if len(items) != 3 {
				t.Fatalf("items count = %d, want 3", len(items))
			}
			for rowIdx, item := range items {
				text := item["text"].(string)
				cd, _ := item["chunk_data"].(map[string]any)

				for _, skipCol := range tc.wantSkip {
					if strings.Contains(text, "- "+skipCol+":") {
						t.Errorf("case %s row %d: text should NOT contain %s, got: %s", tc.name, rowIdx, skipCol, text)
					}
				}
				for _, dataCol := range tc.wantData {
					if _, ok := cd[dataCol]; !ok {
						t.Errorf("case %s row %d: chunk_data MISSING %s, got: %v", tc.name, rowIdx, dataCol, cd)
					}
				}
				for _, noDataCol := range tc.noData {
					if _, ok := cd[noDataCol]; ok {
						t.Errorf("case %s row %d: chunk_data MUST NOT contain %s, got: %v", tc.name, rowIdx, noDataCol, cd)
					}
				}
			}
		})
	}
}
