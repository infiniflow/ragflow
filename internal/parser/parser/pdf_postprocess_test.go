package parser

import (
	"slices"
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

func TestApplyPDFPostProcess_RemoveTOCSingleFormattedEntryKeepsBody(t *testing.T) {
	// Only the first entry after the TOC title is formatted. It must count
	// as a consumed entry so the prefix scan never runs: that scan would
	// otherwise delete every line up to the next "Cha"-prefixed heading
	// ("Chapter Two"), dropping the body text between the TOC and it.
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			{Text: "Contents"},
			{Text: "Chapter One ..... 2"},
			{Text: "Body A"},
			{Text: "Body B"},
			{Text: "Chapter Two"},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeTOC: true})
	var kept []string
	for _, s := range result.Sections {
		kept = append(kept, s.Text)
	}
	want := []string{"Body A", "Body B", "Chapter Two"}
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

func TestApplyPDFPostProcess_RunningHeaderFooterNeedsRepetitionNotPosition(t *testing.T) {
	const pageHeight = 842.0
	// Body blocks sitting fully inside the top 10% of every page but never
	// repeating: position alone must not remove them — the guard requires
	// the same normalized text on >= half of the rendered pages.
	result := &deepdoctype.ParseResult{
		PageHeight: map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight, 3: pageHeight},
		Sections: []deepdoctype.Section{
			makePDFSection("Quarterly results summary", "", 0, 72, 300, 21, 60),
			makePDFSection("Ordinary body text on page zero.", "", 0, 72, 363, 124, 136),
			makePDFSection("Annual meeting minutes", "", 1, 72, 300, 21, 60),
			makePDFSection("Ordinary body text on page one.", "", 1, 72, 363, 124, 136),
			makePDFSection("Risk assessment overview", "", 2, 72, 300, 21, 60),
			makePDFSection("Ordinary body text on page two.", "", 2, 72, 363, 124, 136),
			makePDFSection("Compliance audit findings", "", 3, 72, 300, 21, 60),
			makePDFSection("Ordinary body text on page three.", "", 3, 72, 363, 124, 136),
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	if len(result.Sections) != 8 {
		t.Fatalf("len(Sections) = %d, want 8 (in-zone but non-repeating text must survive)", len(result.Sections))
	}

	// Repetition counts distinct pages, not occurrences: the same text
	// twice on one page still covers a single page of the document.
	result = &deepdoctype.ParseResult{
		PageHeight: map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight},
		Sections: []deepdoctype.Section{
			makePDFSection("Repeated block", "", 0, 72, 300, 21, 40),
			makePDFSection("Repeated block", "", 0, 72, 300, 45, 64),
			makePDFSection("Body text page one.", "", 1, 72, 363, 124, 136),
			makePDFSection("Body text page two.", "", 2, 72, 363, 124, 136),
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeHeaderFooter: true})
	if len(result.Sections) != 4 {
		t.Fatalf("len(Sections) = %d, want 4 (two occurrences on one page count as one page)", len(result.Sections))
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

// TestApplyPDFPostProcess_RemoveTOCEntryLines covers TOC pages that carry no
// "目录"/"contents" heading: the removal must still drop the leader+page
// number entry lines while keeping regular content, and only when removeTOC
// is enabled.
func TestApplyPDFPostProcess_RemoveTOCEntryLines(t *testing.T) {
	sections := []deepdoctype.Section{
		makePDFSection("《道德经》全文及翻译", "text", 1, 50, 550, 100, 120),
		makePDFSection("木瓜树 更多的书籍免费下载 http://forum.law58.cn/?fromuid=381879", "text", 1, 50, 550, 120, 140),
		makePDFSection("前言：……………………………………………………………………1", "text", 2, 50, 550, 100, 120),
		makePDFSection("第1章 “道”…………………………………………………………3", "text", 2, 50, 550, 120, 140),
		makePDFSection("第2章 圣人居无为之事，行不言之教…………………………4", "text", 2, 50, 550, 140, 160),
		makePDFSection("第13章 以身为天下，可寄/托天下……………………………18 I", "text", 3, 50, 550, 100, 120),
		makePDFSection("第17章 太上，不知有之；功成事遂，百姓皆谓“我自然”..25", "text", 3, 50, 550, 120, 140),
		makePDFSection("Introduction to the Dao ....... 12", "text", 3, 50, 550, 140, 160),
		makePDFSection("II", "text", 3, 50, 550, 160, 180),
		makePDFSection("前言：本文是《道德经》的白话翻译，供读者参考。", "text", 4, 50, 550, 100, 120),
		makePDFSection("道可道，非常道。名可名，非常名。", "text", 4, 50, 550, 120, 140),
	}

	untouched := &deepdoctype.ParseResult{Sections: append([]deepdoctype.Section(nil), sections...)}
	applyPDFPostProcess(untouched, pdfPostProcessOptions{removeTOC: false})
	if len(untouched.Sections) != len(sections) {
		t.Fatalf("len(Sections) = %d, want %d when removeTOC is false", len(untouched.Sections), len(sections))
	}

	result := &deepdoctype.ParseResult{Sections: append([]deepdoctype.Section(nil), sections...)}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeTOC: true})
	want := []string{
		"《道德经》全文及翻译",
		"木瓜树 更多的书籍免费下载 http://forum.law58.cn/?fromuid=381879",
		"II",
		"前言：本文是《道德经》的白话翻译，供读者参考。",
		"道可道，非常道。名可名，非常名。",
	}
	if len(result.Sections) != len(want) {
		t.Fatalf("len(Sections) = %d, want %d; got %v", len(result.Sections), len(want), sectionTexts(result.Sections))
	}
	for i := range want {
		if got := result.Sections[i].Text; got != want[i] {
			t.Fatalf("Sections[%d].Text = %q, want %q", i, got, want[i])
		}
	}
}

