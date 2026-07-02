package docx

import (
	"testing"
)

func TestBlocksToSections_Paragraph(t *testing.T) {
	blocks := []RawBlock{
		{Type: "paragraph", Text: "hello world", Style: "Normal"},
	}
	sections := blocksToSections(blocks)

	if len(sections) != 1 {
		t.Fatalf("want 1 section, got %d", len(sections))
	}
	s := sections[0]
	if s.Text != "hello world" {
		t.Errorf("Text: got %q, want %q", s.Text, "hello world")
	}
	if s.DocTypeKwd != "text" {
		t.Errorf("DocTypeKwd: got %q, want %q", s.DocTypeKwd, "text")
	}
}

func TestBlocksToSections_Headings(t *testing.T) {
	blocks := []RawBlock{
		{Type: "paragraph", Text: "Main Title", Style: "Heading 1"},
		{Type: "paragraph", Text: "Sub Title", Style: "Heading 2"},
		{Type: "paragraph", Text: "Deep", Style: "Heading 3"},
		{Type: "paragraph", Text: "Plain", Style: "Normal"},
	}
	sections := blocksToSections(blocks)

	if len(sections) != 4 {
		t.Fatalf("want 4 sections, got %d", len(sections))
	}
	if sections[0].LayoutType != "title" {
		t.Errorf("[0] LayoutType: got %q, want %q", sections[0].LayoutType, "title")
	}
	if sections[1].LayoutType != "title" {
		t.Errorf("[1] LayoutType: got %q, want %q", sections[1].LayoutType, "title")
	}
	if sections[2].LayoutType != "title" {
		t.Errorf("[2] LayoutType: got %q, want %q", sections[2].LayoutType, "title")
	}
	// Normal paragraph is NOT a title
	if sections[3].LayoutType != "text" {
		t.Errorf("[3] LayoutType: got %q, want %q", sections[3].LayoutType, "text")
	}
}

func TestBlocksToSections_Table(t *testing.T) {
	blocks := []RawBlock{
		{Type: "table", Rows: [][]string{
			{"Name", "Age"},
			{"Alice", "30"},
		}},
	}
	sections := blocksToSections(blocks)

	if len(sections) != 1 {
		t.Fatalf("want 1 section, got %d", len(sections))
	}
	s := sections[0]
	if s.DocTypeKwd != "table" {
		t.Errorf("DocTypeKwd: got %q, want %q", s.DocTypeKwd, "table")
	}
	if s.TableItem == nil {
		t.Fatal("TableItem is nil")
	}
	if len(s.TableItem.Rows) != 2 {
		t.Errorf("Rows: want 2, got %d", len(s.TableItem.Rows))
	}
	if s.Text == "" {
		t.Error("Text: expected rendered HTML, got empty string")
	}
}

func TestBlocksToSections_EmptyInput(t *testing.T) {
	sections := blocksToSections(nil)
	if len(sections) != 0 {
		t.Errorf("want 0 sections, got %d", len(sections))
	}
}

func TestBlocksToSections_DocumentOrder(t *testing.T) {
	blocks := []RawBlock{
		{Type: "paragraph", Text: "first", Style: "Normal"},
		{Type: "table", Rows: [][]string{{"a"}}},
		{Type: "paragraph", Text: "second", Style: "Normal"},
		{Type: "paragraph", Text: "third", Style: "Heading 1"},
	}
	sections := blocksToSections(blocks)

	if len(sections) != 4 {
		t.Fatalf("want 4 sections, got %d", len(sections))
	}
	if sections[0].Text != "first" {
		t.Errorf("order[0]: got %q", sections[0].Text)
	}
	if sections[1].DocTypeKwd != "table" {
		t.Errorf("order[1]: expected table")
	}
	if sections[2].Text != "second" {
		t.Errorf("order[2]: got %q", sections[2].Text)
	}
	if sections[3].Text != "third" {
		t.Errorf("order[3]: got %q", sections[3].Text)
	}
}

func TestBlocksToSections_CaptionStyle(t *testing.T) {
	blocks := []RawBlock{
		{Type: "paragraph", Text: "Table 1: Results", Style: "Caption"},
	}
	sections := blocksToSections(blocks)
	if len(sections) != 1 {
		t.Fatalf("want 1 section, got %d", len(sections))
	}
	if sections[0].LayoutType != "text" {
		t.Errorf("Caption: LayoutType should be 'text', got %q", sections[0].LayoutType)
	}
}

func TestBlocksToSections_MixedContent(t *testing.T) {
	blocks := []RawBlock{
		{Type: "paragraph", Text: "Title", Style: "Heading 1"},
		{Type: "paragraph", Text: "Body text.", Style: "Normal"},
		{Type: "table", Rows: [][]string{{"a", "b"}}},
		{Type: "paragraph", Text: "More text.", Style: "Normal"},
	}
	sections := blocksToSections(blocks)

	if len(sections) != 4 {
		t.Fatalf("want 4 sections, got %d", len(sections))
	}
	if sections[0].LayoutType != "title" {
		t.Errorf("[0] heading: got %q", sections[0].LayoutType)
	}
	if sections[1].LayoutType != "text" {
		t.Errorf("[1] body: got %q", sections[1].LayoutType)
	}
	if sections[2].DocTypeKwd != "table" {
		t.Errorf("[2] table: got %q", sections[2].DocTypeKwd)
	}
	if sections[3].DocTypeKwd != "text" {
		t.Errorf("[3] text after table: got %q", sections[3].DocTypeKwd)
	}
}

func TestBlocksToSections_Image(t *testing.T) {
	blocks := []RawBlock{
		{Type: "image", Image: "iVBORw0KGgoAAAANSUhEUg=="},
	}
	sections := blocksToSections(blocks)

	if len(sections) != 1 {
		t.Fatalf("want 1 section, got %d", len(sections))
	}
	if sections[0].DocTypeKwd != "image" {
		t.Errorf("DocTypeKwd: got %q, want %q", sections[0].DocTypeKwd, "image")
	}
	if sections[0].Image != "iVBORw0KGgoAAAANSUhEUg==" {
		t.Error("Image base64 not preserved")
	}
}

func TestBlocksToSections_ImageBetweenText(t *testing.T) {
	blocks := []RawBlock{
		{Type: "paragraph", Text: "before", Style: "Normal"},
		{Type: "image", Image: "b64data"},
		{Type: "paragraph", Text: "after", Style: "Normal"},
	}
	sections := blocksToSections(blocks)

	if len(sections) != 3 {
		t.Fatalf("want 3 sections, got %d", len(sections))
	}
	if sections[0].DocTypeKwd != "text" || sections[0].Text != "before" {
		t.Error("wrong text section before image")
	}
	if sections[1].DocTypeKwd != "image" {
		t.Errorf("image section: got DocTypeKwd %q", sections[1].DocTypeKwd)
	}
	if sections[2].DocTypeKwd != "text" || sections[2].Text != "after" {
		t.Error("wrong text section after image")
	}
}

func TestBlocksToSections_NestedHeadings(t *testing.T) {
	blocks := []RawBlock{
		{Type: "paragraph", Text: "H1", Style: "Heading 1"},
		{Type: "paragraph", Text: "H2", Style: "Heading 2"},
		{Type: "paragraph", Text: "H3", Style: "Heading 3"},
	}
	sections := blocksToSections(blocks)
	for i, want := range []string{"title", "title", "title"} {
		if sections[i].LayoutType != want {
			t.Errorf("[%d] got %q, want %q", i, sections[i].LayoutType, want)
		}
	}
}
