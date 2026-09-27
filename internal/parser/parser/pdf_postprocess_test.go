package parser

import (
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

func makePDFSection(text, layout string, page int, left, right, top, bottom float64) deepdoctype.Section {
	return deepdoctype.Section{
		Text:       text,
		LayoutType: layout,
		Positions: []deepdoctype.Position{{
			PageNumbers: []int{page},
			Left:        left,
			Right:       right,
			Top:         top,
			Bottom:      bottom,
		}},
	}
}

func TestApplyPDFPostProcess_NormalizesLayoutTypes(t *testing.T) {
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "a", LayoutType: ""},
			{Text: "b", LayoutType: "  "},
			{Text: "c", LayoutType: "table"},
			{Text: "d", LayoutType: "  figure  "},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{})
	want := []string{"text", "text", "table", "figure"}
	for i, s := range result.Sections {
		if s.LayoutType != want[i] {
			t.Fatalf("Sections[%d].LayoutType = %q, want %q", i, s.LayoutType, want[i])
		}
	}
}

func TestApplyPDFPostProcess_AssignsDocTypeKeywords(t *testing.T) {
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "a", LayoutType: "table"},
			{Text: "b", LayoutType: "figure"},
			{Text: "c", LayoutType: "text"},
			{Text: "d", LayoutType: "", Image: "abc"},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{})
	// doc_type_kwd is derived from layout type only. A pre-set Image no
	// longer reclassifies a section as "image" — cropping happens lazily
	// at Markdown serialization / chunk time (see pdf_parser_common.go).
	want := []string{"table", "image", "text", "text"}
	for i, s := range result.Sections {
		if s.DocTypeKwd != want[i] {
			t.Fatalf("Sections[%d].DocTypeKwd = %q, want %q", i, s.DocTypeKwd, want[i])
		}
	}
}

func TestApplyPDFPostProcess_FlattenMediaKeepsImagesButMarksText(t *testing.T) {
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "a", LayoutType: "figure", Image: "abc"},
			{Text: "b", LayoutType: "table"},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{flattenMediaToText: true})
	for i, s := range result.Sections {
		if s.DocTypeKwd != "text" {
			t.Fatalf("Sections[%d].DocTypeKwd = %q, want text", i, s.DocTypeKwd)
		}
	}
	if got, want := result.Sections[0].Image, "abc"; got != want {
		t.Fatalf("Sections[0].Image = %q, want %q", got, want)
	}
}

func TestApplyPDFPostProcess_ReordersMultiColumnText(t *testing.T) {
	cases := []struct {
		name      string
		pageWidth float64
		zoom      float64
	}{
		{name: "unit zoom", pageWidth: 600, zoom: 1},
		{name: "pre-normalized width", pageWidth: 200, zoom: 3},
	}
	for _, tc := range cases {
		result := &deepdoctype.ParseResult{
			Sections: []deepdoctype.Section{
				makePDFSection("right", "text", 0, 100, 166, 100, 120),
				makePDFSection("left", "text", 0, 10, 76, 100, 120),
			},
		}
		applyPDFPostProcess(result, pdfPostProcessOptions{pageWidth: tc.pageWidth, zoom: tc.zoom, enableMultiColumn: true})
		if got, want := result.Sections[0].Text, "left"; got != want {
			t.Fatalf("%s: Sections[0].Text = %q, want %q", tc.name, got, want)
		}
	}
}
