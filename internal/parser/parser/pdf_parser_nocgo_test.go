//go:build !cgo

package parser

import (
	"errors"
	"testing"
)

func TestPDFParser_ParseWithResult_NoCGO(t *testing.T) {
	pdf := NewPDFParser()

	empty := pdf.ParseWithResult("empty.pdf", nil)
	if empty.Err != nil {
		t.Fatalf("empty input: want nil err, got %v", empty.Err)
	}
	if empty.OutputFormat != "json" {
		t.Fatalf("empty input OutputFormat = %q, want json", empty.OutputFormat)
	}
	if len(empty.JSON) != 1 {
		t.Fatalf("empty input JSON len = %d, want 1", len(empty.JSON))
	}

	res := pdf.ParseWithResult("a.pdf", []byte("%PDF-1.4"))
	if res.Err == nil {
		t.Fatal("want ErrPDFEngineUnavailable, got nil")
	}
	if !errors.Is(res.Err, ErrPDFEngineUnavailable) {
		t.Fatalf("err = %v, want wraps ErrPDFEngineUnavailable", res.Err)
	}
}
