package parser

import (
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
