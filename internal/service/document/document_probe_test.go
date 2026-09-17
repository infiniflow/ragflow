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
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/parser/parser"

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

// The parser deletes the spreadsheet bookkeeping columns before rendering, so
// the probe must not offer a role for a column that never reaches a chunk.
func TestProbeTable_DropsBookkeepingColumns(t *testing.T) {
	svc := &DocumentService{}

	cols, err := svc.ProbeTable(strings.NewReader("id,name,index,amount\n1,Alice,2,10\n"), "test.csv")
	if err != nil {
		t.Fatalf("ProbeTable: %v", err)
	}

	want := []string{"name", "amount"}
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

	// A delimited header is indexed as read, so the blank cell keeps its empty
	// name; inventing Column_2 here would offer a role for a column that never
	// exists in the index.
	want := []string{"col1", "", "col3"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("got %#v, want %#v", cols, want)
	}
}

// The probe is only useful while a name it reports is the name ingestion
// creates, so both kinds are compared against the parser that indexes the file
// rather than against a second copy of the rules.
func TestProbeTable_MatchesIngestionColumnNames(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
	}{
		{name: "csv", filename: "a.csv", content: " Name,,id,Name,amount\nx,y,z,w,v\n"},
		{name: "tsv", filename: "a.tsv", content: "col1\t\tcol3\n1\t2\t3\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &DocumentService{}
			probed, err := svc.ProbeTable(strings.NewReader(tt.content), tt.filename)
			if err != nil {
				t.Fatalf("ProbeTable: %v", err)
			}

			p := parser.NewCSVParser()
			p.OutputFormat = "json"
			p.ColumnMode = "auto"
			res := p.ParseWithResult(context.Background(), tt.filename, []byte(tt.content))
			if res.Err != nil {
				t.Fatalf("parse: %v", res.Err)
			}
			indexed, _ := res.File["table_column_names"].([]string)

			if !reflect.DeepEqual(probed, indexed) {
				t.Fatalf("probe = %#v, ingestion = %#v", probed, indexed)
			}
		})
	}
}

func TestProbeTable_XLSXMatchesIngestionColumnNames(t *testing.T) {
	f := excelize.NewFile()
	cells := []string{" Product", "", "id", "Product", "Price"}
	for i, name := range cells {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			t.Fatalf("coordinate: %v", err)
		}
		if err := f.SetCellValue("Sheet1", cell, name); err != nil {
			t.Fatalf("set header: %v", err)
		}
	}
	if err := f.SetCellValue("Sheet1", "A2", "widget"); err != nil {
		t.Fatalf("set cell: %v", err)
	}
	// A later sheet can carry columns the first one does not, and ingestion
	// indexes them all, so the probe must offer them too.
	if _, err := f.NewSheet("Sheet2"); err != nil {
		t.Fatalf("new sheet: %v", err)
	}
	for i, name := range []string{"Warehouse", "Product", "Qty"} {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			t.Fatalf("coordinate: %v", err)
		}
		if err := f.SetCellValue("Sheet2", cell, name); err != nil {
			t.Fatalf("set header: %v", err)
		}
	}
	if err := f.SetCellValue("Sheet2", "A2", "W1"); err != nil {
		t.Fatalf("set cell: %v", err)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}
	workbook := buf.Bytes()

	svc := &DocumentService{}
	probed, err := svc.ProbeTable(bytes.NewReader(workbook), "catalog.xlsx")
	if err != nil {
		t.Fatalf("ProbeTable: %v", err)
	}

	p, err := parser.NewXLSXParser("")
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}
	p.OutputFormat = "json"
	p.ColumnMode = "auto"
	res := p.ParseWithResult(context.Background(), "catalog.xlsx", workbook)
	if res.Err != nil {
		t.Fatalf("parse: %v", res.Err)
	}
	indexed, _ := res.File["table_column_names"].([]string)

	if !reflect.DeepEqual(probed, indexed) {
		t.Fatalf("probe = %#v, ingestion = %#v", probed, indexed)
	}
	want := []string{"Product", "Column_2", "Product_2", "Price", "Warehouse", "Qty"}
	if !reflect.DeepEqual(probed, want) {
		t.Fatalf("probe = %#v, want %#v", probed, want)
	}
}

// A .txt table is tab-separated, like a .tsv (rag/app/table.py splits it on
// "\t"), so a comma must not split its header into extra columns.
func TestProbeTable_TXTUsesTabDelimiter(t *testing.T) {
	svc := &DocumentService{}

	cols, err := svc.ProbeTable(strings.NewReader("name,full\tamount\nAlice,Ann\t10\n"), "data.txt")
	if err != nil {
		t.Fatalf("ProbeTable: %v", err)
	}

	want := []string{"name,full", "amount"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("got %#v, want %#v", cols, want)
	}
}

// countingReader records how many bytes a probe pulls from the stream.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

// The probe must not read an unbounded workbook: like the Python endpoint
// (max_excel_probe_bytes), it stops at the cap. The archive is then truncated,
// the open fails, and the client falls back to local extraction instead of the
// server reading an arbitrarily large upload.
func TestProbeTable_XLSXStopsAtProbeLimit(t *testing.T) {
	f := excelize.NewFile()
	if err := f.SetCellValue("Sheet1", "A1", "Product"); err != nil {
		t.Fatalf("set cell: %v", err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	payload := make([]byte, 0, buf.Len()+probeXLSXMaxBytes+1)
	payload = append(payload, buf.Bytes()...)
	payload = append(payload, make([]byte, probeXLSXMaxBytes+1)...)

	counter := &countingReader{r: bytes.NewReader(payload)}
	svc := &DocumentService{}
	if _, err := svc.ProbeTable(counter, "huge.xlsx"); err == nil {
		t.Fatal("expected the oversized workbook stream to be rejected")
	}
	if counter.n > probeXLSXMaxBytes {
		t.Fatalf("probe read %d bytes, want at most %d", counter.n, probeXLSXMaxBytes)
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
