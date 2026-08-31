package parser

import (
	"encoding/base64"
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
// and the uppercase "DeepDOC" produce structured spreadsheet output.
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

			ctx := t.Context()
			res := p.ParseWithResult(ctx, "test.xlsx", buf.Bytes())
			if res.Err != nil {
				t.Fatalf("ParseWithResult(%s): %v", tc.method, res.Err)
			}
			if got, want := res.OutputFormat, "json"; got != want {
				t.Fatalf("OutputFormat = %q, want %q", got, want)
			}
			if len(res.JSON) != 1 {
				t.Fatalf("JSON item count = %d, want 1", len(res.JSON))
			}
			text, _ := res.JSON[0]["text"].(string)
			if !strings.Contains(text, tc.cellValue) {
				t.Fatalf("JSON = %#v, want it to contain cell content %q", res.JSON, tc.cellValue)
			}
		})
	}
}

func TestXLSXParser_ExtractsFloatingImages(t *testing.T) {
	const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC"

	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "table content")
		if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{
			Extension: ".png",
			File:      mustDecodeBase64(t, pngBase64),
			Format:    &excelize.GraphicOptions{AltText: "sheet image"},
		}); err != nil {
			t.Fatalf("AddPictureFromBytes: %v", err)
		}
	})

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(t.Context(), "with-image.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("JSON item count = %d, want table and image", len(res.JSON))
	}
	image := res.JSON[1]
	if image["text"] != "sheet image" || image["doc_type_kwd"] != "image" {
		t.Fatalf("unexpected image item: %#v", image)
	}
	if image["image"] != "data:image/png;base64,"+pngBase64 {
		t.Fatalf("image data = %v, want data URL", image["image"])
	}
}

func TestXLSXParser_ImageWithoutAltTextUsesAnchorCell(t *testing.T) {
	const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC"

	data := newTestXLSX(t, func(f *excelize.File) {
		if err := f.AddPictureFromBytes("Sheet1", "C3", &excelize.Picture{
			Extension: ".png",
			File:      mustDecodeBase64(t, pngBase64),
		}); err != nil {
			t.Fatalf("AddPictureFromBytes: %v", err)
		}
	})

	p, err := NewXLSXParser("")
	if err != nil {
		t.Fatalf("NewXLSXParser: %v", err)
	}
	res := p.ParseWithResult(t.Context(), "without-alt.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 || res.JSON[0]["text"] != "C3" {
		t.Fatalf("image item = %#v, want anchor-cell text C3", res.JSON)
	}
}

func mustDecodeBase64(t *testing.T, encoded string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	return data
}

// TestCSVParser_DeepDocParseMethod asserts the CSV parser accepts the
// default "deepdoc" parse_method and renders the default HTML table.
func TestCSVParser_DeepDocParseMethod(t *testing.T) {
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{"parse_method": "deepdoc"})

	ctx := t.Context()
	res := p.ParseWithResult(ctx, "test.csv", []byte("a,b\n1,2"))
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

	ctx := t.Context()
	res := p.ParseWithResult(ctx, "test.xls", []byte("not a real xls"))
	if res.Err != nil && strings.Contains(res.Err.Error(), "unsupported XLS parse method") {
		t.Fatalf("deepdoc must not be rejected as unsupported: %v", res.Err)
	}
}
