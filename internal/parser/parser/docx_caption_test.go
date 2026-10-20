package parser

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// docxCaptionTableIR wraps a single table element in a minimal IR document.
func docxCaptionTableIR(elements ...string) string {
	return `{"sections":[{"title":"","elements":[` + strings.Join(elements, ",") + `]}]}`
}

const docxCaptionTableBody = `{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]},{"content":[{"type":"paragraph","content":[{"type":"text","text":"B1"}]}]}]}]}`

// docxCaptionFindTableCaption returns the Caption of the (single) table
// section, which buildDOCXJSONSections stores on the structured
// entity.TableData (after projectPDFTable parses the injected <caption>
// out of the table HTML).
func docxCaptionFindTableCaption(t *testing.T, sections []map[string]any) string {
	t.Helper()
	for _, s := range sections {
		if dt, _ := s["doc_type_kwd"].(string); dt == "table" {
			td, ok := s["table"].(*entity.TableData)
			if !ok || td == nil {
				t.Fatalf("table section missing *entity.TableData")
			}
			return td.Caption
		}
	}
	t.Fatal("no table section found")
	return ""
}

// Happy path: a heading immediately before a table yields a "Table Location"
// caption built from the heading hierarchy (mirrors Python __get_nearest_title).
func TestBuildDOCXJSONSections_TableCaption_ByHeading(t *testing.T) {
	ir := docxCaptionTableIR(
		`{"type":"heading","level":1,"content":[{"type":"text","text":"设备台账"}]}`,
		docxCaptionTableBody,
	)
	sections := buildDOCXJSONSections(ir, newEmbeddedMediaBudget())
	if got := docxCaptionFindTableCaption(t, sections); got != "Table Location: 设备台账" {
		t.Errorf("caption = %q, want %q", got, "Table Location: 设备台账")
	}
}

// Nested heading hierarchy collapses to "A > B > C" before the table. The
// caption round-trips through HTML (escaped on injection, unescaped by
// ParseTableHTML), so TableData.Caption holds the plain text form.
func TestBuildDOCXJSONSections_TableCaption_NestedHeading(t *testing.T) {
	ir := docxCaptionTableIR(
		`{"type":"heading","level":1,"content":[{"type":"text","text":"H1"}]}`,
		`{"type":"heading","level":2,"content":[{"type":"text","text":"H2"}]}`,
		`{"type":"heading","level":3,"content":[{"type":"text","text":"H3"}]}`,
		docxCaptionTableBody,
	)
	sections := buildDOCXJSONSections(ir, newEmbeddedMediaBudget())
	if got := docxCaptionFindTableCaption(t, sections); got != "Table Location: H1 > H2 > H3" {
		t.Errorf("caption = %q, want %q", got, "Table Location: H1 > H2 > H3")
	}
}

// A heading AFTER the table must not leak into the caption.
func TestBuildDOCXJSONSections_TableCaption_HeadingAfterTable(t *testing.T) {
	ir := docxCaptionTableIR(
		docxCaptionTableBody,
		`{"type":"heading","level":1,"content":[{"type":"text","text":"后续标题"}]}`,
	)
	sections := buildDOCXJSONSections(ir, newEmbeddedMediaBudget())
	if got := docxCaptionFindTableCaption(t, sections); got != "" {
		t.Errorf("table should have no caption when heading is after it, got %q", got)
	}
}

// A sibling heading supersedes the deeper one: the outline holds only the
// ancestors of the table, not abandoned deeper headings.
func TestBuildDOCXJSONSections_TableCaption_SiblingHeadingSupersedes(t *testing.T) {
	ir := docxCaptionTableIR(
		`{"type":"heading","level":1,"content":[{"type":"text","text":"H1"}]}`,
		`{"type":"heading","level":2,"content":[{"type":"text","text":"H2a"}]}`,
		`{"type":"heading","level":3,"content":[{"type":"text","text":"H3"}]}`,
		`{"type":"heading","level":2,"content":[{"type":"text","text":"H2b"}]}`,
		docxCaptionTableBody,
	)
	sections := buildDOCXJSONSections(ir, newEmbeddedMediaBudget())
	if got := docxCaptionFindTableCaption(t, sections); got != "Table Location: H1 > H2b" {
		t.Errorf("caption = %q, want %q", got, "Table Location: H1 > H2b")
	}
}

// A heading with a deeper level after the table's parent must not resurrect
// an abandoned branch: the outline keeps only true ancestors.
func TestBuildDOCXJSONSections_TableCaption_DeeperHeadingAfterSibling(t *testing.T) {
	ir := docxCaptionTableIR(
		`{"type":"heading","level":2,"content":[{"type":"text","text":"H2a"}]}`,
		`{"type":"heading","level":3,"content":[{"type":"text","text":"H3a"}]}`,
		`{"type":"heading","level":2,"content":[{"type":"text","text":"H2b"}]}`,
		`{"type":"heading","level":3,"content":[{"type":"text","text":"H3b"}]}`,
		docxCaptionTableBody,
	)
	sections := buildDOCXJSONSections(ir, newEmbeddedMediaBudget())
	if got := docxCaptionFindTableCaption(t, sections); got != "Table Location: H2b > H3b" {
		t.Errorf("caption = %q, want %q", got, "Table Location: H2b > H3b")
	}
}

