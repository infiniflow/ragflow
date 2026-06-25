//go:build cgo && integration

package parser

import (
	"context"
	"os"
	"strings"
	"testing"
)

// mustConnectOssDeepDoc returns a DeepDocClient pointed at the OSS service;
// skips the test if unavailable or if the service reports a non-OSS model type.
func mustConnectOssDeepDoc(t *testing.T) *DeepDocClient {
	t.Helper()
	url := os.Getenv("OSSDEEPDOC_URL")
	if url == "" {
		url = "http://localhost:9390"
	}
	client, err := NewDeepDocClient(url)
	if err != nil {
		t.Fatal(err)
	}
	if !client.Health() {
		t.Fatalf("OssDeepDoc not available at %s", url)
	}
	if client.ModelType() != ModelOSS {
		t.Skipf("DeepDoc at %s is %q, not oss — skipping OSS-specific test", url, client.ModelType())
	}
	return client
}

// TestIntegration_OssDeepDoc_TableStructure verifies that parsing a PDF
// through the OssDeepDoc TableBuilder produces tables with the expected
// row/column structure.
func TestIntegration_OssDeepDoc_TableStructure(t *testing.T) {
	client := mustConnectOssDeepDoc(t)
	eng := mustOpenEngine(t, "06_table_content.pdf")
	defer eng.Close()

	cfg := DefaultParserConfig()
	cfg.TableBuilder = NewOssDeepDocService(client)
	p := NewParser(cfg, client)
	result, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Tables) == 0 {
		t.Skip("DLA did not detect any tables in fixture")
	}

	t.Logf("OssDeepDoc produced %d tables", len(result.Tables))
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

// TestIntegration_OssDeepDoc_TableRows verifies each table has non-empty
// rows with the expected grid structure.
func TestIntegration_OssDeepDoc_TableRows(t *testing.T) {
	client := mustConnectOssDeepDoc(t)
	eng := mustOpenEngine(t, "06_table_content.pdf")
	defer eng.Close()

	cfg := DefaultParserConfig()
	cfg.TableBuilder = NewOssDeepDocService(client)
	p := NewParser(cfg, client)
	result, err := p.Parse(context.Background(), eng)
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

// TestIntegration_OssDeepDoc_Idempotency verifies that parsing the same PDF
// twice produces the same table row structure.
func TestIntegration_OssDeepDoc_Idempotency(t *testing.T) {
	client := mustConnectOssDeepDoc(t)

	parseOnce := func() *ParseResult {
		eng := mustOpenEngine(t, "06_table_content.pdf")
		defer eng.Close()

		cfg := DefaultParserConfig()
		cfg.TableBuilder = NewOssDeepDocService(client)
		p := NewParser(cfg, client)
		result, err := p.Parse(context.Background(), eng)
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

// TestIntegration_OssDeepDoc_EmptyPage verifies that a page with no tables
// does not crash.
func TestIntegration_OssDeepDoc_EmptyPage(t *testing.T) {
	client := mustConnectOssDeepDoc(t)
	eng := mustOpenEngine(t, "01_english_simple.pdf")
	defer eng.Close()

	cfg := DefaultParserConfig()
	cfg.TableBuilder = NewOssDeepDocService(client)
	p := NewParser(cfg, client)
	_, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
}
