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
