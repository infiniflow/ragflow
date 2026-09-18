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

package parser

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestDeduplicateColumnNames(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "unique columns unchanged",
			in:   []string{"a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
		{
			name: "duplicates suffixed",
			in:   []string{"col", "col", "col"},
			want: []string{"col", "col_2", "col_3"},
		},
		{
			name: "collision with reserved avoided",
			in:   []string{"col", "col_2", "col"},
			want: []string{"col", "col_2", "col_3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeduplicateColumnNames(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DeduplicateColumnNames(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// Each file family has its own header rule, and the probe must apply the rule
// of the file it is shown: a role configured for a name the probe did not
// report silently matches nothing, and a name the probe invented indexes
// nothing.
func TestTableColumnHeaderNames_PerFileKind(t *testing.T) {
	tests := []struct {
		name        string
		rule        TableHeaderRule
		headerRow   []string
		wantNames   []string
		wantIndexes []int
	}{
		{
			name:        "spreadsheet trims cells and names an empty one by position",
			rule:        TableHeaderRuleSpreadsheet,
			headerRow:   []string{" Name ", "", "id", " id", "Name"},
			wantNames:   []string{"Name", "Column_2", "Name_2"},
			wantIndexes: []int{0, 1, 4},
		},
		{
			name:        "delimited takes the cells as read",
			rule:        TableHeaderRuleDelimited,
			headerRow:   []string{" Name ", "", "id", " id", "Name"},
			wantNames:   []string{" Name ", "", " id", "Name"},
			wantIndexes: []int{0, 1, 3, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names, indexes := TableColumnHeaderNames(tt.headerRow, tt.rule)
			if !reflect.DeepEqual(names, tt.wantNames) {
				t.Errorf("names = %q, want %q", names, tt.wantNames)
			}
			if !reflect.DeepEqual(indexes, tt.wantIndexes) {
				t.Errorf("source indexes = %v, want %v", indexes, tt.wantIndexes)
			}
		})
	}
}

// A CSV reader must key its roles on the spelling the CSV file carries, so the
// renderer has to keep a padded header padded.
func TestRenderRowsToJSONChunks_DelimitedRuleKeepsHeaderSpelling(t *testing.T) {
	rows := [][]string{
		{" Name", "Amount"},
		{"Alice", "10"},
	}
	roles := map[string]string{" Name": "metadata"}

	items, headers := RenderRowsToJSONChunks(rows, "", "manual", roles, TableHeaderRuleDelimited)
	if !reflect.DeepEqual(headers, []string{" Name", "Amount"}) {
		t.Fatalf("headers = %q, want the cells as read", headers)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if text := items[0]["text"]; text != "- Amount: 10" {
		t.Errorf("text = %q, want only the unconfigured column indexed", text)
	}
	cd, _ := items[0]["chunk_data"].(map[string]any)
	if cd[" Name"] != "Alice" {
		t.Errorf("chunk_data = %#v, want the padded role to match", cd)
	}
}

func TestRenderRowsToJSONChunks(t *testing.T) {
	rows := [][]string{
		{"Title", "Content", "Category", "Year"},
		{"Doc A", "First document text", "Tech", "2024"},
		{"Doc B", "Second document text", "Finance", "2023"},
		{"", "", "", ""}, // empty row, should be skipped
	}

	t.Run("auto mode defaults every column to both, matching Python", func(t *testing.T) {
		items, headers := RenderRowsToJSONChunks(rows, "Sheet1", "auto", nil, TableHeaderRuleSpreadsheet)
		if len(headers) != 4 {
			t.Fatalf("expected 4 headers, got %d", len(headers))
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}

		item0 := items[0]
		expectedText := "- Title: Doc A\n- Content: First document text\n- Category: Tech\n- Year: 2024"
		if item0["text"] != expectedText {
			t.Errorf("item0 text = %q, want %q", item0["text"], expectedText)
		}
		cd, ok := item0["chunk_data"].(map[string]any)
		if !ok {
			t.Fatalf("auto mode must default columns to both, chunk_data missing: %v", item0["chunk_data"])
		}
		if cd["Title"] != "Doc A" || cd["Content"] != "First document text" || cd["Category"] != "Tech" || cd["Year"] != "2024" {
			t.Errorf("auto mode chunk_data = %v, want all four columns", cd)
		}
		if item0["sheet"] != "Sheet1" {
			t.Errorf("item0 sheet = %v, want Sheet1", item0["sheet"])
		}
	})

	t.Run("empty header falls back to Column_N, matching Python", func(t *testing.T) {
		items, headers := RenderRowsToJSONChunks([][]string{
			{"Name", "", "Age"},
			{"Alice", "x", "30"},
		}, "", "auto", nil, TableHeaderRuleSpreadsheet)
		if len(headers) != 3 || headers[1] != "Column_2" {
			t.Fatalf("headers = %v, want [Name Column_2 Age]", headers)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		cd, ok := items[0]["chunk_data"].(map[string]any)
		if !ok || cd["Column_2"] != "x" {
			t.Errorf("chunk_data = %v, want Column_2=x", items[0]["chunk_data"])
		}
	})

	t.Run("manual mode maps legacy vectorize to indexing", func(t *testing.T) {
		items, _ := RenderRowsToJSONChunks([][]string{
			{"A", "B"},
			{"1", "2"},
		}, "", "manual", map[string]string{"A": "vectorize"}, TableHeaderRuleSpreadsheet)
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0]["text"] != "- A: 1\n- B: 2" {
			t.Errorf("text = %q, want both columns indexed", items[0]["text"])
		}
		cd, ok := items[0]["chunk_data"].(map[string]any)
		if !ok || cd["B"] != "2" {
			t.Errorf("chunk_data = %v, want B=2", items[0]["chunk_data"])
		}
		if _, hasA := cd["A"]; hasA {
			t.Errorf("vectorize column A must not land in chunk_data: %v", cd)
		}
	})

	t.Run("manual mode respects column roles", func(t *testing.T) {
		roles := map[string]string{
			"Title":    "both",
			"Content":  "indexing",
			"Category": "metadata",
			"Year":     "metadata",
		}
		items, _ := RenderRowsToJSONChunks(rows, "", "manual", roles, TableHeaderRuleSpreadsheet)
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}

		item0 := items[0]
		// Text should only have Title and Content (indexing & both)
		expectedText := "- Title: Doc A\n- Content: First document text"
		if item0["text"] != expectedText {
			t.Errorf("item0 text = %q, want %q", item0["text"], expectedText)
		}
		// chunk_data should only have Title, Category, Year (metadata & both)
		cd, ok := item0["chunk_data"].(map[string]any)
		if !ok {
			t.Fatalf("item0 chunk_data missing or not map[string]any: %v", item0["chunk_data"])
		}
		if _, hasContent := cd["Content"]; hasContent {
			t.Errorf("indexing-only column 'Content' should not be in chunk_data: %v", cd)
		}
		if cd["Title"] != "Doc A" || cd["Category"] != "Tech" || cd["Year"] != "2024" {
			t.Errorf("item0 chunk_data incorrect: %v", cd)
		}
	})
}

func TestCSVParser_ColumnMode(t *testing.T) {
	csvData := []byte("Name,Age,Role\nAlice,30,Admin\nBob,25,User\n")
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"column_mode":   "manual",
		"column_roles": map[string]any{
			"Name": "both",
			"Age":  "metadata",
			"Role": "indexing",
		},
	})

	res := p.ParseWithResult(t.Context(), "users.csv", csvData)
	if res.Err != nil {
		t.Fatalf("CSVParser.ParseWithResult failed: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("len(JSON) = %d, want 2", len(res.JSON))
	}
	// Alice row: text should have Name and Role ("- Name: Alice\n- Role: Admin"), NOT Age
	first := res.JSON[0]
	wantText := "- Name: Alice\n- Role: Admin"
	if first["text"] != wantText {
		t.Errorf("first text = %q, want %q", first["text"], wantText)
	}
	// chunk_data should have Name and Age, NOT Role
	cd, ok := first["chunk_data"].(map[string]any)
	if !ok {
		t.Fatalf("chunk_data not map[string]any: %v", first["chunk_data"])
	}
	if cd["Name"] != "Alice" || cd["Age"] != "30" {
		t.Errorf("chunk_data = %v", cd)
	}
	if _, hasRole := cd["Role"]; hasRole {
		t.Errorf("Role should not be in chunk_data: %v", cd)
	}
}

// A setup that states no column_mode is an unconfigured document, not a request
// for the legacy row rendering: the JSON output format alone picks the
// structured renderer, and every column keeps the default "both" role — which is
// how rag/app/table.py treats a mode other than "manual".
func TestCSVParser_JSONWithoutColumnMode(t *testing.T) {
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "json"})

	res := p.ParseWithResult(t.Context(), "users.csv", []byte("Name,Age\nAlice,30\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("len(JSON) = %d, want 1 structured row", len(res.JSON))
	}
	item := res.JSON[0]
	if want := "- Name: Alice\n- Age: 30"; item["text"] != want {
		t.Errorf("text = %q, want %q", item["text"], want)
	}
	cd, ok := item["chunk_data"].(map[string]any)
	if !ok || cd["Name"] != "Alice" || cd["Age"] != "30" {
		t.Errorf("chunk_data = %v, want every column", item["chunk_data"])
	}
}

func TestXLSXParser_JSONWithoutColumnMode(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	_ = f.SetCellValue("Sheet1", "A1", "Product")
	_ = f.SetCellValue("Sheet1", "B1", "Price")
	_ = f.SetCellValue("Sheet1", "A2", "Laptop")
	_ = f.SetCellValue("Sheet1", "B2", "999")
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{"output_format": "json"})

	res := p.ParseWithResult(t.Context(), "inventory.xlsx", buf.Bytes())
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("len(JSON) = %d, want 1 structured row", len(res.JSON))
	}
	cd, ok := res.JSON[0]["chunk_data"].(map[string]any)
	if !ok || cd["Product"] != "Laptop" || cd["Price"] != "999" {
		t.Errorf("chunk_data = %v, want every column", res.JSON[0]["chunk_data"])
	}
}

