package parser

import (
	"fmt"
	"testing"
)

// TestOfficeContainer verifies the magic-byte sniffing that lets the
// office_oxide parsers recover from a mislabeled extension (e.g. a
// legacy .doc uploaded as .docx). office_oxide::OpenFromBytes takes the
// container format on trust and does no detection of its own.
func TestOfficeContainer(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"ooxml", []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00}, "ooxml"},
		{"ole", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, "ole"},
		{"ole_with_trailing", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0x00, 0xFF}, "ole"},
		{"ole_short_header", []byte{0xD0, 0xCF, 0x11, 0xE0}, ""},
		{"ole_truncated_7", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A}, ""},
		{"empty", []byte{}, ""},
		{"too_short", []byte{0x50, 0x4B}, ""},
		{"ooxml_truncated_3", []byte{0x50, 0x4B, 0x03}, ""},
		{"garbage", []byte{0x00, 0x11, 0x22, 0x33}, ""},
		{"ole_prefix_mismatch", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0x00}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := officeContainer(tt.data); got != tt.want {
				t.Errorf("officeContainer(% x) = %q, want %q", tt.data, got, tt.want)
			}
		})
	}
}

// TestDOCXParser_FallsBackToDOCForOLEHeader ensures a legacy OLE .doc
// payload routed to the DOCXParser (the .doc-as-.docx case) is opened
// with the "doc" container format rather than "docx". A minimal OLE
// header is not a valid document, so office_oxide still errors — but
// the error must come from the DOC path, proving the fallback fired.
// The test uses a seam to capture the effective format without needing
// a full valid OLE document fixture.
func TestDOCXParser_FallsBackToDOCForOLEHeader(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()

	orig := docxExtract
	defer func() { docxExtract = orig }()

	var gotFormat string
	docxExtract = func(data []byte, format string) (string, string, string, error, error) {
		gotFormat = format
		return "", "", "", nil, fmt.Errorf("stub doc open: %s", format)
	}

	data := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	res := p.ParseWithResult(ctx, "renamed.docx", data)
	if res.Err == nil {
		t.Fatal("expected parse error for a non-document OLE header, got nil")
	}
	if gotFormat != "doc" {
		t.Fatalf("docxExtract format = %q, want %q (OLE fallback not triggered)", gotFormat, "doc")
	}
}

// TestDOCXParser_JSONEmptyIRReturnsError locks Finding B: when the native
// backend opens the document but returns an empty structured IR, the JSON
// output path must fail loud (it is built entirely from the IR, which the
// Markdown path can fall back from plain text). This guards against silently
// emitting empty JSON sections.
func TestDOCXParser_JSONEmptyIRReturnsError(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "json"})

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

// TestDOCXParser_MarkdownViewErrorPropagated locks Finding 3: when the
// native backend's Markdown view fails but the other views survive,
// OpenAndExtract surfaces the Markdown error via mdErr and the Markdown output
// path must fail loud instead of silently emitting empty markdown. The JSON
// path is unaffected because it is built from the IR view.
func TestDOCXParser_MarkdownViewErrorPropagated(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "markdown"})

	orig := docxExtract
	defer func() { docxExtract = orig }()
	docxExtract = func(data []byte, format string) (string, string, string, error, error) {
		// IR and plain text survive; only the Markdown view fails.
		return "<ir/>", "", "plain text fallback", fmt.Errorf("markdown boom"), nil
	}

	res := p.ParseWithResult(ctx, "broken.docx", []byte("PK\x03\x04"))
	if res.Err == nil {
		t.Fatal("expected error when only the Markdown view fails, got nil")
	}
}

// TestDOCXParser_AllViewsEmptyMarkdownReturnsError locks the engine-level
// contract from Finding 1 at the parser boundary: when the native backend
// opens the document but every view (IR / Markdown / PlainText) is empty,
// OpenAndExtract reports a joined error, and the default Markdown output path
// must fail loud rather than silently emit empty markdown. (office_oxide
// always returns a structured IR for an openable doc, so this all-empty error
// is simulated via the seam rather than a real fixture.)
func TestDOCXParser_AllViewsEmptyMarkdownReturnsError(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "markdown"})

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

