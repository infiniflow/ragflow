package file

import (
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
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "first"},
			{"text": "second"},
			{"value": 3},
		},
	})
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	want := "first\nsecond\n{\"value\":3}"
	if result != want {
		t.Fatalf("parseResultText = %q, want %q", result, want)
	}
}

func TestParseResultText_SpreadsheetRowsRenderReadableHTMLTable(t *testing.T) {
	result, err := parseResultText(parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{
				"text":         "a; b",
				"doc_type_kwd": "table",
				"ck_type":      "table_header",
				"table_id":     "sheet-1",
				"sheet":        "Data",
				"cells":        []string{"a", "b"},
			},
			{
				"text":         "a：1; b：2 ——Data",
				"doc_type_kwd": "text",
				"ck_type":      "table_row",
				"table_id":     "sheet-1",
				"sheet":        "Data",
				"headers":      []string{"a", "b"},
				"cells":        []string{"1", "2"},
			},
		},
	})
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	for _, want := range []string{"<table", "<caption>Data</caption>", "<th>a</th>", "<td>1</td>"} {
		if !strings.Contains(result, want) {
			t.Errorf("parseResultText = %q, want %q", result, want)
		}
	}
}

func TestParseResultText_SpreadsheetRowsWithoutHeaderDoNotDuplicateFirstRow(t *testing.T) {
	result, err := parseResultText(parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{
				"ck_type": "table_row",
				"headers": []string{"name", "value"},
				"cells":   []string{"alpha", "1"},
			},
		},
	})
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	if !strings.Contains(result, "<th>name</th>") {
		t.Fatalf("row headers were not rendered: %q", result)
	}
	if got := strings.Count(result, "<td>alpha</td>"); got != 1 {
		t.Fatalf("first row rendered %d times, want once: %q", got, result)
	}
}

func TestParseResultText_SpreadsheetRowsWithoutIdentityStaySeparate(t *testing.T) {
	result, err := parseResultText(parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{
				"ck_type": "table_header",
				"cells":   []string{"first"},
			},
			{
				"ck_type": "table_row",
				"cells":   []string{"one"},
			},
			{
				"ck_type": "table_header",
				"cells":   []string{"second"},
			},
			{
				"ck_type": "table_row",
				"cells":   []string{"two"},
			},
		},
	})
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	if got := strings.Count(result, "<table>"); got != 2 {
		t.Fatalf("unknown spreadsheet tables were merged: got %d tables in %q", got, result)
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

func TestParseResultText_EmptyJSONUsesRenderedFallback(t *testing.T) {
	result, err := parseResultText(parser.ParseResult{
		OutputFormat: "json",
		Markdown:     "legacy markdown",
	})
	if err != nil {
		t.Fatalf("parseResultText: %v", err)
	}
	if result != "legacy markdown" {
		t.Fatalf("parseResultText = %q, want rendered fallback", result)
	}
}

func TestParseResultText_EmptyJSONWithoutFallbackReturnsEmpty(t *testing.T) {
	result, err := parseResultText(parser.ParseResult{OutputFormat: "json"})
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
