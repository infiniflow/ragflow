package parser

import (
	"context"
	"errors"
	"testing"
)

func TestCSVParserHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := NewCSVParser().ParseWithResult(ctx, "rows.csv", []byte("key\nvalue\n"))
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("cancelled CSV parse returned %v", result.Err)
	}
}
