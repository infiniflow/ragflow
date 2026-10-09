package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestCaptionKind_MidSentenceTableMentionNotCaption locks the root cause of
// go_bug table-text-interleaved-paragraph-dropped: a body paragraph that only
// MENTIONS "Table 1" mid-sentence must NOT be classified as a table caption.
// Python's is_caption uses re.match (start-anchored), so it keeps such a
// paragraph as a normal text section; Go must match that. Before the fix,
// reTableCaptionText matched "Table N" anywhere in the string, so the
// paragraph was mislabeled a caption and silently dropped by MergeCaptions.
func TestCaptionKind_MidSentenceTableMentionNotCaption(t *testing.T) {
	para := "The following analysis compares product performance across different market segments. Table 1 shows revenuebycategory."
	s := pdf.Section{Text: para, LayoutType: pdf.LayoutTypeText}
	if got := CaptionKind(s); got != "" {
		t.Errorf("CaptionKind(%q) = %q, want \"\" (mid-sentence 'Table 1' is not a caption)", para, got)
	}
}

// TestCaptionKind_StartsWithTableIsCaption confirms the legitimate case still
// classifies a line that BEGINS with "Table N" as a table caption (mirrors
// Python re.match).
func TestCaptionKind_StartsWithTableIsCaption(t *testing.T) {
	s := pdf.Section{Text: "Table 1: Revenue", LayoutType: pdf.LayoutTypeText}
	if got := CaptionKind(s); got != pdf.LayoutTypeTable {
		t.Errorf("CaptionKind(%q) = %q, want %q", s.Text, got, pdf.LayoutTypeTable)
	}
	s2 := pdf.Section{Text: "Figure 2: system architecture overview", LayoutType: pdf.LayoutTypeText}
	if got := CaptionKind(s2); got != pdf.LayoutTypeFigure {
		t.Errorf("CaptionKind(%q) = %q, want %q", s2.Text, got, pdf.LayoutTypeFigure)
	}
}

// TestIsCaptionBox_MidSentenceTableMention confirms IsCaptionBox (via reCaption)
// also keeps mid-sentence "Table N" as non-caption, mirroring Python re.match.
func TestIsCaptionBox_MidSentenceTableMention(t *testing.T) {
	para := "The following analysis compares product performance across different market segments. Table 1 shows revenuebycategory."
	if IsCaptionBox(para, pdf.LayoutTypeText) {
		t.Errorf("IsCaptionBox(%q) = true, want false (mid-sentence 'Table 1' is not a caption)", para)
	}
	if !IsCaptionBox("Table 1: Revenue", pdf.LayoutTypeText) {
		t.Errorf("IsCaptionBox(%q) = false, want true (starts with 'Table 1')", "Table 1: Revenue")
	}
}

// TestMergeCaptions_KeepsInterleavedParagraph locks the end-to-end behavior:
// an interleaved body paragraph that mentions "Table 1" must survive
// MergeCaptions as its own text section (Python keeps it too), not be dropped
// or merged into the table.
func TestMergeCaptions_KeepsInterleavedParagraph(t *testing.T) {
	sections := []pdf.Section{
		{Text: "Product Analysis Report", LayoutType: pdf.LayoutTypeTitle,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 192, Right: 404, Top: 65, Bottom: 82}}},
		{Text: "The following analysis compares product performance across different market segments. Table 1 shows revenuebycategory.",
			LayoutType: pdf.LayoutTypeText,
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 60, Right: 524, Top: 101, Bottom: 124}}},
		{Text: "<table><tr><td >Category</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 159, Right: 393, Top: 130, Bottom: 336}}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)
	if len(result) != 3 {
		t.Fatalf("expected 3 sections (title + paragraph + table), got %d: %v", len(result), textsOf(result))
	}
	found := false
	for _, s := range result {
		if strings.Contains(s.Text, "revenuebycategory") {
			found = true
		}
	}
	if !found {
		t.Errorf("interleaved paragraph dropped; sections = %v", textsOf(result))
	}
}

// TestCaptionKind_CJKBodyParagraphNotCaption locks the fix for go_bug
// cjk-caption-false-positive: Python's is_caption pattern
// `[图表]+[ 0-9:：]{2,}` requires ≥2 marker chars after the 图/表 run, so a
// body paragraph that merely STARTS with 表/图 (Chinese or Japanese) is not a
// caption. The bare `^表`/`^图` used previously misclassified such paragraphs
// as captions, and MergeCaptions then either dropped them (no table nearby)
// or swallowed them into <caption> — content loss Python never has.
func TestCaptionKind_CJKBodyParagraphNotCaption(t *testing.T) {
	for _, text := range []string{
		"表格是一种常见的数据组织形式，本文对其进行对比。", // Chinese: 表格…
		"图中所示的方法在实践中得到广泛应用。",              // Chinese: 图中…
		"表現方法の比較について述べる。",                   // Japanese: 表現…
	} {
		if got := CaptionKind(pdf.Section{Text: text, LayoutType: pdf.LayoutTypeText}); got != "" {
			t.Errorf("CaptionKind(%q) = %q, want \"\" (body paragraph is not a caption)", text, got)
		}
	}
}

// TestCaptionKind_CJKCaptionMarkersStillMatch pins the other side of the
// cjk-caption-false-positive fix: real CJK captions (图/表 + number +
// separator) must still classify — the suffix requirement must not
// over-tighten detection. The kind follows the run's first character.
func TestCaptionKind_CJKCaptionMarkersStillMatch(t *testing.T) {
	for _, tt := range []struct{ text, want string }{
		{"表1：交通工具等级", pdf.LayoutTypeTable},
		{"表 1 出差标准", pdf.LayoutTypeTable},
		{"图1 系统架构总览", pdf.LayoutTypeFigure},
		{"图表1 全球市场规模", pdf.LayoutTypeFigure},
	} {
		if got := CaptionKind(pdf.Section{Text: tt.text, LayoutType: pdf.LayoutTypeText}); got != tt.want {
			t.Errorf("CaptionKind(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func textsOf(secs []pdf.Section) []string {
	out := make([]string, 0, len(secs))
	for _, s := range secs {
		out = append(out, s.Text)
	}
	return out
}
