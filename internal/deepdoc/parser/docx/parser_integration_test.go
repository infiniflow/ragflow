//go:build cgo && manual

package docx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	doctype "ragflow/internal/deepdoc/parser/type"
)

// readFixture reads a DOCX fixture file from testdata/docxs/.
func readFixture(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join("testdata", "docxs", name))
}

func TestParse_Integration_MultiSection(t *testing.T) {
	data, err := readFixture("multi_section.docx")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	result, err := Parse(data, doctype.DefaultParserConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) != 7 {
		t.Errorf("multi_section.docx: want 7 sections, got %d", len(result.Sections))
	}
	// Verify headings
	expected := []string{"Chapter 1", "Section 1.1", "Chapter 2"}
	titleIdx := 0
	for _, s := range result.Sections {
		if s.LayoutType == "title" {
			if titleIdx < len(expected) && s.Text != expected[titleIdx] {
				t.Errorf("heading[%d]: got %q, want %q", titleIdx, s.Text, expected[titleIdx])
			}
			titleIdx++
		}
	}
	if titleIdx != 3 {
		t.Errorf("expected 3 headings, found %d", titleIdx)
	}
}

func TestParse_Integration_WithTable(t *testing.T) {
	data, err := readFixture("with_table.docx")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	result, err := Parse(data, doctype.DefaultParserConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) != 4 {
		t.Fatalf("want 4 sections, got %d", len(result.Sections))
	}
	if result.Sections[2].DocTypeKwd != "table" {
		t.Error("expected table section at index 2")
	}
	if len(result.Sections[2].TableItem.Rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(result.Sections[2].TableItem.Rows))
	}
	if result.Sections[2].TableItem.Rows[0][0] != "Product" {
		t.Errorf("cell[0,0]: got %q", result.Sections[2].TableItem.Rows[0][0])
	}
	// Verify HTML table is rendered.
	if !strings.Contains(result.Sections[2].Text, "<table>") {
		t.Error("table Section.Text should contain HTML <table>")
	}
	if !strings.Contains(result.Sections[2].Text, "<th >Product</th>") {
		t.Errorf("table HTML missing header: %s", result.Sections[2].Text)
	}
}

func TestParse_Integration_WithImage(t *testing.T) {
	data, err := readFixture("with_image.docx")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	result, err := Parse(data, doctype.DefaultParserConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	hasImage := false
	for _, s := range result.Sections {
		if s.DocTypeKwd == "image" && s.Image != "" {
			hasImage = true
		}
	}
	if !hasImage {
		t.Error("expected at least one image section")
	}
}

func TestParse_Integration_NestedHeadings(t *testing.T) {
	data, err := readFixture("nested_headings.docx")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	result, err := Parse(data, doctype.DefaultParserConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) != 5 {
		t.Fatalf("want 5 sections, got %d", len(result.Sections))
	}
	titles := 0
	for _, s := range result.Sections {
		if s.LayoutType == "title" {
			titles++
		}
	}
	if titles != 5 {
		t.Errorf("expected 5 titles, got %d", titles)
	}
}

func TestParse_Integration_WithCaption(t *testing.T) {
	data, err := readFixture("with_caption.docx")
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	result, err := Parse(data, doctype.DefaultParserConfig())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) != 4 {
		t.Fatalf("want 4 sections, got %d", len(result.Sections))
	}

	// Block order: [Figure caption] [body text] [2x2 table] [Table caption]
	// Figure caption (index 0) is text, not title.
	if result.Sections[0].LayoutType != "text" {
		t.Errorf("figure caption: got LayoutType %q", result.Sections[0].LayoutType)
	}
	if !strings.Contains(result.Sections[0].Text, "Figure 1") {
		t.Errorf("figure caption text: %q", result.Sections[0].Text)
	}

	// Table section (index 2) must have HTML rendering.
	s := result.Sections[2]
	if s.DocTypeKwd != "table" {
		t.Errorf("table section: DocTypeKwd=%q", s.DocTypeKwd)
	}
	if !strings.Contains(s.Text, "<table>") {
		t.Fatal("table section missing <table> HTML")
	}
	if !strings.Contains(s.Text, "<th >A</th>") || !strings.Contains(s.Text, "<th >B</th>") {
		t.Errorf("table header cells: %s", s.Text)
	}
	if !strings.Contains(s.Text, "<td >1</td>") || !strings.Contains(s.Text, "<td >2</td>") {
		t.Errorf("table data cells: %s", s.Text)
	}

	// Table caption (index 3) follows the table.
	if !strings.Contains(result.Sections[3].Text, "Table 1") {
		t.Errorf("table caption text: %q", result.Sections[3].Text)
	}
}
