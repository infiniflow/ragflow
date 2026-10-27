package parser

import (
	"context"
	"image"
	"strings"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func pdfParseResultToJSON(filename string, parsed *deepdoctype.ParseResult) ParseResult {
	return NewPDFParser().NormalizeResult(context.Background(), filename, pdfParseResultToJSONWithOptions(filename, parsed, pdfPostProcessOptions{}))
}

func TestPDFParseResultToJSON_NormalizesCoreFields(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{
				Text:       "Title block",
				LayoutType: deepdoctype.LayoutTypeTitle,
				Positions: []deepdoctype.Position{
					{
						PageNumbers: []int{0},
						Left:        10,
						Right:       20,
						Top:         30,
						Bottom:      40,
					},
				},
			},
			{
				Text:       "Figure caption",
				LayoutType: deepdoctype.LayoutTypeFigure,
				Image:      "aGVsbG8=",
				Positions: []deepdoctype.Position{
					{
						PageNumbers: []int{1},
						Left:        1,
						Right:       2,
						Top:         3,
						Bottom:      4,
					},
				},
			},
		},
		Outlines: []deepdoctype.Outline{
			{Title: "Intro", Level: 1, PageNumber: 2},
		},
	}

	res := pdfParseResultToJSON("sample.pdf", parsed)
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSON: %v", res.Err)
	}
	if !res.Produced() {
		t.Fatalf("produced = %t, want true", res.Produced())
	}
	if got, want := res.File["name"], "sample.pdf"; got != want {
		t.Fatalf("File.name = %v, want %v", got, want)
	}
	outline, ok := res.File["outline"].([]map[string]any)
	if !ok {
		t.Fatalf("File.outline type = %T, want []map[string]any", res.File["outline"])
	}
	if len(outline) != 1 || outline[0]["page_number"] != 2 {
		t.Fatalf("File.outline = %+v, want page_number 2", outline)
	}
	if len(res.JSON) != 2 {
		t.Fatalf("JSON len = %d, want 2", len(res.JSON))
	}
	if got, want := res.JSON[0].ToMap()["layout"], "title"; got != want {
		t.Fatalf("JSON[0].layout = %v, want %v", got, want)
	}
	if got, want := res.JSON[0].ToMap()["page_number"], 1; got != want {
		t.Fatalf("JSON[0].page_number = %v, want %v", got, want)
	}
	if got, want := res.JSON[0].ToMap()["doc_type_kwd"], "text"; got != want {
		t.Fatalf("JSON[0].doc_type_kwd = %v, want %v", got, want)
	}
	pdfPositions, ok := res.JSON[0].ToMap()["_pdf_positions"].([][]any)
	if !ok {
		t.Fatalf("JSON[0]._pdf_positions type = %T, want [][]any", res.JSON[0].ToMap()["_pdf_positions"])
	}
	if len(pdfPositions) != 1 || pdfPositions[0][0] != 1 {
		t.Fatalf("JSON[0]._pdf_positions = %+v, want canonical 1-based positions", pdfPositions)
	}
	if got := res.JSON[0].ToMap()["positions"]; got == nil {
		t.Fatal("JSON[0].positions missing after normalization")
	}
	if got, want := res.JSON[1].ToMap()["doc_type_kwd"], "image"; got != want {
		t.Fatalf("JSON[1].doc_type_kwd = %v, want %v", got, want)
	}
	if got, want := res.JSON[1].ToMap()["page_number"], 2; got != want {
		t.Fatalf("JSON[1].page_number = %v, want %v", got, want)
	}
	secondPDFPositions, ok := res.JSON[1].ToMap()["_pdf_positions"].([][]any)
	if !ok {
		t.Fatalf("JSON[1]._pdf_positions type = %T, want [][]any", res.JSON[1].ToMap()["_pdf_positions"])
	}
	if len(secondPDFPositions) != 1 || secondPDFPositions[0][0] != 2 {
		t.Fatalf("JSON[1]._pdf_positions = %+v, want canonical 1-based positions (DeepDoc page 1 → 2)", secondPDFPositions)
	}
	if got, want := res.JSON[1].ToMap()["image"], "data:image/png;base64,aGVsbG8="; got != want {
		t.Fatalf("JSON[1].image = %v, want %v", got, want)
	}
}

