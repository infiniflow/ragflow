package parser

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestNormalizeXLSXParseMethod verifies the parse_method normalization
// shared by the XLSX/XLS/CSV parsers. "deepdoc" is the default
// spreadsheet parse_method (see schema.ParserParam.Defaults and the
// matching Python ParserParam), and the DSL templates ship "DeepDOC".
// Both must normalize to "" so the default Excelize/CSV path is taken,
// matching rag/flow/parser/parser.py:_spreadsheet which only special-cases
// "tcadp parser" and routes everything else to the default parser.
func TestNormalizeXLSXParseMethod(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"deepdoc", ""},
		{"DeepDOC", ""}, // canonical casing used by DSL templates
		{"DEEPDOC", ""},
		{"  deepdoc  ", ""},
		{"deepdoc parser", ""},
		{"DeepDOC Parser", ""},
		{"tcadp parser", "tcadp"},
		{"TCADP Parser", "tcadp"},
		{"excelize", "excelize"},
		{"csv", "csv"},
		{"unknown", "unknown"}, // preserved so the switch rejects it
	}
	for _, c := range cases {
		if got := normalizeXLSXParseMethod(c.in); got != c.want {
			t.Errorf("normalizeXLSXParseMethod(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestXLSXParser_DeepDocParseMethod verifies that both the lowercase "deepdoc"
// and the uppercase "DeepDOC" (as shipped by the ingestion pipeline DSL templates)
// parse_method values produce the default HTML table output.
func TestXLSXParser_DeepDocParseMethod(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		cellValue string
	}{
		{"Lowercase", "deepdoc", "hello"},
		{"Uppercase", "DeepDOC", "world"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := excelize.NewFile()
			defer f.Close()
			if err := f.SetCellValue("Sheet1", "A1", tc.cellValue); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
			buf, err := f.WriteToBuffer()
			if err != nil {
				t.Fatalf("WriteToBuffer: %v", err)
			}

			p, err := NewXLSXParser("")
			if err != nil {
				t.Fatalf("NewXLSXParser: %v", err)
			}
			p.ConfigureFromSetup(map[string]any{"parse_method": tc.method})

			res := p.ParseWithResult("test.xlsx", buf.Bytes())
			if res.Err != nil {
				t.Fatalf("ParseWithResult(%s): %v", tc.method, res.Err)
			}
			if got, want := res.OutputFormat, "html"; got != want {
				t.Fatalf("OutputFormat = %q, want %q", got, want)
			}
			if !strings.Contains(res.HTML, tc.cellValue) {
				t.Fatalf("HTML = %q, want it to contain cell content %q", res.HTML, tc.cellValue)
			}
		})
	}
}

// TestCSVParser_DeepDocParseMethod asserts the CSV parser accepts the
// default "deepdoc" parse_method and renders the default HTML table.
func TestCSVParser_DeepDocParseMethod(t *testing.T) {
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{"parse_method": "deepdoc"})

	res := p.ParseWithResult("test.csv", []byte("a,b\n1,2"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult(deepdoc): %v", res.Err)
	}
	if got, want := res.OutputFormat, "html"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if !strings.Contains(res.HTML, "<table>") {
		t.Fatalf("HTML = %q, want a rendered <table>", res.HTML)
	}
}

// TestXLSParser_DeepDocParseMethod_NoUnsupportedError asserts the XLS
// parser no longer rejects "deepdoc". A real .xls blob is hard to
// synthesize in a unit test, so we only assert that the error is not the
// "unsupported XLS parse method" rejection; an open/parse failure from
// the fake blob is acceptable. The shared normalization is already
// covered by TestNormalizeXLSXParseMethod.
func TestXLSParser_DeepDocParseMethod_NoUnsupportedError(t *testing.T) {
	p, err := NewXLSParser("")
	if err != nil {
		t.Fatalf("NewXLSParser: %v", err)
	}
	p.ConfigureFromSetup(map[string]any{"parse_method": "deepdoc"})

	res := p.ParseWithResult("test.xls", []byte("not a real xls"))
	if res.Err != nil && strings.Contains(res.Err.Error(), "unsupported XLS parse method") {
		t.Fatalf("deepdoc must not be rejected as unsupported: %v", res.Err)
	}
}