func TestXLSXParser_ColumnMode(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "Product")
	_ = f.SetCellValue(sheet, "B1", "Price")
	_ = f.SetCellValue(sheet, "C1", "Stock")

	_ = f.SetCellValue(sheet, "A2", "Laptop")
	_ = f.SetCellValue(sheet, "B2", "999")
	_ = f.SetCellValue(sheet, "C2", "50")

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
		"column_mode":   "manual",
		"column_roles": map[string]any{
			"Product": "indexing",
			"Price":   "metadata",
			"Stock":   "both",
		},
	})

	res := p.ParseWithResult(t.Context(), "inventory.xlsx", buf.Bytes())
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("len(JSON) = %d, want 1", len(res.JSON))
	}
	item := res.JSON[0]
	// Product (indexing) + Stock (both) should be in text
	wantText := "- Product: Laptop\n- Stock: 50"
	if item["text"] != wantText {
		t.Errorf("item text = %q, want %q", item["text"], wantText)
	}
	// Price (metadata) + Stock (both) should be in chunk_data, NOT Product
	cd, ok := item["chunk_data"].(map[string]any)
	if !ok {
		t.Fatalf("chunk_data missing or not map: %v", item["chunk_data"])
	}
	if cd["Price"] != "999" || cd["Stock"] != "50" {
		t.Errorf("chunk_data = %v", cd)
	}
	if _, hasProduct := cd["Product"]; hasProduct {
		t.Errorf("Product should not be in chunk_data: %v", cd)
	}
}

