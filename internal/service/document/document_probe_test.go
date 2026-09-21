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
	"errors"
	"io"
	"reflect"
	"testing"

	"ragflow/internal/parser/parser"

	"github.com/xuri/excelize/v2"
)

// A probe is only useful while a name it reports is the name ingestion creates,
// so it reads through the parser's own header rules. These cases pin the shapes
// the dispatch has to carry over from each reader: a header that does not start
// the file, a repeated or blank name, the bookkeeping columns the renderer
// deletes, a BOM, and the delimiter each suffix selects.
func TestProbeTable_ColumnNames(t *testing.T) {
	workbook := func(rows [][]string) []byte {
		f := excelize.NewFile()
		defer f.Close()
		for i, row := range rows {
			for j, cell := range row {
				name, err := excelize.CoordinatesToCellName(j+1, i+1)
				if err != nil {
					t.Fatalf("coordinate: %v", err)
				}
				if err := f.SetCellValue("Sheet1", name, cell); err != nil {
					t.Fatalf("set cell: %v", err)
				}
			}
		}
		var buf bytes.Buffer
		if err := f.Write(&buf); err != nil {
			t.Fatalf("write xlsx: %v", err)
		}
		return buf.Bytes()
	}

	cases := []struct {
		name     string
		filename string
		content  []byte
		want     []string
	}{
		{
			name:     "a repeated name is suffixed once the rows above the header are skipped",
			filename: "test.csv",
			content:  []byte("\n\n  \nName,City,Name\nAlice,Paris,Bob\n"),
			want:     []string{"Name", "City", "Name_2"},
		},
		{
			name:     "bookkeeping columns never reach a chunk, so the probe must not offer them",
			filename: "test.csv",
			content:  []byte("id,name,index,amount\n1,Alice,2,10\n"),
			want:     []string{"name", "amount"},
		},
		{
			name:     "a BOM is not part of the first name",
			filename: "bom.csv",
			content:  []byte("\ufeffName,City\nAlice,Paris\n"),
			want:     []string{"Name", "City"},
		},
		{
			name:     "a blank delimited cell keeps its empty name",
			filename: "data.tsv",
			content:  []byte("col1\t\tcol3\nval1\tval2\tval3\n"),
			want:     []string{"col1", "", "col3"},
		},
		{
			name:     "a txt table is tab-separated, so a comma does not split its header",
			filename: "data.txt",
			content:  []byte("name,full\tamount\nAlice,Ann\t10\n"),
			want:     []string{"name,full", "amount"},
		},
		{
			name:     "a sheet whose first row is empty is read from its next row",
			filename: "catalog.xlsx",
			content:  workbook([][]string{{""}, {"Product", "Price", "Product"}}),
			want:     []string{"Product", "Price", "Product_2"},
		},
	}

	svc := &DocumentService{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cols, err := svc.ProbeTable(bytes.NewReader(tc.content), tc.filename)
			if err != nil {
				t.Fatalf("ProbeTable: %v", err)
			}
			if !reflect.DeepEqual(cols, tc.want) {
				t.Fatalf("got %#v, want %#v", cols, tc.want)
			}
		})
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
			probed, err := svc.ProbeTable(bytes.NewReader([]byte(tt.content)), tt.filename)
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

// The probe must not read an unbounded workbook: it stops at the cap. The
// archive is then truncated,
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

// Binary xls and any other suffix are declined with the sentinel the client
// reads as "extract the header locally", not as a server failure.
func TestProbeTable_UnsupportedFormats(t *testing.T) {
	svc := &DocumentService{}
	for _, filename := range []string{"data.xls", "doc.pdf"} {
		t.Run(filename, func(t *testing.T) {
			cols, err := svc.ProbeTable(bytes.NewReader([]byte("not a table")), filename)
			if err == nil {
				t.Fatalf("expected an error, got cols: %v", cols)
			}
			if !errors.Is(err, ErrUnsupportedTableFormat) {
				t.Fatalf("must report the unsupported-format sentinel, got %v", err)
			}
		})
	}
}
