//go:build cgo && manual

package parser

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustConnectDeepDoc returns a DeepDocClient; skips the test if unavailable.
func mustConnectDeepDoc(t *testing.T) *DeepDocClient {
	t.Helper()
	url := os.Getenv("DEEPDOC_URL")
	if url == "" {
		url = "http://localhost:8000"
	}
	client, err := NewDeepDocClient(url)
	if err != nil {
		t.Fatal(err)
	}
	if !client.Health() {
		t.Fatalf("DeepDoc not available at %s", url)
	}
	return client
}

// TestIntegration_NoCrash runs Parse on every small fixture PDF and checks it
// does not panic or error. It does NOT require golden files.
//
// Build tag: cgo && manual — skipped in regular integration runs due to
// long runtime (27+ PDFs each requiring DeepDoc DLA+TSR+OCR).
func TestIntegration_NoCrash(t *testing.T) {
	client := mustConnectDeepDoc(t)

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

			cfg := DefaultParserConfig()
			p := NewParser(cfg, client)
			result, err := p.Parse(context.Background(), eng)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			// Structural invariants — these should always hold.
			for i, s := range result.Sections {
				if s.PositionTag == "" {
					t.Errorf("section[%d] has empty PositionTag", i)
				}
				if s.LayoutType != "" && s.Image != "" {
					// Section with an image should have valid base64.
					if _, err := base64.StdEncoding.DecodeString(s.Image); err != nil {
						t.Errorf("section[%d] Image: not valid base64: %v", i, err)
					}
				}
				if s.TableItem != nil {
					// Cross-reference: TableItem in section should appear in tables list.
					found := false
					for _, tbl := range result.Tables {
						if &tbl == s.TableItem {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("section[%d] TableItem not found in tables list", i)
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
