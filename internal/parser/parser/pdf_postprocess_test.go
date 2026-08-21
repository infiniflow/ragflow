package parser

import (
	"strings"
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

func TestApplyPDFPostProcess_HeaderFooterFilteringIsOptional(t *testing.T) {
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "header", LayoutType: "header"},
			{Text: "body", LayoutType: "text"},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{})
	if len(result.Sections) != 2 {
		t.Fatalf("len(Sections) = %d, want 2 when removeHeaderFooter is false", len(result.Sections))
	}

	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	if len(result.Sections) != 1 {
		t.Fatalf("len(Sections) = %d, want 1 when removeHeaderFooter is true", len(result.Sections))
	}
	if got, want := result.Sections[0].Text, "body"; got != want {
		t.Fatalf("remaining section = %q, want %q", got, want)
	}
}

func TestIsPDFTOCEntry(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"Chapter One Introduction .............. 2", true},
		{"Chapter Two Methods ................... 3", true},
		{"1.1 Overview ……… 12", true},
		{"Preface    7", true},
		{"3.2 Methods ..... 15", true},
		{"Chapter One Introduction", false}, // body heading: no page number
		{"Page 3 of 10", false},             // no letters before number
		{"- 2 -", false},                    // page-number footer, not a TOC entry
		{"See the results in 2024.", false}, // single space before number
		{"abc", false},                      // too short
		{"", false},
	}
	for _, c := range cases {
		if got := isPDFTOCEntry(c.text); got != c.want {
			t.Errorf("isPDFTOCEntry(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestApplyPDFPostProcess_RemoveTOCRemovesAllDotLeaderEntries(t *testing.T) {
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "Contents"},
			{Text: "Chapter One Introduction .............. 2"},
			{Text: "Chapter Two Methods ................... 3"},
			{Text: "Chapter Three Results ................. 4"},
			{Text: "Chapter One Introduction"},
			{Text: "Body text that must survive parsing."},
			{Text: "Chapter Two Methods"},
			{Text: "More body text."},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeTOC: true})
	var kept []string
	for _, s := range result.Sections {
		kept = append(kept, s.Text)
	}
	want := []string{
		"Chapter One Introduction",
		"Body text that must survive parsing.",
		"Chapter Two Methods",
		"More body text.",
	}
	if len(kept) != len(want) {
		t.Fatalf("kept = %v, want %v", kept, want)
	}
	for i := range want {
		if kept[i] != want[i] {
			t.Fatalf("kept = %v, want %v", kept, want)
		}
	}
}

func TestApplyPDFPostProcess_RemoveTOCPrefixScanWithoutEntryPattern(t *testing.T) {
	// Pattern-less TOC entries take the prefix scan: title and first entry
	// go, then everything up to the first line sharing the first entry's
	// prefix (the real body heading).
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "Contents"},
			{Text: "1. Introduction"},
			{Text: "2. Background"},
			{Text: "1. Introduction"},
			{Text: "Body text."},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeTOC: true})
	var kept []string
	for _, s := range result.Sections {
		kept = append(kept, s.Text)
	}
	want := []string{"1. Introduction", "Body text."}
	if len(kept) != len(want) {
		t.Fatalf("kept = %v, want %v", kept, want)
	}
	for i := range want {
		if kept[i] != want[i] {
			t.Fatalf("kept = %v, want %v", kept, want)
		}
	}
}

