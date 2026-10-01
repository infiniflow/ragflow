package parser

import (
	"errors"
	"testing"
)

// TestDOCParser_EmptyViewsReturnsError locks Finding A from the PR review:
// when the native office backend opens the document but yields no text in any
// of its three views (IR / Markdown / PlainText), ParseWithResult must surface
// an error instead of silently returning empty content (which would produce 0
// chunks with no signal). This restores the pre-PR "no text at all" fail-loud
// semantics that the string-returning OpenAndExtract contract would otherwise
// hide.
func TestDOCParser_EmptyViewsReturnsError(t *testing.T) {
	ctx := t.Context()
	p := NewDOCParser()

	orig := docExtract
	defer func() { docExtract = orig }()
	// Open succeeds, but all three views come back empty.
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return "", "", "", nil, nil
	}

	res := p.ParseWithResult(ctx, "empty.doc", []byte("PK\x03\x04"))
	if res.Err == nil {
		t.Fatal("expected error for a document with no extractable text, got nil")
	}
	if errors.Is(res.Err, ErrOfficeCGORequired) {
		t.Fatalf("got ErrOfficeCGORequired, want a content-extraction error: %v", res.Err)
	}
}
