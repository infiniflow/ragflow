//go:build cgo && manual

package pdf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"sort"
	"strings"
	"testing"
)

// TestScanAllPDFs iterates over all PDFs in testdata/pdfs/, parses each
// with OssDeepDoc TSR, and prints a summary. Run with:
//
//	CGO_ENABLED=1 CGO_LDFLAGS="..." go test -tags=manual -run TestScanAllPDFs -v -count=1
func TestScanAllPDFs(t *testing.T) {
	client := mustConnectInferenceClient(t)

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
		cfg := pdf.DefaultParserConfig()
		p := NewParser(cfg)
		result, err := p.ParseRaw(context.Background(), eng, client)
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
