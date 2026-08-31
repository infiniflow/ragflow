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

func TestPDFParseResultToJSONWithOptions_FiltersHeaderFooterWhenEnabled(t *testing.T) {
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

	res := pdfParseResultToJSONWithOptions("filtered.pdf", parsed, pdfPostProcessOptions{removeHeaderFooter: true})
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSONWithOptions: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
	if got, want := res.JSON[0]["text"], "Body"; got != want {
		t.Fatalf("JSON[0].text = %v, want %v", got, want)
	}
}

func TestPDFParseResultToJSONWithOptions_RemovesTOCByOutline(t *testing.T) {
	parsed := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{
				Text:       "Contents",
				LayoutType: "text",
				Positions: []deepdoctype.Position{{
					PageNumbers: []int{1},
					Left:        10,
					Right:       20,
					Top:         30,
					Bottom:      40,
				}},
			},
			{
				Text:       "Body",
				LayoutType: "text",
				Positions: []deepdoctype.Position{{
					PageNumbers: []int{3},
					Left:        10,
					Right:       20,
					Top:         30,
					Bottom:      40,
				}},
			},
		},
		Outlines: []deepdoctype.Outline{
			{Title: "目录", Level: 0, PageNumber: 1},
			{Title: "Chapter 1", Level: 0, PageNumber: 3},
		},
	}

	res := pdfParseResultToJSONWithOptions("toc.pdf", parsed, pdfPostProcessOptions{removeTOC: true})
	if res.Err != nil {
		t.Fatalf("pdfParseResultToJSONWithOptions: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
	if got, want := res.JSON[0]["text"], "Body"; got != want {
		t.Fatalf("JSON[0].text = %v, want %v", got, want)
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

func TestPDFParseResultToJSON_CropsMediaSectionsWithEngine(t *testing.T) {
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

	// Figure should have cropped image populated
	figImg, ok := res.JSON[0]["image"].(string)
	if !ok || figImg == "" {
		t.Fatalf("Figure JSON[0].image should be non-empty base64 data url, got %v", res.JSON[0]["image"])
	}
	if !strings.HasPrefix(figImg, "data:image/png;base64,") {
		t.Fatalf("Figure JSON[0].image prefix mismatch, got %q", figImg)
	}

	// Table should have cropped image populated
	tblImg, ok := res.JSON[1]["image"].(string)
	if !ok || tblImg == "" {
		t.Fatalf("Table JSON[1].image should be non-empty base64 data url, got %v", res.JSON[1]["image"])
	}
	if !strings.HasPrefix(tblImg, "data:image/png;base64,") {
		t.Fatalf("Table JSON[1].image prefix mismatch, got %q", tblImg)
	}

	// Plain text should NOT have cropped image
	textImg, _ := res.JSON[2]["image"].(string)
	if textImg != "" {
		t.Fatalf("Text JSON[2].image should be empty, got %q", textImg)
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
