//go:build cgo

package pdfoxide

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
)

// pdfFixture is one PDF used by the table-driven tests below.
type pdfFixture struct {
	name string
	data []byte
}

// TestPageDimsCacheMatchesPerPageLookup proves the document-wide size pass
// reproduces exactly what the former per-page pdfium call returned. The
// generated fixture keeps the test meaningful in a clean checkout; every real
// PDF shipped next to the parser is swept as well when present.
func TestPageDimsCacheMatchesPerPageLookup(t *testing.T) {
	const pages = 3
	generated := buildTestPDF(t, pages, 90)

	paths, err := filepath.Glob(filepath.Join("..", "testdata", "real_pdfs", "*.pdf"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	cases := []pdfFixture{{name: "generated", data: generated}}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		cases = append(cases, pdfFixture{name: filepath.Base(path), data: data})
	}
	if len(paths) == 0 {
		t.Log("no real PDFs in ../testdata/real_pdfs; generated fixture only")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, err := NewEngine(tc.data)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			defer eng.Close()

			pageCount, err := eng.PageCount()
			if err != nil {
				t.Fatalf("PageCount: %v", err)
			}
			if pageCount == 0 {
				t.Fatalf("PageCount = 0")
			}

			for page := 0; page < pageCount; page++ {
				wantW, wantH, wantErr := pdfium.PageSize(tc.data, page)
				gotD, gotOK := eng.pageSize(page)
				if wantErr != nil {
					if gotOK {
						t.Fatalf("page %d: cached size despite pdfium error %v", page, wantErr)
					}
					continue
				}
				if !gotOK {
					t.Fatalf("page %d: size missing from cache", page)
				}
				if gotD[0] != wantW || gotD[1] != wantH {
					t.Fatalf("page %d: cached %.4fx%.4f, want %.4fx%.4f", page, gotD[0], gotD[1], wantW, wantH)
				}
			}
		})
	}
}

// TestPageSizeBatchPartialFailure pins the failure contract: a page missing
// from the batch is retried on its own, the retry is cached, and the batch is
// never re-run because some page failed.
func TestPageSizeBatchPartialFailure(t *testing.T) {
	eng, err := NewEngine(buildTestPDF(t, 2, 0))
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	var batchCalls, singleCalls int
	eng.pageSizesBatch = func(context.Context, []byte, []int) (map[int][2]float64, int, error) {
		batchCalls++
		// Page 1 is unreadable in this pass; page 0 succeeded.
		return map[int][2]float64{0: {200, 100}}, 2, errors.New("page 1 unreadable")
	}
	eng.pageSizeSingle = func([]byte, int) (float64, float64, error) {
		singleCalls++
		return 200, 100, nil
	}

	if d, ok := eng.pageSize(0); !ok || d != [2]float64{200, 100} {
		t.Fatalf("page 0: got %v ok=%v, want cached batch size", d, ok)
	}
	if singleCalls != 0 {
		t.Fatalf("page 0 must come from the batch, single-page lookups = %d", singleCalls)
	}

	if d, ok := eng.pageSize(1); !ok || d != [2]float64{200, 100} {
		t.Fatalf("page 1: got %v ok=%v, want single-page retry result", d, ok)
	}
	if singleCalls != 1 {
		t.Fatalf("page 1 must be retried once, got %d lookups", singleCalls)
	}

	if _, ok := eng.pageSize(1); !ok || singleCalls != 1 {
		t.Fatalf("page 1 retry must be cached, lookups = %d", singleCalls)
	}
	if _, ok := eng.pageSize(0); !ok || batchCalls != 1 {
		t.Fatalf("partial batch must not run again, batch calls = %d", batchCalls)
	}
}

