package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestMergeCaptions_LabeledTableCaptionProseContinuationKept locks #21884 on
// the shape measured in the 27-page RAG paper (page 6, 1-based): DLA marks the
// two-line tail of a body paragraph as "table caption" at confidence 0.868
// ([305,73]-[526,98] pt), higher than the real caption's 0.733
// ([367,204]-[462,218] pt), because it sits directly above Table 1. The
// sentence continues a line that ends mid-sentence ("... reducing redundant"),
// so the label must not move it into <caption>.
func TestMergeCaptions_LabeledTableCaptionProseContinuationKept(t *testing.T) {
	const (
		unfinished = "structure or evidence semantics. Instead, it improves inference efficiency by reducing redundant"
		prose1     = "generation calls among sibling leaves that share"
		prose2     = "overlapping retrieved evidence."
		caption    = "Table 1: Data statistics."
	)
	sections := []pdf.Section{
		{Text: unfinished, LayoutType: pdf.LayoutTypeText,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 305, Right: 527, Top: 750, Bottom: 763}}},
		{Text: prose1, LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 304, Right: 526, Top: 73, Bottom: 86}}},
		{Text: prose2, LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 305, Right: 443, Top: 87, Bottom: 99}}},
		{Text: "<table><tr><td>data</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 305, Right: 528, Top: 111, Bottom: 197}}},
		{Text: caption, LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 367, Right: 462, Top: 204, Bottom: 218}}},
	}

	result := MergeCaptions(sections)
	if len(result) != 4 {
		t.Fatalf("expected four sections (body tail + two prose lines + table), got %d: %v", len(result), textsOf(result))
	}
	var table *pdf.Section
	kept := map[string]string{}
	for i := range result {
		if result[i].LayoutType == pdf.LayoutTypeTable {
			table = &result[i]
			continue
		}
		kept[result[i].Text] = result[i].LayoutType
	}
	if table == nil {
		t.Fatal("table section missing")
	}
	if !strings.Contains(table.Text, "<caption>"+caption+"</caption>") {
		t.Errorf("table caption must contain only the real caption, got %q", table.Text)
	}
	if strings.Contains(table.Text, "generation calls") {
		t.Errorf("prose injected into the table caption: %q", table.Text)
	}
	if len(table.Positions) != 2 {
		t.Errorf("table geometry must be table + real caption (2 boxes), got %d: %+v", len(table.Positions), table.Positions)
	}
	if hasBox(table.Positions, 73, 86) || hasBox(table.Positions, 87, 99) {
		t.Errorf("prose boxes leaked into the table's highlight geometry: %+v", table.Positions)
	}
	for _, text := range []string{prose1, prose2} {
		lt, ok := kept[text]
		if !ok {
			t.Errorf("prose %q dropped; sections = %v", text, textsOf(result))
			continue
		}
		if lt != pdf.LayoutTypeText {
			t.Errorf("prose %q kept as %q, want %q", text, lt, pdf.LayoutTypeText)
		}
	}
}

// TestMergeCaptions_LabeledTableCaptionContinuationKept pins the other side: a
// real caption's lowercase continuation line must still merge. Table 3 in the
// reported paper continues "... vertical gath-" / "ering, lateral gathering,
// ..." / "respectively." — the continuation lines start lowercase.
func TestMergeCaptions_LabeledTableCaptionContinuationKept(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td>data</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 304, Right: 526, Top: 69, Bottom: 206}}},
		// First caption line: the text marker makes it a caption even without
		// the label, and it ends with a comma (an unfinished-looking line).
		{Text: "Table 3: Component-wise ablation of APT-RAG. AP,", LayoutType: pdf.LayoutTypeText,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 304, Right: 527, Top: 214, Bottom: 226}}},
		{Text: "ering, lateral gathering, and evidence-guided clustering,", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 304, Right: 527, Top: 227, Bottom: 239}}},
		{Text: "respectively.", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 306, Right: 355, Top: 240, Bottom: 251}}},
	}

	result := MergeCaptions(sections)
	if len(result) != 1 {
		t.Fatalf("expected one table section, got %d: %v", len(result), textsOf(result))
	}
	for _, want := range []string{"Table 3: Component-wise ablation", "ering, lateral gathering", "respectively."} {
		if !strings.Contains(result[0].Text, want) {
			t.Errorf("caption lost %q: %q", want, result[0].Text)
		}
	}
	if len(result[0].Positions) != 4 {
		t.Errorf("expected table + three caption boxes, got %d: %+v", len(result[0].Positions), result[0].Positions)
	}
}

// TestMergeCaptions_LabeledTableCaptionProseWithoutTargetKept locks the
// content-loss half: a labeled-prose line with no nearby table would be deleted
// by the no-target branch before #21884.
func TestMergeCaptions_LabeledTableCaptionProseWithoutTargetKept(t *testing.T) {
	const (
		leading = "the previous sentence introduces the next clause and"
		prose   = "continues on the following line."
	)
	sections := []pdf.Section{
		// Unannotated body box: an empty LayoutType is text at this stage.
		{Text: leading, LayoutType: "",
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 100, Bottom: 112}}},
		{Text: prose, LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 113, Bottom: 125}}},
	}

	result := MergeCaptions(sections)
	if len(result) != 2 {
		t.Fatalf("labeled prose with no target must be kept as body text, got %d: %v", len(result), textsOf(result))
	}
	if result[1].Text != prose || result[1].LayoutType != pdf.LayoutTypeText {
		t.Errorf("prose = (%q, %q), want (%q, %q)", result[1].Text, result[1].LayoutType, prose, pdf.LayoutTypeText)
	}
}

// TestMergeCaptions_LabelOnlyCaptionBelowTableAttaches pins the guard's
// precondition: the preceding section must be body text that ends mid-sentence.
// A label-only caption directly below a table is not prose just because it
// starts lowercase (the table's serialized text never ends in punctuation).
func TestMergeCaptions_LabelOnlyCaptionBelowTableAttaches(t *testing.T) {
	sections := []pdf.Section{
		{Text: "<table><tr><td>data</td></tr></table>", LayoutType: pdf.LayoutTypeTable,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 500, Top: 100, Bottom: 200}}},
		{Text: "quarterly totals", LayoutType: pdf.DLALabelTableCaption,
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 150, Right: 450, Top: 205, Bottom: 217}}},
	}

	result := MergeCaptions(sections)
	if len(result) != 1 {
		t.Fatalf("expected one table section, got %d: %v", len(result), textsOf(result))
	}
	if !strings.Contains(result[0].Text, "<caption>quarterly totals</caption>") {
		t.Errorf("label-only caption below the table was rejected: %q", result[0].Text)
	}
}

// TestCaptionKind_LabeledLowercaseProseStaysTable documents why #21884 is fixed
// in MergeCaptions rather than CaptionKind: whether a labeled lowercase line is
// prose depends on the preceding section (caption continuation versus paragraph
// tail), context CaptionKind does not receive. A lowercase-first rule inside
// CaptionKind would split real multi-line captions such as Table 3's.
func TestCaptionKind_LabeledLowercaseProseStaysTable(t *testing.T) {
	s := pdf.Section{
		Text:       "generation calls among sibling leaves that share",
		LayoutType: pdf.DLALabelTableCaption,
	}
	if got := CaptionKind(s); got != pdf.LayoutTypeTable {
		t.Errorf("CaptionKind(%q) = %q, want %q (context-free label precedence)", s.Text, got, pdf.LayoutTypeTable)
	}
}
