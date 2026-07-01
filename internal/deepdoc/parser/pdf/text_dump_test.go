//go:build cgo && manual

package pdf

import (
	"context"
	"os"
	"path/filepath"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"strings"
	"testing"
)

// TestDumpTextOutput runs Parse on real PDFs and saves per-PDF text
// to testdata/output/go/noocr/text/{pdf}.txt. Set DUMP_COUNT env to limit first N PDFs.
func TestDumpTextOutput(t *testing.T) {
	pdfDir := filepath.Join("testdata", "real_pdfs")
	outDir := filepath.Join("testdata", "output", "go", "noocr", "text")
	os.MkdirAll(outDir, 0755)

	entries, err := os.ReadDir(pdfDir)
	if err != nil {
		t.Fatal(err)
	}

	count := len(entries)
	if n := os.Getenv("DUMP_COUNT"); n != "" {
		c := 0
		for _, ch := range n {
			c = c*10 + int(ch-'0')
		}
		if c > 0 && c < count {
			count = c
		}
	}

	totalChars := 0
	for i, e := range entries {
		if i >= count {
			break
		}
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			continue
		}
		name := e.Name()
		outPath := filepath.Join(outDir, name+".txt")
		if _, err := os.Stat(outPath); err == nil {
			data, _ := os.ReadFile(outPath)
			n := len(data)
			totalChars += n
			t.Logf("[%d/%d] %s — SKIP (%d chars)", i+1, count, name, n)
			continue
		}

		pdfPath := filepath.Join(pdfDir, name)
		data, err := os.ReadFile(pdfPath)
		if err != nil {
			t.Logf("[%d/%d] %s — read error: %v", i+1, count, name, err)
			continue
		}

		eng, err := NewEngine(data)
		if err != nil {
			t.Logf("[%d/%d] %s — engine error: %v", i+1, count, name, err)
			continue
		}

		cfg := pdf.DefaultParserConfig()
		p := NewParser(cfg)
		result, err := p.ParseRaw(context.Background(), eng, &MockDocAnalyzer{Healthy: true})
		eng.Close()
		if err != nil {
			t.Logf("[%d/%d] %s — parse error: %v", i+1, count, name, err)
			continue
		}

		var sb strings.Builder
		for _, s := range result.Sections {
			sb.WriteString(s.Text)
			sb.WriteByte('\n')
		}
		text := sb.String()
		os.WriteFile(outPath, []byte(text), 0644)

		totalChars += len(text)
		t.Logf("[%d/%d] %s — %d chars", i+1, count, name, len(text))
	}

	t.Logf("Done. %d chars total. Output: %s/", totalChars, outDir)
}
