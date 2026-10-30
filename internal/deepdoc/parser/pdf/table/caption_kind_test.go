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
// table-caption detection matched "Table N" anywhere in the string, so the
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

// TestIsCaptionBox_MidSentenceTableMention confirms IsCaptionBox (via the
// shared caption detection) also keeps mid-sentence "Table N" as non-caption,
// mirroring Python re.match.
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
	result := MergeCaptions(sections)
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
		"图中所示的方法在实践中得到广泛应用。",       // Chinese: 图中…
		"表現方法の比較について述べる。",          // Japanese: 表現…
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

// TestCaptionKind_EnglishTableCrossReferenceNotCaption locks the fix for
// #20672: a body sentence that STARTS with a table cross-reference is not a
// caption. Python's pattern-only is_caption classifies these as captions and
// swallows them into the table; this Go path requires a caption boundary after
// the number, so the sentence stays a standalone text section.
//
// IsCaptionBox deliberately keeps the pattern-only verdict: it decides cell
// filling, no-merge marking, and caption-row stripping, all of which must keep
// matching Python's is_caption.
func TestCaptionKind_EnglishTableCrossReferenceNotCaption(t *testing.T) {
	for _, text := range []string{
		"Table 1, we present the F1 scores and success rates for KNOWAGENT.",
		"Table 9 reports the adaptive expansion ablation, as shown in Figure 5.",
		"Table 1 shows revenue by category for the fiscal year.",
		"Table 2 (see Appendix A) reports the remaining configurations.",
		"Table 1's results show a 12% improvement over the baseline.",
		"Table 1.5 shows the ablation without retrieval.",
	} {
		if got := CaptionKind(pdf.Section{Text: text, LayoutType: pdf.LayoutTypeText}); got != "" {
			t.Errorf("CaptionKind(%q) = %q, want \"\" (cross-reference is body text)", text, got)
		}
		if !IsCaptionBox(text, pdf.LayoutTypeText) {
			t.Errorf("IsCaptionBox(%q) = false, want true (pattern-only contract)", text)
		}
	}
}

// TestCaptionKind_EnglishTableCaptionBoundaries pins the positive side of the
// boundary rule: separators, title-like words, end of line, letter-suffixed
// numbers, and parenthesised continuations/panel labels still classify as table
// captions.
func TestCaptionKind_EnglishTableCaptionBoundaries(t *testing.T) {
	for _, text := range []string{
		"Table 2: Comparative Experiment of ReAct",
		"Table 1. Data statistics",
		"Table 1",
		"Table 2 Performance comparison",
		"Table 2 (continued)",
		"Table 2 (cont'd)",
		"Table 2 (a) Confusion matrix",
		"Table 1a. Results",
		"Table 1b: Results",
		"Table 1 (iv) Confusion matrix",
		"Table 1 (12) Results",
		"Table 1.5: Ablation results",
		"Table 1\nRevenue by category",
		"Table 1\u00a0Revenue by category",
		"Table 1\u3000Revenue by category",
	} {
		if got := CaptionKind(pdf.Section{Text: text, LayoutType: pdf.LayoutTypeText}); got != pdf.LayoutTypeTable {
			t.Errorf("CaptionKind(%q) = %q, want %q", text, got, pdf.LayoutTypeTable)
		}
		if !IsCaptionBox(text, pdf.LayoutTypeText) {
			t.Errorf("IsCaptionBox(%q) = false, want true", text)
		}
	}
}

