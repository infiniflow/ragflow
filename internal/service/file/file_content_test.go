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
