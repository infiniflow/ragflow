package layout

import (
	"fmt"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func tb(text string, page int, x0, x1, top, bottom float64) pdf.TextBox {
	return pdf.TextBox{
		Text:       text,
		PageNumber: page,
		X0:         x0,
		X1:         x1,
		Top:        top,
		Bottom:     bottom,
	}
}

func texts(boxes []pdf.TextBox) []string {
	out := make([]string, 0, len(boxes))
	for _, b := range boxes {
		out = append(out, b.Text)
	}
	return out
}

func equalText(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRemoveTOCBoxes_DropsDottedTOCPage: a page made of leader-dot entries is
// removed entirely, while a body page on a different page is untouched.
func TestRemoveTOCBoxes_DropsDottedTOCPage(t *testing.T) {
	boxes := []pdf.TextBox{
		// page 0: TOC with leader dots in their own boxes.
		tb("Contents", 0, 72, 150, 100, 120),
		tb("Chapter One", 0, 72, 175, 160, 175),
		tb("..........................", 0, 175, 400, 160, 175),
		tb("3", 0, 500, 520, 160, 175),
		tb("Chapter Two", 0, 72, 175, 190, 205),
		tb("..........................", 0, 175, 400, 190, 205),
		tb("4", 0, 500, 520, 190, 205),
		tb("Chapter Three", 0, 72, 180, 220, 235),
		tb("..........................", 0, 175, 400, 220, 235),
		tb("5", 0, 500, 520, 220, 235),
		// page 1: normal body with multiple prose paragraphs.
		tb("The way that can be told of is not the eternal way; the name that can be named is not the eternal name. The nameless is the origin of heaven and earth.", 1, 72, 500, 160, 180),
		tb("The named is the mother of ten thousand things. Ever desireless, one can see the mystery. Ever desiring, one can see the manifestations.", 1, 72, 500, 190, 210),
		tb("These two spring from the same source but differ in name. This appears as darkness. Darkness within darkness. The gate to all mystery.", 1, 72, 500, 220, 240),
	}
	got := RemoveTOCBoxes(boxes, nil, true)
	// Only the body paragraphs on page 1 should survive.
	want := []string{
		"The way that can be told of is not the eternal way; the name that can be named is not the eternal name. The nameless is the origin of heaven and earth.",
		"The named is the mother of ten thousand things. Ever desireless, one can see the mystery. Ever desiring, one can see the manifestations.",
		"These two spring from the same source but differ in name. This appears as darkness. Darkness within darkness. The gate to all mystery.",
	}
	if !equalText(texts(got), want) {
		t.Fatalf("got %v, want %v", texts(got), want)
	}
}

// TestRemoveTOCBoxes_KeepsBodyPage: a page without leader dots is never removed.
func TestRemoveTOCBoxes_KeepsBodyPage(t *testing.T) {
	boxes := []pdf.TextBox{
		tb("Section 1.1 Overview", 0, 72, 200, 100, 120),
		tb("This paragraph explains the background of the work in some detail so the reader can follow.", 0, 72, 500, 130, 150),
		tb("It continues with more context and supporting material for the main argument.", 0, 72, 500, 160, 180),
	}
	got := RemoveTOCBoxes(boxes, nil, true)
	if len(got) != len(boxes) {
		t.Fatalf("body page should be untouched, got %d boxes, want %d", len(got), len(boxes))
	}
}

// TestRemoveTOCBoxes_ThresholdBoundary: 2 entries (below min) keeps the page,
// 3 entries removes it.
func TestRemoveTOCBoxes_ThresholdBoundary(t *testing.T) {
	mkPage := func(n int, page int) []pdf.TextBox {
		var boxes []pdf.TextBox
		boxes = append(boxes, tb("Contents", page, 72, 150, 100, 120))
		for i := 0; i < n; i++ {
			y := float64(160 + i*30)
			boxes = append(boxes,
				tb("Item", page, 72, 120, y, y+15),
				tb("..........................", page, 175, 400, y, y+15),
				tb(string(rune('0'+i+1)), page, 500, 520, y, y+15),
			)
		}
		return boxes
	}
	below := append(mkPage(2, 0),
		tb("Body text here that is long enough to matter and exceeds sixty runes for sure absolutely yes it does.", 1, 72, 400, 160, 180),
		tb("Another paragraph of body text that is also quite long and exceeds the thirty rune threshold easily.", 1, 72, 400, 190, 210),
		tb("A third paragraph of body text that ensures the book is not classified as compact by the detector.", 1, 72, 400, 220, 240),
	)
	if got := RemoveTOCBoxes(below, nil, true); len(got) != len(below) {
		t.Fatalf("2 entries should keep page: got %d boxes, want %d", len(got), len(below))
	}
	above := append(mkPage(3, 0),
		tb("Body text here that is long enough to matter and exceeds sixty runes for sure absolutely yes it does.", 1, 72, 400, 160, 180),
		tb("Another paragraph of body text that is also quite long and exceeds the thirty rune threshold easily.", 1, 72, 400, 190, 210),
		tb("A third paragraph of body text that ensures the book is not classified as compact by the detector.", 1, 72, 400, 220, 240),
	)
	if got := RemoveTOCBoxes(above, nil, true); len(got) != 3 {
		t.Fatalf("3 entries should drop the whole TOC page: got %d boxes, want 3", len(got))
	}
}

// TestRemoveTOCBoxes_LeaderMergedIntoBox: when the leader dots are merged into
// the same box as the title+page-number, the entry is still detected.
func TestRemoveTOCBoxes_LeaderMergedIntoBox(t *testing.T) {
	boxes := []pdf.TextBox{
		tb("Contents", 0, 72, 150, 100, 120),
		tb("Chapter One .......................... 3", 0, 72, 520, 160, 175),
		tb("Chapter Two .......................... 4", 0, 72, 520, 190, 205),
		tb("Chapter Three .......................... 5", 0, 72, 520, 220, 235),
		tb("Body paragraph that must survive on page 1 and is definitely longer than thirty runes.", 1, 72, 400, 160, 180),
		tb("Another body paragraph that ensures the book has enough long boxes to not be compact.", 1, 72, 400, 190, 210),
	}
	got := RemoveTOCBoxes(boxes, nil, true)
	want := []string{
		"Body paragraph that must survive on page 1 and is definitely longer than thirty runes.",
		"Another body paragraph that ensures the book has enough long boxes to not be compact.",
	}
	if !equalText(texts(got), want) {
		t.Fatalf("got %v, want %v", texts(got), want)
	}
}

// TestRemoveTOCBoxes_ChineseChapterMarkers: Chinese "第N章" style TOC.
func TestRemoveTOCBoxes_ChineseChapterMarkers(t *testing.T) {
	boxes := []pdf.TextBox{
		tb("目录", 0, 72, 110, 100, 120),
		tb("第一章 道可道", 0, 72, 200, 160, 175),
		tb("..........................", 0, 200, 420, 160, 175),
		tb("1", 0, 500, 520, 160, 175),
		tb("第二章 天下皆知", 0, 72, 210, 190, 205),
		tb("..........................", 0, 200, 420, 190, 205),
		tb("5", 0, 500, 520, 190, 205),
		tb("第三章 不尚贤", 0, 72, 200, 220, 235),
		tb("..........................", 0, 200, 420, 220, 235),
		tb("9", 0, 500, 520, 220, 235),
		tb("正文内容，道可道，非常道。名可名，非常名。无名天地之始，有名万物之母。故常无欲以观其妙，常有欲以观其徼。此两者同出而异名，同谓之玄。玄之又玄，众妙之门。", 1, 72, 500, 160, 180),
		tb("天下皆知美之为美，斯恶已。皆知善之为善，斯不善已。故有无相生，难易相成，长短相形，高下相倾，音声相和，前后相随。", 1, 72, 500, 190, 210),
		tb("是以圣人处无为之事，行不言之教。万物作焉而不辞，生而不有，为而不恃，功成而弗居。夫唯弗居，是以不去。", 1, 72, 500, 220, 240),
	}
	got := RemoveTOCBoxes(boxes, nil, true)
	want := []string{
		"正文内容，道可道，非常道。名可名，非常名。无名天地之始，有名万物之母。故常无欲以观其妙，常有欲以观其徼。此两者同出而异名，同谓之玄。玄之又玄，众妙之门。",
		"天下皆知美之为美，斯恶已。皆知善之为善，斯不善已。故有无相生，难易相成，长短相形，高下相倾，音声相和，前后相随。",
		"是以圣人处无为之事，行不言之教。万物作焉而不辞，生而不有，为而不恃，功成而弗居。夫唯弗居，是以不去。",
	}
	if !equalText(texts(got), want) {
		t.Fatalf("got %v, want %v", texts(got), want)
	}
}

// TestRemoveTOCBoxes_BodyPageWithLongBoxIsNotTOC: a page with long prose boxes
// is never mistaken for a TOC even if it happens to contain a short dot line.
func TestRemoveTOCBoxes_BodyPageWithLongBoxIsNotTOC(t *testing.T) {
	boxes := []pdf.TextBox{
		tb("This is a long body paragraph that clearly belongs to the main content of the document and should never be classified as a table of contents entry under any reasonable heuristic.", 0, 72, 500, 100, 120),
		tb("Another sentence here to make the point clear and ensure the page is recognized as a body page.", 0, 72, 500, 130, 150),
	}
	got := RemoveTOCBoxes(boxes, nil, true)
	if len(got) != len(boxes) {
		t.Fatalf("body page with long boxes must be kept, got %d, want %d", len(got), len(boxes))
	}
}

// tocPageBoxes builds one page of dotted TOC entries: a heading plus n entries,
// each entry as its own box, which is the shape the box signal expects.
func tocPageBoxes(pg, n int) []pdf.TextBox {
	out := []pdf.TextBox{tb("目录", pg, 72, 110, 100, 120)}
	for i := 0; i < n; i++ {
		y := float64(160 + i*30)
		out = append(out,
			tb("第一章 道可道", pg, 72, 200, y, y+15),
			tb("..........................", pg, 200, 420, y, y+15),
			tb(string(rune('0'+i+1)), pg, 500, 520, y, y+15),
		)
	}
	return out
}

// bodyPageBoxes builds one page of prose, which no signal may drop.
func bodyPageBoxes(pg int) []pdf.TextBox {
	return []pdf.TextBox{
		tb("The way that can be told of is not the eternal way and the name that can be named is not the eternal name.", pg, 72, 500, 160, 180),
		tb("The named is the mother of ten thousand things and ever desireless one can see the mystery of it all here.", pg, 72, 500, 190, 210),
		tb("These two spring from the same source but differ in name and this appears as darkness within darkness.", pg, 72, 500, 220, 240),
	}
}

func countPage(boxes []pdf.TextBox, pg int) int {
	n := 0
	for _, b := range boxes {
		if b.PageNumber == pg {
			n++
		}
	}
	return n
}

// TestTOCPageRangeFromOutlines pins the 1-based to 0-based conversion: pdfium
// reports outline pages 1-based while TextBox.PageNumber is 0-based, and the
// section-level detector this replaces compared the two directly.
func TestTOCPageRangeFromOutlines(t *testing.T) {
	toc := TOCPageRangeFromOutlines([]pdf.Outline{
		{Title: "目录", Level: 0, PageNumber: 1},
		{Title: "第一章", Level: 0, PageNumber: 4},
	})
	if len(toc) != 3 || !toc[0] || !toc[1] || !toc[2] || toc[3] {
		t.Fatalf("outline pages 1-3 must map to 0-based {0,1,2} and stop before the content page, got %v", toc)
	}
	if got := TOCPageRangeFromOutlines(nil); got != nil {
		t.Fatalf("no outlines must yield nil, got %v", got)
	}
	if got := TOCPageRangeFromOutlines([]pdf.Outline{{Title: "第一章", Level: 0, PageNumber: 2}}); got != nil {
		t.Fatalf("a first bookmark that is a chapter names no TOC page, got %v", got)
	}
	// No following entry at the same level: only the TOC page itself is known.
	if got := TOCPageRangeFromOutlines([]pdf.Outline{{Title: "Contents", Level: 0, PageNumber: 2}}); len(got) != 1 || !got[1] {
		t.Fatalf("a lone TOC bookmark should claim printed page 2 only, got %v", got)
	}
	// The "@@" anchor suffix pdfium appends must not defeat the title match.
	if got := TOCPageRangeFromOutlines([]pdf.Outline{
		{Title: "Contents@@5", Level: 0, PageNumber: 1},
		{Title: "Chapter One", Level: 0, PageNumber: 2},
	}); len(got) != 1 || !got[0] {
		t.Fatalf("an anchored TOC title should claim 0-based page 0, got %v", got)
	}
}

// TestTOCPageRangeFromOutlines_AcknowledgementsDoesNotStart: only an entry that
// names the TOC starts a range. A book with an acknowledgements bookmark and no
// contents bookmark used to have its acknowledgements page claimed and dropped.
func TestTOCPageRangeFromOutlines_AcknowledgementsDoesNotStart(t *testing.T) {
	cases := []struct {
		name string
		in   []pdf.Outline
	}{
		{"chinese, followed by back matter", []pdf.Outline{
			{Title: "致谢", Level: 0, PageNumber: 5},
			{Title: "参考文献", Level: 0, PageNumber: 8},
		}},
		{"chinese, alone", []pdf.Outline{
			{Title: "致谢", Level: 0, PageNumber: 5},
		}},
		{"english plural", []pdf.Outline{
			{Title: "Acknowledgements", Level: 0, PageNumber: 5},
			{Title: "References", Level: 0, PageNumber: 8},
		}},
	}
	for _, c := range cases {
		if got := TOCPageRangeFromOutlines(c.in); got != nil {
			t.Errorf("%s: an acknowledgements bookmark must not open a TOC range, got %v", c.name, got)
		}
	}
}

// TestTOCPageRangeFromOutlines_EndMarkerAbandonsRange: a front/back-matter
// heading sitting next to the TOC does not say where the entries stop, so the
// range is abandoned rather than extended to whatever entry follows it.
func TestTOCPageRangeFromOutlines_EndMarkerAbandonsRange(t *testing.T) {
	// Before: 致谢 was skipped and the range ran to 参考文献, claiming printed
	// pages 2-11 including the acknowledgements themselves.
	if got := TOCPageRangeFromOutlines([]pdf.Outline{
		{Title: "目录", Level: 0, PageNumber: 2},
		{Title: "致谢", Level: 0, PageNumber: 10},
		{Title: "参考文献", Level: 0, PageNumber: 12},
	}); len(got) != 1 || !got[1] {
		t.Fatalf("the acknowledgements entry must abandon the range, claiming printed page 2 only, got %v", got)
	}
	// The English heading now matches at all; before, it was treated as an
	// ordinary boundary and claimed printed pages 2-9.
	if got := TOCPageRangeFromOutlines([]pdf.Outline{
		{Title: "Contents", Level: 0, PageNumber: 2},
		{Title: "Acknowledgements", Level: 0, PageNumber: 10},
	}); len(got) != 1 || !got[1] {
		t.Fatalf("the english end marker must abandon the range too, got %v", got)
	}
	// A chapter entry is still a boundary: the range stops just before it.
	if got := TOCPageRangeFromOutlines([]pdf.Outline{
		{Title: "目录", Level: 0, PageNumber: 2},
		{Title: "第一章", Level: 0, PageNumber: 5},
		{Title: "致谢", Level: 0, PageNumber: 20},
	}); len(got) != 3 || !got[1] || !got[2] || !got[3] || got[4] {
		t.Fatalf("a chapter entry must still close the range at printed page 5, got %v", got)
	}
}

// TestRemoveTOCBoxes_OutlineCoversLeaderlessMergedBlock: a TOC whose entries
// were merged into one box with no leader runs left is invisible to the box
// signal — neither short boxes nor leader runs survive — and is covered by the
// outline signal alone.
func TestRemoveTOCBoxes_OutlineCoversLeaderlessMergedBlock(t *testing.T) {
	merged := "目录 第一章 道可道非常道名可名非常名 3 第二章 天下皆知美之为美斯恶已皆知善 7 第三章 不尚贤使民不争不贵难得之货 15 第四章 道冲而用之或不盈渊兮似万物之宗 21"
	boxes := []pdf.TextBox{
		tb("目录", 0, 72, 110, 100, 120),
		tb(merged, 0, 72, 520, 160, 200),
	}
	boxes = append(boxes, bodyPageBoxes(1)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	if got := RemoveTOCBoxes(boxes, nil, true); len(got) != len(boxes) {
		t.Fatalf("box shape alone cannot see a leaderless merged TOC block, got %d of %d", len(got), len(boxes))
	}
	got := RemoveTOCBoxes(boxes, map[int]bool{0: true}, true)
	if n := countPage(got, 0); n != 0 {
		t.Fatalf("an outline-selected page must be dropped, %d boxes survived", n)
	}
}

// TestRemoveTOCBoxes_MergedBlockWithLeaders: DeepDoc joins the lines of a text
// block into one box, so a TOC page can arrive as a single long box. Its leader
// runs are the only evidence left, and the page must still be dropped without
// any bookmark.
func TestRemoveTOCBoxes_MergedBlockWithLeaders(t *testing.T) {
	merged := "目录 第一章 道可道非常道名可名非常名 ..........3 第二章 天下皆知美之为美斯恶已皆知善 ..........7 第三章 不尚贤使民不争不贵难得之货 ..........15"
	boxes := []pdf.TextBox{
		tb("目录", 0, 72, 110, 100, 120),
		tb(merged, 0, 72, 520, 160, 200),
	}
	boxes = append(boxes, bodyPageBoxes(1)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	got := RemoveTOCBoxes(boxes, nil, true)
	if n := countPage(got, 0); n != 0 {
		t.Fatalf("a merged TOC block carrying leader runs must be dropped, %d boxes survived", n)
	}
	if n := countPage(got, 1); n != 3 {
		t.Fatalf("the body page must be kept, %d of 3 boxes", n)
	}
}

// TestRemoveTOCBoxes_MergedBlockNeedsThreeRuns: two leader runs in a long box
// are not enough. A paragraph can pick up the odd "……12", so the threshold sits
// above that rather than at the first run.
func TestRemoveTOCBoxes_MergedBlockNeedsThreeRuns(t *testing.T) {
	merged := "目录 第一章 道可道非常道名可名非常名 ..........3 第二章 天下皆知美之为美斯恶已皆知善 ..........7 第三章 不尚贤使民不争不贵难得之货"
	boxes := []pdf.TextBox{
		tb("目录", 0, 72, 110, 100, 120),
		tb(merged, 0, 72, 520, 160, 200),
	}
	boxes = append(boxes, bodyPageBoxes(1)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	if got := RemoveTOCBoxes(boxes, nil, true); len(got) != len(boxes) {
		t.Fatalf("two leader runs are below the merged-block threshold, got %d of %d", len(got), len(boxes))
	}
}

// TestRemoveTOCBoxes_MergedBlockFullWidthPageNumbers: the leader-run count uses
// the same digit class as pageNumberPattern, so a CJK TOC numbering its entries
// in full-width digits is still recognised.
func TestRemoveTOCBoxes_MergedBlockFullWidthPageNumbers(t *testing.T) {
	merged := "目录 第一章 道可道非常道名可名非常名 …………３ 第二章 天下皆知美之为美斯恶已皆知善 …………７ 第三章 不尚贤使民不争不贵难得之货 …………１５"
	boxes := []pdf.TextBox{
		tb("目录", 0, 72, 110, 100, 120),
		tb(merged, 0, 72, 520, 160, 200),
	}
	boxes = append(boxes, bodyPageBoxes(1)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	got := RemoveTOCBoxes(boxes, nil, true)
	if n := countPage(got, 0); n != 0 {
		t.Fatalf("full-width page numbers in a merged TOC block must be counted, %d boxes survived", n)
	}
}

// TestRemoveTOCBoxes_MergedBlockRespectsProseGuard: a page that carries body
// text is kept even when one of its boxes is a merged TOC block. The merged
// signal is the one that could plausibly be talked into deleting a prose page,
// so the guard is asserted against it rather than against the short-box shape.
func TestRemoveTOCBoxes_MergedBlockRespectsProseGuard(t *testing.T) {
	merged := "目录 第一章 道可道 ..........3 第二章 天下皆知美之为美 ..........7 第三章 不尚贤使民不争 ..........15"
	boxes := []pdf.TextBox{
		tb(merged, 0, 72, 520, 100, 140),
		tb("This is a long body paragraph that certainly belongs to the main content of the document and would be lost.", 0, 72, 520, 160, 180),
		tb("This is a second long body paragraph that certainly belongs to the main content of the document as well.", 0, 72, 520, 190, 210),
		tb("This is a third long body paragraph that certainly belongs to the main content of the document too.", 0, 72, 520, 220, 240),
	}
	boxes = append(boxes, bodyPageBoxes(1)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	got := RemoveTOCBoxes(boxes, nil, true)
	if n := countPage(got, 0); n != 4 {
		t.Fatalf("a page carrying prose must never be dropped, %d of 4 boxes survived", n)
	}
}

// TestRemoveTOCBoxes_OutlineRespectsProseGuard: a stale or malformed bookmark
// that names a content page must not delete body text, while the box signal
// still drops the real TOC page.
func TestRemoveTOCBoxes_OutlineRespectsProseGuard(t *testing.T) {
	boxes := tocPageBoxes(0, 3)
	boxes = append(boxes, bodyPageBoxes(1)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	got := RemoveTOCBoxes(boxes, map[int]bool{1: true}, true)
	if n := countPage(got, 1); n != 3 {
		t.Fatalf("a page carrying prose must never be dropped, %d of 3 boxes survived", n)
	}
	if n := countPage(got, 0); n != 0 {
		t.Fatalf("the box signal must still drop the TOC page, %d boxes survived", n)
	}
}

// TestRemoveTOCBoxes_KeepsTOCOnlyDocument: a source file that is nothing but a
// TOC page keeps it rather than being emptied.
func TestRemoveTOCBoxes_KeepsTOCOnlyDocument(t *testing.T) {
	boxes := tocPageBoxes(0, 3)
	if got := RemoveTOCBoxes(boxes, nil, true); len(got) != len(boxes) {
		t.Fatalf("a document consisting only of a TOC must be left untouched, got %d of %d", len(got), len(boxes))
	}
}

// TestRemoveTOCBoxes_MidDocumentParseSkipsShapeSignal: when the parse covers a
// later page range, the first parsed page is not a document prefix. Reading it
// as one deleted content from the middle of the book.
func TestRemoveTOCBoxes_MidDocumentParseSkipsShapeSignal(t *testing.T) {
	var boxes []pdf.TextBox
	boxes = append(boxes, tocPageBoxes(10, 3)...)
	boxes = append(boxes, bodyPageBoxes(11)...)

	if got := RemoveTOCBoxes(boxes, nil, false); len(got) != len(boxes) {
		t.Fatalf("a page range starting mid-document must not be read as a TOC prefix, dropped %d of %d",
			len(boxes)-len(got), len(boxes))
	}
	if got := RemoveTOCBoxes(boxes, nil, true); len(got) == len(boxes) {
		t.Fatalf("the same boxes as a document prefix must still drop the TOC page")
	}
	// The outline signal carries absolute page numbers, so the gate must not
	// silence it.
	got := RemoveTOCBoxes(boxes, map[int]bool{10: true}, false)
	if n := countPage(got, 10); n != 0 {
		t.Fatalf("an outline-selected page must be dropped even mid-document, %d boxes survived", n)
	}
	if n := countPage(got, 11); n != 3 {
		t.Fatalf("the body page must survive, %d of 3 boxes", n)
	}
}

// TestRemoveTOCBoxes_MultiPageTOC: one TOC spanning consecutive pages is dropped
// as a whole, not just its first page.
func TestRemoveTOCBoxes_MultiPageTOC(t *testing.T) {
	boxes := tocPageBoxes(0, 3)
	boxes = append(boxes, tocPageBoxes(1, 3)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	got := RemoveTOCBoxes(boxes, nil, true)
	if n := countPage(got, 0); n != 0 {
		t.Fatalf("TOC page 0 must be dropped, %d boxes survived", n)
	}
	if n := countPage(got, 1); n != 0 {
		t.Fatalf("TOC page 1 must be dropped, %d boxes survived", n)
	}
	if n := countPage(got, 2); n != 3 {
		t.Fatalf("body page 2 must survive, %d of 3 boxes", n)
	}
}

// TestRemoveTOCBoxes_LongTOCRun: the run length follows the page shapes, not a
// fixed cap. A real TOC can be longer than any constant small enough to be a
// useful bound, and truncating the run leaks the remaining TOC pages into the
// output — the tail page of a six-page TOC was kept while the cap was five.
func TestRemoveTOCBoxes_LongTOCRun(t *testing.T) {
	const tocPages = 6
	var boxes []pdf.TextBox
	for pg := 0; pg < tocPages; pg++ {
		boxes = append(boxes, tocPageBoxes(pg, 3)...)
	}
	boxes = append(boxes, bodyPageBoxes(tocPages)...)

	got := RemoveTOCBoxes(boxes, nil, true)
	for pg := 0; pg < tocPages; pg++ {
		if n := countPage(got, pg); n != 0 {
			t.Fatalf("TOC page %d must be dropped, %d boxes survived", pg, n)
		}
	}
	if n := countPage(got, tocPages); n != 3 {
		t.Fatalf("the body page after the TOC must survive, %d of 3 boxes", n)
	}
}

// TestRemoveTOCBoxes_TOCBehindCoverPage: a cover page carrying several short
// boxes must not close the candidate window before the TOC is reached.
func TestRemoveTOCBoxes_TOCBehindCoverPage(t *testing.T) {
	cover := []pdf.TextBox{
		tb("The Analects", 0, 150, 400, 200, 230),
		tb("Translated by Someone", 0, 150, 400, 260, 280),
		tb("Second Edition", 0, 150, 400, 300, 320),
		tb("Some Press, 2024", 0, 150, 400, 340, 360),
	}
	boxes := append(cover, tocPageBoxes(1, 3)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	got := RemoveTOCBoxes(boxes, nil, true)
	if n := countPage(got, 0); n != 4 {
		t.Fatalf("the cover page must be kept, %d of 4 boxes", n)
	}
	if n := countPage(got, 1); n != 0 {
		t.Fatalf("the TOC behind the cover must be dropped, %d boxes survived", n)
	}
	if n := countPage(got, 2); n != 3 {
		t.Fatalf("body page 2 must survive, %d of 3 boxes", n)
	}
}

// TestRemoveTOCBoxes_StopsAtProsePage: widening the candidate window must not
// reach past a page of body text. A TOC-shaped page behind one is kept.
func TestRemoveTOCBoxes_StopsAtProsePage(t *testing.T) {
	boxes := []pdf.TextBox{tb("The Analects", 0, 150, 400, 200, 230)}
	boxes = append(boxes, bodyPageBoxes(1)...)
	boxes = append(boxes, tocPageBoxes(2, 3)...)
	boxes = append(boxes, bodyPageBoxes(3)...)

	got := RemoveTOCBoxes(boxes, nil, true)
	if n := countPage(got, 2); n != 10 {
		t.Fatalf("a TOC-shaped page behind a body page must be kept, %d of 10 boxes", n)
	}
}

// TestRemoveTOCBoxes_BareCJKChapterTitles pins the shape reported in #19744: a
// CJK TOC whose entries are bare chapter titles, with no leader dots and no page
// numbers. The section-level prefix scan under-deleted these entries; the box
// signal drops the page as a whole.
func TestRemoveTOCBoxes_BareCJKChapterTitles(t *testing.T) {
	boxes := []pdf.TextBox{
		tb("目录", 0, 72, 110, 100, 120),
		tb("第一章 绪论", 0, 72, 200, 160, 175),
		tb("第二章 背景", 0, 72, 200, 190, 205),
		tb("第三章 方法", 0, 72, 200, 220, 235),
		tb("第四章 结论", 0, 72, 200, 250, 265),
	}
	boxes = append(boxes, tb("第一章 绪论", 1, 72, 200, 160, 175))
	boxes = append(boxes, bodyPageBoxes(1)...)

	got := RemoveTOCBoxes(boxes, nil, true)
	if n := countPage(got, 0); n != 0 {
		t.Fatalf("the bare-title TOC page must be dropped, %d boxes survived", n)
	}
	if n := countPage(got, 1); n != 4 {
		t.Fatalf("the body page must be kept, %d of 4 boxes", n)
	}
}

func TestRemoveTOCText_Normalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello   World", "hello world"},
		{"- 12 -", "- # -"},
		{"Chapter 1: Intro", "chapter #: intro"},
		{"第 １ 页", "第 # 页"},
	}
	for _, c := range cases {
		if got := normalizeRunningText(c.in); got != c.want {
			t.Fatalf("normalizeRunningText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestRemoveHeaderFooterBoxes_DropsRepeatedHeader: identical header box on every
// page is removed; body is kept.
func TestRemoveHeaderFooterBoxes_DropsRepeatedHeader(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	boxes := []pdf.TextBox{
		// identical header in top 10% on every page
		tb("THE WAY OF GO", 0, 72, 200, 30, 45),
		tb("Body text on page zero that is long enough.", 0, 72, 400, 160, 180),
		tb("THE WAY OF GO", 1, 72, 200, 30, 45),
		tb("Body text on page one that is long enough.", 1, 72, 400, 160, 180),
		tb("THE WAY OF GO", 2, 72, 200, 30, 45),
		tb("Body text on page two that is long enough.", 2, 72, 400, 160, 180),
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	var headers, bodies int
	for _, b := range got {
		if strings.Contains(b.Text, "THE WAY") {
			headers++
		} else {
			bodies++
		}
	}
	if headers != 0 {
		t.Fatalf("expected all repeated headers removed, got %d", headers)
	}
	if bodies != 3 {
		t.Fatalf("expected 3 body boxes kept, got %d", bodies)
	}
}

// TestRemoveHeaderFooterBoxes_KeepsUniqueZoneText: per-page unique headers are
// kept (the repetition guard must not delete genuine content).
func TestRemoveHeaderFooterBoxes_KeepsUniqueZoneText(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	boxes := []pdf.TextBox{
		tb("Chapter One: Getting Started", 0, 72, 250, 30, 45),
		tb("Body zero.", 0, 72, 400, 160, 180),
		tb("Chapter Two: Types and Values", 1, 72, 250, 30, 45),
		tb("Body one.", 1, 72, 400, 160, 180),
		tb("Chapter Three: Concurrency", 2, 72, 260, 30, 45),
		tb("Body two.", 2, 72, 400, 160, 180),
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	if len(got) != len(boxes) {
		t.Fatalf("unique per-page headers must be kept, got %d, want %d", len(got), len(boxes))
	}
}

// TestRemoveHeaderFooterBoxes_PageCountGuard: documents with < 3 pages are not
// touched.
func TestRemoveHeaderFooterBoxes_PageCountGuard(t *testing.T) {
	heights := map[int]float64{0: 842, 1: 842}
	boxes := []pdf.TextBox{
		tb("Repeated", 0, 72, 150, 30, 45),
		tb("Body zero.", 0, 72, 400, 160, 180),
		tb("Repeated", 1, 72, 150, 30, 45),
		tb("Body one.", 1, 72, 400, 160, 180),
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	if len(got) != len(boxes) {
		t.Fatalf("short documents must be untouched, got %d, want %d", len(got), len(boxes))
	}
}

// TestRemoveHeaderFooterBoxes_SkipsNonTextLayout: a table/figure box in the
// header zone must not be treated as a running header.
func TestRemoveHeaderFooterBoxes_SkipsNonTextLayout(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	var boxes []pdf.TextBox
	for pg := 0; pg < 3; pg++ {
		boxes = append(boxes,
			// Same text, same zone, but a table: never a running header.
			pdf.TextBox{Text: "Running head", PageNumber: pg, X0: 72, X1: 200, Top: 30, Bottom: 45, LayoutType: "table"},
			// Positive control in the same zone: this one must go.
			tb("Running head", pg, 280, 400, 30, 45),
			tb("Body text.", pg, 72, 400, 160, 180),
		)
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)

	var tableKept, textKept int
	for _, b := range got {
		switch {
		case b.LayoutType == "table":
			tableKept++
		case b.Text == "Running head":
			textKept++
		}
	}
	if textKept != 0 {
		t.Fatalf("the repeated text header must be removed, %d survived", textKept)
	}
	if tableKept != 3 {
		t.Fatalf("table boxes in the header zone must be kept, %d of 3 survived", tableKept)
	}
}

// TestRemoveHeaderFooterBoxes_FullWidthPageNumbers: full-width page numbers get
// a distinct key per page unless they are masked like ASCII digits, so the
// footer never reaches minPages and survives every page.
func TestRemoveHeaderFooterBoxes_FullWidthPageNumbers(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	var boxes []pdf.TextBox
	for pg := 0; pg < 3; pg++ {
		boxes = append(boxes,
			tb("正文内容。", pg, 72, 400, 160, 180),
			tb("第 "+string(rune('０'+pg+1))+" 页", pg, 280, 340, 820, 835),
		)
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if b.Top >= 800 {
			t.Fatalf("full-width page-number footer %q must be removed", b.Text)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected only the 3 body boxes, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_HalfOfOddPageCount: "at least half the pages" has
// to round up on an odd page count. Rounding down would let 2 pages of a
// five-page document pass as half, and this repetition rule is the only guard
// keeping genuine content out of the drop set.
func TestRemoveHeaderFooterBoxes_HalfOfOddPageCount(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight, 3: pageHeight, 4: pageHeight}

	// 2 of 5 pages is below half: too rare to be a running header.
	below := []pdf.TextBox{
		tb("THE WAY OF GO", 0, 72, 200, 30, 45),
		tb("Body zero.", 0, 72, 400, 160, 180),
		tb("Body one.", 1, 72, 400, 160, 180),
		tb("THE WAY OF GO", 2, 72, 200, 30, 45),
		tb("Body two.", 2, 72, 400, 160, 180),
		tb("Body three.", 3, 72, 400, 160, 180),
		tb("Body four.", 4, 72, 400, 160, 180),
	}
	if got := RemoveHeaderFooterBoxes(below, heights); len(got) != len(below) {
		t.Fatalf("2 of 5 pages is not half, the boxes must be kept: got %d, want %d", len(got), len(below))
	}

	// 3 of 5 pages is half: a running header.
	above := []pdf.TextBox{
		tb("THE WAY OF GO", 0, 72, 200, 30, 45),
		tb("Body zero.", 0, 72, 400, 160, 180),
		tb("Body one.", 1, 72, 400, 160, 180),
		tb("THE WAY OF GO", 2, 72, 200, 30, 45),
		tb("Body two.", 2, 72, 400, 160, 180),
		tb("THE WAY OF GO", 3, 72, 200, 30, 45),
		tb("Body three.", 3, 72, 400, 160, 180),
		tb("Body four.", 4, 72, 400, 160, 180),
	}
	got := RemoveHeaderFooterBoxes(above, heights)
	if len(got) != 5 {
		t.Fatalf("3 of 5 pages is half, the headers must be dropped: got %d boxes, want 5", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_DropsPageNumberFooter: identical "- N -" footers
// (digit-masked to the same key) are removed.
func TestRemoveHeaderFooterBoxes_DropsPageNumberFooter(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	boxes := []pdf.TextBox{
		tb("Body zero.", 0, 72, 400, 160, 180),
		tb("- 1 -", 0, 280, 320, 820, 835),
		tb("Body one.", 1, 72, 400, 160, 180),
		tb("- 2 -", 1, 280, 320, 820, 835),
		tb("Body two.", 2, 72, 400, 160, 180),
		tb("- 3 -", 2, 280, 320, 820, 835),
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "- ") && strings.Contains(b.Text, " -") {
			t.Fatalf("footer %q should have been removed", b.Text)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected only the 3 body boxes, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_AlternatingEvenOdd verifies bilateral alternating
// headers where even pages have the book title and odd pages have the chapter title.
func TestRemoveHeaderFooterBoxes_AlternatingEvenOdd(t *testing.T) {
	pageHeight := 842.0
	heights := make(map[int]float64, 8)
	var boxes []pdf.TextBox
	for pg := 0; pg < 8; pg++ {
		heights[pg] = pageHeight
		if pg%2 == 0 {
			boxes = append(boxes, tb("DEEP LEARNING BOOK", pg, 72, 250, 30, 45))
		} else {
			boxes = append(boxes, tb("CHAPTER THREE: CONVOLUTION", pg, 250, 500, 30, 45))
		}
		boxes = append(boxes, tb("Body text on this page that represents substantial content.", pg, 72, 450, 160, 180))
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "DEEP LEARNING") || strings.Contains(b.Text, "CHAPTER THREE") {
			t.Fatalf("alternating header %q should have been removed", b.Text)
		}
	}
	if len(got) != 8 {
		t.Fatalf("expected 8 body boxes remaining, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_ChapterVaryingConsecutiveRun verifies that chapter-varying
// running headers appearing on consecutive pages with stable Y coordinates are removed.
func TestRemoveHeaderFooterBoxes_ChapterVaryingConsecutiveRun(t *testing.T) {
	pageHeight := 842.0
	heights := make(map[int]float64, 10)
	for pg := 0; pg < 10; pg++ {
		heights[pg] = pageHeight
	}

	var boxes []pdf.TextBox
	// Pages 2, 3, 4: Chapter 1 (only 3 of 10 pages, 30% < 50%)
	for pg := 2; pg <= 4; pg++ {
		boxes = append(boxes, tb("Chapter 1: Overview", pg, 72, 220, 35.0, 48.0))
		boxes = append(boxes, tb("Body paragraph for chapter 1.", pg, 72, 450, 160, 180))
	}
	// Pages 5, 6, 7: Chapter 2 (only 3 of 10 pages, 30% < 50%)
	for pg := 5; pg <= 7; pg++ {
		boxes = append(boxes, tb("Chapter 2: Methods", pg, 72, 220, 35.1, 48.1))
		boxes = append(boxes, tb("Body paragraph for chapter 2.", pg, 72, 450, 160, 180))
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "Chapter 1:") || strings.Contains(b.Text, "Chapter 2:") {
			t.Fatalf("chapter header %q should have been removed by locality run", b.Text)
		}
	}
	if len(got) != 6 {
		t.Fatalf("expected 6 body boxes remaining, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_WhitespaceGapExpandedZone verifies that a header sitting
// beyond the base 10% ratio (at 11.3%) is removed when separated from body text by >= 18pt gap.
func TestRemoveHeaderFooterBoxes_WhitespaceGapExpandedZone(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight, 3: pageHeight}
	var boxes []pdf.TextBox
	for pg := 0; pg < 4; pg++ {
		// Bottom at 95.0 on 842.0 is 11.28% (> 10%)
		boxes = append(boxes, tb("COMPANY CONFIDENTIAL HEADER", pg, 72, 300, 40, 95))
		// Body starts at 135.0 (Gap = 135 - 95 = 40pt >= 18pt)
		boxes = append(boxes, tb("First line of body text sitting comfortably below the gap.", pg, 72, 450, 135, 155))
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "COMPANY CONFIDENTIAL") {
			t.Fatalf("expanded zone header %q should have been removed", b.Text)
		}
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 body boxes, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_WhitespaceGapProtectsTightBody verifies that text at 11%
// without a whitespace gap to content below is NOT removed as a header.
func TestRemoveHeaderFooterBoxes_WhitespaceGapProtectsTightBody(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight, 3: pageHeight}
	var boxes []pdf.TextBox
	for pg := 0; pg < 4; pg++ {
		// Top at 75, Bottom at 92 (10.9% > 10%)
		boxes = append(boxes, tb("Section Heading At Top", pg, 72, 250, 75, 92))
		// Content right below at Top: 98 (Gap = 98 - 92 = 6pt < 18pt)
		boxes = append(boxes, tb("Tight paragraph line directly following heading.", pg, 72, 450, 98, 115))
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	var headings int
	for _, b := range got {
		if strings.Contains(b.Text, "Section Heading") {
			headings++
		}
	}
	if headings != 4 {
		t.Fatalf("tight heading must be preserved by whitespace gap protection, got %d of 4", headings)
	}
}

// TestRemoveHeaderFooterBoxes_ShortDocumentPageNumbers verifies that formatted page
// numbers are removed even on 1- or 2-page documents.
func TestRemoveHeaderFooterBoxes_ShortDocumentPageNumbers(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight}
	boxes := []pdf.TextBox{
		tb("Invoice Body Page 1", 0, 72, 400, 160, 180),
		tb("Page 1 of 2", 0, 250, 350, 810, 825),
		tb("Invoice Body Page 2", 1, 72, 400, 160, 180),
		tb("Page 2 of 2", 1, 250, 350, 810, 825),
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "of 2") {
			t.Fatalf("page number %q must be removed on 2-page doc", b.Text)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected only the 2 body boxes, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_DLASemanticTagDirectDrop verifies that boxes explicitly
// tagged as LayoutType header/footer in margin zones are dropped immediately.
func TestRemoveHeaderFooterBoxes_DLASemanticTagDirectDrop(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight}
	boxes := []pdf.TextBox{
		{Text: "Header Tagged By DLA", PageNumber: 0, X0: 72, X1: 300, Top: 30, Bottom: 45, LayoutType: "header"},
		tb("Body text here.", 0, 72, 400, 160, 180),
		{Text: "Footer Tagged By DLA", PageNumber: 0, X0: 72, X1: 300, Top: 820, Bottom: 835, LayoutType: "footer"},
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	if len(got) != 1 || got[0].Text != "Body text here." {
		t.Fatalf("DLA tagged header and footer must be dropped, got %v", got)
	}
}

// TestRemoveHeaderFooterBoxes_LayoutTypeTitleAllowed verifies that a running header
// misclassified by DLA as LayoutTypeTitle is still processed and removed.
func TestRemoveHeaderFooterBoxes_LayoutTypeTitleAllowed(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	boxes := []pdf.TextBox{
		{Text: "PROCEEDINGS OF ACM", PageNumber: 0, X0: 72, X1: 300, Top: 30, Bottom: 45, LayoutType: "title"},
		tb("Body on page 0.", 0, 72, 400, 160, 180),
		{Text: "PROCEEDINGS OF ACM", PageNumber: 1, X0: 72, X1: 300, Top: 30, Bottom: 45, LayoutType: "title"},
		tb("Body on page 1.", 1, 72, 400, 160, 180),
		{Text: "PROCEEDINGS OF ACM", PageNumber: 2, X0: 72, X1: 300, Top: 30, Bottom: 45, LayoutType: "title"},
		tb("Body on page 2.", 2, 72, 400, 160, 180),
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "PROCEEDINGS") {
			t.Fatalf("header %q with LayoutType 'title' should have been removed", b.Text)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 body boxes, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_RomanNumerals verifies roman numeral page numbers.
func TestRemoveHeaderFooterBoxes_RomanNumerals(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	boxes := []pdf.TextBox{
		tb("Preface body zero.", 0, 72, 400, 160, 180),
		tb("- iv -", 0, 280, 320, 820, 835),
		tb("Preface body one.", 1, 72, 400, 160, 180),
		tb("- v -", 1, 280, 320, 820, 835),
		tb("Preface body two.", 2, 72, 400, 160, 180),
		tb("- vi -", 2, 280, 320, 820, 835),
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "- ") {
			t.Fatalf("roman numeral footer %q should have been removed", b.Text)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected only 3 body boxes, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_ChineseNumerals verifies Chinese page numbers.
func TestRemoveHeaderFooterBoxes_ChineseNumerals(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	boxes := []pdf.TextBox{
		tb("正文第一页。", 0, 72, 400, 160, 180),
		tb("第 一 页", 0, 280, 350, 820, 835),
		tb("正文第二页。", 1, 72, 400, 160, 180),
		tb("第 二 页", 1, 280, 350, 820, 835),
		tb("正文第三页。", 2, 72, 400, 160, 180),
		tb("第 三 页", 2, 280, 350, 820, 835),
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.HasPrefix(b.Text, "第 ") {
			t.Fatalf("Chinese page number footer %q should have been removed", b.Text)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected only 3 body boxes, got %d", len(got))
	}
}

// TestStrictDecoratedPagePattern_ChineseSlashGong tests Chinese page-number formats
// with "/共", "共", and comma separators.
func TestStrictDecoratedPagePattern_ChineseSlashGong(t *testing.T) {
	cases := []string{
		"第1页/共10页",
		"第 1 页 / 共 10 页",
		"第1页/共10",
		"第1页 共10页",
		"第1页/10页",
		"第1页,共10页",
		"第1页，共10页",
		"第一页/共十页",
		"第一页 共十页",
		"第 1 页",
		"第一页",
	}
	for _, c := range cases {
		if !strictDecoratedPagePattern.MatchString(c) {
			t.Errorf("expected %q to match strictDecoratedPagePattern, but did not", c)
		}
	}
}

// TestRemoveHeaderFooterBoxes_ChineseSlashGong verifies that "第1页/共10页" is removed.
func TestRemoveHeaderFooterBoxes_ChineseSlashGong(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	boxes := []pdf.TextBox{
		tb("正文第一页内容。", 0, 72, 400, 160, 180),
		tb("第1页/共10页", 0, 280, 380, 820, 835),
		tb("正文第二页内容。", 1, 72, 400, 160, 180),
		tb("第2页/共10页", 1, 280, 380, 820, 835),
		tb("正文第三页内容。", 2, 72, 400, 160, 180),
		tb("第3页/共10页", 2, 280, 380, 820, 835),
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "共10页") {
			t.Fatalf("Chinese page footer %q should have been removed", b.Text)
		}
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 body boxes, got %d", len(got))
	}
}

// TestFindStableConsecutiveRunIndices_SlidingWindow tests sliding window behavior in
// findStableConsecutiveRunIndices, specifically verifying that initial outliers do
// not block subsequent qualifying runs.
func TestFindStableConsecutiveRunIndices_SlidingWindow(t *testing.T) {
	// Case 1: Outlier at page 1 (top 30), stable run at pages 2, 3, 4 (top 40)
	metas1 := []boxMeta{
		{page: 1, top: 30},
		{page: 2, top: 40},
		{page: 3, top: 40},
		{page: 4, top: 40},
	}
	if len(findStableConsecutiveRunIndices(metas1, 3, 4.0)) == 0 {
		t.Errorf("expected pages 2..4 stable run despite page 1 outlier")
	}

	// Case 2: Stable run at pages 1, 2, 3 (top 40), outlier at page 4 (top 30)
	metas2 := []boxMeta{
		{page: 1, top: 40},
		{page: 2, top: 40},
		{page: 3, top: 40},
		{page: 4, top: 30},
	}
	if len(findStableConsecutiveRunIndices(metas2, 3, 4.0)) == 0 {
		t.Errorf("expected pages 1..3 stable run")
	}

	// Case 3: Divergent tops across all pages (no 3 consecutive pages within 4.0)
	metas3 := []boxMeta{
		{page: 1, top: 10},
		{page: 2, top: 30},
		{page: 3, top: 50},
		{page: 4, top: 70},
	}
	if len(findStableConsecutiveRunIndices(metas3, 3, 4.0)) != 0 {
		t.Errorf("expected no run for divergent tops")
	}

	// Case 4: Page gap breaks consecutive run
	metas4 := []boxMeta{
		{page: 1, top: 40},
		{page: 2, top: 40},
		{page: 4, top: 40},
		{page: 5, top: 40},
	}
	if len(findStableConsecutiveRunIndices(metas4, 3, 4.0)) != 0 {
		t.Errorf("expected no run when no consecutive run reaches minRun=3")
	}

	// Case 5: Run after a page gap meets minRun=3
	metas5 := []boxMeta{
		{page: 1, top: 40},
		{page: 2, top: 40},
		{page: 4, top: 40},
		{page: 5, top: 40},
		{page: 6, top: 40},
	}
	if len(findStableConsecutiveRunIndices(metas5, 3, 4.0)) == 0 {
		t.Errorf("expected pages 4..6 run after page gap")
	}
}

// TestRemoveHeaderFooterBoxes_LocalRunPreservesDistantIsolatedBox verifies that a local
// consecutive run (pages 1, 2, 3) removes those running headers while preserving a distant,
// isolated occurrence of the exact same text on page 10.
func TestRemoveHeaderFooterBoxes_LocalRunPreservesDistantIsolatedBox(t *testing.T) {
	pageHeight := 842.0
	heights := make(map[int]float64, 12)
	for pg := 0; pg < 12; pg++ {
		heights[pg] = pageHeight
	}

	var boxes []pdf.TextBox
	// Running chapter header on consecutive pages 1, 2, 3 (top = 35)
	for pg := 1; pg <= 3; pg++ {
		boxes = append(boxes, tb("Chapter 1: Foundations", pg, 72, 250, 35, 48))
		boxes = append(boxes, tb(fmt.Sprintf("Body text on page %d.", pg), pg, 72, 450, 160, 180))
	}
	// Pages 4..9 have other body text
	for pg := 4; pg <= 9; pg++ {
		boxes = append(boxes, tb(fmt.Sprintf("Regular body on page %d.", pg), pg, 72, 450, 160, 180))
	}
	// Page 10 has an isolated text box in the margin with the SAME text "Chapter 1: Foundations",
	// but it is NOT part of a consecutive run (isolated single occurrence).
	isolatedBox := tb("Chapter 1: Foundations", 10, 72, 250, 35, 48)
	boxes = append(boxes, isolatedBox)
	boxes = append(boxes, tb("Body text on page 10 referencing chapter 1.", 10, 72, 450, 160, 180))

	got := RemoveHeaderFooterBoxes(boxes, heights)

	// Pages 1, 2, 3 headers must be removed:
	for _, b := range got {
		if (b.PageNumber >= 1 && b.PageNumber <= 3) && strings.Contains(b.Text, "Chapter 1: Foundations") {
			t.Errorf("running header on page %d should have been removed", b.PageNumber)
		}
	}

	// Page 10 isolated box must be PRESERVED:
	foundIsolated := false
	for _, b := range got {
		if b.PageNumber == 10 && b.Text == "Chapter 1: Foundations" {
			foundIsolated = true
			break
		}
	}
	if !foundIsolated {
		t.Errorf("distant isolated box on page 10 should be preserved, but was dropped!")
	}
}

// TestRemoveHeaderFooterBoxes_BareFooterPageNumberWithGapDropped verifies that a bare
// Arabic page number in the footer zone with clear whitespace above is cleanly removed.
func TestRemoveHeaderFooterBoxes_BareFooterPageNumberWithGapDropped(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight}
	boxes := []pdf.TextBox{
		tb("Page 0 body content.", 0, 72, 400, 160, 200),
		tb("1", 0, 290, 310, 810, 825), // gapAbove = 810 - 200 = 610pt >> 18pt
		tb("Page 1 body content.", 1, 72, 400, 160, 200),
		tb("2", 1, 290, 310, 810, 825),
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if b.Text == "1" || b.Text == "2" {
			t.Fatalf("bare page number %q with clear whitespace above must be dropped", b.Text)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 body boxes remaining, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_FootnoteTightNumberPreserved verifies that a tight bare
// number in the bottom 10% (e.g. footnote index with gapAbove < 18pt) is protected.
func TestRemoveHeaderFooterBoxes_FootnoteTightNumberPreserved(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight}
	boxes := []pdf.TextBox{
		tb("Main body paragraph here.", 0, 72, 400, 100, 200),
		tb("Footnote explanation text at bottom.", 0, 72, 400, 790, 804), // bottom = 804
		tb("1", 0, 72, 85, 810, 822),                                     // top = 810, gapAbove = 6pt < 18pt!
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	foundFootnoteNum := false
	for _, b := range got {
		if b.Text == "1" {
			foundFootnoteNum = true
			break
		}
	}
	if !foundFootnoteNum {
		t.Fatalf("tight footnote number '1' (gapAbove < 18pt) must be preserved, but was dropped!")
	}
	if len(got) != 3 {
		t.Fatalf("expected all 3 boxes to be kept, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_FooterYearPreservedOnShortDocument verifies that a 4-digit
// year (e.g. "2024") in the footer margin on a 2-page document is NOT misidentified as a page number.
func TestRemoveHeaderFooterBoxes_FooterYearPreservedOnShortDocument(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight}
	boxes := []pdf.TextBox{
		tb("Legal Agreement Page 1", 0, 72, 400, 160, 200),
		tb("2024", 0, 280, 320, 810, 825), // gapAbove >> 18pt, but value 2024 > 20
		tb("Legal Agreement Page 2", 1, 72, 400, 160, 200),
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	foundYear := false
	for _, b := range got {
		if b.Text == "2024" {
			foundYear = true
			break
		}
	}
	if !foundYear {
		t.Fatalf("footer year '2024' (value > maxAllowed) must be preserved, but was dropped!")
	}
	if len(got) != 3 {
		t.Fatalf("expected all 3 boxes to be kept, got %d", len(got))
	}
}

// tightFooter builds one page for the sequence-track tests: a body line ending
// at bottom, a bare-number box 5pt under it at Top=755 (below the 757.8 footer
// line, inside the 0.86 expanded band without its gap) — geometry the zone gate
// rejects, leaving the +1 sequence as the only evidence.
func tightFooter(pg int, numText string) []pdf.TextBox {
	return []pdf.TextBox{
		tb(fmt.Sprintf("Body paragraph on page %d with a little extra length to stay unique.", pg), pg, 72, 500, 100, 750),
		tb(numText, pg, 400, 420, 755, 766),
	}
}

// TestRemoveHeaderFooterBoxes_TightSequenceFootersRemoved verifies that bare page
// numbers sitting tight under body text — rejected by both the base footer line
// and the expanded band's whitespace requirement — are removed by the sequence
// track when they step by one across consecutive pages.
func TestRemoveHeaderFooterBoxes_TightSequenceFootersRemoved(t *testing.T) {
	pageHeight := 842.0
	t.Run("arabic", func(t *testing.T) {
		heights := make(map[int]float64, 10)
		var boxes []pdf.TextBox
		for pg := 0; pg < 10; pg++ {
			heights[pg] = pageHeight
			boxes = append(boxes, tightFooter(pg, fmt.Sprintf("%d", pg+1))...)
		}
		got := RemoveHeaderFooterBoxes(boxes, heights)
		for _, b := range got {
			if _, ok := parseBareNumberValue(b.Text); ok {
				t.Fatalf("page number %q survived the sequence track", b.Text)
			}
		}
		if len(got) != 10 {
			t.Fatalf("expected 10 body boxes, got %d", len(got))
		}
	})
	t.Run("roman", func(t *testing.T) {
		heights := make(map[int]float64, 6)
		boxes := []pdf.TextBox{}
		romans := []string{"I", "II", "III", "IV", "V", "VI"}
		for pg, r := range romans {
			heights[pg] = pageHeight
			boxes = append(boxes, tightFooter(pg, r)...)
		}
		got := RemoveHeaderFooterBoxes(boxes, heights)
		for _, b := range got {
			if _, ok := parseBareNumberValue(b.Text); ok {
				t.Fatalf("Roman page number %q survived the sequence track", b.Text)
			}
		}
		if len(got) != 6 {
			t.Fatalf("expected 6 body boxes, got %d", len(got))
		}
	})
}

// TestRemoveHeaderFooterBoxes_NumbersNoSequencePreserved verifies the sequence
// track's guards: numbers that do not step by one, and runs shorter than 3
// consecutive pages, are left alone.
func TestRemoveHeaderFooterBoxes_NumbersNoSequencePreserved(t *testing.T) {
	pageHeight := 842.0
	t.Run("non_stepping", func(t *testing.T) {
		heights := make(map[int]float64, 5)
		var boxes []pdf.TextBox
		nums := []string{"3", "7", "12", "15", "19"}
		for pg, n := range nums {
			heights[pg] = pageHeight
			boxes = append(boxes, tightFooter(pg, n)...)
		}
		got := RemoveHeaderFooterBoxes(boxes, heights)
		if len(got) != 10 {
			t.Fatalf("non-stepping numbers must be preserved, got %d kept boxes", len(got))
		}
	})
	t.Run("run_too_short", func(t *testing.T) {
		// +1 steps exist (1,2 and 4,5) but each chain is broken before length 3.
		heights := make(map[int]float64, 6)
		var boxes []pdf.TextBox
		for pg := 0; pg < 6; pg++ {
			heights[pg] = pageHeight
		}
		boxes = append(boxes, tightFooter(0, "1")...)
		boxes = append(boxes, tightFooter(1, "2")...)
		boxes = append(boxes, tightFooter(3, "4")...)
		boxes = append(boxes, tightFooter(4, "5")...)
		boxes = append(boxes, tb("Plain body line, no number box on this page.", 2, 72, 500, 100, 750))
		boxes = append(boxes, tb("Plain body line here too.", 5, 72, 500, 100, 750))
		got := RemoveHeaderFooterBoxes(boxes, heights)
		if len(got) != 10 {
			t.Fatalf("2-page number chains must be preserved, got %d kept boxes", len(got))
		}
	})
}

// TestRemoveHeaderFooterBoxes_SequenceDriftSpanGuard verifies the sequence
// track bounds TOTAL Y span across a chain, not just each hop: per-hop drift
// that accumulates past seqMaxDy must not remove the run, while a chain whose
// full span stays inside the threshold still does.
func TestRemoveHeaderFooterBoxes_SequenceDriftSpanGuard(t *testing.T) {
	pageHeight := 842.0
	footerAt := func(pg int, numText string, top float64) []pdf.TextBox {
		return []pdf.TextBox{
			tb(fmt.Sprintf("Body paragraph on page %d with a little extra length to stay unique.", pg), pg, 72, 500, 100, 674),
			tb(numText, pg, 400, 420, top, top+11), // Top 680-ish: wide band, zone stays empty (gap 6pt, above 724.1 line only)
		}
	}
	t.Run("accumulating_drift_preserved", func(t *testing.T) {
		heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
		var boxes []pdf.TextBox
		tops := []float64{680, 684, 688} // each hop 4pt, total span 8pt > seqMaxDy
		for pg, top := range tops {
			boxes = append(boxes, footerAt(pg, fmt.Sprintf("%d", pg+1), top)...)
		}
		got := RemoveHeaderFooterBoxes(boxes, heights)
		if len(got) != 6 {
			t.Fatalf("chain drifting 8pt in total must be preserved, got %d kept boxes", len(got))
		}
	})
	t.Run("within_span_removed", func(t *testing.T) {
		heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
		var boxes []pdf.TextBox
		tops := []float64{680, 681.5, 683} // total span 3pt <= seqMaxDy
		for pg, top := range tops {
			boxes = append(boxes, footerAt(pg, fmt.Sprintf("%d", pg+1), top)...)
		}
		got := RemoveHeaderFooterBoxes(boxes, heights)
		for _, b := range got {
			if _, ok := parseBareNumberValue(b.Text); ok {
				t.Fatalf("page number %q survived the sequence track", b.Text)
			}
		}
		if len(got) != 3 {
			t.Fatalf("chain within total-drift span must be removed, got %d kept boxes", len(got))
		}
	})
}

// TestRemoveHeaderFooterBoxes_YearFooterSequenceCeilingPreserved verifies that a
// stepping year sequence in the footer band ("2024","2025","2026") is protected
// by the page-count ceiling even though it would otherwise satisfy the +1 chain.
func TestRemoveHeaderFooterBoxes_YearFooterSequenceCeilingPreserved(t *testing.T) {
	pageHeight := 842.0
	heights := make(map[int]float64, 3)
	var boxes []pdf.TextBox
	years := []string{"2024", "2025", "2026"}
	for pg, y := range years {
		heights[pg] = pageHeight
		boxes = append(boxes, tightFooter(pg, y)...)
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	keptYears := make(map[string]bool, len(years))
	for _, b := range got {
		keptYears[b.Text] = true
	}
	for _, y := range years {
		if !keptYears[y] {
			t.Fatalf("year footer %q must be preserved: its value exceeds the page-number ceiling", y)
		}
	}
	if len(got) != 6 {
		t.Fatalf("year footers exceed the page-number ceiling and must be preserved, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_TightBottomTextHeadingPreserved verifies the wide
// sequence bands never admit non-numeric text: a repeated section heading 6pt
// above the page bottom must survive even though its text repeats on every page.
func TestRemoveHeaderFooterBoxes_TightBottomTextHeadingPreserved(t *testing.T) {
	pageHeight := 842.0
	heights := make(map[int]float64, 4)
	var boxes []pdf.TextBox
	for pg := 0; pg < 4; pg++ {
		heights[pg] = pageHeight
		boxes = append(boxes, tb("Body text filling most of the page above the heading.", pg, 72, 500, 100, 724))
		boxes = append(boxes, tb("Quarterly Report Summary", pg, 72, 300, 730, 742)) // gapAbove = 6pt, repeats identically
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	var headings int
	for _, b := range got {
		if b.Text == "Quarterly Report Summary" {
			headings++
		}
	}
	if headings != 4 || len(got) != 8 {
		t.Fatalf("tight repeated text headings must be preserved, got %d of 4 headings (%d boxes)", headings, len(got))
	}
}

// TestRemoveHeaderFooterBoxes_SitePromoVariantInHeaderRemoved verifies that margin
// lines advertising a download site are removed even when the wrapper text varies
// page to page and no recurrence track can see them.
func TestRemoveHeaderFooterBoxes_SitePromoVariantInHeaderRemoved(t *testing.T) {
	pageHeight := 842.0
	heights := make(map[int]float64, 4)
	var boxes []pdf.TextBox
	for pg := 0; pg < 4; pg++ {
		heights[pg] = pageHeight
		// The body is written out per page; only the promo lines matter.
		boxes = append(boxes, tb(fmt.Sprintf("Substantive body content of page %d goes here.", pg), pg, 72, 450, 160, 700))
		if pg < 2 {
			boxes = append(boxes, tb("更多的书籍免费下载 http://forum.law58.cn/?fromuid=381879", pg, 60, 300, 40, 52))
		} else {
			boxes = append(boxes, tb("欢迎访问！http://forum.law58.cn/?fromuid=381879", pg, 60, 300, 40, 52))
		}
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "law58") {
			t.Fatalf("site promo %q survived despite varying wrappers", b.Text)
		}
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 body boxes, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_BodyUrlKept verifies the site-promo rule cannot
// reach into the body: a mid-page URL box and a long prose line containing a
// URL are both kept.
func TestRemoveHeaderFooterBoxes_BodyUrlKept(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight}
	boxes := []pdf.TextBox{
		// Mid-page short URL: inside no band at all.
		tb("http://a.cn/x", 0, 72, 130, 400, 415),
		// > 60 runes: length-capped out even though it starts in the top band.
		tb("See http://example.com/very/long/path/for/the/annual/errata/annex which continues well beyond sixty runes.", 1, 72, 500, 40, 55),
		tb("Ordinary body text.", 1, 72, 300, 500, 520),
	}
	got := RemoveHeaderFooterBoxes(boxes, heights)
	if len(got) != 3 {
		t.Fatalf("body URLs must be preserved, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_DropsPromoCompanion verifies that a fixed slogan
// riding in the same margin band as a dropped site-promo ad is learned from the
// pages where they co-occur and propagated to every page, so the ad is gone
// even where its URL line was never detected.
func TestRemoveHeaderFooterBoxes_DropsPromoCompanion(t *testing.T) {
	pageHeight := 842.0 // header band: bottom <= 117.9; body starts well below.
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight, 3: pageHeight}
	const slogan = "木瓜树  更多的书籍免费下载"
	var boxes []pdf.TextBox
	for pg := 0; pg < 3; pg++ { // pages 0,1,2: ad URL + slogan companion
		boxes = append(boxes,
			tb("http://forum.law58.cn/?fromuid=381879", pg, 60, 300, 40, 52),
			tb(slogan, pg, 60, 300, 60, 72),
			tb(fmt.Sprintf("Substantive body content of page %d.", pg), pg, 72, 450, 200, 700),
		)
	}
	// page 3 carries only the slogan line, no URL.
	boxes = append(boxes,
		tb(slogan, 3, 60, 300, 60, 72),
		tb("Substantive body content of page 3.", 3, 72, 450, 200, 700),
	)

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if strings.Contains(b.Text, "law58") {
			t.Fatalf("site promo survived: %q", b.Text)
		}
		if strings.Contains(b.Text, "木瓜树") {
			t.Fatalf("promo companion slogan survived: %q", b.Text)
		}
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 body boxes, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_PromoCompanionKeepsTitle verifies a LayoutType
// "title" line that merely shares the header band with an ad is never harvested
// as a companion, so legitimate running section titles survive.
func TestRemoveHeaderFooterBoxes_PromoCompanionKeepsTitle(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	titles := []string{"[图甲]", "[图乙]", "[图丙]"} // unique per page: no recurrence to lean on
	var boxes []pdf.TextBox
	for pg := 0; pg < 3; pg++ {
		boxes = append(boxes,
			tb("http://forum.law58.cn/?fromuid=381879", pg, 60, 300, 40, 52),
			pdf.TextBox{Text: titles[pg], PageNumber: pg, X0: 60, X1: 200, Top: 60, Bottom: 72, LayoutType: "title"},
			tb(fmt.Sprintf("Body content of page %d here.", pg), pg, 72, 450, 200, 700),
		)
	}

	got := RemoveHeaderFooterBoxes(boxes, heights)
	seenTitle := 0
	for _, b := range got {
		if strings.Contains(b.Text, "law58") {
			t.Fatalf("site promo survived: %q", b.Text)
		}
		if strings.HasPrefix(b.Text, "[图") {
			seenTitle++
		}
	}
	if seenTitle != 3 {
		t.Fatalf("all 3 title lines must survive as companions excluded, got %d", seenTitle)
	}
	if len(got) != 6 {
		t.Fatalf("expected 3 titles + 3 bodies, got %d", len(got))
	}
}

// TestRemoveHeaderFooterBoxes_PromoCompanionKeepsRarePeer verifies a short
// non-title band peer that co-occurs with the ad on only one page is not
// harvested — the page-count floor stops one-off neighbours.
func TestRemoveHeaderFooterBoxes_PromoCompanionKeepsRarePeer(t *testing.T) {
	pageHeight := 842.0
	heights := map[int]float64{0: pageHeight, 1: pageHeight, 2: pageHeight}
	const slogan = "木瓜树  更多的书籍免费下载"
	var boxes []pdf.TextBox
	for pg := 0; pg < 3; pg++ {
		boxes = append(boxes,
			tb("http://forum.law58.cn/?fromuid=381879", pg, 60, 300, 40, 52),
			tb(slogan, pg, 60, 300, 60, 72),
			tb(fmt.Sprintf("Body content of page %d here.", pg), pg, 72, 450, 200, 700),
		)
	}
	// A short slogan-like peer present only on page 0, next to its ad.
	boxes = append(boxes, tb("扫码加入读书群", 0, 60, 300, 80, 92))

	got := RemoveHeaderFooterBoxes(boxes, heights)
	var keptRare int
	for _, b := range got {
		if strings.Contains(b.Text, "law58") || strings.Contains(b.Text, "木瓜树") {
			t.Fatalf("promo/slogan survived: %q", b.Text)
		}
		if b.Text == "扫码加入读书群" {
			keptRare++
		}
	}
	if keptRare != 1 {
		t.Fatalf("rare single-page peer must survive, got %d", keptRare)
	}
	if len(got) != 4 {
		t.Fatalf("expected 3 bodies + 1 rare peer, got %d", len(got))
	}
}

// seriesPages builds the trusted, gap-separated page-number footers for the
// series-fit tests: value = page - 5, sitting at the base footer line with a
// body box far above so Tier 1 drops them (and the fit counts them).
func seriesPages(heights map[int]float64) []pdf.TextBox {
	var boxes []pdf.TextBox
	for pg := 5; pg <= 9; pg++ {
		heights[pg] = 842.0
		boxes = append(boxes,
			tb(fmt.Sprintf("Body on page %d ends high up.", pg), pg, 72, 500, 100, 690),
			tb(fmt.Sprintf("%d", pg-5), pg, 400, 420, 770, 782), // value=page-5, base footer zone
		)
	}
	return boxes
}

// TestRemoveHeaderFooterBoxes_DropsSeriesPageNumberLeak verifies that once the
// document's page numbers fit value = page - offset, an isolated number sitting
// tight in the wide band (zone rejected, no +1 run) is dropped against the fit.
func TestRemoveHeaderFooterBoxes_DropsSeriesPageNumberLeak(t *testing.T) {
	heights := map[int]float64{}
	boxes := seriesPages(heights)
	// page 10 tight number, value 5 (=10-5): body ends 6pt above, below base line.
	heights[10] = 842.0
	boxes = append(boxes,
		tb("Body on page 10 runs low.", 10, 72, 500, 100, 674),
		tb("5", 10, 400, 420, 680, 692),
	)

	got := RemoveHeaderFooterBoxes(boxes, heights)
	for _, b := range got {
		if b.PageNumber == 10 && b.Text == "5" {
			t.Fatalf("tight series page number leaked through")
		}
	}
}

// TestRemoveHeaderFooterBoxes_SeriesProtectsOffSeriesNumber verifies the fit is
// conservative: a tight band number that breaks value = page - offset survives,
// and with too few samples the rule never arms.
func TestRemoveHeaderFooterBoxes_SeriesProtectsOffSeriesNumber(t *testing.T) {
	t.Run("off_series", func(t *testing.T) {
		heights := map[int]float64{}
		boxes := seriesPages(heights)
		heights[10] = 842.0
		boxes = append(boxes,
			tb("Body on page 10 runs low.", 10, 72, 500, 100, 674),
			tb("9", 10, 400, 420, 680, 692), // 9 != 10-5
		)
		got := RemoveHeaderFooterBoxes(boxes, heights)
		found := false
		for _, b := range got {
			if b.PageNumber == 10 && b.Text == "9" {
				found = true
			}
		}
		if !found {
			t.Fatalf("off-series tight number must be preserved")
		}
	})
	t.Run("too_few_samples", func(t *testing.T) {
		// Only pages 5,6,7 (3 samples < minSeriesSamples=5): the tight number
		// on page 8 matching page-5 must NOT be dropped.
		heights := map[int]float64{}
		var boxes []pdf.TextBox
		for pg := 5; pg <= 7; pg++ {
			heights[pg] = 842.0
			boxes = append(boxes,
				tb(fmt.Sprintf("Body on page %d.", pg), pg, 72, 500, 100, 690),
				tb(fmt.Sprintf("%d", pg-5), pg, 400, 420, 770, 782),
			)
		}
		heights[8] = 842.0
		boxes = append(boxes,
			tb("Body on page 8 runs low.", 8, 72, 500, 100, 674),
			tb("3", 8, 400, 420, 680, 692),
		)
		got := RemoveHeaderFooterBoxes(boxes, heights)
		found := false
		for _, b := range got {
			if b.PageNumber == 8 && b.Text == "3" {
				found = true
			}
		}
		if !found {
			t.Fatalf("with too few samples the series rule must stay off")
		}
	})
}
