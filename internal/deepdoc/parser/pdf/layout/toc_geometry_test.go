package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// medianH is the per-page median box height used by the contiguity rules in
// FilterTOCBoxes; these tests use a single page with 12pt-tall lines.
var medianH = map[int]float64{0: 12}

func texts(boxes []pdf.TextBox) []string {
	out := make([]string, 0, len(boxes))
	for _, b := range boxes {
		out = append(out, b.Text)
	}
	return out
}

func contains(out []pdf.TextBox, want string) bool {
	for _, b := range out {
		if b.Text == want {
			return true
		}
	}
	return false
}

// TestFilterTOCBoxes_RealColumnSplitPage replicates the observed DeepDoc
// layout of 道德经.pdf page 1: the TOC is extracted as THREE independent
// columns — chapter markers (第N章), chapter titles, and a right-aligned
// page-number column whose entries carry only 0-2 leader dots ("6", ".11",
// "..8"). The current regex path survives all of this; the geometric filter
// must drop the whole page because every non-entry line is a short isolated
// run.
func TestFilterTOCBoxes_RealColumnSplitPage(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 61, 199, 42, 54, "木瓜树  更多的书籍免费下载"),
		makeBox(0, 199, 351, 80, 92, "《道德经》全文及翻译"),
		makeBox(0, 109, 141, 121, 133, "前言："),
	}
	// left column: chapter markers
	leftTop := []float64{160, 197, 234, 272, 308, 346, 384}
	for i, top := range leftTop {
		boxes = append(boxes, makeBox(0, 110, 149, top, top+12, "第"+string(rune('1'+i))+"章"))
	}
	// left column: entries whose title is embedded
	embedded := []struct {
		top  float64
		text string
	}{
		{422, "第8章  夫唯不争，故无尤."},
		{459, "第9章  功成身退，天之道也"},
		{495, "第10章  长而不宰，是谓玄德."},
		{534, "第11章  有之以为利，无之以为用"},
		{571, "第12章"},
		{609, "第13章  以身为天下，可寄/托天下."},
	}
	for _, e := range embedded {
		boxes = append(boxes, makeBox(0, 111, 300, e.top, e.top+12, e.text))
	}
	// middle column: chapter titles
	middle := []struct {
		top  float64
		text string
	}{
		{159, "“道”"},
		{197, "圣人居无为之事，行不言之教."},
		{234, "无为而治."},
		{272, "道冲.."},
		{308, "“守中”"},
		{346, "“谷神”"},
		{383, "后其身而身先，外其身而身存."},
		{572, "为腹不为目,故去彼取此"},
	}
	for _, m := range middle {
		boxes = append(boxes, makeBox(0, 155, 316, m.top, m.top+12, m.text))
	}
	// right column: bare page references, right-aligned
	refs := []struct {
		top  float64
		text string
	}{
		{239, "6"}, {276, "6"}, {348, "..8"}, {386, "10"}, {424, ".11"},
		{460, ".13"}, {499, ".14"}, {535, ".15"}, {573, ".17"}, {611, ".18"},
	}
	for _, r := range refs {
		boxes = append(boxes, makeBox(0, 434, 447, r.top, r.top+12, r.text))
	}

	out := FilterTOCBoxes(boxes, medianH)
	if len(out) != 0 {
		t.Fatalf("column-split TOC page: expected whole-page removal, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_DedicatedInlineEntryPage covers the classic inline
// shape (title+leader+page on one line). The page also carries a watermark,
// cover title and a roman page marker — all short isolated lines — so the
// whole page is dropped.
func TestFilterTOCBoxes_DedicatedInlineEntryPage(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 61, 199, 40, 52, "木瓜树  更多的书籍免费下载"),
		makeBox(0, 199, 351, 70, 82, "《道德经》全文及翻译"),
		makeBox(0, 50, 550, 120, 140, "第1章 道可道…………3"),
		makeBox(0, 50, 550, 160, 180, "Introduction ....... 12"),
		makeBox(0, 50, 550, 200, 220, "Chapter Three ...... 45"),
		makeBox(0, 440, 448, 240, 252, "II"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	if len(out) != 0 {
		t.Fatalf("dedicated inline-entry TOC page: expected whole-page removal, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_LeaderVariants pins that detection depends on the
// page-number column, not on the leader character. Every line below is a
// valid entry despite leaders that the regex path rejects (single dot, bare
// number, spaced dots, single ellipsis, fullwidth dot, U+2025).
func TestFilterTOCBoxes_LeaderVariants(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 61, 199, 40, 52, "木瓜树  更多的书籍免费下载"),
		makeBox(0, 199, 351, 70, 82, "《道德经》全文及翻译"),
		makeBox(0, 50, 550, 100, 120, "6"),
		makeBox(0, 50, 550, 140, 160, "..8"),
		makeBox(0, 50, 550, 180, 200, "前言：.11"),
		makeBox(0, 50, 550, 220, 240, "第1章 “道”...14"),
		makeBox(0, 50, 550, 260, 280, "第2章 圣人居无为之事，行不言之教..17"),
		makeBox(0, 50, 550, 300, 320, "第3章 无为而治。.20"),
		makeBox(0, 440, 448, 340, 352, "II"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	if len(out) != 0 {
		t.Fatalf("leader-variant TOC page: expected whole-page removal, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_MixedPageKeepsProse protects a page where a continuous
// body paragraph shares the page with an aligned page-number column. The
// entries are dropped; the paragraph (whose every single line is short but
// whose continuous run exceeds 50 runes) and the watermark survive.
func TestFilterTOCBoxes_MixedPageKeepsProse(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 61, 199, 40, 52, "木瓜树  更多的书籍免费下载"),
		makeBox(0, 434, 447, 100, 112, ".20"),
		makeBox(0, 434, 447, 140, 152, ".22"),
		makeBox(0, 434, 447, 180, 192, ".25"),
		makeBox(0, 60, 500, 300, 312, "这是前言的第一行文字内容，用来模拟正文段落。"),
		makeBox(0, 60, 500, 320, 332, "第二行继续，依然是一段连续的正文文字内容。"),
		makeBox(0, 60, 500, 340, 352, "第三行收尾，这一段累计字数一定超过五十字限制。"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	for _, keep := range []string{
		"木瓜树  更多的书籍免费下载",
		"这是前言的第一行文字内容，用来模拟正文段落。",
		"第二行继续，依然是一段连续的正文文字内容。",
		"第三行收尾，这一段累计字数一定超过五十字限制。",
	} {
		if !contains(out, keep) {
			t.Errorf("mixed page: expected to keep %q, got %v", keep, texts(out))
		}
	}
	for _, drop := range []string{".20", ".22", ".25"} {
		if contains(out, drop) {
			t.Errorf("mixed page: expected to drop %q, got %v", drop, texts(out))
		}
	}
}

// TestFilterTOCBoxes_WrappedEntryArbitraryTitle deletes a wrapped entry whose
// title line ("朴素贝叶斯分类") sits directly above its bare page reference —
// a pairing the character-regex path cannot make because the title is not a
// 第N章/Chapter-style heading. Prose on the page is kept.
func TestFilterTOCBoxes_WrappedEntryArbitraryTitle(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 434, 447, 100, 112, ".20"),
		makeBox(0, 434, 447, 140, 152, ".22"),
		makeBox(0, 440, 460, 160, 172, "朴素贝叶斯分类"),
		makeBox(0, 440, 447, 180, 192, "……39"),
		makeBox(0, 60, 500, 300, 312, "这是前言的第一行文字内容，用来模拟正文段落。"),
		makeBox(0, 60, 500, 320, 332, "第二行继续，依然是一段连续的正文文字内容。"),
		makeBox(0, 60, 500, 340, 352, "第三行收尾，这一段累计字数一定超过五十字限制。"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	if contains(out, "朴素贝叶斯分类") {
		t.Errorf("wrapped title must be dropped with its bare page ref, got %v", texts(out))
	}
	if contains(out, "……39") || contains(out, ".20") || contains(out, ".22") {
		t.Errorf("bare page refs must be dropped, got %v", texts(out))
	}
	if !contains(out, "这是前言的第一行文字内容，用来模拟正文段落。") {
		t.Errorf("prose must survive, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_TableExcluded: rows of a detected table end in
// right-aligned numbers but must never be treated as TOC entries.
func TestFilterTOCBoxes_TableExcluded(t *testing.T) {
	var boxes []pdf.TextBox
	for i, top := range []float64{100, 140, 180, 220, 260} {
		boxes = append(boxes, pdf.TextBox{
			X0: 100, X1: 447, Top: top, Bottom: top + 12,
			Text:       "第" + string(rune('A'+i)) + "项 " + string(rune('1'+i)) + "2",
			PageNumber: 0, LayoutType: pdf.LayoutTypeTable,
		})
	}
	// only two non-table aligned lines: below the cluster minimum
	boxes = append(boxes,
		makeBox(0, 434, 447, 300, 312, ".30"),
		makeBox(0, 434, 447, 340, 352, ".35"),
	)
	out := FilterTOCBoxes(boxes, medianH)
	if len(out) != len(boxes) {
		t.Fatalf("table rows and sub-threshold lines must survive, got %d of %d: %v", len(out), len(boxes), texts(out))
	}
}

// TestFilterTOCBoxes_BelowClusterMinimum: two aligned entries prove nothing.
func TestFilterTOCBoxes_BelowClusterMinimum(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 434, 447, 100, 112, ".20"),
		makeBox(0, 434, 447, 140, 152, ".22"),
		makeBox(0, 60, 500, 300, 312, "正文段落文字内容。"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	if len(out) != len(boxes) {
		t.Fatalf("below-cluster-minimum page must be untouched, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_NonMonotonicRejected: right-aligned numbers that do not
// run in page order are a price/statistics column, not a TOC.
func TestFilterTOCBoxes_NonMonotonicRejected(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 434, 447, 100, 112, ".10"),
		makeBox(0, 434, 447, 140, 152, ".05"),
		makeBox(0, 434, 447, 180, 192, ".03"),
		makeBox(0, 60, 500, 300, 312, "正文段落文字内容。"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	if len(out) != len(boxes) {
		t.Fatalf("non-monotonic column must be untouched, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_HeadingOnMixedPage drops the standalone "目录" heading
// above a mixed page's entry column while keeping the prose.
func TestFilterTOCBoxes_HeadingOnMixedPage(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 100, 130, 100, 112, "目录"),
		makeBox(0, 434, 447, 130, 142, ".20"),
		makeBox(0, 434, 447, 170, 182, ".22"),
		makeBox(0, 434, 447, 210, 222, ".25"),
		makeBox(0, 60, 500, 300, 312, "这是前言的第一行文字内容，用来模拟正文段落。"),
		makeBox(0, 60, 500, 320, 332, "第二行继续，依然是一段连续的正文文字内容。"),
		makeBox(0, 60, 500, 340, 352, "第三行收尾，这一段累计字数一定超过五十字限制。"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	if contains(out, "目录") {
		t.Errorf("TOC heading must be dropped on the mixed page, got %v", texts(out))
	}
	if contains(out, ".20") || contains(out, ".22") || contains(out, ".25") {
		t.Errorf("entries must be dropped, got %v", texts(out))
	}
	if !contains(out, "这是前言的第一行文字内容，用来模拟正文段落。") {
		t.Errorf("prose must survive, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_TwoColumnTOC: two right-aligned page-number columns are
// detected independently and both are dropped on a mixed page.
func TestFilterTOCBoxes_TwoColumnTOC(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 190, 200, 100, 112, ".10"),
		makeBox(0, 190, 200, 140, 152, ".12"),
		makeBox(0, 190, 200, 180, 192, ".14"),
		makeBox(0, 430, 447, 100, 112, ".20"),
		makeBox(0, 430, 447, 140, 152, ".22"),
		makeBox(0, 430, 447, 180, 192, ".25"),
		makeBox(0, 60, 500, 300, 312, "这是前言的第一行文字内容，用来模拟正文段落。"),
		makeBox(0, 60, 500, 320, 332, "第二行继续，依然是一段连续的正文文字内容。"),
		makeBox(0, 60, 500, 340, 352, "第三行收尾，这一段累计字数一定超过五十字限制。"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	for _, drop := range []string{".10", ".12", ".14", ".20", ".22", ".25"} {
		if contains(out, drop) {
			t.Errorf("two-column TOC: expected to drop %q, got %v", drop, texts(out))
		}
	}
	if !contains(out, "这是前言的第一行文字内容，用来模拟正文段落。") {
		t.Errorf("two-column TOC: prose must survive, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_URLWatermarkExempt pins that a single isolated line is
// exempt from the short-run cap even when it is longer than
// tocShortRunMaxRunes: the 道德经 TOC pages carry a ~54-rune URL watermark,
// and the page must still be whole-page deleted because the watermark is not
// prose. Only chained multi-line runs are length-checked.
func TestFilterTOCBoxes_URLWatermarkExempt(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 61, 406, 43, 55, "木瓜树  更多的书籍免费下载 http://forum.law58.cn/?fromuid=381879"),
		makeBox(0, 111, 287, 78, 90, "第14章  执古之道，以御今之有."),
		makeBox(0, 111, 297, 114, 127, "第15章  夫唯不盈，故能蔽而新成"),
		makeBox(0, 111, 262, 152, 165, "第16章  道乃久，没身不殆."),
		makeBox(0, 434, 447, 80, 89, ".20"),
		makeBox(0, 434, 447, 116, 126, ".22"),
		makeBox(0, 434, 447, 154, 164, ".23"),
		makeBox(0, 441, 449, 637, 644, "II"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	if len(out) != 0 {
		t.Fatalf("URL-watermark TOC page: expected whole-page removal, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_ToleratesOccasionalNonMonotonic pins the relaxed order
// check: one or two out-of-order page numbers (a misplaced section, an
// appendix) do not disqualify an otherwise page-ordered column.
func TestFilterTOCBoxes_ToleratesOccasionalNonMonotonic(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 434, 447, 100, 112, ".10"),
		makeBox(0, 434, 447, 140, 152, ".12"),
		makeBox(0, 434, 447, 180, 192, ".20"),
		makeBox(0, 434, 447, 220, 232, ".18"), // one dip: tolerated
		makeBox(0, 434, 447, 260, 272, ".25"),
		makeBox(0, 60, 500, 300, 312, "这是前言的第一行文字内容，用来模拟正文段落。"),
		makeBox(0, 60, 500, 320, 332, "第二行继续，依然是一段连续的正文文字内容。"),
		makeBox(0, 60, 500, 340, 352, "第三行收尾，这一段累计字数一定超过五十字限制。"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	for _, drop := range []string{".10", ".12", ".20", ".18", ".25"} {
		if contains(out, drop) {
			t.Errorf("one-dip column: expected to drop %q, got %v", drop, texts(out))
		}
	}
	if !contains(out, "这是前言的第一行文字内容，用来模拟正文段落。") {
		t.Errorf("one-dip column: prose must survive, got %v", texts(out))
	}
}

// TestFilterTOCBoxes_InterleavedColumnsKeepProse regresses the per-column run
// tracking: two columns interleave by Top, and a body paragraph in one column
// is interleaved with a TOC number column in the other. A single global run
// would reset the paragraph's accumulation every time the other column sorts
// between its lines, under-counting it below tocShortRunMaxRunes and wrongly
// whole-page deleting the prose.
func TestFilterTOCBoxes_InterleavedColumnsKeepProse(t *testing.T) {
	boxes := []pdf.TextBox{
		// column 1: a three-line body paragraph (each line short, run > 50 runes)
		{X0: 60, X1: 500, Top: 100, Bottom: 112, Text: "这是前言的第一行文字内容，用来模拟正文段落。", PageNumber: 0, ColID: 1},
		{X0: 60, X1: 500, Top: 120, Bottom: 132, Text: "第二行继续，依然是一段连续的正文文字内容。", PageNumber: 0, ColID: 1},
		{X0: 60, X1: 500, Top: 140, Bottom: 152, Text: "第三行收尾，这一段累计字数一定超过五十字限制。", PageNumber: 0, ColID: 1},
		// column 2: an aligned TOC page-number column interleaved by Top
		{X0: 434, X1: 447, Top: 105, Bottom: 117, Text: ".20", PageNumber: 0, ColID: 2},
		{X0: 434, X1: 447, Top: 125, Bottom: 137, Text: ".22", PageNumber: 0, ColID: 2},
		{X0: 434, X1: 447, Top: 145, Bottom: 157, Text: ".25", PageNumber: 0, ColID: 2},
	}
	out := FilterTOCBoxes(boxes, medianH)
	for _, keep := range []string{
		"这是前言的第一行文字内容，用来模拟正文段落。",
		"第二行继续，依然是一段连续的正文文字内容。",
		"第三行收尾，这一段累计字数一定超过五十字限制。",
	} {
		if !contains(out, keep) {
			t.Errorf("interleaved columns: expected to keep %q, got %v", keep, texts(out))
		}
	}
	for _, drop := range []string{".20", ".22", ".25"} {
		if contains(out, drop) {
			t.Errorf("interleaved columns: expected to drop %q, got %v", drop, texts(out))
		}
	}
}

// TestFilterTOCBoxes_StructuredContentRejectsWholePage regresses the
// structured-content guard: a page that would otherwise qualify for
// whole-page deletion carries a DLA-detected table box. Whole-page deletion
// must be rejected — the table and the short lines survive, only the TOC
// entries are removed.
func TestFilterTOCBoxes_StructuredContentRejectsWholePage(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 61, 199, 40, 52, "木瓜树  更多的书籍免费下载"),
		makeBox(0, 434, 447, 100, 112, ".20"),
		makeBox(0, 434, 447, 140, 152, ".22"),
		makeBox(0, 434, 447, 180, 192, ".25"),
		{ // a table row that ends in a right-aligned number
			X0: 100, X1: 447, Top: 220, Bottom: 232,
			Text: "列A 列B 12", PageNumber: 0, LayoutType: pdf.LayoutTypeTable,
		},
		makeBox(0, 441, 449, 260, 272, "II"),
	}
	out := FilterTOCBoxes(boxes, medianH)
	if !contains(out, "列A 列B 12") {
		t.Errorf("table box must survive, got %v", texts(out))
	}
	if !contains(out, "木瓜树  更多的书籍免费下载") {
		t.Errorf("watermark must survive, got %v", texts(out))
	}
	for _, drop := range []string{".20", ".22", ".25"} {
		if contains(out, drop) {
			t.Errorf("structured-content page: expected to drop %q, got %v", drop, texts(out))
		}
	}
}
