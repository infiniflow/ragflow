package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestXLSXParser_NormalizesInvalidSheetName(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		mustSetCell(t, f, "Sheet1", "B1", "Amount")
		mustSetCell(t, f, "Sheet1", "A2", "Alice")
		mustSetCell(t, f, "Sheet1", "B2", "100")
	})
	data = xlsxWithWorkbookSheetName(t, data, "Visible:Data")

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(t.Context(), "invalid-sheet-name.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	html := xlsxTableHTML(res)
	for _, want := range []string{"Name", "Amount", "Alice", "100"} {
		if !strings.Contains(html, want) {
			t.Fatalf("parsed table = %q, want %q", html, want)
		}
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "Visible:Data") {
		t.Fatalf("warnings = %v, want sheet-name normalization warning", res.Warnings)
	}
}

func TestNormalizeXLSXForReadPreservesWorkbookXMLOutsideSheetName(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
	})
	data = xlsxWithWorkbookSheetName(t, data, "Visible:Data")
	original := readXLSXEntry(t, data, xlsxWorkbookXML)

	normalized, _, changed, err := normalizeXLSXForRead(data)
	if err != nil {
		t.Fatalf("normalizeXLSXForRead: %v", err)
	}
	if !changed {
		t.Fatal("normalizeXLSXForRead: want changed workbook")
	}

	want := strings.Replace(string(original), `name="Visible:Data"`, `name="Visible_Data"`, 1)
	if got := string(readXLSXEntry(t, normalized, xlsxWorkbookXML)); got != want {
		t.Fatalf("normalized workbook.xml changed bytes outside sheet name:\n got: %s\nwant: %s", got, want)
	}
}

func TestXLSXParser_NormalizesEmojiSheetNameWithinUTF16Limit(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		mustSetCell(t, f, "Sheet1", "A2", "Alice")
	})
	data = xlsxWithWorkbookSheetName(t, data, strings.Repeat("😀", 31)+":")

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(t.Context(), "emoji-sheet-name.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(xlsxTableHTML(res), "Alice") {
		t.Fatalf("parsed table = %q, want Alice", xlsxTableHTML(res))
	}
}

func TestNormalizeWorkbookSheetNamesAvoidsEqualFoldCollisions(t *testing.T) {
	workbook := []byte(`<workbook><sheets><sheet name="'Σ'"/><sheet name="ς"/></sheets></workbook>`)
	normalized, _, changed, err := normalizeWorkbookSheetNames(workbook)
	if err != nil {
		t.Fatalf("normalizeWorkbookSheetNames: %v", err)
	}
	if !changed {
		t.Fatal("normalizeWorkbookSheetNames: want changed workbook")
	}

	var result struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(normalized, &result); err != nil {
		t.Fatalf("Unmarshal normalized workbook: %v", err)
	}
	if len(result.Sheets) != 2 {
		t.Fatalf("normalized sheets = %d, want 2", len(result.Sheets))
	}
	if strings.EqualFold(result.Sheets[0].Name, result.Sheets[1].Name) {
		t.Fatalf("normalized sheet names %q and %q collide under EqualFold", result.Sheets[0].Name, result.Sheets[1].Name)
	}
}

func TestXLSXParser_ReturnsUnrecoverableReadError(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
	})
	data = xlsxWithWorksheetXML(t, data, `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1048577"><c r="A1048577" t="inlineStr"><is><t>invalid row</t></is></c></row></sheetData></worksheet>`)

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(t.Context(), "invalid-row.xlsx", data)
	if res.Err == nil {
		t.Fatal("ParseWithResult: want read error, got nil")
	}
	if !strings.Contains(res.Err.Error(), "row number exceeds") {
		t.Fatalf("error = %q, want row-number context", res.Err)
	}
}

func TestXLSXParser_WarnsWhenMergedCellsCannotBeRead(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		mustSetCell(t, f, "Sheet1", "A2", "Alice")
	})
	data = xlsxWithWorksheetXML(t, data, `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Name</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>Alice</t></is></c></row></sheetData><mergeCells count="1"><mergeCell ref="not-a-range"/></mergeCells></worksheet>`)

	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "bad-merge.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(xlsxTableHTML(res), "Alice") {
		t.Fatalf("parsed table = %q, want sheet data", xlsxTableHTML(res))
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "merged cells") {
		t.Fatalf("warnings = %v, want merged-cells warning", res.Warnings)
	}
}

func TestXLSXParser_WarnsWhenTableMetadataCannotBeRead(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		mustSetCell(t, f, "Sheet1", "A2", "Alice")
		mustAddTable(t, f, "Sheet1", &excelize.Table{Range: "A1:A2", Name: "People"})
	})
	data = rewriteXLSXEntry(t, data, "xl/tables/table1.xml", func([]byte) []byte {
		return []byte(`<table`)
	})

	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "bad-table.xml", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(xlsxTableHTML(res), "Alice") {
		t.Fatalf("parsed table = %q, want sheet data", xlsxTableHTML(res))
	}
	if !strings.Contains(strings.Join(res.Warnings, "\n"), "table metadata") {
		t.Fatalf("warnings = %v, want table-metadata warning", res.Warnings)
	}
}

func xlsxWithWorkbookSheetName(t *testing.T, data []byte, name string) []byte {
	return rewriteXLSXEntry(t, data, "xl/workbook.xml", func(content []byte) []byte {
		updated := strings.Replace(string(content), `name="Sheet1"`, `name="`+name+`"`, 1)
		if updated == string(content) {
			t.Fatal("workbook.xml did not contain Sheet1")
		}
		return []byte(updated)
	})
}

func xlsxWithWorksheetXML(t *testing.T, data []byte, worksheet string) []byte {
	return rewriteXLSXEntry(t, data, "xl/worksheets/sheet1.xml", func([]byte) []byte {
		return []byte(worksheet)
	})
}

func readXLSXEntry(t *testing.T, data []byte, entry string) []byte {
	t.Helper()
	src, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open source xlsx: %v", err)
	}
	for _, file := range src.File {
		if file.Name != entry {
			continue
		}
		r, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", entry, err)
		}
		content, err := io.ReadAll(r)
		closeErr := r.Close()
		if err != nil {
			t.Fatalf("read %s: %v", entry, err)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", entry, closeErr)
		}
		return content
	}
	t.Fatalf("XLSX entry %q not found", entry)
	return nil
}

func rewriteXLSXEntry(t *testing.T, data []byte, entry string, rewrite func([]byte) []byte) []byte {
	t.Helper()
	src, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open source xlsx: %v", err)
	}

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range src.File {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: file.Name, Method: zip.Deflate})
		if err != nil {
			t.Fatalf("create %s: %v", file.Name, err)
		}
		if file.Name == entry {
			r, err := file.Open()
			if err != nil {
				t.Fatalf("open %s: %v", entry, err)
			}
			content, err := io.ReadAll(r)
			closeErr := r.Close()
			if err != nil {
				t.Fatalf("read %s: %v", entry, err)
			}
			if closeErr != nil {
				t.Fatalf("close %s: %v", entry, closeErr)
			}
			if _, err := w.Write(rewrite(content)); err != nil {
				t.Fatalf("write %s: %v", entry, err)
			}
			continue
		}
		r, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", file.Name, err)
		}
		_, err = io.Copy(w, r)
		closeErr := r.Close()
		if err != nil {
			t.Fatalf("copy %s: %v", file.Name, err)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", file.Name, closeErr)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close xlsx: %v", err)
	}
	return out.Bytes()
}