// TestCaptionKind_LabelKeepsDeclaredKind pins label precedence: a layout label
// names the parent kind even when its text carries no marker (a DLA caption
// such as "Quarterly revenue breakdown"), including non-canonical labels such as
// "table_caption".
func TestCaptionKind_LabelKeepsDeclaredKind(t *testing.T) {
	for _, tt := range []struct{ text, label, want string }{
		{"Quarterly revenue breakdown", pdf.DLALabelTableCaption, pdf.LayoutTypeTable},
		{"Quarterly revenue breakdown", "table_caption", pdf.LayoutTypeTable},
		{"Table 9: Fine-grained retrieval recall.", pdf.DLALabelTableCaption, pdf.LayoutTypeTable},
		{"Figure 1 shows the architecture.", "figure_caption", pdf.LayoutTypeFigure},
		{"Figure 1 shows the architecture.", pdf.DLALabelFigureCaption, pdf.LayoutTypeFigure},
	} {
		if got := CaptionKind(pdf.Section{Text: tt.text, LayoutType: tt.label}); got != tt.want {
			t.Errorf("CaptionKind(%q, label %q) = %q, want %q", tt.text, tt.label, got, tt.want)
		}
	}
}

// TestCaptionKind_LabeledCrossReferenceNotCaption covers the DLA half of
// #20672: when the layout labels a CROSS-REFERENCE sentence as a caption, the
// label must not override the boundary rule, or MergeCaptions injects the prose
// into the table's <caption> (or deletes it when no table is near).
func TestCaptionKind_LabeledCrossReferenceNotCaption(t *testing.T) {
	const prose = "Table 1, we present the F1 scores and success rates."
	for _, label := range []string{
		pdf.DLALabelTableCaption,
		"table_caption",
		pdf.DLALabelFigureCaption,
		"figure_caption",
		"caption",
	} {
		if got := CaptionKind(pdf.Section{Text: prose, LayoutType: label}); got != "" {
			t.Errorf("CaptionKind(labeled %q, %q) = %q, want \"\" (prose is not a caption)", prose, label, got)
		}
	}
}

// TestCaptionKind_FigureBoundaryUnchanged documents the scope of #20672:
// sentence-style FIGURE captions ("Figure 1 shows …") remain captions, because
// that form is a common legitimate caption style and the issue covers tables.
func TestCaptionKind_FigureBoundaryUnchanged(t *testing.T) {
	text := "Figure 1 shows the overall architecture of the system."
	if got := CaptionKind(pdf.Section{Text: text, LayoutType: pdf.LayoutTypeText}); got != pdf.LayoutTypeFigure {
		t.Errorf("CaptionKind(%q) = %q, want %q", text, got, pdf.LayoutTypeFigure)
	}
}

// TestCaptionKind_ProperNounContinuationIsCaption pins the KNOWN LIMITATION of
// the boundary rule: it keys on the first character after "Table N", so a
// cross-reference that continues with a proper noun classifies as a caption
// ("Table 3 Smith et al. report …"), exactly as a real caption title would
// ("Table 3 Smith-Kappa-Rosenberg parameters"). Nothing in the string separates
// them without part-of-speech tagging, so the rule keeps the caption reading.
// The consequence is placement, not deletion: MergeCaptions moves the sentence
// into the nearest table's <caption>, so it stays indexed
// (TestMergeCaptions_ProperNounCrossReferenceLandsInCaption).
func TestCaptionKind_ProperNounContinuationIsCaption(t *testing.T) {
	for _, text := range []string{
		"Table 3 Smith et al. report the F1 scores.",
		"Table 3 Gaussian noise dominates above 40 dB.",
	} {
		if got := CaptionKind(pdf.Section{Text: text, LayoutType: pdf.LayoutTypeText}); got != pdf.LayoutTypeTable {
			t.Errorf("CaptionKind(%q) = %q, want %q (documented limitation)", text, got, pdf.LayoutTypeTable)
		}
	}
}

// hasBox reports whether positions contains a box with the given top and
// bottom edges. Tests use it to assert that a merged caption's box reached the
// section it was merged into.
func hasBox(positions []pdf.Position, top, bottom float64) bool {
	for _, p := range positions {
		if p.Top == top && p.Bottom == bottom {
			return true
		}
	}
	return false
}

func textsOf(secs []pdf.Section) []string {
	out := make([]string, 0, len(secs))
	for _, s := range secs {
		out = append(out, s.Text)
	}
	return out
}
