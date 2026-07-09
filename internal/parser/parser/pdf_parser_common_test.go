package parser

import (
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
	if got, want := res.JSON[1]["page_number"], 1; got != want {
		t.Fatalf("JSON[1].page_number = %v, want %v", got, want)
	}
	secondPDFPositions, ok := res.JSON[1]["_pdf_positions"].([][]any)
	if !ok {
		t.Fatalf("JSON[1]._pdf_positions type = %T, want [][]any", res.JSON[1]["_pdf_positions"])
	}
	if len(secondPDFPositions) != 1 || secondPDFPositions[0][0] != 1 {
		t.Fatalf("JSON[1]._pdf_positions = %+v, want canonical 1-based positions", secondPDFPositions)
	}
	if got, want := res.JSON[1]["image"], "data:image/png;base64,aGVsbG8="; got != want {
		t.Fatalf("JSON[1].image = %v, want %v", got, want)
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
	if got, want := res.JSON[0]["page_number"], 3; got != want {
		t.Fatalf("JSON[1].page_number = %v, want %v", got, want)
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
	if got, want := res.JSON[0]["text"], "Header"; got != want {
		t.Fatalf("JSON[0].text = %v, want %v", got, want)
	}
	if got, want := res.JSON[1]["text"], "Body"; got != want {
		t.Fatalf("JSON[1].text = %v, want %v", got, want)
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
		t.Fatalf("Markdown = %q, want inline image", res.Markdown)
	}
	if !strings.Contains(res.Markdown, "Body") {
		t.Fatalf("Markdown = %q, want body text", res.Markdown)
	}
	if len(res.JSON) != 0 {
		t.Fatalf("JSON len = %d, want 0 for markdown output", len(res.JSON))
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