// TestPDFParseResultToJSON_ClassifiesFigureCaptionWithImage pins the
// positions-driven contract for figure-caption classification. The parser's
// JSON path no longer inlines cropped media, so a figure caption is promoted to
// doc_type_kwd "image" only when it carries a usable PDF positions matrix
// (the on-demand crop source) — NOT merely because an image is present. This
// locks down the fix for the previous loose `v != nil` check, which promoted
// captions whose _pdf_positions were empty or not a real matrix to "image".
func TestPDFParseResultToJSON_ClassifiesFigureCaptionWithImage(t *testing.T) {
	// A figure caption WITH positions is classified as image: the on-demand
	// VLM/chunker crop path fires.
	withPos := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{{
			Text:       "小灰灰",
			LayoutType: deepdoctype.DLALabelFigureCaption,
			Positions: []deepdoctype.Position{{
				PageNumbers: []int{0},
				Left:        10,
				Right:       50,
				Top:         10,
				Bottom:      50,
			}},
		}},
	}
	res := pdfParseResultToJSON("with-pos.pdf", withPos)
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSON: %v", res.Err)
	}
	if got := res.JSON[0].ToMap()["doc_type_kwd"]; got != "image" {
		t.Fatalf("figure caption with positions doc_type_kwd = %v, want image", got)
	}

	// A figure caption with an inlined image but NO positions is NOT promoted
	// to image: positions are the sole classifier now. (In the PDF JSON path
	// media are not cropped — the chunker crops on demand from positions — so
	// an inlined image without positions does not occur; this documents that
	// the presence of a stray image alone is insufficient.)
	withImageNoPos := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{{
			Text:       "小灰灰",
			LayoutType: deepdoctype.DLALabelFigureCaption,
			Image:      "aGVsbG8=",
		}},
	}
	res2 := pdfParseResultToJSON("with-image-no-pos.pdf", withImageNoPos)
	if res2.Err != nil {
		t.Fatalf("pdfParseResultToJSON: %v", res2.Err)
	}
	if got := res2.JSON[0].ToMap()["doc_type_kwd"]; got != "text" {
		t.Fatalf("figure caption with image but no positions doc_type_kwd = %v, want text", got)
	}
}

// TestNormalizePDFPageNumber_UnconditionalIncrement pins the contract that
// DeepDoc emits 0-indexed page numbers and normalizeDeepDocPageNumber is the
// SINGLE conversion point to 1-indexed. It must add +1 unconditionally —
// not just for v<=0 — so that downstream addPDFPositions (a passthrough) and
// PositionsFromMatrix (which subtracts 1 for the 0-indexed PDFium engine)
// each see a consistent 1-indexed value.
func TestNormalizePDFPageNumber_UnconditionalIncrement(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{"zero (first page, 0-indexed)", 0, 1, true},
		{"one (second page, 0-indexed)", 1, 2, true},
		{"five", 5, 6, true},
		{"int64", int64(2), 3, true},
		{"float64", float64(3), 4, true},
		{"page list takes last element", []any{float64(0), float64(1)}, 2, true},
		{"int list takes last element", []int{0, 1, 2}, 3, true},
		{"empty list", []any{}, 0, false},
		{"non-numeric", "x", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := normalizeDeepDocPageNumber(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("got = %d, want %d (unconditional +1)", got, tc.want)
			}
		})
	}
}

func TestPDFParseResultToJSONConvertsPositiveSourcePageNumbers(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{
				Text:       "<table><tr><td>Page four</td></tr></table>",
				LayoutType: deepdoctype.LayoutTypeTable,
				Positions: []deepdoctype.Position{
					{
						PageNumbers: []int{3},
						Left:        10,
						Right:       20,
						Top:         30,
						Bottom:      40,
					},
				},
			},
		},
	}

	res := pdfParseResultToJSON("one-based.pdf", parsed)
	// DeepDoc page 3 is 0-indexed (the 4th page); normalizeDeepDocPageNumber
	// converts it to 1-indexed page 4.
	if got, want := res.JSON[0].ToMap()["page_number"], 4; got != want {
		t.Fatalf("JSON[0].page_number = %v, want %v", got, want)
	}
	if got, want := res.JSON[0].ToMap()["doc_type_kwd"], "table"; got != want {
		t.Fatalf("JSON[0].doc_type_kwd = %v, want %v", got, want)
	}
}