// TestApplyPDFPostProcess_RemoveTOCEntryLinesWithPageOneOutlines covers the
// book-PDF shape: the first outline sits on page 1 and none of its titles
// names the TOC ("目录"/"contents"), so removePDFTOCByOutlines finds no page
// range and is a no-op. The leader+page-number entry filter must still run on
// this dispatch branch, otherwise a TOC without a 目录 heading survives it.
func TestApplyPDFPostProcess_RemoveTOCEntryLinesWithPageOneOutlines(t *testing.T) {
	result := &deepdoctype.ParseResult{
		Sections: []deepdoctype.Section{
			makePDFSection("《道德经》全文及翻译", "text", 1, 50, 550, 100, 120),
			makePDFSection("前言：……………………………………………………………………1", "text", 2, 50, 550, 100, 120),
			makePDFSection("第1章 “道”…………………………………………………………3", "text", 2, 50, 550, 120, 140),
			makePDFSection("Introduction to the Dao ....... 12", "text", 2, 50, 550, 140, 160),
			makePDFSection("道可道，非常道。名可名，非常名。", "text", 3, 50, 550, 100, 120),
		},
		Outlines: []deepdoctype.Outline{
			{Title: "前言", Level: 0, PageNumber: 1},
			{Title: "Chapter 1", Level: 1, PageNumber: 3},
		},
	}
	applyPDFPostProcess(result, pdfPostProcessOptions{removeTOC: true})
	want := []string{"《道德经》全文及翻译", "道可道，非常道。名可名，非常名。"}
	if !slices.Equal(sectionTexts(result.Sections), want) {
		t.Fatalf("sections = %v, want %v", sectionTexts(result.Sections), want)
	}
}

func sectionTexts(sections []deepdoctype.Section) []string {
	texts := make([]string, 0, len(sections))
	for _, s := range sections {
		texts = append(texts, s.Text)
	}
	return texts
}

// TestFilterPDFTOCEntries_MergedBlockTrailingNonEntry covers a TOC page that
// DeepDoc merges into ONE section whose trailing text is not an entry (a
// footer watermark), so the anchored entry pattern alone never fires. The
// block must still be dropped because it is dominated by leader+page-number
// runs (issue: agent workflow PDF "remove original table of contents" not
// taking effect).
func TestFilterPDFTOCEntries_MergedBlockTrailingNonEntry(t *testing.T) {
	sections := []deepdoctype.Section{
		makePDFSection("《道德经》全文及翻译", "text", 1, 50, 550, 100, 120),
		makePDFSection("前言：………………………………………………1 第1章 “道”………………………………………………3 第2章 圣人居无为之事，行不言之教……………………4 第3章 无为而治…………………………………………6 木瓜树 更多的书籍免费下载 http://forum.law58.cn/?fromuid=381879", "text", 1, 50, 550, 120, 300),
		makePDFSection("道可道，非常道。名可名，非常名。", "text", 3, 50, 550, 100, 120),
	}
	got := filterPDFTOCEntries(sections)
	want := []string{"《道德经》全文及翻译", "道可道，非常道。名可名，非常名。"}
	if !slices.Equal(sectionTexts(got), want) {
		t.Fatalf("sections = %v, want %v", sectionTexts(got), want)
	}
}

