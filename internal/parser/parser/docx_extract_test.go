package parser

import (
	"fmt"
	"testing"
)

// TestDOCXParser_JSONEmptyIRReturnsError locks Finding B: when the native
// backend opens the document but returns an empty structured IR, the JSON
// output path must fail loud (it is built entirely from the IR, which the
// Markdown path can fall back from plain text). This guards against silently
// emitting empty JSON sections.
func TestDOCXParser_JSONEmptyIRReturnsError(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()

	orig := docxExtract
	defer func() { docxExtract = orig }()
	docxExtract = func(data []byte, format string) (string, string, string, error, error) {
		// Open succeeds, but the IR view is empty.
		return "", "", "", nil, nil
	}

	res := p.ParseWithResult(ctx, "empty.docx", []byte("PK\x03\x04"))
	if res.Err == nil {
		t.Fatal("expected error for JSON output with empty IR, got nil")
	}
}

// TestDOCXParser_AllViewsEmptyReturnsError locks the engine-level contract
// at the parser boundary: when the native backend opens the document but
// every view (IR / Markdown / PlainText) is empty, OpenAndExtract reports a
// joined error, and the parser must fail loud rather than silently emit
// empty JSON. (office_oxide always returns a structured IR for an openable
// doc, so this all-empty error is simulated via the seam rather than a real
// fixture.)
func TestDOCXParser_AllViewsEmptyReturnsError(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()

	orig := docxExtract
	defer func() { docxExtract = orig }()
	docxExtract = func(data []byte, format string) (string, string, string, error, error) {
		return "", "", "", nil, fmt.Errorf("office_oxide docx: all text views failed")
	}

	res := p.ParseWithResult(ctx, "empty.docx", []byte("PK\x03\x04"))
	if res.Err == nil {
		t.Fatal("expected error for a document with no extractable text in any view, got nil")
	}
}