func TestPDFParseResultToJSON_EmptySectionsStillEmitPlaceholder(t *testing.T) {
	res := pdfParseResultToJSON("empty.pdf", &deepdoctype.ParseResult{})
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSON: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
	if got, want := res.JSON[0].ToMap()["doc_type_kwd"], "text"; got != want {
		t.Fatalf("JSON[0].doc_type_kwd = %v, want %v", got, want)
	}
}

func TestPDFParseResultToJSON_DefaultKeepsHeaderFooterLikePython(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "Header", LayoutType: "header"},
			{
				Text:       "Body",
				LayoutType: "",
				Positions: []deepdoctype.Position{{
					PageNumbers: []int{0},
					Left:        10,
					Right:       20,
					Top:         30,
					Bottom:      40,
				}},
			},
			{Text: "Footer", LayoutType: "footer"},
		},
	}

	res := pdfParseResultToJSON("filtered.pdf", parsed)
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSON: %v", res.Err)
	}
	if len(res.JSON) != 3 {
		t.Fatalf("JSON len = %d, want 3", len(res.JSON))
	}
	// Sections are now sorted by (page, top, left). Header and Footer have
	// no position data (page=0, top=0), Body has top=30, so the sorted order
	// is Header/Footer (tied top=0, stable) then Body (top=30).
	if got, want := res.JSON[0].ToMap()["text"], "Header"; got != want {
		t.Fatalf("JSON[0].text = %v, want %v", got, want)
	}
	if got, want := res.JSON[1].ToMap()["text"], "Footer"; got != want {
		t.Fatalf("JSON[1].text = %v, want %v", got, want)
	}
	if got, want := res.JSON[2].ToMap()["text"], "Body"; got != want {
		t.Fatalf("JSON[2].text = %v, want %v", got, want)
	}
}

func TestPDFParser_ConfigureFromSetup(t *testing.T) {
	p := NewPDFParser()
	p.ConfigureFromSetup(map[string]any{
		"parse_method":          "deepdoc",
		"enable_multi_column":   true,
		"flatten_media_to_text": true,
		"remove_toc":            true,
		"remove_header_footer":  true,
	})
	if !p.EnableMultiColumn {
		t.Fatal("EnableMultiColumn = false, want true")
	}
	if got, want := p.ParseMethod, "deepdoc"; got != want {
		t.Fatalf("ParseMethod = %q, want %q", got, want)
	}
	if !p.FlattenMediaToText {
		t.Fatal("FlattenMediaToText = false, want true")
	}
	if !p.RemoveTOC {
		t.Fatal("RemoveTOC = false, want true")
	}
	if !p.RemoveHeaderFooter {
		t.Fatal("RemoveHeaderFooter = false, want true")
	}
}

func TestPDFParser_ValidateParseMethod(t *testing.T) {
	p := NewPDFParser()
	if err := p.validateParseMethod(); err != nil {
		t.Fatalf("default validateParseMethod: %v", err)
	}

	p.ConfigureFromSetup(map[string]any{"parse_method": "tenant@provider@SoMark"})
	if err := p.validateParseMethod(); err != nil {
		t.Fatalf("validateParseMethod(tenant@provider@SoMark): %v", err)
	}

	if got, want := normalizePDFParseMethod("tenant@provider@OpenDataLoader"), "opendataloader"; got != want {
		t.Fatalf("normalizePDFParseMethod(OpenDataLoader suffix) = %q, want %q", got, want)
	}

	p.ConfigureFromSetup(map[string]any{"parse_method": "CustomVLM"})
	err := p.validateParseMethod()
	if err == nil {
		t.Fatal("validateParseMethod: want error for unsupported parse_method, got nil")
	}
	if !strings.Contains(err.Error(), "parse_method") {
		t.Fatalf("validateParseMethod error = %q, want parse_method context", err.Error())
	}
	if !strings.Contains(err.Error(), "IMAGE2TEXT") {
		t.Fatalf("validateParseMethod error = %q, want IMAGE2TEXT/VLM guidance", err.Error())
	}
}