// TestDOCXParser_TruncatedOLENotFallback ensures a truncated OLE prefix
// (only 4 bytes) does not trigger the DOC fallback — it must stay "docx"
// so truncated garbage is not misclassified as OLE.
func TestDOCXParser_TruncatedOLENotFallback(t *testing.T) {
	ctx := t.Context()
	p := NewDOCXParser()

	orig := docxExtract
	defer func() { docxExtract = orig }()

	var gotFormat string
	docxExtract = func(data []byte, format string) (string, string, string, error, error) {
		gotFormat = format
		return "", "", "", nil, fmt.Errorf("stub")
	}

	data := []byte{0xD0, 0xCF, 0x11, 0xE0}
	res := p.ParseWithResult(ctx, "truncated.docx", data)
	if res.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if gotFormat != "docx" {
		t.Fatalf("format = %q, want %q for truncated OLE (should not fallback)", gotFormat, "docx")
	}
}

// TestPPTXParser_FallsBackToPPTForOLEHeader ensures a legacy OLE .ppt
// payload routed to PPTXParser is opened with "ppt" rather than "pptx".
func TestPPTXParser_FallsBackToPPTForOLEHeader(t *testing.T) {
	ctx := t.Context()
	p := NewPPTXParser()

	orig := pptxExtract
	defer func() { pptxExtract = orig }()

	var gotFormat string
	pptxExtract = func(data []byte, format string) (string, string, string, error, error) {
		gotFormat = format
		return "", "", "", nil, fmt.Errorf("stub ppt open: %s", format)
	}

	data := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	res := p.ParseWithResult(ctx, "renamed.pptx", data)
	if res.Err == nil {
		t.Fatal("expected parse error for OLE header, got nil")
	}
	if gotFormat != "ppt" {
		t.Fatalf("pptxExtract format = %q, want %q", gotFormat, "ppt")
	}
	if p.format != "pptx" {
		t.Fatalf("PPTXParser.format mutated to %q, want %q (must use local effFormat)", p.format, "pptx")
	}
}

// TestPPTParser_FallsBackToPPTXForOOXMLHeader ensures an OOXML .pptx
// payload uploaded as .ppt ( PPTParser with p.format == "ppt") is
// correctly routed to "pptx" via the bidirectional container sniffing.
func TestPPTParser_FallsBackToPPTXForOOXMLHeader(t *testing.T) {
	ctx := t.Context()
	p := NewPPTParser() // underlying PPTXParser{format:"ppt"}

	orig := pptxExtract
	defer func() { pptxExtract = orig }()

	var gotFormat string
	pptxExtract = func(data []byte, format string) (string, string, string, error, error) {
		gotFormat = format
		return "", "", "", nil, fmt.Errorf("stub ppt open: %s", format)
	}

	// Minimal OOXML ZIP local file header.
	data := []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00}
	res := p.ParseWithResult(ctx, "renamed.ppt", data)
	if res.Err == nil {
		t.Fatal("expected parse error for OOXML header stub, got nil")
	}
	if gotFormat != "pptx" {
		t.Fatalf("pptxExtract format = %q, want %q (OOXML fallback ppt->pptx)", gotFormat, "pptx")
	}
	// Underlying PPTXParser must not be mutated permanently.
	if p.pptx.format != "ppt" {
		t.Fatalf("PPTParser.pptx.format mutated to %q, want %q (must use local effFormat)", p.pptx.format, "ppt")
	}
}

// TestPPTXParser_NoStatePollution verifies that after an OLE file mutates
// the effective format for that call, a subsequent OOXML file still uses
// "pptx" — i.e., the parser instance is not permanently polluted.
func TestPPTXParser_NoStatePollution(t *testing.T) {
	ctx := t.Context()
	p := NewPPTXParser()

	orig := pptxExtract
	defer func() { pptxExtract = orig }()

	calls := []string{}
	pptxExtract = func(data []byte, format string) (string, string, string, error, error) {
		calls = append(calls, format)
		return "", "", "", nil, fmt.Errorf("stub")
	}

	oleData := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	_ = p.ParseWithResult(ctx, "a.pptx", oleData)
	ooxmlData := []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00}
	_ = p.ParseWithResult(ctx, "b.pptx", ooxmlData)

	if len(calls) != 2 {
		t.Fatalf("calls = %v, want 2", calls)
	}
	if calls[0] != "ppt" {
		t.Errorf("first call format = %q, want %q", calls[0], "ppt")
	}
	if calls[1] != "pptx" {
		t.Errorf("second call format = %q, want %q (state polluted)", calls[1], "pptx")
	}
}
