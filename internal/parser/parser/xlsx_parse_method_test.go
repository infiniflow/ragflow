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
		{"tcadp", "tcadp"},
		{"TCADP", "tcadp"},
		{"  tcadp  ", "tcadp"},
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
			if got, want := res.Produced(), true; got != want {
				t.Fatalf("produced = %t, want %t", got, want)
			}
			if len(res.JSON) != 1 {
				t.Fatalf("JSON item count = %d, want header-only result", len(res.JSON))
			}
			text := tableItemHTML(t, res.JSON[0].ToMap())
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
	ctx, cache := officeMediaTestContext()
	res := p.ParseWithResult(ctx, "with-image.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("JSON item count = %d, want table and image", len(res.JSON))
	}
	image := res.JSON[1].ToMap()
	if image["text"] != "sheet image" || image["doc_type_kwd"] != "image" {
		t.Fatalf("unexpected image item: %#v", image)
	}
	ref := requireOfficeMediaRef(t, image)
	if ref.Format != officeMediaFormatXLSX || ref.MIME != "image/png" {
		t.Fatalf("media_ref = %+v, want xlsx image/png", ref)
	}
	if len(cache.puts) != 1 || string(cache.puts[0].data) != string(mustDecodeBase64(t, pngBase64)) {
		t.Fatalf("cache writes = %+v, want the picture bytes", cache.puts)
	}
	if image["sheet_index"] != 1 || image["row_start"] != 3 || image["row_end"] != 3 || image["col_start"] != 3 || image["col_end"] != 3 {
		t.Fatalf("image coordinates = %#v, want Sheet1!C3", image)
	}
	if image["sheet"] != "Sheet1" || image["cell"] != "C3" {
		t.Fatalf("image identity = %#v, want Sheet1!C3", image)
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
	ctx, _ := officeMediaTestContext()
	res := p.ParseWithResult(ctx, "without-alt.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 || res.JSON[0].ToMap()["text"] != "C3" {
		t.Fatalf("image item = %#v, want anchor-cell text C3", res.JSON)
	}
	requireOfficeMediaRef(t, res.JSON[0].ToMap())
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
// default "deepdoc" parse_method and emits the segmented HTML table wire
// shape.
func TestCSVParser_DeepDocParseMethod(t *testing.T) {
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{"parse_method": "deepdoc"})

	ctx := t.Context()
	res := p.ParseWithResult(ctx, "test.csv", []byte("a,b\n1,2"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult(deepdoc): %v", res.Err)
	}
	if got, want := res.Produced(), true; got != want {
		t.Fatalf("produced = %t, want %t", got, want)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON items = %d, want one HTML table item", len(res.JSON))
	}
	if res.JSON[0].ToMap()["doc_type_kwd"] != "table" {
		t.Fatalf("JSON item = %#v, want a table item", res.JSON[0])
	}
}

// TestXLSParser_ParseMethod_FallsThroughToBIFF8 asserts that every
// non-TCADP parse_method continues with the local BIFF8 parser and
// produces spreadsheet output from a valid .xls workbook.
func TestXLSParser_ParseMethod_FallsThroughToBIFF8(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(genuineBIFF8FixtureBase64)
	if err != nil {
		t.Fatalf("decode BIFF8 fixture: %v", err)
	}
	for _, method := range []string{"", "deepdoc", "DeepDOC", "excelize", "biff", "biff8", "PaddleOCR", "MinerU", "General"} {
		t.Run(method, func(t *testing.T) {
			p, err := NewXLSParser("")
			if err != nil {
				t.Fatalf("NewXLSParser: %v", err)
			}
			p.ConfigureFromSetup(map[string]any{"parse_method": method})

			ctx := t.Context()
			res := p.ParseWithResult(ctx, "test.xls", data)
			if res.Err != nil {
				t.Fatalf("method %q: ParseWithResult: %v", method, res.Err)
			}
			if got := res.File["format"]; got != "xls" {
				t.Fatalf("method %q: format = %v, want xls", method, got)
			}
			if len(res.JSON) == 0 {
				t.Fatalf("method %q: JSON is empty, want BIFF8 sheet items", method)
			}
		})
	}
}