// TestNormalizePDFParseMethod_PlainTextSpellings pins the plain-text
// spellings the dataset configuration UI can persist. The UI option is
// labelled "Naive" and stores "Plain Text", so both the spaced and
// spaceless spellings must land on the canonical "plain_text" method
// instead of being rejected as unsupported (or, upstream, mistaken for a
// vision model name).
func TestNormalizePDFParseMethod_PlainTextSpellings(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain_text", "plain_text"},
		{"Plain_Text", "plain_text"},
		{"plaintext", "plain_text"},
		{"PlainText", "plain_text"},
		{"plain text", "plain_text"},
		{"Plain Text", "plain_text"},
		{"  Plain Text  ", "plain_text"},
	}
	for _, c := range cases {
		if got := normalizePDFParseMethod(c.in); got != c.want {
			t.Errorf("normalizePDFParseMethod(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestIsPDFParseMethod pins the vocabulary shared by the construction-time
// check and the runtime vision dispatcher. "plain text"/"plaintext" are the
// spellings the dataset configuration UI writes for its plain-text option;
// treating them as model names breaks PDF parsing with "provider name
// missing in model name".
func TestIsPDFParseMethod(t *testing.T) {
	named := []string{
		"deepdoc", "plain_text", "plaintext", "plain text", "mineru",
		"monkeyocrv2", "docling", "opendataloader", "tcadp parser",
		"paddleocr", "somark",
		"DeepDoc", "PLAIN_TEXT", "MinerU", "DocLing",
		"OpenDataLoader", "TCADP Parser", "PaddleOCR", "SoMark",
		"Plain Text", "PlainText", "PLAIN TEXT",
	}
	for _, v := range named {
		if !IsPDFParseMethod(v) {
			t.Errorf("IsPDFParseMethod(%q) = false, want true", v)
		}
	}

	// Anything else is a VLM model name: the UI never writes the bare
	// "tcadp" abbreviation, and empty values are rejected separately.
	notNamed := []string{
		"tcadp",
		"CustomVLM", "some_vlm", "gpt-4o",
		"", "  ",
	}
	for _, v := range notNamed {
		if IsPDFParseMethod(v) {
			t.Errorf("IsPDFParseMethod(%q) = true, want false", v)
		}
	}
}

// TestIsPDFParseMethodLayoutSuffixes pins that "@"-suffixed
// layout_recognizer selectors are not parse methods; they are resolved from
// the layout_recognizer field separately.
func TestIsPDFParseMethodLayoutSuffixes(t *testing.T) {
	suffixed := []string{
		"foo@mineru", "@mineru",
		"foo@monkeyocrv2", "@monkeyocrv2",
		"foo@paddleocr", "@paddleocr",
		"foo@somark", "@somark",
		"foo@opendataloader", "@opendataloader",
		"foo@unknown",
	}
	for _, v := range suffixed {
		if IsPDFParseMethod(v) {
			t.Errorf("IsPDFParseMethod(%q) = true, want false (layout_recognizer selector, not a parse_method)", v)
		}
	}
}

// TestPDFParseMethodTablesAgree verifies executable parser methods and
// dispatcher-handled OCR methods remain recognized by the selector vocabulary.
func TestPDFParseMethodTablesAgree(t *testing.T) {
	const unsetSentinel = ""
	dispatcherHandled := map[string]bool{"monkeyocrv2": true, "paddleocr": true}
	reachable := make(map[string]bool, len(pdfParseMethodSpellings))
	for spelling, canonical := range pdfParseMethodSpellings {
		if canonical == unsetSentinel {
			t.Errorf("spelling %q maps to the unset sentinel %q; no spelling may mean \"unset\"", spelling, unsetSentinel)
			continue
		}
		reachable[canonical] = true
		if dispatcherHandled[canonical] {
			continue
		}
		if _, ok := supportedPDFParseMethods[canonical]; !ok {
			t.Errorf("spelling %q maps to canonical %q, which supportedPDFParseMethods cannot execute; add %q to that set, or handle it in the ingestion dispatcher like PaddleOCR",
				spelling, canonical, canonical)
		}
	}
	for supported := range supportedPDFParseMethods {
		if supported == unsetSentinel || reachable[supported] {
			continue
		}
		t.Errorf("supportedPDFParseMethods contains %q that no spelling maps to; without a spelling, Check() and the vision dispatcher route it to the VLM path", supported)
	}
	for method := range dispatcherHandled {
		if !IsPDFParseMethod(method) {
			t.Errorf("dispatcher-handled method %q must remain recognized", method)
		}
	}
}

type mockPDFEngineForCommonTest struct {
	closed bool
}

func (m *mockPDFEngineForCommonTest) ExtractChars(pageNum int) ([]deepdoctype.TextChar, error) {
	return nil, nil
}
func (m *mockPDFEngineForCommonTest) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	return nil, nil
}
func (m *mockPDFEngineForCommonTest) RenderPageImage(pageNum int, dpi float64) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 100, 100)), nil
}
func (m *mockPDFEngineForCommonTest) RawData() []byte { return nil }
func (m *mockPDFEngineForCommonTest) PageCount() (int, error) {
	return 1, nil
}
func (m *mockPDFEngineForCommonTest) Outlines() ([]deepdoctype.Outline, error) { return nil, nil }
func (m *mockPDFEngineForCommonTest) Close() error {
	m.closed = true
	return nil
}

