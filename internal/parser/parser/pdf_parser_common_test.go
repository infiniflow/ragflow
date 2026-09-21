package parser

import (
	"image"
	"strings"
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

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
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want json", res.OutputFormat)
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
	if got, want := res.JSON[0]["layout"], "title"; got != want {
		t.Fatalf("JSON[0].layout = %v, want %v", got, want)
	}
	if got, want := res.JSON[0]["page_number"], 1; got != want {
		t.Fatalf("JSON[0].page_number = %v, want %v", got, want)
	}
	if got, want := res.JSON[0]["doc_type_kwd"], "text"; got != want {
		t.Fatalf("JSON[0].doc_type_kwd = %v, want %v", got, want)
	}
	pdfPositions, ok := res.JSON[0]["_pdf_positions"].([][]any)
	if !ok {
		t.Fatalf("JSON[0]._pdf_positions type = %T, want [][]any", res.JSON[0]["_pdf_positions"])
	}
	if len(pdfPositions) != 1 || pdfPositions[0][0] != 1 {
		t.Fatalf("JSON[0]._pdf_positions = %+v, want canonical 1-based positions", pdfPositions)
	}
	if got := res.JSON[0]["positions"]; got == nil {
		t.Fatal("JSON[0].positions missing after normalization")
	}
	if got, want := res.JSON[1]["doc_type_kwd"], "image"; got != want {
		t.Fatalf("JSON[1].doc_type_kwd = %v, want %v", got, want)
	}
	if got, want := res.JSON[1]["page_number"], 2; got != want {
		t.Fatalf("JSON[1].page_number = %v, want %v", got, want)
	}
	secondPDFPositions, ok := res.JSON[1]["_pdf_positions"].([][]any)
	if !ok {
		t.Fatalf("JSON[1]._pdf_positions type = %T, want [][]any", res.JSON[1]["_pdf_positions"])
	}
	if len(secondPDFPositions) != 1 || secondPDFPositions[0][0] != 2 {
		t.Fatalf("JSON[1]._pdf_positions = %+v, want canonical 1-based positions (DeepDoc page 1 → 2)", secondPDFPositions)
	}
	if got, want := res.JSON[1]["image"], "data:image/png;base64,aGVsbG8="; got != want {
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
	if got := res.JSON[0]["doc_type_kwd"]; got != "image" {
		t.Fatalf("figure caption with positions doc_type_kwd = %v, want image", got)
	}

	// A figure caption with an inlined image but NO positions is NOT promoted
	// to image: positions are the sole classifier now. (In the PDF JSON path
	// an inlined image without positions does not occur — cropMediaSections was
	// removed — so this documents that the presence of a stray image alone is
	// insufficient.)
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
	if got := res2.JSON[0]["doc_type_kwd"]; got != "text" {
		t.Fatalf("figure caption with image but no positions doc_type_kwd = %v, want text", got)
	}
}

// TestNormalizePDFPageNumber_UnconditionalIncrement pins the contract that
// DeepDoc emits 0-indexed page numbers and normalizePDFPageNumber is the
// SINGLE conversion point to 1-indexed. It must add +1 unconditionally —
// not just for v<=0 — so that downstream AddPositions (a passthrough) and
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
			got, ok := normalizePDFPageNumber(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("got = %d, want %d (unconditional +1)", got, tc.want)
			}
		})
	}
}

