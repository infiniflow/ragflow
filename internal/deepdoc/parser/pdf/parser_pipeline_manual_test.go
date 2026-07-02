//go:build cgo && manual

package pdf

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"strings"
	"testing"
)

// TestIntegration_NoCrash runs Parse on every small fixture PDF and checks it
// does not panic or error. It does NOT require golden files.
//
// Build tag: cgo && manual — skipped in regular integration runs due to
// long runtime (27+ PDFs each requiring DeepDoc DLA+TSR+OCR).
func TestIntegration_NoCrash(t *testing.T) {
	client := mustConnectInferenceClient(t)

	pdfDir := filepath.Join("testdata", "pdfs")
	entries, err := os.ReadDir(pdfDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pdf") {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pdfPath := filepath.Join(pdfDir, name)
			data, err := os.ReadFile(pdfPath)
			if err != nil {
				t.Fatal(err)
			}

			eng, err := NewEngine(data)
			if err != nil {
				t.Fatalf("engine: %v", err)
			}
			defer eng.Close()

			cfg := pdf.DefaultParserConfig()
			p := NewParser(cfg)
			result, err := p.ParseRaw(context.Background(), eng, client)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			// Structural invariants — these should always hold.
			for i, s := range result.Sections {
				if s.PositionTag == "" {
					t.Errorf("section[%d] has empty PositionTag", i)
				}
				if s.LayoutType != "" && s.Image != "" {
					// pdf.Section with an image should have valid base64.
					if _, err := base64.StdEncoding.DecodeString(s.Image); err != nil {
						t.Errorf("section[%d] Image: not valid base64: %v", i, err)
					}
				}
				if s.TableItem != nil {
					// Cross-reference: pdf.TableItem in section should appear in tables list.
					found := false
					for _, tbl := range result.Tables {
						if &tbl == s.TableItem {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("section[%d] pdf.TableItem not found in tables list", i)
					}
				}
			}

			for i, tbl := range result.Tables {
				if tbl.ImageB64 == "" {
					t.Errorf("table[%d] ImageB64 is empty", i)
				}
				if len(tbl.Positions) == 0 {
					t.Errorf("table[%d] has no positions", i)
				}
			}

			t.Logf("%s: %d sections, %d tables", name, len(result.Sections), len(result.Tables))
		})
	}
}