// TestPDFParseResultToJSON_NoInlineRetainsPositions pins the new cgo contract:
// the parser no longer inlines base64 media into the JSON items (that bounded
// the parser-phase memory peak). Figure/table sections keep their PDF
// positions so the chunker and VLM can crop on demand. The crop logic itself
// is still exercised by the Markdown inline tests.
func TestPDFParseResultToJSON_NoInlineRetainsPositions(t *testing.T) {
	mockEngine := &mockPDFEngineForCommonTest{}
	parsed := &deepdoctype.ParseResult{
		Engine:     mockEngine,
		PageHeight: map[int]float64{0: 100},
		Sections: []deepdoctype.Section{
			{
				Text:        "",
				LayoutType:  deepdoctype.LayoutTypeFigure,
				PositionTag: "@@0\t10\t50\t10\t50##",
				Positions: []deepdoctype.Position{
					{PageNumbers: []int{0}, Left: 10, Right: 50, Top: 10, Bottom: 50},
				},
			},
			{
				Text:        "Table body",
				LayoutType:  deepdoctype.LayoutTypeTable,
				PositionTag: "@@0\t20\t60\t20\t60##",
				Positions: []deepdoctype.Position{
					{PageNumbers: []int{0}, Left: 20, Right: 60, Top: 20, Bottom: 60},
				},
			},
			{
				Text:        "Plain text paragraph",
				LayoutType:  deepdoctype.LayoutTypeText,
				PositionTag: "@@0\t0\t100\t70\t90##",
				Positions: []deepdoctype.Position{
					{PageNumbers: []int{0}, Left: 0, Right: 100, Top: 70, Bottom: 90},
				},
			},
		},
	}

	res := pdfParseResultToJSON("media.pdf", parsed)
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSON: %v", res.Err)
	}
	if !mockEngine.closed {
		t.Fatal("Engine should be closed after pdfParseResultToJSON")
	}
	if len(res.JSON) != 3 {
		t.Fatalf("JSON len = %d, want 3", len(res.JSON))
	}

	// A textless figure must remain an image crop item without inlining an image.
	figure := res.JSON[0].ToMap()
	if got, want := figure["doc_type_kwd"], "image"; got != want {
		t.Fatalf("textless figure doc_type_kwd = %v, want %v", got, want)
	}
	figPos, ok := figure["_pdf_positions"].([][]any)
	if !ok || len(figPos) == 0 {
		t.Fatalf("textless figure should retain _pdf_positions, got %v", figure["_pdf_positions"])
	}
	if !HasPDFCropRegion(figure) {
		t.Fatal("textless figure should carry a crop region for VLM and chunker")
	}
	if img, _ := figure["image"].(string); img != "" {
		t.Fatalf("Figure should not inline image under cgo, got %q", img)
	}

	// Table likewise retains positions, no inline image.
	tblPos, ok := res.JSON[1].ToMap()["_pdf_positions"].([][]any)
	if !ok || len(tblPos) == 0 {
		t.Fatalf("Table should retain _pdf_positions, got %v", res.JSON[1].ToMap()["_pdf_positions"])
	}
	if img, _ := res.JSON[1].ToMap()["image"].(string); img != "" {
		t.Fatalf("Table should not inline image under cgo, got %q", img)
	}

	// Plain text: no image, positions retained.
	if img, _ := res.JSON[2].ToMap()["image"].(string); img != "" {
		t.Fatalf("Text should not inline image, got %q", img)
	}
}

