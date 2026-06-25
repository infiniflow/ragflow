//go:build cgo && manual

package parser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mustConnectOssDeepDoc returns a DeepDocClient pointed at the OSS service.
func mustConnectOssDeepDoc(t *testing.T) *DeepDocClient {
	t.Helper()
	url := os.Getenv("OSSDEEPDOC_URL")
	if url == "" {
		url = "http://localhost:8124"
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

// mustOpenEngine opens a PDF from testdata/pdfs/ and returns a PDFEngine.
func mustOpenEngine(t *testing.T, name string) PDFEngine {
	t.Helper()
	pdfPath := filepath.Join("testdata", "pdfs", name)
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("open engine %s: %v", name, err)
	}
	return eng
}

// TestScanAllPDFs iterates over all PDFs in testdata/pdfs/, parses each
// with OssDeepDoc TSR, and prints a summary. Run with:
//
//	CGO_ENABLED=1 CGO_LDFLAGS="..." go test -tags=manual -run TestScanAllPDFs -v -count=1
func TestScanAllPDFs(t *testing.T) {
	client := mustConnectOssDeepDoc(t)

	pdfDir := filepath.Join("testdata", "pdfs")
	entries, err := os.ReadDir(pdfDir)
	if err != nil {
		t.Fatalf("read pdf dir: %v", err)
	}

	var pdfs []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			pdfs = append(pdfs, e.Name())
		}
	}
	sort.Strings(pdfs)

	fmt.Printf("\n╔══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║  OssDeepDoc PDF Parse Report  (%d PDFs)                      ║\n", len(pdfs))
	fmt.Printf("╚══════════════════════════════════════════════════════════════╝\n")

	for _, name := range pdfs {
		fmt.Printf("\n── %s %s\n", name, strings.Repeat("─", maxint(1, 68-len(name))))

		eng := mustOpenEngine(t, name)
		cfg := DefaultParserConfig()
		cfg.TableBuilder = NewOssDeepDocService(client)
		p := NewParser(cfg, client)
		result, err := p.Parse(context.Background(), eng)
		eng.Close()
		if err != nil {
			fmt.Printf("  ❌ ERROR: %v\n", err)
			continue
		}

		// Sections.
		nSections := len(result.Sections)
		layoutTypes := map[string]int{}
		for _, s := range result.Sections {
			lt := s.LayoutType
			if lt == "" {
				lt = "(empty)"
			}
			layoutTypes[lt]++
		}
		fmt.Printf("  Sections: %d  [", nSections)
		first := true
		for lt, cnt := range layoutTypes {
			if !first {
				fmt.Print(", ")
			}
			fmt.Printf("%s:%d", lt, cnt)
			first = false
		}
		fmt.Println("]")

		// Tables.
		nTables := len(result.Tables)
		fmt.Printf("  Tables:   %d\n", nTables)
		for i, tbl := range result.Tables {
			nr := len(tbl.Grid)
			nc := 0
			if nr > 0 {
				nc = len(tbl.Grid[0])
			}
			sample := ""
			for _, row := range tbl.Grid {
				for _, cell := range row {
					s := strings.TrimSpace(cell.Text)
					if s != "" {
						sample = s
						goto found
					}
				}
			}
		found:
			if len(sample) > 40 {
				sample = sample[:40] + "..."
			}
			fmt.Printf("    [%d] %d×%d  %q\n", i, nr, nc, sample)
		}

		// First text snippet.
		textLen := 0
		for _, s := range result.Sections {
			txt := strings.TrimSpace(s.Text)
			if txt == "" || s.LayoutType == "table" {
				continue
			}
			if textLen == 0 {
				if len(txt) > 80 {
					txt = txt[:80] + "..."
				}
				fmt.Printf("  First text: %q\n", txt)
			}
			textLen += len(txt)
			if textLen > 160 {
				break
			}
		}
	}
	fmt.Println()
}

func maxint(a, b int) int {
	if a > b {
		return a
	}
	return b
}
