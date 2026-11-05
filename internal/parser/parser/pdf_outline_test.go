package parser

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// stubPDFOutlines replaces the bookmark reader for one test and reports how
// many times it was consulted. The seam is package-global, so tests using it
// must not run in parallel.
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

// remotePDFResult is the file meta a remote engine produces: pdfFileMeta seeds
// an empty outline.
func remotePDFResult(filename string) ParseResult {
	return pdfItemsToResult(filename, []map[string]any{{"text": "body", "page_number": 1}}, 3)
}

// TestAttachPDFOutlinesFillsRemoteOutline pins the #21221 fix for the remote
// PDF engines: their result carries an empty outline, so the bookmarks read
// from the source PDF are what the title chunker uses for heading levels.
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

// TestAttachPDFOutlinesKeepsEngineOutline asserts the engine that already read
// the bookmarks stays authoritative and is not re-read.
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

// TestAttachPDFOutlinesKeepsEmptyOutlineWhenAbsent covers a PDF without
// bookmarks: the key stays an empty slice, never a nil the consumers would have
// to special-case. An errored or file-less result is untouched.
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
	if got := AttachPDFOutlines(ParseResult{}, []byte("%PDF")); got.File != nil {
		t.Fatalf("result without file meta gained one: %#v", got.File)
	}
}

// TestParseWithResultAttachesOutline pins the attachment on the built-in parse
// method boundary (ParseWithResult). The dispatched tenant and vision engines
// normalize outside this package and have their own call site, pinned by
// TestPDFDispatchAttachesPDFOutlines in the component package. The bookmark
// reader is stubbed here, so only a structurally valid PDF is needed as input.
func TestParseWithResultAttachesOutline(t *testing.T) {
	stubPDFOutlines(t, []deepdoctype.Outline{{Title: "Appendix", Level: 1, PageNumber: 2}})
	p := &PDFParser{ParseMethod: "plain_text"}
	result := p.ParseWithResult(t.Context(), "bookmarked.pdf", bookmarkedPDF(t))
	if result.Err != nil {
		t.Fatalf("ParseWithResult: %v", result.Err)
	}
	outline, ok := result.File["outline"].([]map[string]any)
	if !ok || len(outline) != 1 || outline[0]["title"] != "Appendix" {
		t.Fatalf("File[outline] = %#v, want the stubbed bookmark", result.File["outline"])
	}
}

// bookmarkedPDF builds a two-page PDF whose outline is Chapter 1 (page 1) with
// a Section 1.1 child (page 2), then Chapter 2 (page 2). It is hand-written so
// the native extraction test has a deterministic fixture with no binary asset.
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
		"<< /Type /Pages /Kids [5 0 R 7 0 R] /Count 2 >>",
		"<< /Type /Outlines /First 4 0 R /Last 6 0 R /Count 2 >>",
		"<< /Title (Chapter 1) /Parent 3 0 R /First 8 0 R /Last 8 0 R /Count 1 /Next 6 0 R /Dest [5 0 R /XYZ null null null] >>",
		page(10),
		"<< /Title (Chapter 2) /Parent 3 0 R /Prev 4 0 R /Dest [7 0 R /XYZ null null null] >>",
		page(11),
		"<< /Title (Section 1.1) /Parent 4 0 R /Dest [7 0 R /XYZ null null null] >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		body("page one"),
		body("page two"),
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