// TestPageSizeFallbackWhenBothLookupsFail keeps the original fallback chain:
// only when the batch and the single-page retry both fail does PageSize answer
// with pdf_oxide's raw dimensions.
func TestPageSizeFallbackWhenBothLookupsFail(t *testing.T) {
	data := buildTestPDF(t, 1, 0)
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	eng.pageSizesBatch = func(context.Context, []byte, []int) (map[int][2]float64, int, error) {
		return nil, 1, errors.New("batch failed")
	}
	eng.pageSizeSingle = func([]byte, int) (float64, float64, error) {
		return 0, 0, errors.New("single page failed")
	}

	wantW, wantH, err := eng.doc.PageSize(0)
	if err != nil {
		t.Fatalf("oxide PageSize: %v", err)
	}
	gotW, gotH, err := eng.PageSize(0)
	if err != nil {
		t.Fatalf("PageSize: %v", err)
	}
	if gotW != wantW || gotH != wantH {
		t.Fatalf("PageSize = %.4fx%.4f, want oxide fallback %.4fx%.4f", gotW, gotH, wantW, wantH)
	}
}

// TestExtractCharsUsesSinglePageRetry covers the regression the batch cache
// could introduce: a page missing from the batch must still get its pdfium
// size (and therefore correct rotation) instead of silently falling back to
// the unrotated pdf_oxide dimensions.
func TestExtractCharsUsesSinglePageRetry(t *testing.T) {
	const rotate = 90
	data := buildTestPDF(t, 2, rotate)
	wantW, wantH, err := pdfium.PageSize(data, 1)
	if err != nil {
		t.Fatalf("pdfium.PageSize: %v", err)
	}

	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer eng.Close()

	eng.pageSizesBatch = func(context.Context, []byte, []int) (map[int][2]float64, int, error) {
		return map[int][2]float64{0: {200, 100}}, 2, errors.New("page 1 unreadable")
	}
	eng.pageSizeSingle = func([]byte, int) (float64, float64, error) {
		return wantW, wantH, nil
	}

	w, h, err := eng.PageSize(1)
	if err != nil {
		t.Fatalf("PageSize: %v", err)
	}
	if w != wantW || h != wantH {
		t.Fatalf("PageSize = %.2fx%.2f, want %.2fx%.2f", w, h, wantW, wantH)
	}

	chars, err := eng.ExtractChars(1)
	if err != nil {
		t.Fatalf("ExtractChars: %v", err)
	}
	assertCharsInsidePage(t, chars, w, h)
}

// TestExtractCharsRotatedPageCoordinates is the clean-checkout coordinate
// check the previous manual tier owned: a rotated page must yield characters
// inside the post-rotation page box.
func TestExtractCharsRotatedPageCoordinates(t *testing.T) {
	cases := []struct {
		rotate int
		wantW  float64
		wantH  float64
	}{
		{0, 200, 100},
		{90, 100, 200},
		{180, 200, 100},
		{270, 100, 200},
	}
	const eps = 0.01
	for _, tc := range cases {
		t.Run(fmt.Sprintf("rotate-%d", tc.rotate), func(t *testing.T) {
			data := buildTestPDF(t, 1, tc.rotate)
			eng, err := NewEngine(data)
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			defer eng.Close()

			w, h, err := eng.PageSize(0)
			if err != nil {
				t.Fatalf("PageSize: %v", err)
			}
			if math.Abs(w-tc.wantW) > eps || math.Abs(h-tc.wantH) > eps {
				t.Fatalf("PageSize = %.2fx%.2f, want %.2fx%.2f", w, h, tc.wantW, tc.wantH)
			}

			chars, err := eng.ExtractChars(0)
			if err != nil {
				t.Fatalf("ExtractChars: %v", err)
			}
			assertCharsInsidePage(t, chars, w, h)
		})
	}
}

// assertCharsInsidePage fails when any extracted character falls outside the
// post-rotation page box.
func assertCharsInsidePage(t *testing.T, chars []Char, w, h float64) {
	t.Helper()
	if len(chars) == 0 {
		t.Fatalf("no characters extracted")
	}
	const eps = 0.01
	for _, c := range chars {
		if c.X0 < -eps || c.X1 > w+eps || c.Top < -eps || c.Bottom > h+eps {
			t.Fatalf("char %q out of post-rotation box %.1fx%.1f: x=[%.2f,%.2f] y=[%.2f,%.2f]",
				c.Text, w, h, c.X0, c.X1, c.Top, c.Bottom)
		}
	}
}
