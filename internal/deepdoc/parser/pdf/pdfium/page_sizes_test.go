//go:build cgo

package pdfium

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestPageSizesKeepsValidRequestedPages(t *testing.T) {
	data, err := os.ReadFile("testdata/page_sizes.pdf")
	if err != nil {
		t.Fatal(err)
	}
	sizes, count, err := PageSizes(t.Context(), data, []int{2, 99, 0, 2})
	if err == nil {
		t.Fatal("expected invalid page error")
	}
	want := map[int][2]float64{0: {595.5, 842}, 2: {300, 400}}
	if count != 3 || !reflect.DeepEqual(sizes, want) {
		t.Fatalf("count=%d sizes=%v", count, sizes)
	}
}

func TestPageSizesHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := PageSizes(ctx, []byte("%PDF-1.4"), []int{0})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}
