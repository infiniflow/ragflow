package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadLegacyXLSFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "sample_form.xls"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if officeContainer(data) != "ole" {
		t.Fatal("fixture is not an OLE2 container")
	}
	return data
}

func TestXLSParser_BIFFWorkbookFormSheet(t *testing.T) {
	data := loadLegacyXLSFixture(t)
	p, err := NewXLSParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}
	res := p.ParseWithResult(context.Background(), "sample_form.xls", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("no items produced")
	}
	var sawKryci, sawKVValue, sawVykaz bool
	for _, item := range res.JSON {
		text, _ := item["text"].(string)
		if strings.Contains(text, "Sheet: Krycí list") {
			sawKryci = true
			if strings.Contains(text, "Cena bez DPH") {
				sawKVValue = true
			}
		}
		if strings.Contains(text, "<th>Kód</th>") || strings.Contains(text, "<th>Množstvo</th>") {
			sawVykaz = true
		}
	}
	if !sawKryci {
		t.Errorf("missing KV chunk for form sheet 'Krycí list'; items=%v", res.JSON)
	}
	if !sawKVValue {
		t.Errorf("KV chunk missing 'Cena bez DPH' label")
	}
	if !sawVykaz {
		t.Errorf("missing table chunk with header cells for sheet 'Výkaz'")
	}
}

func TestXLSParser_BIFFLegacyXlrdSource(t *testing.T) {
	data := loadLegacyXLSFixture(t)
	src, err := newXlrdBIFFSource(data)
	if err != nil {
		t.Fatalf("newXlrdBIFFSource: %v", err)
	}
	defer src.Close()
	if src.SheetCount() != 2 {
		t.Fatalf("SheetCount = %d, want 2", src.SheetCount())
	}
	var names []string
	var values []any
	for src.NextSheet() {
		names = append(names, src.SheetName())
		for src.Read() {
			for c := 0; c < src.FieldCount(); c++ {
				if v := src.GetValue(c); v != nil {
					values = append(values, v)
				}
			}
		}
	}
	if names[0] != "Krycí list" || names[1] != "Výkaz" {
		t.Errorf("sheet names = %v", names)
	}
	foundPrice := false
	for _, v := range values {
		if f, ok := v.(float64); ok && f == 5391273.95 {
			foundPrice = true
		}
	}
	if !foundPrice {
		t.Error("xlrd source did not surface numeric cell 5391273.95")
	}
}

func TestXLSXParser_OLEPayloadWithXlsxName(t *testing.T) {
	data := loadLegacyXLSFixture(t)
	p, err := NewXLSXParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(context.Background(), "misnamed.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("no items produced for OLE payload")
	}
}

func TestXLSParser_InvalidData(t *testing.T) {
	p, err := NewXLSParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}
	res := p.ParseWithResult(context.Background(), "bad.xls", []byte("not a spreadsheet"))
	if res.Err == nil {
		t.Fatal("expected error for invalid data")
	}
}
