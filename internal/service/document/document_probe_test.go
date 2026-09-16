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
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestProbeTable_CSV(t *testing.T) {
	svc := &DocumentService{}

	csvData := "\n\n  \nName,City,Name\nAlice,Paris,Bob\n"
	cols, err := svc.ProbeTable(strings.NewReader(csvData), "test.csv")
	if err != nil {
		t.Fatalf("ProbeTable: %v", err)
	}

	want := []string{"Name", "City", "Name_2"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("got %#v, want %#v", cols, want)
	}
}

func TestProbeTable_CSVStripsBOM(t *testing.T) {
	svc := &DocumentService{}

	cols, err := svc.ProbeTable(strings.NewReader("\ufeffName,City\nAlice,Paris\n"), "bom.csv")
	if err != nil {
		t.Fatalf("ProbeTable: %v", err)
	}

	// The parser strips the BOM too (csv_parser.go), so the probe must report
	// the same names; a leading U+FEFF would never match a column role.
	want := []string{"Name", "City"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("got %#v, want %#v", cols, want)
	}
}

func TestProbeTable_TSV(t *testing.T) {
	svc := &DocumentService{}

	tsvData := "col1\t\tcol3\nval1\tval2\tval3\n"
	cols, err := svc.ProbeTable(strings.NewReader(tsvData), "data.tsv")
	if err != nil {
		t.Fatalf("ProbeTable: %v", err)
	}

	want := []string{"col1", "Column_2", "col3"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("got %#v, want %#v", cols, want)
	}
}

func TestProbeTable_XLSX(t *testing.T) {
	svc := &DocumentService{}

	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "")
	_ = f.SetCellValue(sheet, "A2", "Product")
	_ = f.SetCellValue(sheet, "B2", "Price")
	_ = f.SetCellValue(sheet, "C2", "Product")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	cols, err := svc.ProbeTable(&buf, "catalog.xlsx")
	if err != nil {
		t.Fatalf("ProbeTable: %v", err)
	}

	want := []string{"Product", "Price", "Product_2"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("got %#v, want %#v", cols, want)
	}
}

func TestProbeTable_XLS_Fallback(t *testing.T) {
	svc := &DocumentService{}
	cols, err := svc.ProbeTable(strings.NewReader("fake xls binary"), "data.xls")
	if err == nil {
		t.Fatalf("expected error for binary xls probe, got cols: %v", cols)
	}
	if !strings.Contains(err.Error(), "binary xls") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestProbeTable_UnsupportedFormat(t *testing.T) {
	svc := &DocumentService{}
	cols, err := svc.ProbeTable(strings.NewReader("pdf content"), "doc.pdf")
	if err == nil {
		t.Fatalf("expected error for pdf probe, got cols: %v", cols)
	}
	if !strings.Contains(err.Error(), "unsupported table format") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
