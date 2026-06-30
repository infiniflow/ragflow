package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestCharsToBoxes(t *testing.T) {
	t.Run("empty chars", func(t *testing.T) {
		if boxes := CharsToBoxes(nil, 0, false); boxes != nil {
			t.Error("nil chars → nil boxes")
		}
		if boxes := CharsToBoxes([]pdf.TextChar{}, 0, false); boxes != nil {
			t.Error("empty chars → nil boxes")
		}
	})
	t.Run("single char", func(t *testing.T) {
		chars := []pdf.TextChar{
			{X0: 50, X1: 58, Top: 100, Bottom: 112, Text: "A", PageNumber: 0},
		}
		boxes := CharsToBoxes(chars, 0, false)
		if len(boxes) != 1 {
			t.Fatalf("expected 1 box, got %d", len(boxes))
		}
		if boxes[0].Text != "A" {
			t.Errorf("Text = %q, want 'A'", boxes[0].Text)
		}
	})
	t.Run("two lines", func(t *testing.T) {
		chars := []pdf.TextChar{
			{X0: 50, X1: 58, Top: 100, Bottom: 112, Text: "A", PageNumber: 0},
			{X0: 60, X1: 68, Top: 100, Bottom: 112, Text: "B", PageNumber: 0},
			{X0: 50, X1: 58, Top: 114, Bottom: 126, Text: "C", PageNumber: 0},
		}
		boxes := CharsToBoxes(chars, 0, false)
		if len(boxes) != 2 {
			t.Errorf("expected 2 lines, got %d", len(boxes))
		}
	})
	t.Run("preserves whitespace lines", func(t *testing.T) {
		chars := []pdf.TextChar{
			{Text: " ", X0: 10, Top: 100, X1: 15, Bottom: 112},
			{Text: "Hello", X0: 10, Top: 120, X1: 50, Bottom: 132},
		}
		boxes := CharsToBoxes(chars, 0, false)
		if len(boxes) != 2 {
			t.Errorf("expected 2 boxes (whitespace preserved), got %d", len(boxes))
		}
	})
}

func TestGroupCharsToLines(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if lines := GroupCharsToLines(nil, false); lines != nil {
			t.Error("nil → nil")
		}
	})
	t.Run("single char", func(t *testing.T) {
		chars := []pdf.TextChar{{X0: 50, X1: 58, Top: 100, Bottom: 112, Text: "A"}}
		lines := GroupCharsToLines(chars, false)
		if len(lines) != 1 || len(lines[0]) != 1 {
			t.Error("single char → single line")
		}
	})
	t.Run("multi column same line", func(t *testing.T) {
		chars := []pdf.TextChar{
			{X0: 50, X1: 58, Top: 100, Bottom: 112, Text: "H"},
			{X0: 60, X1: 68, Top: 100, Bottom: 112, Text: "i"},
			{X0: 300, X1: 308, Top: 100, Bottom: 112, Text: "B"},
			{X0: 50, X1: 58, Top: 114, Bottom: 126, Text: "A"},
		}
		lines := GroupCharsToLines(chars, false)
		if len(lines) != 2 {
			t.Errorf("expected 2 lines, got %d", len(lines))
		}
	})
}

func TestLineToTextBox(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		chars := []pdf.TextChar{
			{X0: 50, X1: 58, Top: 100, Bottom: 112, Text: "H"},
			{X0: 60, X1: 68, Top: 100, Bottom: 112, Text: "i"},
		}
		box := LineToTextBox(chars)
		if box.Text != "Hi" {
			t.Errorf("Text = %q, want 'Hi'", box.Text)
		}
		if box.X0 != 50 || box.X1 != 68 {
			t.Errorf("bbox = [%f, %f], want [50, 68]", box.X0, box.X1)
		}
	})
	t.Run("empty chars", func(t *testing.T) {
		box := LineToTextBox(nil)
		if box.Text != "" {
			t.Errorf("nil chars → empty box, got %q", box.Text)
		}
	})
	t.Run("single char", func(t *testing.T) {
		chars := []pdf.TextChar{{X0: 10, X1: 18, Top: 100, Bottom: 112, Text: "X"}}
		box := LineToTextBox(chars)
		if box.Text != "X" {
			t.Errorf("Text = %q, want 'X'", box.Text)
		}
	})
}
