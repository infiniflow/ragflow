package layout

import (
	"reflect"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestJoinPageContinuationsCrossPageCJK(t *testing.T) {
	sections := []pdf.Section{
		textSection("物资保障", 3, 80, 520, 700, 715),
		textSection("组、宣传工作组。", 4, 80, 200, 100, 115),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 1 {
		t.Fatalf("sections = %d, want 1", len(joined))
	}
	if got, want := joined[0].Text, "物资保障组、宣传工作组。"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if got, want := joined[0].Positions, []pdf.Position{
		{PageNumbers: []int{3}, Left: 80, Right: 520, Top: 700, Bottom: 715},
		{PageNumbers: []int{4}, Left: 80, Right: 200, Top: 100, Bottom: 115},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %#v, want %#v", got, want)
	}
}

func TestJoinPageContinuationsKeepsEnglishSpace(t *testing.T) {
	sections := []pdf.Section{
		textSection("work with the", 1, 80, 520, 700, 715),
		textSection("team to finish.", 2, 80, 200, 100, 115),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 1 || joined[0].Text != "work with the team to finish." {
		t.Fatalf("joined = %#v", joined)
	}
}

func TestJoinPageContinuationsJoinsKanaWithoutSpace(t *testing.T) {
	sections := []pdf.Section{
		textSection("これは", 1, 80, 520, 700, 715),
		textSection("テストです。", 2, 80, 200, 100, 115),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 1 || joined[0].Text != "これはテストです。" {
		t.Fatalf("joined = %#v", joined)
	}
}

func TestJoinPageContinuationsJoinsProlongedSoundMarkWithoutSpace(t *testing.T) {
	sections := []pdf.Section{
		textSection("コンピュー", 1, 80, 520, 700, 715),
		textSection("ター。", 2, 80, 200, 100, 115),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 1 || joined[0].Text != "コンピューター。" {
		t.Fatalf("joined = %#v", joined)
	}
}

func TestJoinPageContinuationsAbbreviationIsNotListMarker(t *testing.T) {
	sections := []pdf.Section{
		textSection("references,", 1, 80, 520, 700, 715),
		textSection("e.g. this article.", 2, 80, 200, 100, 115),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 1 || joined[0].Text != "references, e.g. this article." {
		t.Fatalf("joined = %#v", joined)
	}
}

func TestJoinPageContinuationsThreePages(t *testing.T) {
	sections := []pdf.Section{
		textSection("为保障工作", 1, 80, 520, 700, 715),
		textSection("有序推进，设立物资保障", 2, 80, 520, 100, 700),
		textSection("组。", 3, 80, 200, 100, 115),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 1 {
		t.Fatalf("sections = %d, want 1", len(joined))
	}
	if got, want := joined[0].Text, "为保障工作有序推进，设立物资保障组。"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if got, want := len(joined[0].Positions), 3; got != want {
		t.Fatalf("positions = %d, want %d", got, want)
	}
}

func TestJoinPageContinuationsKeepsParagraphBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		first     string
		second    string
		layout    string
		page      int
		left, top float64
	}{
		{"sentence ended", "第一段。", "下一段。", pdf.LayoutTypeText, 4, 80, 100},
		{"numbered item", "负责物资保障", "（二）宣传工作组", pdf.LayoutTypeText, 4, 80, 100},
		{"lettered item", "负责物资保障", "b) 宣传工作组", pdf.LayoutTypeText, 4, 80, 100},
		{"lettered item no space", "负责物资保障", "b)宣传工作组", pdf.LayoutTypeText, 4, 80, 100},
		{"title", "负责物资保障", "管理办法", "title", 4, 80, 100},
		{"new indentation", "负责物资保障", "下一段内容", pdf.LayoutTypeText, 4, 112, 100},
		{"same page", "负责物资保障", "下一段内容", pdf.LayoutTypeText, 3, 80, 740},
		{"missing page", "负责物资保障", "下一段内容", pdf.LayoutTypeText, 5, 80, 100},
		{"blank line", "负责物资保障\n\n", "下一段内容", pdf.LayoutTypeText, 4, 80, 100},
		{"spaced blank line", "负责物资保障\n \t\n", "下一段内容", pdf.LayoutTypeText, 4, 80, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sections := []pdf.Section{
				textSection(tc.first, 3, 80, 520, 700, 715),
				textSection(tc.second, tc.page, tc.left, 520, tc.top, tc.top+15),
			}
			sections[1].LayoutType = tc.layout
			joined := JoinPageContinuations(sections)
			if len(joined) != 2 {
				t.Fatalf("sections = %d, want 2 (must not join)", len(joined))
			}
		})
	}
}

func TestJoinPageContinuationsRejectsRegionBelowPreviousBottom(t *testing.T) {
	sections := []pdf.Section{
		textSection("负责物资保障", 3, 80, 520, 100, 115),
		textSection("下一段内容", 4, 80, 520, 500, 515),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 2 {
		t.Fatalf("sections = %d, want 2", len(joined))
	}
}

func TestJoinPageContinuationsTallParagraphKeepsIndentation(t *testing.T) {
	sections := []pdf.Section{
		textSection("上一段没有结束标点", 1, 80, 520, 600, 720),
		textSection("新的缩进段落。", 2, 112, 520, 100, 250),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 2 {
		t.Fatalf("sections = %d, want 2", len(joined))
	}
}

func TestJoinPageContinuationsDoesNotCrossMedia(t *testing.T) {
	sections := []pdf.Section{
		textSection("物资保障", 1, 80, 520, 700, 715),
		{Text: "插图", LayoutType: pdf.LayoutTypeFigure, DocTypeKwd: "image", Positions: []pdf.Position{{PageNumbers: []int{2}, Left: 80, Right: 520, Top: 100, Bottom: 200}}},
		textSection("组、宣传工作组。", 3, 80, 200, 100, 115),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 3 {
		t.Fatalf("sections = %d, want 3", len(joined))
	}
}

func TestJoinPageContinuationsUpdatesPositionTag(t *testing.T) {
	sections := []pdf.Section{
		textSection("物资保障", 3, 80, 520, 700, 715),
		textSection("组。", 4, 80, 200, 100, 115),
	}
	joined := JoinPageContinuations(sections)
	if len(joined) != 1 {
		t.Fatalf("sections = %d, want 1", len(joined))
	}
	if got, want := joined[0].PositionTag, "@@4-5\t80.0\t200.0\t700.0\t115.0##"; got != want {
		t.Fatalf("position tag = %q, want %q", got, want)
	}
}

func textSection(text string, page int, left, right, top, bottom float64) pdf.Section {
	return pdf.Section{
		Text:       text,
		LayoutType: pdf.LayoutTypeText,
		Positions: []pdf.Position{{
			PageNumbers: []int{page},
			Left:        left,
			Right:       right,
			Top:         top,
			Bottom:      bottom,
		}},
	}
}
