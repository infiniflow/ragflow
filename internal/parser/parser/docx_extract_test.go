package parser

import (
	"fmt"
	"strings"
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

func TestDOCXParser_ParseWithResult_FlattenMediaToText(t *testing.T) {
	ctx := t.Context()
	orig := docxExtract
	defer func() { docxExtract = orig }()

	tableIR := `{"sections":[{"title":"","elements":[
		{"type":"paragraph","content":[{"type":"text","text":"Intro"}],"style":"Normal"},
		{"type":"table","rows":[
			{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"Cell A"}]}]},{"content":[{"type":"paragraph","content":[{"type":"text","text":"Cell B"}]}]}]}
		]}
	]}]}`
	docxExtract = func(data []byte, format string) (string, string, string, error, error) {
		return tableIR, "", "", nil, nil
	}

	// 1. By default FlattenMediaToText is false: table section preserves doc_type_kwd = "table"
	pDefault := NewDOCXParser()
	resDefault := pDefault.ParseWithResult(ctx, "sample.docx", []byte("PK\x03\x04"))
	if resDefault.Err != nil {
		t.Fatalf("ParseWithResult default: %v", resDefault.Err)
	}
	itemsDefault := ItemsToMaps(resDefault.JSON)
	if len(itemsDefault) != 2 {
		t.Fatalf("got %d items, want 2", len(itemsDefault))
	}
	if got := itemsDefault[1]["doc_type_kwd"]; got != "table" {
		t.Errorf("default doc_type_kwd = %v, want 'table'", got)
	}

	// 2. With FlattenMediaToText = true: table section is flattened to doc_type_kwd = "text"
	pFlatten := NewDOCXParser()
	pFlatten.ConfigureFromSetup(map[string]any{"flatten_media_to_text": true})
	resFlatten := pFlatten.ParseWithResult(ctx, "sample.docx", []byte("PK\x03\x04"))
	if resFlatten.Err != nil {
		t.Fatalf("ParseWithResult flatten: %v", resFlatten.Err)
	}
	itemsFlatten := ItemsToMaps(resFlatten.JSON)
	if len(itemsFlatten) != 2 {
		t.Fatalf("got %d items, want 2", len(itemsFlatten))
	}
	if got := itemsFlatten[1]["doc_type_kwd"]; got != "text" {
		t.Errorf("flattened doc_type_kwd = %v, want 'text'", got)
	}
	tableHTML, _ := itemsFlatten[1]["text"].(string)
	if !strings.Contains(tableHTML, "<table>") || !strings.Contains(tableHTML, "Cell A") {
		t.Errorf("flattened table text = %q, want rendered HTML table", tableHTML)
	}
}