// TestPDFParseResultToJSON_FigureCaptionNoInline pins that a figure caption is
// still classified as doc_type_kwd "image" (so the chunker/VLM crop it on
// demand) but the parser does not inline the cropped image for it.
func TestPDFParseResultToJSON_FigureCaptionNoInline(t *testing.T) {
	mockEngine := &mockPDFEngineForCommonTest{}
	parsed := &deepdoctype.ParseResult{
		Engine:     mockEngine,
		PageHeight: map[int]float64{0: 100},
		Sections: []deepdoctype.Section{{
			Text:       "Figure caption",
			LayoutType: deepdoctype.DLALabelFigureCaption,
			Positions: []deepdoctype.Position{{
				PageNumbers: []int{0},
				Left:        10,
				Right:       50,
				Top:         10,
				Bottom:      50,
			}},
		}},
	}

	res := pdfParseResultToJSON("figure-caption.pdf", parsed)
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSON: %v", res.Err)
	}
	if got, want := res.JSON[0].ToMap()["doc_type_kwd"], "image"; got != want {
		t.Fatalf("doc_type_kwd = %v, want %v", got, want)
	}
	if pos, ok := res.JSON[0].ToMap()["_pdf_positions"].([][]any); !ok || len(pos) == 0 {
		t.Fatal("figure caption should retain _pdf_positions for on-demand crop")
	}
	if image, _ := res.JSON[0].ToMap()["image"].(string); image != "" {
		t.Fatalf("figure caption should not be inlined by the parser, got %q", image)
	}
}

