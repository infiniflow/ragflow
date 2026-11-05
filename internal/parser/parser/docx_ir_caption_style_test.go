package parser

import (
	"testing"

	"ragflow/internal/entity"
)

// docxCountTables returns the number of top-level table sections.
func docxCountTables(sections []map[string]any) int {
	n := 0
	for _, s := range sections {
		if dt, _ := s["doc_type_kwd"].(string); dt == "table" {
			n++
		}
	}
	return n
}

// docxFirstTableCaption returns the Caption of the first table section.
func docxFirstTableCaption(t *testing.T, sections []map[string]any) string {
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

// docxHasText reports whether a standalone text section carries exactly want.
func docxHasText(sections []map[string]any, want string) bool {
	for _, s := range sections {
		if dt, _ := s["doc_type_kwd"].(string); dt == "text" {
			if txt, _ := s["text"].(string); txt == want {
				return true
			}
		}
	}
	return false
}

// A Caption-styled paragraph (office_oxide 0.1.16 emits "style":"Caption")
// immediately above a table must be attached as the table caption and must
// NOT be emitted as its own text section. Before office_oxide exposed the
// style field this required re-parsing word/document.xml (docx_caption_scan.go);
// now the IR carries it directly.
func TestBuildDOCXJSONSections_TableCaption_FromStyle(t *testing.T) {
	ir := `{"sections":[{"elements":[
		{"type":"paragraph","style":"Caption","content":[{"type":"text","text":"设备流转明细"}]},
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]},{"content":[{"type":"paragraph","content":[{"type":"text","text":"B1"}]}]}]}]}
	]}]}`
	sections := buildDOCXJSONSections(ir, newOfficeMediaSink(t.Context(), officeMediaFormatDOCX))
	if got := docxFirstTableCaption(t, sections); got != "设备流转明细" {
		t.Errorf("caption = %q, want %q", got, "设备流转明细")
	}
	if docxHasText(sections, "设备流转明细") {
		t.Error("Caption-styled paragraph must be consumed as caption, not left as a text section")
	}
}

// An explicit Caption-styled paragraph wins over the synthesized
// heading-location caption when both precede the table.
func TestBuildDOCXJSONSections_TableCaption_StylePreferredOverHeading(t *testing.T) {
	ir := `{"sections":[{"elements":[
		{"type":"heading","level":1,"content":[{"type":"text","text":"设备台账"}]},
		{"type":"paragraph","style":"Caption","content":[{"type":"text","text":"设备流转明细"}]},
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]},{"content":[{"type":"paragraph","content":[{"type":"text","text":"B1"}]}]}]}]}
	]}]}`
	sections := buildDOCXJSONSections(ir, newOfficeMediaSink(t.Context(), officeMediaFormatDOCX))
	if got := docxFirstTableCaption(t, sections); got != "设备流转明细" {
		t.Errorf("caption = %q, want explicit caption %q", got, "设备流转明细")
	}
}

// office_oxide reports built-in Caption as the capitalized name; the match
// must be case-insensitive so "caption" (or any casing) is still recognized.
func TestBuildDOCXJSONSections_TableCaption_StyleCaseInsensitive(t *testing.T) {
	ir := `{"sections":[{"elements":[
		{"type":"paragraph","style":"caption","content":[{"type":"text","text":"设备流转明细"}]},
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]}]}]}
	]}]}`
	sections := buildDOCXJSONSections(ir, newOfficeMediaSink(t.Context(), officeMediaFormatDOCX))
	if got := docxFirstTableCaption(t, sections); got != "设备流转明细" {
		t.Errorf("caption = %q, want %q", got, "设备流转明细")
	}
}

// A Caption-styled paragraph with NO following table is not a caption; it
// must be emitted as an ordinary text section (never silently dropped).
func TestBuildDOCXJSONSections_TableCaption_StyleWithoutTableEmittedAsText(t *testing.T) {
	ir := `{"sections":[{"elements":[
		{"type":"paragraph","style":"Caption","content":[{"type":"text","text":"孤立题注"}]}
	]}]}`
	sections := buildDOCXJSONSections(ir, newOfficeMediaSink(t.Context(), officeMediaFormatDOCX))
	if docxCountTables(sections) != 0 {
		t.Errorf("expected no table sections, got %d", docxCountTables(sections))
	}
	if !docxHasText(sections, "孤立题注") {
		t.Error("orphan Caption-styled paragraph must be emitted as a text section")
	}
}

// A plain (non-Caption) paragraph above a table must never become a caption;
// only the heading hierarchy is used for it.
func TestBuildDOCXJSONSections_TableCaption_NonCaptionParagraphIgnored(t *testing.T) {
	ir := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"设备流转明细"}]},
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]}]}]}
	]}]}`
	sections := buildDOCXJSONSections(ir, newOfficeMediaSink(t.Context(), officeMediaFormatDOCX))
	if got := docxFirstTableCaption(t, sections); got == "设备流转明细" {
		t.Errorf("plain paragraph must not become a caption, got %q", got)
	}
}

// Regression for the old .doc split workaround (office_oxide#456): a 1-row
// table immediately followed by a same-width multi-row table must NOT be
// merged into one section. office_oxide 0.1.16 no longer splits .doc tables,
// so the structural proxy in buildDOCXJSONSectionsCore is gone and two
// adjacent tables stay distinct.
func TestBuildDOCXJSONSections_AdjacentTablesNotMerged(t *testing.T) {
	ir := `{"sections":[{"elements":[
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"H"}]}]}]}]},
		{"type":"table","rows":[
			{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]}]},
			{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A2"}]}]}]},
			{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A3"}]}]}]}
		]}
	]}]}`
	sections := buildDOCXJSONSections(ir, newOfficeMediaSink(t.Context(), officeMediaFormatDOCX))
	if got := docxCountTables(sections); got != 2 {
		t.Errorf("adjacent tables must stay distinct, got %d tables (want 2)", got)
	}
}