func TestApplyPDFPostProcess_RemoveRunningHeaderFooterWithoutDLA(t *testing.T) {
	const pageHeight = 842.0
	result := &deepdoctype.ParseResult{
		PageHeight: map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight, 3: pageHeight},
		Sections: []deepdoctype.Section{
			makePDFSection("Contents", "", 0, 72, 158, 40, 60),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 1, 72, 210, 21, 30),
			makePDFSection("Chapter One Introduction", "", 1, 72, 206, 88, 100),
			makePDFSection("Body text one.", "", 1, 72, 363, 124, 136),
			makePDFSection("- 2 -", "", 1, 289, 305, 807, 817),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 2, 72, 210, 21, 30),
			makePDFSection("Chapter Two Methods", "", 2, 72, 190, 88, 100),
			makePDFSection("Body text two.", "", 2, 72, 374, 124, 136),
			makePDFSection("- 3 -", "", 2, 289, 305, 807, 817),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 3, 72, 210, 21, 30),
			makePDFSection("Chapter Three Results", "", 3, 72, 192, 88, 100),
			makePDFSection("Body text three.", "", 3, 72, 367, 124, 136),
			makePDFSection("- 4 -", "", 3, 289, 305, 807, 817),
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	for _, s := range result.Sections {
		if strings.Contains(s.Text, "CONFIDENTIAL") {
			t.Fatalf("running header survived: %q", s.Text)
		}
		if strings.HasPrefix(strings.TrimSpace(s.Text), "- ") {
			t.Fatalf("page-number footer survived: %q", s.Text)
		}
	}
	if len(result.Sections) != 7 {
		t.Fatalf("len(Sections) = %d, want 7 (body content preserved)", len(result.Sections))
	}
}

func TestApplyPDFPostProcess_RunningHeaderFooterKeepsUniqueZoneText(t *testing.T) {
	const pageHeight = 842.0
	result := &deepdoctype.ParseResult{
		PageHeight: map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight},
		Sections: []deepdoctype.Section{
			// Different top-of-page text on every page: not a running header.
			makePDFSection("Unique top line one", "", 0, 72, 210, 21, 30),
			makePDFSection("Body one.", "", 0, 72, 363, 124, 136),
			makePDFSection("Unique top line two", "", 1, 72, 210, 21, 30),
			makePDFSection("Body two.", "", 1, 72, 363, 124, 136),
			makePDFSection("Unique top line three", "", 2, 72, 210, 21, 30),
			makePDFSection("Body three.", "", 2, 72, 363, 124, 136),
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	if len(result.Sections) != 6 {
		t.Fatalf("len(Sections) = %d, want 6 (unique zone text must survive)", len(result.Sections))
	}
}

func TestApplyPDFPostProcess_RunningHeaderFooterSkipsShortDocuments(t *testing.T) {
	const pageHeight = 842.0
	result := &deepdoctype.ParseResult{
		PageHeight: map[int]float64{0: pageHeight, 1: pageHeight},
		Sections: []deepdoctype.Section{
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 0, 72, 210, 21, 30),
			makePDFSection("Body one.", "", 0, 72, 363, 124, 136),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 1, 72, 210, 21, 30),
			makePDFSection("Body two.", "", 1, 72, 363, 124, 136),
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	if len(result.Sections) != 4 {
		t.Fatalf("len(Sections) = %d, want 4 (2-page doc must not be touched)", len(result.Sections))
	}
}

func TestApplyPDFPostProcess_RunningHeaderFooterCountsAllRenderedPages(t *testing.T) {
	const pageHeight = 842.0
	// Pages 3 and 4 are blank or image-only: they produced no sections but
	// still count toward the page total. A candidate repeated on 2 of the 5
	// rendered pages occurs on fewer than half of the document's pages and
	// must survive.
	result := &deepdoctype.ParseResult{
		PageHeight: map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight, 3: pageHeight, 4: pageHeight},
		Sections: []deepdoctype.Section{
			makePDFSection("Chapter One Introduction", "", 0, 72, 206, 88, 100),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 1, 72, 210, 21, 30),
			makePDFSection("Body text one.", "", 1, 72, 363, 124, 136),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 2, 72, 210, 21, 30),
			makePDFSection("Body text two.", "", 2, 72, 374, 124, 136),
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	if len(result.Sections) != 5 {
		t.Fatalf("len(Sections) = %d, want 5 (2 of 5 rendered pages is below the repetition threshold)", len(result.Sections))
	}

	// The same header on 3 of the 5 rendered pages meets the threshold and
	// is removed even though two pages contributed no sections.
	result = &deepdoctype.ParseResult{
		PageHeight: map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight, 3: pageHeight, 4: pageHeight},
		Sections: []deepdoctype.Section{
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 0, 72, 210, 21, 30),
			makePDFSection("Body text one.", "", 0, 72, 363, 124, 136),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 1, 72, 210, 21, 30),
			makePDFSection("Body text two.", "", 1, 72, 363, 124, 136),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 2, 72, 210, 21, 30),
			makePDFSection("Body text three.", "", 2, 72, 367, 124, 136),
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	if len(result.Sections) != 3 {
		t.Fatalf("len(Sections) = %d, want 3 (running header removed on 3 of 5 rendered pages)", len(result.Sections))
	}
	for _, s := range result.Sections {
		if strings.Contains(s.Text, "CONFIDENTIAL") {
			t.Fatalf("running header survived: %q", s.Text)
		}
	}
}

