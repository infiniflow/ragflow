//go:build cgo && manual

package pdf

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
)

// TestPdfsyncSharedMutexAcrossBindings hammers both the cgo pdfium binding and
// the Rust pdf_oxide binding concurrently on the same document. Both bindings
// link a single PDFium instance (build.sh collapses the duplicate FPDF_*
// symbols with --allow-multiple-definition), so each binding must serialize
// through the SAME process-wide pdfsync.Mu. Two independent mutexes would
// still interleave calls onto the shared PDFium heap and SIGSEGV. The test
// therefore only passes if it completes without crashing — it directly
// exercises the shared-mutex invariant described in pdfsync/pdfsync.go.
func TestPdfsyncSharedMutexAcrossBindings(t *testing.T) {
	pdfPath := filepath.Join("testdata", "pdfs", "01_english_simple.pdf")
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 16
	const iterations = 3

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			usePdfium := id%2 == 0
			for j := 0; j < iterations; j++ {
				if usePdfium {
					if _, _, err := pdfium.PageSize(data, 0); err != nil {
						t.Errorf("pdfium.PageSize: %v", err)
						return
					}
					img, err := pdfium.RenderPage(data, 0, 72)
					if err != nil {
						t.Errorf("pdfium.RenderPage: %v", err)
						return
					}
					if img.Bounds().Dx() <= 0 {
						t.Error("pdfium.RenderPage returned zero-width image")
						return
					}
				} else {
					doc, err := pdfoxide.OpenBytes(data)
					if err != nil {
						t.Errorf("pdfoxide.OpenBytes: %v", err)
						return
					}
					res, err := doc.RenderPage(0, 72)
					doc.Close()
					if err != nil {
						t.Errorf("pdfoxide.RenderPage: %v", err)
						return
					}
					if res.Width <= 0 {
						t.Error("pdfoxide.RenderPage returned zero-width image")
						return
					}
				}
			}
		}(i)
	}
	wg.Wait()
}
