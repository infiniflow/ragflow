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

package io

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

func TestWriteXLSX_ParsesTablesIntoSheets(t *testing.T) {
	content := "Intro line.\n\nTable 1: Sales\n\n| Name | Amount |\n|---|---|\n| Alice | 1,234 |\n| Bob | 00123 |\n\n## Report\n\n| Q | Revenue |\n|---|---|\n| Q1 | 42.5 |\n"
	payload, err := WriteXLSX(content, XLSXOptions{})
	if err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenReader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("sheets = %v, want 2", sheets)
	}
	wantSheets := map[string]bool{"Table 1 Sales": false, "Report": false}
	for _, s := range sheets {
		if _, ok := wantSheets[s]; ok {
			wantSheets[s] = true
		}
	}
	for name, found := range wantSheets {
		if !found {
			t.Errorf("sheet %q missing, got %v", name, sheets)
		}
	}

	// Numeric coercion: "1,234" → 1234 (number), "00123" stays text.
	sales := sheets[0]
	if v, err := f.GetCellValue(sales, "B2"); err != nil || v != "1234" {
		t.Errorf("B2 = %q (%v), want 1234", v, err)
	}
	if v, err := f.GetCellValue(sales, "B3"); err != nil || v != "00123" {
		t.Errorf("B3 = %q (%v), want 00123 kept as text", v, err)
	}
	report := sheets[1]
	if v, err := f.GetCellValue(report, "B2"); err != nil || v != "42.5" {
		t.Errorf("report B2 = %q (%v), want 42.5", v, err)
	}
}

func TestWriteXLSX_NoTableFallsBackToDataSheet(t *testing.T) {
	payload, err := WriteXLSX("Just plain text\nwithout tables", XLSXOptions{})
	if err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenReader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Data" {
		t.Fatalf("sheets = %v, want [Data]", sheets)
	}
	v, err := f.GetCellValue("Data", "A1")
	if err != nil || !strings.Contains(v, "Just plain text") {
		t.Errorf("A1 = %q (%v), want the whole content", v, err)
	}
}

func TestWriteXLSX_SheetNameTruncatesByCharacters(t *testing.T) {
	// A CJK title (3 bytes per character) must be clamped to 31
	// characters, not 31 bytes: byte slicing would split a rune and
	// produce a garbled sheet name.
	title := "## " + strings.Repeat("销", 40)
	content := title + "\n\n| 列一 | 列二 |\n|---|---|\n| 值 | 1 |\n"

	payload, err := WriteXLSX(content, XLSXOptions{})
	if err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenReader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 1 {
		t.Fatalf("sheets = %v, want 1", sheets)
	}
	name := sheets[0]
	if !utf8.ValidString(name) {
		t.Errorf("sheet name %q is not valid UTF-8", name)
	}
	if got := utf8.RuneCountInString(name); got != 31 {
		t.Errorf("sheet name %q has %d characters, want 31", name, got)
	}
	if want := strings.Repeat("销", 31); !strings.HasPrefix(name, want) {
		t.Errorf("sheet name = %q, want the first 31 characters of the title (%q...)", name, want)
	}
	if v, err := f.GetCellValue(name, "A2"); err != nil || v != "值" {
		t.Errorf("A2 = %q (%v), want 值", v, err)
	}
}

func TestWriteXLSX_DuplicateSheetNamesGetUnderscoreSuffix(t *testing.T) {
	content := "## Report\n\n| A |\n|---|\n| 1 |\n\n## Report\n\n| B |\n|---|\n| 2 |\n"

	payload, err := WriteXLSX(content, XLSXOptions{})
	if err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenReader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("sheets = %v, want 2", sheets)
	}
	want := map[string]bool{"Report": false, "Report_1": false}
	for _, s := range sheets {
		if _, ok := want[s]; ok {
			want[s] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("sheet %q missing, got %v", name, sheets)
		}
	}
}

func TestWriteXLSX_EmptyTablesAreDropped(t *testing.T) {
	// A header-only table with no data rows produces no sheet.
	content := "## Empty\n\n| A | B |\n|---|---|\n\n## Full\n\n| C |\n|---|\n| 2 |\n"

	payload, err := WriteXLSX(content, XLSXOptions{})
	if err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenReader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Full" {
		t.Fatalf("sheets = %v, want [Full] (header-only table dropped)", sheets)
	}
}

func TestWriteXLSX_AllTablesEmptyFallsBackToDataSheet(t *testing.T) {
	content := "| A |\n|---|\n"

	payload, err := WriteXLSX(content, XLSXOptions{})
	if err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenReader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "Data" {
		t.Fatalf("sheets = %v, want [Data]", sheets)
	}
	v, err := f.GetCellValue("Data", "A1")
	if err != nil || !strings.Contains(v, "| A |") {
		t.Errorf("A1 = %q (%v), want the whole content", v, err)
	}
}

func TestWriteXLSX_SheetNameCollisionCaseInsensitive(t *testing.T) {
	// Excelize resolves sheet names case-insensitively (EqualFold), so
	// "Report" and "report" must not land on the same sheet: the second
	// table would silently overwrite the first one's rows.
	content := "## Report\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n## report\n\n| C | D |\n|---|---|\n| 3 | 4 |\n"

	payload, err := WriteXLSX(content, XLSXOptions{})
	if err != nil {
		t.Fatalf("WriteXLSX: %v", err)
	}
	f, err := excelize.OpenReader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("sheets = %v, want 2 distinct sheets", sheets)
	}
	if sheets[0] != "Report" || sheets[1] != "report_1" {
		t.Errorf("sheets = %v, want [Report report_1] (case-insensitive collision suffixed)", sheets)
	}
	// Both tables' data must survive; before the fix the second table
	// overwrote the first sheet's rows.
	if v, err := f.GetCellValue("Report", "A2"); err != nil || v != "1" {
		t.Errorf("Report A2 = %q (%v), want 1", v, err)
	}
	if v, err := f.GetCellValue("Report", "B2"); err != nil || v != "2" {
		t.Errorf("Report B2 = %q (%v), want 2", v, err)
	}
	if v, err := f.GetCellValue("report_1", "A2"); err != nil || v != "3" {
		t.Errorf("report_1 A2 = %q (%v), want 3", v, err)
	}
	if v, err := f.GetCellValue("report_1", "B2"); err != nil || v != "4" {
		t.Errorf("report_1 B2 = %q (%v), want 4", v, err)
	}
}