func TestApplyPDFPostProcess_RunningHeaderFooterRequiresFullZone(t *testing.T) {
	const pageHeight = 842.0
	// A section that merely starts in the top 10% but extends past it
	// (bottom 200 > 842*0.10) is body content, not a header candidate —
	// even when it repeats on every page. Same for a section that ends in
	// the bottom 10% but starts above it (top 700 < 842*0.90).
	result := &deepdoctype.ParseResult{
		PageHeight: map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight},
		Sections: []deepdoctype.Section{
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 0, 72, 210, 40, 200),
			makePDFSection("Long legal disclaimer note", "", 0, 72, 363, 700, 800),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 1, 72, 210, 40, 200),
			makePDFSection("Long legal disclaimer note", "", 1, 72, 363, 700, 800),
			makePDFSection("CONFIDENTIAL DRAFT HEADER", "", 2, 72, 210, 40, 200),
			makePDFSection("Long legal disclaimer note", "", 2, 72, 363, 700, 800),
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	if len(result.Sections) != 6 {
		t.Fatalf("len(Sections) = %d, want 6 (sections crossing the zone boundary must survive)", len(result.Sections))
	}
}

func TestApplyPDFPostProcess_RemoveTOCByOutlines(t *testing.T) {
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			makePDFSection("目录", "text", 1, 50, 550, 100, 120),
			makePDFSection("章节列表", "text", 2, 50, 550, 120, 140),
			makePDFSection("正文", "text", 3, 50, 550, 100, 120),
		},
		Outlines: []deepdoctype.Outline{
			{Title: "目录", Level: 0, PageNumber: 1},
			{Title: "第一章", Level: 0, PageNumber: 3},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeTOC: true})
	if len(result.Sections) != 1 {
		t.Fatalf("len(Sections) = %d, want 1", len(result.Sections))
	}
	if got, want := result.Sections[0].Text, "正文"; got != want {
		t.Fatalf("remaining section = %q, want %q", got, want)
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

// TestFilterPDFHeaderFooter_SubstringMatch pins #5: Python's remove_header_footer
// uses a substring match re.search(r"(header|footer|number)", ...) (rag/flow/parser/parser.py:754),
// while Go used an anchored exact match ^(header|footer|number)$. A layout type
// that merely CONTAINS one of those words (e.g. "page-footer") must be stripped to
// match Python, not silently kept.
func TestFilterPDFHeaderFooter_SubstringMatch(t *testing.T) {
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "real header", LayoutType: "header"},
			{Text: "real footer", LayoutType: "footer"},
			{Text: "page 1", LayoutType: "number"},
			{Text: "a page-footer note", LayoutType: "page-footer"}, // composite -> substring match
			{Text: "body text", LayoutType: "text"},
		},
	}
	filterPDFHeaderFooter(result)

	kept := map[string]bool{}
	for _, s := range result.Sections {
		kept[s.LayoutType] = true
	}
	for _, lt := range []string{"header", "footer", "number"} {
		if kept[lt] {
			t.Errorf("#5 header/footer: %q should be stripped", lt)
		}
	}
	// Composite "page-footer" must be stripped by substring match (Python-equivalent).
	if kept["page-footer"] {
		t.Errorf("#5 header/footer: composite layout type %q should be stripped by substring match", "page-footer")
	}
	if !kept["text"] {
		t.Errorf("#5 header/footer: body text %q should be kept", "text")
	}
}
