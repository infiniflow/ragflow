//go:build cgo

package io

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
)

func TestWritePDF_WatermarkOnEveryPage(t *testing.T) {
	requirePDFLatinFont(t)

	const watermark = "CONFIDENTIALWM"
	var b strings.Builder
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&b, "filler line %03d for pagination\n", i)
	}
	out, err := WritePDF(b.String(), PDFOptions{WatermarkText: watermark})
	if err != nil {
		t.Fatalf("WritePDF: %v", err)
	}

	eng, err := deepdocpdf.NewEngine(out)
	if err != nil {
		t.Fatalf("open generated PDF: %v", err)
	}
	defer eng.Close()

	pages, err := eng.PageCount()
	if err != nil {
		t.Fatalf("PageCount: %v", err)
	}
	if pages < 2 {
		t.Fatalf("expected a multi-page PDF, got %d pages", pages)
	}
	for p := 0; p < pages; p++ {
		chars, err := eng.ExtractChars(p)
		if err != nil {
			t.Fatalf("ExtractChars page %d: %v", p+1, err)
		}
		var glyphs []rune
		for _, c := range chars {
			if c.FontSize >= 30 {
				glyphs = append(glyphs, []rune(c.Text)...)
			}
		}
		sort.Slice(glyphs, func(i, j int) bool { return glyphs[i] < glyphs[j] })
		want := []rune(watermark)
		sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
		if string(glyphs) != string(want) {
			t.Errorf("page %d/%d watermark glyphs = %q, want %q", p+1, pages, string(glyphs), string(want))
		}
	}
}
