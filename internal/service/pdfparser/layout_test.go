package pdfparser

import (
	"strings"
	"testing"
)

func TestAssignColumn(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, X0: 50, Text: "col0-left"},
		{PageNumber: 0, X0: 55, Text: "col0-mid"},
		{PageNumber: 0, X0: 400, Text: "col1"},
		{PageNumber: 1, X0: 50, Text: "pg1-col0"},
	}
	result := AssignColumn(boxes, 3)
	if len(result) != 4 {
		t.Fatal("expected 4 boxes")
	}
	if result[0].ColID != result[1].ColID {
		t.Error("boxes 0 and 1 (close x0) should be same column")
	}
	if result[0].ColID == result[2].ColID {
		t.Error("boxes 0 and 2 (far apart) should be different columns")
	}
}

func TestTextMerge(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, ColID: 0, X0: 50, X1: 250, Top: 100, Bottom: 112, Text: "左半", LayoutType: "text", LayoutNo: "1"},
		{PageNumber: 0, ColID: 0, X0: 252, X1: 550, Top: 100, Bottom: 112, Text: "右半", LayoutType: "text", LayoutNo: "1"},
	}
	meanH := map[int]float64{0: 12}
	result := TextMerge(boxes, meanH, 3)
	if len(result) != 1 {
		t.Errorf("expected 1 merged box, got %d", len(result))
	}
}

func TestTextMergeNoMerge_DiffLayout(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, ColID: 0, X0: 50, X1: 250, Top: 100, Bottom: 112, Text: "text", LayoutType: "text", LayoutNo: "1"},
		{PageNumber: 0, ColID: 0, X0: 252, X1: 550, Top: 100, Bottom: 112, Text: "table", LayoutType: "table", LayoutNo: "2"},
	}
	meanH := map[int]float64{0: 12}
	result := TextMerge(boxes, meanH, 3)
	if len(result) != 2 {
		t.Error("table and text should not merge")
	}
}

func TestFinalReadingOrderMerge(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 1, ColID: 1, Top: 50, Text: "pg1-col1"},
		{PageNumber: 0, ColID: 0, Top: 100, Text: "pg0-col0"},
		{PageNumber: 0, ColID: 0, Top: 50, Text: "pg0-col0-top"},
	}
	result := FinalReadingOrderMerge(boxes)
	if result[0].Text != "pg0-col0-top" {
		t.Errorf("first should be pg0-col0-top: %q", result[0].Text)
	}
	if result[2].Text != "pg1-col1" {
		t.Errorf("last should be pg1-col1: %q", result[2].Text)
	}
}


func TestContainsRune(t *testing.T) {
	if !containsRune("。？！", '。') {
		t.Error("should find 。")
	}
	if containsRune("abc", 'z') {
		t.Error("should not find z")
	}
}

func TestEndsWithOneOf(t *testing.T) {
	if !endsWithOneOf("句子结束。", "。？！?") {
		t.Error("should match 。")
	}
	if endsWithOneOf("no match", "。？！?") {
		t.Error("should not match")
	}
}

func TestCharsToBoxes(t *testing.T) {
	p := &Parser{Config: DefaultConfig()}
	chars := []TextChar{
		{X0: 50, X1: 58, Top: 100, Bottom: 112, Text: "A", PageNumber: 0},
		{X0: 60, X1: 68, Top: 100, Bottom: 112, Text: "B", PageNumber: 0},
		{X0: 50, X1: 58, Top: 114, Bottom: 126, Text: "C", PageNumber: 0},
	}
	boxes := p.charsToBoxes(chars, 0)
	if len(boxes) == 0 {
		t.Fatal("expected at least 1 box")
	}
	// A and B should be in the same line, C in a different line
	if len(boxes) != 2 {
		t.Errorf("expected 2 lines, got %d", len(boxes))
	}
}

func TestBoxesToSections(t *testing.T) {
	p := &Parser{Config: DefaultConfig()}
	boxes := []TextBox{
		{PageNumber: 0, X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "标题"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 200, Bottom: 212, Text: ""},
	}
	sections := p.boxesToSections(boxes)
	if len(sections) != 1 {
		t.Errorf("expected 1 section (empty box skipped), got %d", len(sections))
	}
	if len(sections) > 0 {
		// Text is clean — position tag lives in PositionTag field (matching Python)
		if strings.Contains(sections[0].Text, "@@") {
			t.Error("section text should NOT contain position tag")
		}
		if !strings.Contains(sections[0].PositionTag, "##") {
			t.Error("position tag should end with ##")
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Zoom != 3 {
		t.Error("default zoom should be 3")
	}
	if cfg.ToPage != -1 {
		t.Error("default to_page should be -1")
	}
}

func TestHasColor(t *testing.T) {
	if !HasColor(TextChar{}) {
		t.Error("HasColor should return true by default")
	}
}
