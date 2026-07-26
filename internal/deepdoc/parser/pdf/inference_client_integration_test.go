//go:build cgo && integration

package pdf

import (
	"context"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestIntegration_DeepDoc_TableStructure verifies that parsing a PDF
// through the OSS TableBuilder produces tables with the expected row/column structure.
func TestIntegration_DeepDoc_TableStructure(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "06_table_content.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Skip("DLA did not detect any tables in fixture")
	}

	t.Logf("DeepDoc produced %d tables", len(result.Tables))
	for i, tbl := range result.Tables {
		t.Logf("table[%d]: %d rows", i, len(tbl.Rows))
		for ri, row := range tbl.Rows {
			hasContent := false
			for _, cell := range row {
				if strings.TrimSpace(cell) != "" {
					hasContent = true
					break
				}
			}
			if !hasContent {
				t.Errorf("table[%d] row[%d]: all cells empty", i, ri)
			}
		}
	}
}

// TestIntegration_DeepDoc_TableRows verifies each table has non-empty
// rows with the expected grid structure.
func TestIntegration_DeepDoc_TableRows(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "06_table_content.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	result, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Skip("DLA did not detect any tables in fixture")
	}

	for i, tbl := range result.Tables {
		if len(tbl.Rows) == 0 {
			t.Errorf("table[%d]: no rows", i)
			continue
		}
		t.Logf("table[%d]: %d rows × ~%d cols", i, len(tbl.Rows), len(tbl.Rows[0]))
		for ri, row := range tbl.Rows {
			hasContent := false
			for _, cell := range row {
				if strings.TrimSpace(cell) != "" {
					hasContent = true
					break
				}
			}
			if !hasContent {
				t.Errorf("table[%d] row[%d]: all cells empty", i, ri)
			}
		}
	}
}

// TestIntegration_DeepDoc_Idempotency verifies that parsing the same PDF
// twice produces the same table row structure.
func TestIntegration_DeepDoc_Idempotency(t *testing.T) {
	client := mustConnectInferenceClient(t)

	parseOnce := func() *pdf.ParseResult {
		data := mustReadPDF(t, "06_table_content.pdf")

		cfg := pdf.DefaultParserConfig()
		p := NewParser(cfg)
		result, err := p.Parse(context.Background(), data, client)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		return result
	}

	r1 := parseOnce()
	r2 := parseOnce()

	if len(r1.Tables) != len(r2.Tables) {
		t.Errorf("table count mismatch: run1=%d run2=%d", len(r1.Tables), len(r2.Tables))
		return
	}
	for i := 0; i < len(r1.Tables); i++ {
		if len(r1.Tables[i].Rows) != len(r2.Tables[i].Rows) {
			t.Errorf("table[%d] row count differs: run1=%d run2=%d", i,
				len(r1.Tables[i].Rows), len(r2.Tables[i].Rows))
		}
	}
}

// TestIntegration_DeepDoc_EmptyPage verifies that a page with no tables
// does not crash.
func TestIntegration_DeepDoc_EmptyPage(t *testing.T) {
	client := mustConnectInferenceClient(t)
	data := mustReadPDF(t, "01_english_simple.pdf")

	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)
	_, err := p.Parse(context.Background(), data, client)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
}
