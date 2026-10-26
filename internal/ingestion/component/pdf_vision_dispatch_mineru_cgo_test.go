//go:build cgo

package component

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestMineruPageSizesReadsEveryPage locks the cgo bridge MinerU positions
// depend on: content_list bboxes are 0-1000 normalized, so every page must
// report its real size in PDF points.
func TestMineruPageSizesReadsEveryPage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deepdoc", "parser", "pdf", "pdfium", "testdata", "page_sizes.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	sizes, err := mineruPdfiumPageSizes(t.Context(), data)
	if err != nil {
		t.Fatalf("mineruPdfiumPageSizes: %v", err)
	}
	if len(sizes) != 3 {
		t.Fatalf("sizes = %v, want 3 pages", sizes)
	}
	for idx, want := range map[int][2]float64{0: {595.5, 842}, 2: {300, 400}} {
		got, ok := sizes[idx]
		if !ok || math.Abs(got[0]-want[0]) > 1e-6 || math.Abs(got[1]-want[1]) > 1e-6 {
			t.Errorf("page %d size = %v, want %v", idx, got, want)
		}
	}
}

func TestMineruPageSizesHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := mineruPdfiumPageSizes(ctx, []byte("%PDF-1.4")); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
