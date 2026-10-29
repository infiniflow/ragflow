package file

import (
	"archive/zip"
	"bytes"
	"strings"

	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
	"testing"
)

func TestBytesLooksLikePDF(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"valid header", []byte("%PDF-1.4 content"), true},
		{"too short", []byte("%PD"), false},
		{"plain text", []byte("hello"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := utility.BytesLooksLikePDF(c.data); got != c.want {
			t.Errorf("%s: utility.BytesLooksLikePDF = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLooksLikeHTML(t *testing.T) {
	if !utility.LooksLikeHTML([]byte("<html><body>x</body></html>")) {
		t.Error("should detect <html>")
	}
	if !utility.LooksLikeHTML([]byte("<DIV>x</DIV>")) {
		t.Error("should detect <div> case-insensitively")
	}
	if !utility.LooksLikeHTML([]byte("<body>x</body>")) {
		t.Error("should detect <body>")
	}
	if utility.LooksLikeHTML([]byte("just text")) {
		t.Error("should not detect plain text")
	}
}

func TestParseFileContent_JSONItems(t *testing.T) {
	ctx := t.Context()
	// CSV parser now produces JSON table items. sys.files still needs the
	// readable item text, not the original CSV bytes.
	result := parseFileContent(ctx, "data.csv", []byte("a,b,c\n1,2,3\n"))
	if result == "" {
		t.Fatal("CSV parser returned empty content")
	}
	if strings.Contains(result, "a,b,c\n1,2,3\n") {
		t.Errorf("CSV content used original bytes instead of parsed item text: %q", result)
	}
	if !strings.Contains(result, "<table") || !strings.Contains(result, "1") {
		t.Errorf("CSV content lost readable table text: %q", result)
	}
}

func TestParseResultText_JSONItemsPreservesOrderAndFallsBackToJSON(t *testing.T) {
	result, err := parseResultText(parser.ParseResult{
		JSON: parser.MapsToItems([]map[string]any{
			{"text": "first"},
			{"text": "second"},
			{"value": 3},
		}),
	})
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	want := "first\nsecond\n{\"value\":3}"
	if result != want {
		t.Fatalf("parseResultText = %q, want %q", result, want)
	}
}

// TestParseResultText_SpreadsheetSegmentsAreTheirOwnMarkup: the wire emits
// self-describing HTML table segments, so the readable preview is exactly
// the item text — no parser-side table re-rendering.
func TestParseResultText_SpreadsheetSegmentsAreTheirOwnMarkup(t *testing.T) {
	segment := "<table><caption>Data</caption>\n<tr><th>a</th><th>b</th></tr>\n<tr><td>1</td><td>2</td></tr>\n</table>\n"
	result, err := parseResultText(parser.ParseResult{
		JSON: parser.MapsToItems([]map[string]any{
			{"text": segment, "doc_type_kwd": "table", "sheet": "Data", "sheet_index": 1},
		}),
	})
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	if result != segment {
		t.Fatalf("parseResultText = %q, want the segment markup whole", result)
	}
}

func TestParseResultText_EmailJSONKeepsSearchableHeaders(t *testing.T) {
	raw := []byte("From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Parser contract\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nEmail body")
	p := parser.NewEmailParser()
	p.ConfigureFromSetup(map[string]any{"output_format": "json", "fields": []string{"from", "to", "subject", "body"}})
	parsed := p.ParseWithResult(t.Context(), "message.eml", raw)
	if parsed.Err != nil {
		t.Fatalf("ParseWithResult: %v", parsed.Err)
	}
	content, err := parseResultText(parsed)
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	for _, want := range []string{"from:sender@example.com", "to:recipient@example.com", "subject:Parser contract", "Email body"} {
		if !strings.Contains(content, want) {
			t.Errorf("sys.files text missing %q: %q", want, content)
		}
	}
}

func TestParseResultText_EmptyJSONWithoutFallbackReturnsEmpty(t *testing.T) {
	result, err := parseResultText(parser.ParseResult{})
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	if result != "" {
		t.Fatalf("parseResultText = %q, want empty text", result)
	}
}

func TestParseAgentUploadContent_JSONUsesParsedItemText(t *testing.T) {
	content, err := parseAgentUploadContent(t.Context(), "data.csv", []byte("a,b\n1,2\n"), "")
	if err != nil {
		t.Fatalf("parseAgentUploadContent: %v", err)
	}
	if strings.Contains(content, "a,b\n1,2\n") {
		t.Fatalf("upload content used original bytes instead of parsed item text: %q", content)
	}
	if !strings.Contains(content, "<table") || !strings.Contains(content, "1") {
		t.Fatalf("upload content lost readable table text: %q", content)
	}
}

func TestParseFileContent_MarkdownOutputFormat(t *testing.T) {
	ctx := t.Context()
	// .md files fall through to default parser or produce text/markdown.
	result := parseFileContent(ctx, "doc.md", []byte("# Title\n\nBody"))
	// This is integration-dependent; verify it doesn't crash and returns something.
	if result == "" {
		t.Skip("markdown parser returned empty (integration-only)")
	}
}

func TestHTMLContentExtractionWithoutMediaCache(t *testing.T) {
	input := []byte(`<p>before</p><img alt="figure" src="data:image/png;base64,aGVsbG8="><p>after</p>`)
	for name, parse := range map[string]func() (string, error){
		"file":  func() (string, error) { return parseFileContent(t.Context(), "sample.html", input), nil },
		"agent": func() (string, error) { return parseAgentUploadContent(t.Context(), "sample.html", input, "") },
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parse()
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"before", "figure", "after"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q: %q", want, got)
				}
			}
			if strings.Contains(got, "base64,") || strings.Contains(got, "<img") {
				t.Fatalf("raw HTML leaked: %q", got)
			}
		})
	}
}

func buildTestEPUBForContent(t *testing.T) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	write := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("META-INF/container.xml", `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	write("OEBPS/content.opf", `<?xml version="1.0" encoding="utf-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><manifest><item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/><item id="img" href="Images/cover.png" media-type="image/png"/></manifest><spine><itemref idref="c1"/></spine></package>`)
	write("OEBPS/ch1.xhtml", `<?xml version="1.0" encoding="utf-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>Test</title></head><body><p>before</p><img src="Images/cover.png" alt="figure"><img src="data:image/png;base64,aGVsbG8=" alt="inline"><p>after</p></body></html>`)
	write("OEBPS/Images/cover.png", "cover-bytes")
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestEPUBContentExtractionWithoutMediaCache(t *testing.T) {
	input := buildTestEPUBForContent(t)
	for name, parse := range map[string]func() (string, error){
		"file":  func() (string, error) { return parseFileContent(t.Context(), "book.epub", input), nil },
		"agent": func() (string, error) { return parseAgentUploadContent(t.Context(), "book.epub", input, "") },
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parse()
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"before", "figure", "inline", "after"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q: %q", want, got)
				}
			}
			if strings.HasPrefix(got, "PK") || strings.Contains(got, "cover-bytes") || strings.Contains(got, "base64,") {
				t.Fatalf("raw EPUB bytes leaked: %q", got)
			}
		})
	}
}