func TestDecodeTableColumnConfig(t *testing.T) {
	// Nil setup
	mode, roles := DecodeTableColumnConfig(nil)
	if mode != "" || roles != nil {
		t.Errorf("expected empty for nil setup, got mode=%q, roles=%v", mode, roles)
	}

	// map[string]any roles
	setup1 := map[string]any{
		"column_mode": "manual",
		"column_roles": map[string]any{
			"colA": "indexing",
			"colB": "metadata",
		},
	}
	mode1, roles1 := DecodeTableColumnConfig(setup1)
	if mode1 != "manual" {
		t.Errorf("expected mode=manual, got %q", mode1)
	}
	if roles1["colA"] != "indexing" || roles1["colB"] != "metadata" {
		t.Errorf("expected roles colA=indexing, colB=metadata, got %v", roles1)
	}

	// map[string]string roles
	setup2 := map[string]any{
		"column_mode": "auto",
		"column_roles": map[string]string{
			"colC": "both",
		},
	}
	mode2, roles2 := DecodeTableColumnConfig(setup2)
	if mode2 != "auto" {
		t.Errorf("expected mode=auto, got %q", mode2)
	}
	if roles2["colC"] != "both" {
		t.Errorf("expected roles colC=both, got %v", roles2)
	}
}

func TestRenderRowsToJSONChunks_SkipLeadingEmptyRows(t *testing.T) {
	rows := [][]string{
		{"", "  ", ""},
		{"Name", "Age", "Role"},
		{"Alice", "30", "Engineer"},
	}
	items, headers := RenderRowsToJSONChunks(rows, "Sheet1", "auto", nil, TableHeaderRuleSpreadsheet)
	if len(headers) != 3 || headers[0] != "Name" || headers[1] != "Age" || headers[2] != "Role" {
		t.Fatalf("expected headers [Name Age Role], got %v", headers)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	text := items[0]["text"].(string)
	if !strings.Contains(text, "Alice") {
		t.Errorf("expected text to contain Alice, got %q", text)
	}
}
