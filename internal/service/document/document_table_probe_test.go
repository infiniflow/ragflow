package document

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
	ingestiontable "ragflow/internal/ingestion/table"
)

func probeFailureCode(t *testing.T, err error) string {
	t.Helper()
	var probeErr *TableProbeError
	if !errors.As(err, &probeErr) {
		t.Fatalf("error is %T (%v), want a *TableProbeError", err, err)
	}
	return probeErr.Code
}

// TestProbeTableColumnsReadsTheCanonicalHeader: the probe must report what the
// real parser produces, including the position-derived name of an empty header
// and the suffix a repeated header gets, because those strings are what a role
// configuration has to name.
func TestProbeTableColumnsReadsTheCanonicalHeader(t *testing.T) {
	svc := testDocumentService(t)
	csv := "订单,金额,金额,,备注\nDD-1,100,200,300,急\nDD-2,400,500,600,慢\n"

	result, err := svc.ProbeTableColumns(context.Background(), "sales.csv", []byte(csv))
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.Source != "file" {
		t.Errorf("source = %q", result.Source)
	}
	if len(result.Sheets) != 1 {
		t.Fatalf("sheets = %d, want one for csv", len(result.Sheets))
	}
	sheet := result.Sheets[0]
	if sheet.SheetIndex != 1 {
		t.Errorf("sheet_index = %d, want 1", sheet.SheetIndex)
	}
	if sheet.RowCount != 2 {
		t.Errorf("row_count = %d, want 2", sheet.RowCount)
	}
	if len(sheet.Columns) != 5 {
		t.Fatalf("columns = %v", sheet.Columns)
	}

	want := ingestiontable.DeriveColumns([]string{"订单", "金额", "金额", "", "备注"})
	for i, column := range sheet.Columns {
		if column.Index != i+1 {
			t.Errorf("column %d index = %d, want %d", i, column.Index, i+1)
		}
		if column.Key != want[i].Key {
			t.Errorf("column %d key = %q, want the chunker's %q", i, column.Key, want[i].Key)
		}
		if column.DisplayName != want[i].DisplayName {
			t.Errorf("column %d display = %q, want %q", i, column.DisplayName, want[i].DisplayName)
		}
		if column.DataKey != want[i].DataKey {
			t.Errorf("column %d data key = %q, want %q", i, column.DataKey, want[i].DataKey)
		}
	}
	// The empty header stays addressable rather than colliding with its
	// neighbours, and the repeated header is distinguished from the first.
	if sheet.Columns[3].Key != "#column:4" {
		t.Errorf("empty header key = %q", sheet.Columns[3].Key)
	}
	if sheet.Columns[1].Key == sheet.Columns[2].Key {
		t.Error("repeated headers share a key")
	}
}

func TestProbeTableColumnsRejectsUnsupportedFormats(t *testing.T) {
	svc := testDocumentService(t)
	for _, name := range []string{"notes.tsv", "legacy.xls", "report.xlsx.txt"} {
		_, err := svc.ProbeTableColumns(context.Background(), name, []byte("a,b\n1,2\n"))
		if err == nil {
			t.Errorf("%s was accepted", name)
			continue
		}
		if code := probeFailureCode(t, err); code != TableProbeUnsupportedFormat {
			t.Errorf("%s code = %s, want %s", name, code, TableProbeUnsupportedFormat)
		}
	}
}

func TestProbeTableColumnsRefusesOversizedFile(t *testing.T) {
	svc := testDocumentService(t)
	data := make([]byte, TableProbeMaxFileBytes+1)
	_, err := svc.ProbeTableColumns(context.Background(), "big.csv", data)
	if code := probeFailureCode(t, err); code != TableProbeLimit {
		t.Errorf("code = %s, want %s", code, TableProbeLimit)
	}
}

func TestProbeTableColumnsWithoutHeader(t *testing.T) {
	svc := testDocumentService(t)
	_, err := svc.ProbeTableColumns(context.Background(), "empty.csv", []byte("\n\n\n"))
	if code := probeFailureCode(t, err); code != TableProbeHeaderNotFound {
		t.Errorf("code = %s, want %s", code, TableProbeHeaderNotFound)
	}
}

// TestStaleRoleWarnings: a duplicate header takes its suffix from its position,
// so reordering a sheet can strand a role under a key the file no longer has.
func TestStaleRoleWarnings(t *testing.T) {
	doc := &entity.Document{ParserConfig: entity.JSONMap{
		"TableChunker:FastFoxesJump": map[string]any{
			"column_mode":  "manual",
			"column_roles": map[string]any{"金额": "metadata", "折扣": "both"},
		},
		"Tokenizer:SomeNode": map[string]any{"chunk_token_size": 256},
	}}
	sheets := []TableProbeSheet{{Columns: []TableProbeColumn{
		{Index: 1, Key: "金额"},
	}}}

	warnings := staleRoleWarnings(doc, sheets)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "折扣") {
		t.Fatalf("warnings = %v, want only the stranded role", warnings)
	}
	if strings.Contains(warnings[0], "金额") {
		t.Errorf("a role that still resolves was reported: %q", warnings[0])
	}
}