// TestFilterPDFTOCEntries_WrappedEntryTitlePairsPageRef covers wrapped TOC
// entries whose leader+page number is split onto its own line/section: the
// bare page reference drops the preceding title line, but a preceding
// non-title line (prose, watermark) is kept, and a lone leader run inside
// prose is not enough to drop anything.
func TestFilterPDFTOCEntries_WrappedEntryTitlePairsPageRef(t *testing.T) {
	sections := []deepdoctype.Section{
		makePDFSection("第27章 不贵其师，不爱其资，虽智大迷，是谓要妙", "text", 2, 50, 550, 100, 120),
		makePDFSection("…………………………………………………………………………………………39", "text", 2, 50, 550, 120, 140),
		makePDFSection("木瓜树 更多的书籍免费下载 http://forum.law58.cn/?fromuid=381879", "text", 2, 50, 550, 140, 160),
		makePDFSection("第28章 朴散则为器，圣人用之，则为官长，故大制不割？？", "text", 2, 50, 550, 160, 180),
		makePDFSection("………39 II", "text", 2, 50, 550, 180, 200),
		makePDFSection("第1章", "text", 3, 50, 550, 100, 120),
		makePDFSection("道可道，非常道。名可名，非常名。", "text", 3, 50, 550, 120, 140),
	}
	got := filterPDFTOCEntries(sections)
	want := []string{
		"木瓜树 更多的书籍免费下载 http://forum.law58.cn/?fromuid=381879",
		"第1章",
		"道可道，非常道。名可名，非常名。",
	}
	if !slices.Equal(sectionTexts(got), want) {
		t.Fatalf("sections = %v, want %v", sectionTexts(got), want)
	}
}

// TestFilterPDFTOCEntries_KeepsProseWithSingleLeaderRun pins the guard rails:
// prose containing one leader+number mention stays, and a bare page
// reference preceded by prose (no chapter/section title) keeps the prose.
func TestFilterPDFTOCEntries_KeepsProseWithSingleLeaderRun(t *testing.T) {
	sections := []deepdoctype.Section{
		makePDFSection("更多说明见第三章……12 的表格。", "text", 1, 50, 550, 100, 120),
		makePDFSection("………34", "text", 1, 50, 550, 120, 140),
		makePDFSection("天下皆知美之为美，斯恶已。", "text", 2, 50, 550, 100, 120),
	}
	got := filterPDFTOCEntries(sections)
	want := []string{
		"更多说明见第三章……12 的表格。",
		"天下皆知美之为美，斯恶已。",
	}
	if !slices.Equal(sectionTexts(got), want) {
		t.Fatalf("sections = %v, want %v", sectionTexts(got), want)
	}
}

// TestFilterPDFTOCEntries_KeepsProseEndingInWideSpaces pins that a wide-space
// run is not a TOC leader: prose ending in "  12" survives, while the same
// shape with a dot leader is a real entry and is dropped.
func TestFilterPDFTOCEntries_KeepsProseEndingInWideSpaces(t *testing.T) {
	sections := []deepdoctype.Section{
		makePDFSection("See the installation guide for details  12", "text", 1, 50, 550, 100, 120),
		makePDFSection("Troubleshooting ....... 27", "text", 2, 50, 550, 100, 120),
	}
	got := filterPDFTOCEntries(sections)
	want := []string{"See the installation guide for details  12"}
	if !slices.Equal(sectionTexts(got), want) {
		t.Fatalf("sections = %v, want %v", sectionTexts(got), want)
	}
}

// TestFilterPDFTOCEntries_TwoRunProseTradeOff pins the accepted trade-off of
// the merged-block heuristic: prose quoting two leader+number references is
// dropped, because a DeepDoc-merged TOC page has exactly that shape and the
// anchored pattern alone never fires for it. A single run keeps the prose
// (see KeepsProseWithSingleLeaderRun); raising the threshold would
// under-delete real merged TOC blocks.
func TestFilterPDFTOCEntries_TwoRunProseTradeOff(t *testing.T) {
	sections := []deepdoctype.Section{
		makePDFSection("见第三章……12 和第五章……34 的说明。", "text", 1, 50, 550, 100, 120),
	}
	got := filterPDFTOCEntries(sections)
	if len(got) != 0 {
		t.Fatalf("sections = %v, want empty: two leader+number runs read as a merged TOC block", sectionTexts(got))
	}
}

// TestFilterPDFTOCEntries_ChapterTitleWithoutBarePageRefSurvives pins the
// guard of the wrapped-entry pairing: a "第N章"/"Chapter N" line is only
// dropped when the following section is a bare leader+page-number reference
// (see WrappedEntryTitlePairsPageRef); followed by ordinary prose it is body
// content and must stay.
func TestFilterPDFTOCEntries_ChapterTitleWithoutBarePageRefSurvives(t *testing.T) {
	sections := []deepdoctype.Section{
		makePDFSection("第1章 道可道，非常道", "text", 3, 50, 550, 100, 120),
		makePDFSection("道可道，非常道。名可名，非常名。", "text", 3, 50, 550, 120, 140),
	}
	got := filterPDFTOCEntries(sections)
	want := []string{"第1章 道可道，非常道", "道可道，非常道。名可名，非常名。"}
	if !slices.Equal(sectionTexts(got), want) {
		t.Fatalf("sections = %v, want %v", sectionTexts(got), want)
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
