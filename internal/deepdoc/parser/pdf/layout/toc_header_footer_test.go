package layout

import (
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
	got := RemoveTOCBoxes(boxes, nil)
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
	got := RemoveTOCBoxes(boxes, nil)
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
	if got := RemoveTOCBoxes(below, nil); len(got) != len(below) {
		t.Fatalf("2 entries should keep page: got %d boxes, want %d", len(got), len(below))
	}
	above := append(mkPage(3, 0),
		tb("Body text here that is long enough to matter and exceeds sixty runes for sure absolutely yes it does.", 1, 72, 400, 160, 180),
		tb("Another paragraph of body text that is also quite long and exceeds the thirty rune threshold easily.", 1, 72, 400, 190, 210),
		tb("A third paragraph of body text that ensures the book is not classified as compact by the detector.", 1, 72, 400, 220, 240),
	)
	if got := RemoveTOCBoxes(above, nil); len(got) != 3 {
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
	got := RemoveTOCBoxes(boxes, nil)
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
	got := RemoveTOCBoxes(boxes, nil)
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
	got := RemoveTOCBoxes(boxes, nil)
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

	if got := RemoveTOCBoxes(boxes, nil); len(got) != len(boxes) {
		t.Fatalf("box shape alone cannot see a leaderless merged TOC block, got %d of %d", len(got), len(boxes))
	}
	got := RemoveTOCBoxes(boxes, map[int]bool{0: true})
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

	got := RemoveTOCBoxes(boxes, nil)
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

	if got := RemoveTOCBoxes(boxes, nil); len(got) != len(boxes) {
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

	got := RemoveTOCBoxes(boxes, nil)
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

	got := RemoveTOCBoxes(boxes, nil)
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

	got := RemoveTOCBoxes(boxes, map[int]bool{1: true})
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
	if got := RemoveTOCBoxes(boxes, nil); len(got) != len(boxes) {
		t.Fatalf("a document consisting only of a TOC must be left untouched, got %d of %d", len(got), len(boxes))
	}
}

// TestRemoveTOCBoxes_MultiPageTOC: one TOC spanning consecutive pages is dropped
// as a whole, not just its first page.
func TestRemoveTOCBoxes_MultiPageTOC(t *testing.T) {
	boxes := tocPageBoxes(0, 3)
	boxes = append(boxes, tocPageBoxes(1, 3)...)
	boxes = append(boxes, bodyPageBoxes(2)...)

	got := RemoveTOCBoxes(boxes, nil)
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

	got := RemoveTOCBoxes(boxes, nil)
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

	got := RemoveTOCBoxes(boxes, nil)
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

	got := RemoveTOCBoxes(boxes, nil)
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
