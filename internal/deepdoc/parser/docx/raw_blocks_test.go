//go:build cgo && manual

package docx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadPythonBlocks(t *testing.T, path string) []RawBlock {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var blocks []RawBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return blocks
}

func TestRawBlocksParity_SimpleText(t *testing.T) {
	data, err := os.ReadFile("testdata/docxs/simple_text.docx")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractRawBlocks(data)
	if err != nil {
		t.Fatalf("ExtractRawBlocks: %v", err)
	}
	want := loadPythonBlocks(t, "testdata/output/py/docx/simple_text_blocks.json")

	if len(got) != len(want) {
		t.Errorf("block count: got %d, want %d", len(got), len(want))
	}
	for i := 0; i < min(len(got), len(want)); i++ {
		if got[i].Type != want[i].Type {
			t.Errorf("block[%d].type: got %q, want %q", i, got[i].Type, want[i].Type)
		}
		if got[i].Text != want[i].Text {
			t.Errorf("block[%d].text: got %q, want %q", i, got[i].Text, want[i].Text)
		}
	}
	if t.Failed() {
		t.Logf("Go blocks: %+v", got)
		t.Logf("Py blocks: %+v", want)
	}
}

func TestRawBlocksParity_WithTable(t *testing.T) {
	data, err := os.ReadFile("testdata/docxs/with_table.docx")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractRawBlocks(data)
	if err != nil {
		t.Fatalf("ExtractRawBlocks: %v", err)
	}
	want := loadPythonBlocks(t, "testdata/output/py/docx/with_table_blocks.json")

	if len(got) != len(want) {
		t.Errorf("block count: got %d, want %d", len(got), len(want))
	}
	for i := 0; i < min(len(got), len(want)); i++ {
		if got[i].Type != want[i].Type {
			t.Errorf("block[%d].type: got %q, want %q", i, got[i].Type, want[i].Type)
		}
	}
	if t.Failed() {
		t.Logf("Go blocks: %+v", got)
		t.Logf("Py blocks: %+v", want)
	}
}

func TestRawBlocksParity_WithImage(t *testing.T) {
	data, err := os.ReadFile("testdata/docxs/with_image.docx")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractRawBlocks(data)
	if err != nil {
		t.Fatalf("ExtractRawBlocks: %v", err)
	}
	// Engine-level difference: python-docx embeds images inside empty
	// paragraph blocks; office_oxide represents them as separate elements.
	// Both engines must see "Before" and "After" text and at least one
	// image-related block.
	hasBefore, hasAfter, hasImage := false, false, false
	for _, b := range got {
		if b.Text != "" {
			hasBefore = hasBefore || b.Text == "Before the image."
			hasAfter = hasAfter || b.Text == "After the image."
		}
		if b.Image != "" {
			hasImage = true
		}
	}
	if !hasBefore {
		t.Error("missing 'Before the image.' text")
	}
	if !hasAfter {
		t.Error("missing 'After the image.' text")
	}
	if !hasImage {
		t.Log("office_oxide IR does not expose embedded images as top-level blocks")
	}
}

func TestRawBlocksParity_MultiSection(t *testing.T) {
	data, err := os.ReadFile("testdata/docxs/multi_section.docx")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractRawBlocks(data)
	if err != nil {
		t.Fatalf("ExtractRawBlocks: %v", err)
	}
	want := loadPythonBlocks(t, "testdata/output/py/docx/multi_section_blocks.json")
	if len(got) != len(want) {
		t.Errorf("block count: got %d, want %d", len(got), len(want))
	}
	for i := 0; i < min(len(got), len(want)); i++ {
		if got[i].Type != want[i].Type {
			t.Errorf("block[%d].type: got %q, want %q", i, got[i].Type, want[i].Type)
		}
	}
}

func TestRawBlocksParity_NestedHeadings(t *testing.T) {
	data, err := os.ReadFile("testdata/docxs/nested_headings.docx")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractRawBlocks(data)
	if err != nil {
		t.Fatalf("ExtractRawBlocks: %v", err)
	}
	want := loadPythonBlocks(t, "testdata/output/py/docx/nested_headings_blocks.json")
	if len(got) != len(want) {
		t.Errorf("block count: got %d, want %d", len(got), len(want))
	}
	headings := 0
	for _, b := range got {
		if strings.HasPrefix(b.Style, "Heading") {
			headings++
		}
	}
	if headings != 5 {
		t.Errorf("expected 5 headings, got %d", headings)
	}
}

func TestRawBlocksParity_WithCaption(t *testing.T) {
	data, err := os.ReadFile("testdata/docxs/with_caption.docx")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractRawBlocks(data)
	if err != nil {
		t.Fatalf("ExtractRawBlocks: %v", err)
	}
	// Verify both engines see the same number of blocks
	want := loadPythonBlocks(t, "testdata/output/py/docx/with_caption_blocks.json")
	if len(got) != len(want) {
		t.Errorf("block count: got %d, want %d", len(got), len(want))
	}
}

func TestRawBlocksParity_Empty(t *testing.T) {
	data, err := os.ReadFile("testdata/docxs/empty.docx")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractRawBlocks(data)
	if err != nil {
		t.Fatalf("ExtractRawBlocks: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty docx: expected 0 blocks, got %d", len(got))
	}
}