// An empty heading between a real heading and the table is ignored, so it
// neither truncates the outline nor becomes part of the caption.
func TestBuildDOCXJSONSections_TableCaption_EmptyHeadingIgnored(t *testing.T) {
	ir := docxCaptionTableIR(
		`{"type":"heading","level":1,"content":[{"type":"text","text":"H1"}]}`,
		`{"type":"heading","level":2,"content":[{"type":"text","text":"  "}]}`,
		docxCaptionTableBody,
	)
	sections := buildDOCXJSONSections(ir, newEmbeddedMediaBudget())
	if got := docxCaptionFindTableCaption(t, sections); got != "Table Location: H1" {
		t.Errorf("caption = %q, want %q", got, "Table Location: H1")
	}
}

// The heading outline spans IR sections: a heading in an earlier section
// still captions a table in a later one.
func TestBuildDOCXJSONSections_TableCaption_OutlineSpansSections(t *testing.T) {
	ir := `{"sections":[
		{"title":"","elements":[{"type":"heading","level":1,"content":[{"type":"text","text":"第一章"}]}]},
		{"title":"","elements":[` + docxCaptionTableBody + `]}
	]}`
	sections := buildDOCXJSONSections(ir, newEmbeddedMediaBudget())
	if got := docxCaptionFindTableCaption(t, sections); got != "Table Location: 第一章" {
		t.Errorf("caption = %q, want %q", got, "Table Location: 第一章")
	}
}

// A plain (non-heading) paragraph before a table is NOT turned into a
// caption — only the heading hierarchy is used.
func TestBuildDOCXJSONSections_TableCaption_PlainParagraphNotCaption(t *testing.T) {
	ir := docxCaptionTableIR(
		`{"type":"paragraph","content":[{"type":"text","text":"设备流转明细"}]}`,
		docxCaptionTableBody,
	)
	sections := buildDOCXJSONSections(ir, newEmbeddedMediaBudget())
	if got := docxCaptionFindTableCaption(t, sections); got != "" {
		t.Errorf("plain paragraph must not become a caption, got %q", got)
	}
}

// Corner: empty IR yields no sections.
func TestBuildDOCXJSONSections_TableCaption_Empty(t *testing.T) {
	sections := buildDOCXJSONSections(`{"sections":[]}`, newEmbeddedMediaBudget())
	if len(sections) != 0 {
		t.Errorf("empty IR should yield no sections, got %d", len(sections))
	}
}

// TEMPORARY Caption-style detection (docx_caption_scan.go): an explicit
// caption supplied for the table overrides the synthesized heading-location
// caption.
func TestBuildDOCXJSONSections_TableCaption_ExplicitPreferredOverLocation(t *testing.T) {
	ir := docxCaptionTableIR(
		`{"type":"heading","level":1,"content":[{"type":"text","text":"设备台账"}]}`,
		docxCaptionTableBody,
	)
	sections := buildDOCXJSONSectionsWithCaptions(ir, newEmbeddedMediaBudget(), []string{"设备流转明细"})
	if got := docxCaptionFindTableCaption(t, sections); got != "设备流转明细" {
		t.Errorf("explicit caption = %q, want %q", got, "设备流转明细")
	}
}

// An explicit caption is attached even when the table sits under no heading.
func TestBuildDOCXJSONSections_TableCaption_ExplicitWithNoHeading(t *testing.T) {
	ir := docxCaptionTableIR(docxCaptionTableBody)
	sections := buildDOCXJSONSectionsWithCaptions(ir, newEmbeddedMediaBudget(), []string{"设备流转明细"})
	if got := docxCaptionFindTableCaption(t, sections); got != "设备流转明细" {
		t.Errorf("explicit caption = %q, want %q", got, "设备流转明细")
	}
}

// A blank entry never overrides a valid heading-location caption.
func TestBuildDOCXJSONSections_TableCaption_BlankDoesNotOverrideLocation(t *testing.T) {
	ir := docxCaptionTableIR(
		`{"type":"heading","level":1,"content":[{"type":"text","text":"设备台账"}]}`,
		docxCaptionTableBody,
	)
	sections := buildDOCXJSONSectionsWithCaptions(ir, newEmbeddedMediaBudget(), []string{""})
	if got := docxCaptionFindTableCaption(t, sections); got != "Table Location: 设备台账" {
		t.Errorf("caption = %q, want %q", got, "Table Location: 设备台账")
	}
}
