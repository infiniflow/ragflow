package utility

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestCountXLSXRowsSumsAllSheets(t *testing.T) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	writeSheet := func(name string, rows int) {
		t.Helper()
		file, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		var sheet bytes.Buffer
		sheet.WriteString(`<worksheet><sheetData>`)
		for i := 0; i < rows; i++ {
			sheet.WriteString(`<row r="1"></row>`)
		}
		sheet.WriteString(`</sheetData></worksheet>`)
		if _, err := file.Write(sheet.Bytes()); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}

	writeSheet("xl/worksheets/sheet1.xml", 3)
	writeSheet("xl/worksheets/sheet2.xml", 5)

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	rows, err := CountXLSXRows(buf.Bytes())
	if err != nil {
		t.Fatalf("CountXLSXRows returned error: %v", err)
	}
	if rows != 8 {
		t.Fatalf("CountXLSXRows() = %d, want 8", rows)
	}
}

func TestCountSpreadsheetRowsUsesSummedXLSXRows(t *testing.T) {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	for name, rows := range map[string]int{
		"xl/worksheets/sheet1.xml": 2,
		"xl/worksheets/sheet2.xml": 4,
	} {
		file, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		var sheet bytes.Buffer
		sheet.WriteString(`<worksheet><sheetData>`)
		for i := 0; i < rows; i++ {
			sheet.WriteString(`<row></row>`)
		}
		sheet.WriteString(`</sheetData></worksheet>`)
		if _, err := file.Write(sheet.Bytes()); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}

	if got := CountSpreadsheetRows("book.xlsx", buf.Bytes()); got != 6 {
		t.Fatalf("CountSpreadsheetRows() = %d, want 6", got)
	}
}