func TestPDFParseResultToJSON_PreservesPositivePageNumbers(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{
				Text:       "Already one-based",
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
	// DeepDoc page 3 is 0-indexed (the 4th page); normalizePDFPageNumber
	// converts it to 1-indexed page 4.
	if got, want := res.JSON[0]["page_number"], 4; got != want {
		t.Fatalf("JSON[0].page_number = %v, want %v", got, want)
	}
	if got, want := res.JSON[0]["doc_type_kwd"], "table"; got != want {
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
	if got, want := res.JSON[0]["doc_type_kwd"], "text"; got != want {
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
	if got, want := res.JSON[0]["text"], "Header"; got != want {
		t.Fatalf("JSON[0].text = %v, want %v", got, want)
	}
	if got, want := res.JSON[1]["text"], "Footer"; got != want {
		t.Fatalf("JSON[1].text = %v, want %v", got, want)
	}
	if got, want := res.JSON[2]["text"], "Body"; got != want {
		t.Fatalf("JSON[2].text = %v, want %v", got, want)
	}
}

func TestPDFParser_ConfigureFromSetup(t *testing.T) {
	p := NewPDFParser()
	p.ConfigureFromSetup(map[string]any{
		"parse_method":          "deepdoc",
		"output_format":         "markdown",
		"enable_multi_column":   true,
		"flatten_media_to_text": true,
		"remove_toc":            true,
		"remove_header_footer":  true,
	})
	if got, want := p.OutputFormat, "markdown"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
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

func TestPDFParseResultToMarkdownWithOptions_RendersLikePython(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "Title", LayoutType: deepdoctype.LayoutTypeTitle},
			{Text: "Figure", LayoutType: deepdoctype.LayoutTypeFigure, Image: "aGVsbG8="},
			{Text: "WhitespaceFigureText", LayoutType: deepdoctype.LayoutTypeFigure, Image: "   \t\n"},
			{Text: "<table><tr><td>cell</td></tr></table>", LayoutType: deepdoctype.LayoutTypeTable, Image: "dGFibGVpbWc="},
			{Text: "", LayoutType: deepdoctype.LayoutTypeTable, Image: "dGFibGVvbmx5"},
			{Text: "ImageCaption", LayoutType: "image", Image: "aW1hZ2Vvbmx5"},
			{Text: "Body", LayoutType: deepdoctype.LayoutTypeText},
		},
	}

	res := pdfParseResultToMarkdownWithOptions("sample.pdf", parsed, pdfPostProcessOptions{})
	if res.Err != nil {
		t.Fatalf("pdfParseResultToMarkdownWithOptions: %v", res.Err)
	}
	if got, want := res.OutputFormat, "markdown"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if res.Markdown == "" {
		t.Fatal("Markdown is empty; want rendered content")
	}
	if !strings.Contains(res.Markdown, "## Title") {
		t.Fatalf("Markdown = %q, want title heading", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "![Image](data:image/png;base64,aGVsbG8=)") {
		t.Fatalf("Markdown = %q, want inline figure image", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "WhitespaceFigureText") {
		t.Fatalf("Markdown = %q, want whitespace figure text preserved", res.Markdown)
	}
	if strings.Contains(res.Markdown, "![Image]()") {
		t.Fatalf("Markdown = %q, unexpected empty image tag", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "<table><tr><td>cell</td></tr></table>") {
		t.Fatalf("Markdown = %q, want table text", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "![Image](data:image/png;base64,dGFibGVvbmx5)") {
		t.Fatalf("Markdown = %q, want fallback table image when text is empty", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "![Image](data:image/png;base64,aW1hZ2Vvbmx5)") {
		t.Fatalf("Markdown = %q, want image section", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "Body") {
		t.Fatalf("Markdown = %q, want body text", res.Markdown)
	}
	if len(res.JSON) != 0 {
		t.Fatalf("JSON len = %d, want 0 for markdown output", len(res.JSON))
	}
}

func TestPDFParseResultToMarkdownWithOptions_TableFallback(t *testing.T) {
	// 1. Table with text + image -> renders table text
	withText := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "<table><tr><td>content</td></tr></table>", LayoutType: deepdoctype.LayoutTypeTable, Image: "dGFibGVpbWc="},
		},
	}
	resText := pdfParseResultToMarkdownWithOptions("table.pdf", withText, pdfPostProcessOptions{})
	if !strings.Contains(resText.Markdown, "<table><tr><td>content</td></tr></table>") {
		t.Fatalf("Markdown = %q, want table text", resText.Markdown)
	}
	if strings.Contains(resText.Markdown, "![Image]") {
		t.Fatalf("Markdown = %q, unexpected image tag when table text is present", resText.Markdown)
	}

	// 2. Table with empty text + image -> falls back to image tag
	emptyText := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "", LayoutType: deepdoctype.LayoutTypeTable, Image: "dGFibGVvbmx5"},
		},
	}
	resFallback := pdfParseResultToMarkdownWithOptions("table.pdf", emptyText, pdfPostProcessOptions{})
	if !strings.Contains(resFallback.Markdown, "![Image](data:image/png;base64,dGFibGVvbmx5)") {
		t.Fatalf("Markdown = %q, want fallback table image tag", resFallback.Markdown)
	}

	// 3. Table with empty text + whitespace-only image -> empty string, no broken tags
	wsImg := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "", LayoutType: deepdoctype.LayoutTypeTable, Image: "   \t\n"},
		},
	}
	resWS := pdfParseResultToMarkdownWithOptions("table.pdf", wsImg, pdfPostProcessOptions{})
	if strings.TrimSpace(resWS.Markdown) != "" {
		t.Fatalf("Markdown = %q, want empty output for empty text + whitespace image", resWS.Markdown)
	}
}