// TestExtractPDFPositions locks down the single source of truth for "does this
// item carry a usable PDF crop region". The historical bug was a loose
// `v != nil` test that accepted empty slices and stray scalars; this test
// proves the contract is now strict (non-empty matrix only) and that both the
// canonical _pdf_positions key and the legacy positions key are honored, in
// both the typed [][]any form and the JSON-decoded []any form.
func TestExtractPDFPositions(t *testing.T) {
	cases := []struct {
		name string
		item map[string]any
		want bool
		rows int
		key  string
	}{
		{
			name: "typed non-empty matrix under _pdf_positions",
			item: map[string]any{"_pdf_positions": [][]any{{float64(1), float64(2), float64(3), float64(4), float64(1)}}},
			want: true, rows: 1, key: "_pdf_positions",
		},
		{
			name: "json-decoded []any rows under _pdf_positions",
			item: map[string]any{"_pdf_positions": []any{[]any{float64(1), float64(2), float64(3), float64(4), float64(1)}}},
			want: true, rows: 1, key: "_pdf_positions",
		},
		{
			name: "legacy positions key honored",
			item: map[string]any{"positions": [][]any{{float64(1), float64(2), float64(3), float64(4), float64(1)}}},
			want: true, rows: 1, key: "positions",
		},
		{
			name: "empty typed matrix rejected",
			item: map[string]any{"_pdf_positions": [][]any{}},
			want: false,
		},
		{
			name: "empty json-decoded matrix rejected",
			item: map[string]any{"_pdf_positions": []any{}},
			want: false,
		},
		{
			name: "nil value rejected",
			item: map[string]any{"_pdf_positions": nil},
			want: false,
		},
		{
			name: "missing key rejected",
			item: map[string]any{"doc_type_kwd": "image"},
			want: false,
		},
		{
			name: "stray scalar rejected (old v != nil bug)",
			item: map[string]any{"_pdf_positions": "not-a-matrix"},
			want: false,
		},
		{
			name: "non-slice numeric rejected",
			item: map[string]any{"_pdf_positions": float64(42)},
			want: false,
		},
		{
			name: "[]any with non-row element rejected",
			item: map[string]any{"_pdf_positions": []any{"bad", float64(1)}},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matrix, ok := ExtractPDFPositions(tc.item)
			if ok != tc.want {
				t.Fatalf("ExtractPDFPositions ok = %v, want %v", ok, tc.want)
			}
			if tc.want {
				if len(matrix) != tc.rows {
					t.Fatalf("matrix rows = %d, want %d", len(matrix), tc.rows)
				}
				// Every returned row must be []any (normalized form).
				for i, row := range matrix {
					if row == nil {
						t.Fatalf("matrix[%d] is nil after normalization", i)
					}
				}
				// The honored key must carry the matrix in normalized [][]any.
				if _, present := tc.item[tc.key]; !present {
					t.Fatalf("expected honored key %q missing", tc.key)
				}
			}
		})
	}
}

// TestNormalizePDFDocType_FigureCaptionPositionsGate proves Finding B: a figure
// caption is only promoted to doc_type_kwd "image" (which lights up the
// on-demand VLM/chunker crop) when it actually carries a usable positions
// matrix. The previous loose `v != nil` check promoted captions whose
// _pdf_positions were empty or not a real matrix, wrongly flagging them for
// cropping. normalizePDFDocType must delegate to ExtractPDFPositions.
func TestNormalizePDFDocType_FigureCaptionPositionsGate(t *testing.T) {
	withPos := map[string]any{
		"layout_type":    deepdoctype.DLALabelFigureCaption,
		"_pdf_positions": [][]any{{float64(1), float64(2), float64(3), float64(4), float64(1)}},
	}
	normalizePDFDocType(withPos)
	if got := withPos["doc_type_kwd"]; got != "image" {
		t.Fatalf("with positions doc_type_kwd = %v, want image", got)
	}

	for _, empty := range []any{
		[]any{},
		[][]any{},
		nil,
		"stray-scalar",
	} {
		withoutPos := map[string]any{
			"layout_type":    deepdoctype.DLALabelFigureCaption,
			"_pdf_positions": empty,
		}
		normalizePDFDocType(withoutPos)
		if got := withoutPos["doc_type_kwd"]; got != "text" {
			t.Fatalf("_pdf_positions=%#v doc_type_kwd = %v, want text (must not promote empty/non-matrix)", empty, got)
		}
	}
}

func TestPDFParseResultToJSON_EngineNilGraceful(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Engine:     nil,
		PageHeight: map[int]float64{0: 100},
		Sections: []deepdoctype.Section{
			{
				Text:       "Figure caption",
				LayoutType: deepdoctype.LayoutTypeFigure,
				Positions: []deepdoctype.Position{
					{PageNumbers: []int{0}, Left: 10, Right: 50, Top: 10, Bottom: 50},
				},
			},
		},
	}

	res := pdfParseResultToJSON("nil_engine.pdf", parsed)
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSON with nil engine returned err: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
}

func TestPDFParserPaddleOCRRequiresTenantDispatch(t *testing.T) {
	p := NewPDFParser()
	p.ConfigureFromSetup(map[string]any{"parse_method": "paddleocr"})
	result := p.ParseWithResult(t.Context(), "document.pdf", []byte("%PDF-1.4"))
	if result.Err == nil || !strings.Contains(result.Err.Error(), "unsupported PDF parse_method") {
		t.Fatalf("standalone parser must not submit OCR jobs: %v", result.Err)
	}
}
