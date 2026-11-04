//go:build cgo

package pdfoxide

import (
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
)

// TestPageDimsCacheMatchesPerPageLookup proves the document-wide size pass the
// engine now runs reproduces exactly what the former per-page pdfium call
// returned for every page.
func TestPageDimsCacheMatchesPerPageLookup(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "testdata", "real_pdfs", "*.pdf"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no real PDFs in ../testdata/real_pdfs")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			eng, err := NewEngine(data)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			defer eng.Close()

			pageCount, err := eng.PageCount()
			if err != nil {
				t.Fatalf("PageCount: %v", err)
			}

			eng.loadPageDims()
			if eng.pageDims == nil {
				t.Fatalf("document-wide size pass did not complete")
			}
			if len(eng.pageDims) != pageCount {
				t.Fatalf("cached %d page sizes, want %d", len(eng.pageDims), pageCount)
			}

			for page := 0; page < pageCount; page++ {
				wantW, wantH, wantErr := pdfium.PageSize(data, page)
				gotW, gotH, gotOK := eng.pageSizeFromCache(page)
				if wantErr != nil {
					if gotOK {
						t.Fatalf("page %d: cached size despite pdfium error %v", page, wantErr)
					}
					continue
				}
				if !gotOK {
					t.Fatalf("page %d: size missing from cache", page)
				}
				if gotW != wantW || gotH != wantH {
					t.Fatalf("page %d: cached %.4fx%.4f, want %.4fx%.4f", page, gotW, gotH, wantW, wantH)
				}
			}
		})
	}
}