func TestSectionsToMarkdown_DocTypeKwdImage(t *testing.T) {
	// LayoutType is not figure/image, but DocTypeKwd == "image"
	sections := []deepdoctype.Section{
		{Text: "Caption", LayoutType: "custom_block", DocTypeKwd: "image", Image: "aW1hZ2Vvbmx5"},
	}
	got := sectionsToMarkdown(sections)
	want := "\n![Image](data:image/png;base64,aW1hZ2Vvbmx5)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	// DocTypeKwd == "image" with whitespace image preserves text and drops empty tag
	sectionsWS := []deepdoctype.Section{
		{Text: "Caption", LayoutType: "custom_block", DocTypeKwd: "image", Image: "   \t\n"},
	}
	gotWS := sectionsToMarkdown(sectionsWS)
	wantWS := "Caption\n"
	if gotWS != wantWS {
		t.Fatalf("got %q, want %q", gotWS, wantWS)
	}
}

func TestPDFParseResultToMarkdownWithOptions_ImageSection(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "Caption", LayoutType: "image", Image: "aW1hZ2Vvbmx5"},
		},
	}
	res := pdfParseResultToMarkdownWithOptions("doc.pdf", parsed, pdfPostProcessOptions{})
	if !strings.Contains(res.Markdown, "![Image](data:image/png;base64,aW1hZ2Vvbmx5)") {
		t.Fatalf("Markdown = %q, want image embed for LayoutType == 'image'", res.Markdown)
	}
}

func TestPDFParseResultToMarkdownWithOptions_WhitespaceOnlyImage(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "Figure Caption", LayoutType: deepdoctype.LayoutTypeFigure, Image: "   \r\n\t "},
		},
	}
	res := pdfParseResultToMarkdownWithOptions("doc.pdf", parsed, pdfPostProcessOptions{})
	if !strings.Contains(res.Markdown, "Figure Caption") {
		t.Fatalf("Markdown = %q, want figure caption preserved", res.Markdown)
	}
	if strings.Contains(res.Markdown, "![Image]") {
		t.Fatalf("Markdown = %q, unexpected image tag for whitespace-only image", res.Markdown)
	}
}

func TestPDFParser_ValidateParseMethod(t *testing.T) {
	p := NewPDFParser()
	if err := p.validateParseMethod(); err != nil {
		t.Fatalf("default validateParseMethod: %v", err)
	}

	p.ConfigureFromSetup(map[string]any{"parse_method": "PaddleOCR"})
	if err := p.validateParseMethod(); err != nil {
		t.Fatalf("validateParseMethod(PaddleOCR): %v", err)
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
				Text:        "Figure caption",
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

	// Figure must retain positions but NOT inline an image.
	figPos, ok := res.JSON[0]["_pdf_positions"].([][]any)
	if !ok || len(figPos) == 0 {
		t.Fatalf("Figure should retain _pdf_positions, got %v", res.JSON[0]["_pdf_positions"])
	}
	if img, _ := res.JSON[0]["image"].(string); img != "" {
		t.Fatalf("Figure should not inline image under cgo, got %q", img)
	}

	// Table likewise retains positions, no inline image.
	tblPos, ok := res.JSON[1]["_pdf_positions"].([][]any)
	if !ok || len(tblPos) == 0 {
		t.Fatalf("Table should retain _pdf_positions, got %v", res.JSON[1]["_pdf_positions"])
	}
	if img, _ := res.JSON[1]["image"].(string); img != "" {
		t.Fatalf("Table should not inline image under cgo, got %q", img)
	}

	// Plain text: no image, positions retained.
	if img, _ := res.JSON[2]["image"].(string); img != "" {
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
	if got, want := res.JSON[0]["doc_type_kwd"], "image"; got != want {
		t.Fatalf("doc_type_kwd = %v, want %v", got, want)
	}
	if pos, ok := res.JSON[0]["_pdf_positions"].([][]any); !ok || len(pos) == 0 {
		t.Fatal("figure caption should retain _pdf_positions for on-demand crop")
	}
	if image, _ := res.JSON[0]["image"].(string); image != "" {
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
