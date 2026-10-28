package parser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func stubPDFOutlines(t *testing.T, outlines []deepdoctype.Outline) *int {
	t.Helper()
	calls := 0
	previous := pdfOutlinesFromPDF
	pdfOutlinesFromPDF = func([]byte) []deepdoctype.Outline {
		calls++
		return outlines
	}
	t.Cleanup(func() { pdfOutlinesFromPDF = previous })
	return &calls
}

func remotePDFResult(filename string) ParseResult {
	return pdfItemsToResult(filename, []map[string]any{{"text": "body", "page_number": 1}}, 3)
}

func TestAttachPDFOutlinesFillsRemoteOutline(t *testing.T) {
	stubPDFOutlines(t, []deepdoctype.Outline{{Title: "1 Introduction", Level: 0, PageNumber: 2}})
	result := AttachPDFOutlines(remotePDFResult("remote.pdf"), []byte("%PDF-1.4 fake"))
	outline, ok := result.File["outline"].([]map[string]any)
	if !ok || len(outline) != 1 {
		t.Fatalf("File[outline] = %#v, want one entry", result.File["outline"])
	}
	if outline[0]["title"] != "1 Introduction" || outline[0]["level"] != 0 || outline[0]["page_number"] != 2 {
		t.Fatalf("outline entry = %#v", outline[0])
	}
}

func TestAttachPDFOutlinesKeepsEngineOutline(t *testing.T) {
	calls := stubPDFOutlines(t, []deepdoctype.Outline{{Title: "Ignored", Level: 0, PageNumber: 1}})
	result := AttachPDFOutlines(ParseResult{
		File: map[string]any{"name": "deep.pdf", "outline": []map[string]any{{"title": "Chapter 1", "level": 0, "page_number": 3}}},
	}, []byte("%PDF-1.4 fake"))
	if got := result.File["outline"].([]map[string]any); len(got) != 1 || got[0]["title"] != "Chapter 1" {
		t.Fatalf("engine outline changed: %#v", result.File["outline"])
	}
	if *calls != 0 {
		t.Fatalf("bookmark reader consulted %d times, want 0", *calls)
	}
}

func TestAttachPDFOutlinesKeepsEmptyOutlineWhenAbsent(t *testing.T) {
	stubPDFOutlines(t, nil)
	result := AttachPDFOutlines(remotePDFResult("plain.pdf"), []byte("%PDF-1.4 fake"))
	outline, ok := result.File["outline"].([]map[string]any)
	if !ok || len(outline) != 0 {
		t.Fatalf("File[outline] = %#v (ok=%v), want empty []map[string]any", result.File["outline"], ok)
	}
	if failed := AttachPDFOutlines(ParseResult{Err: errors.New("parse failed")}, []byte("%PDF")); failed.File != nil {
		t.Fatalf("errored result gained a file meta: %#v", failed.File)
	}
}

func TestFinalizePDFResultAttachesOutline(t *testing.T) {
	stubPDFOutlines(t, []deepdoctype.Outline{{Title: "Appendix", Level: 1, PageNumber: 2}})
	p := &PDFParser{ParseMethod: "plain_text"}
	raw := remotePDFResult("bookmarked.pdf")
	result := p.FinalizePDFResult([]byte("%PDF"), p.NormalizeResult(t.Context(), "bookmarked.pdf", raw))
	outline, ok := result.File["outline"].([]map[string]any)
	if !ok || len(outline) != 1 || outline[0]["title"] != "Appendix" {
		t.Fatalf("File[outline] = %#v, want the stubbed bookmark", result.File["outline"])
	}
}

func TestFinalizePDFResultRemovesHeaderFooterLayouts(t *testing.T) {
	stubPDFOutlines(t, nil)
	p := &PDFParser{RemoveHeaderFooter: true}
	result := pdfItemsToResult("doc.pdf", []map[string]any{
		{"text": "body", "layout": "text", "page_number": 1},
		{"text": "header text", "layout": "page-header", "page_number": 1},
		{"text": "footer text", "layout": "footer", "page_number": 1},
	}, 1)
	result = p.FinalizePDFResult(nil, result)
	if len(result.JSON) != 1 {
		t.Fatalf("len(JSON)=%d, want 1 after header/footer removal", len(result.JSON))
	}
	if text := result.JSON[0].ToMap()["text"]; text != "body" {
		t.Fatalf("remaining text = %#v", text)
	}
}

func TestFinalizePDFResultRemoveTOCContentsHeading(t *testing.T) {
	stubPDFOutlines(t, nil)
	p := &PDFParser{RemoveTOC: true}
	result := pdfItemsToResult("doc.pdf", []map[string]any{
		{"text": "Contents", "layout": "title", "page_number": 1},
		{"text": "Intro .......... 1", "layout": "text", "page_number": 1},
		{"text": "Real body chapter.", "layout": "text", "page_number": 2},
	}, 2)
	result = p.FinalizePDFResult(nil, result)
	for _, item := range result.JSON {
		text := itemText(item.ToMap())
		if strings.Contains(strings.ToLower(text), "contents") || strings.Contains(text, "..........") {
			t.Fatalf("TOC-like text still present: %q in %#v", text, result.JSON)
		}
	}
}

func bookmarkedPDF(t *testing.T) []byte {
	t.Helper()
	page := func(contentsRef int) string {
		return fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 9 0 R >> >> /Contents %d 0 R >>", contentsRef)
	}
	body := func(text string) string {
		content := fmt.Sprintf("BT /F1 24 Tf 72 700 Td (%s) Tj ET", text)
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R /Outlines 3 0 R >>",
		"<< /Type /Pages /Kids [4 0 R 5 0 R] /Count 2 >>",
		"<< /Type /Outlines /First 6 0 R /Last 8 0 R /Count 3 >>",
		page(10),
		page(11),
		"<< /Title (Chapter 1) /Parent 3 0 R /First 7 0 R /Last 7 0 R /Count 1 /Dest [4 0 R /XYZ null null null] >>",
		"<< /Title (Section 1.1) /Parent 6 0 R /Dest [5 0 R /XYZ null null null] >>",
		"<< /Title (Chapter 2) /Parent 3 0 R /Dest [5 0 R /XYZ null null null] >>",
		body("Page one"),
		body("Page two"),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, obj := range objects {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return buf.Bytes()
}
