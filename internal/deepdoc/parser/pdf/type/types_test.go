package pdftype

import (
	"testing"
)

func TestIsCJK(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{"chinese", '中', true},
		{"chinese2", '国', true},
		{"hiragana", 'あ', true},
		{"katakana", 'ア', true},
		{"hangul", '한', true},
		{"latin", 'A', false},
		{"digit", '1', false},
		{"space", ' ', false},
		{"punctuation", '.', false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCJK(tt.r); got != tt.want {
				t.Errorf("IsCJK(%q) = %v, want %v", tt.r, got, tt.want)
			}
		})
	}
}

func TestCollectFigures(t *testing.T) {
	t.Run("mixed layout types", func(t *testing.T) {
		sections := []Section{
			{LayoutType: "figure", Text: "fig1", Image: "img1"},
			{LayoutType: "text", Text: "text1"},
			{LayoutType: "table", Text: "tbl1"},
			{LayoutType: "figure", Text: "fig2", Image: "img2"},
			{LayoutType: "title", Text: "title1"},
		}
		figures := CollectFigures(sections)
		if len(figures) != 2 {
			t.Fatalf("expected 2 figures, got %d", len(figures))
		}
		if figures[0].Text != "fig1" || figures[0].Image != "img1" {
			t.Errorf("first figure: got (%s, %s)", figures[0].Text, figures[0].Image)
		}
		if figures[1].Text != "fig2" || figures[1].Image != "img2" {
			t.Errorf("second figure: got (%s, %s)", figures[1].Text, figures[1].Image)
		}
	})
	t.Run("no figures", func(t *testing.T) {
		figures := CollectFigures([]Section{
			{LayoutType: "text"}, {LayoutType: "table"}, {LayoutType: "title"},
		})
		if len(figures) != 0 {
			t.Fatalf("expected 0, got %d", len(figures))
		}
	})
	t.Run("nil input", func(t *testing.T) {
		if figures := CollectFigures(nil); figures != nil {
			t.Fatalf("expected nil, got %d elements", len(figures))
		}
	})
	t.Run("empty input", func(t *testing.T) {
		figures := CollectFigures([]Section{})
		if figures == nil || len(figures) != 0 {
			t.Fatal("expected empty slice for empty input")
		}
	})
	t.Run("case sensitive", func(t *testing.T) {
		figures := CollectFigures([]Section{
			{LayoutType: "Figure"}, {LayoutType: "FIGURE"}, {LayoutType: "figure", Text: "fig3"},
		})
		if len(figures) != 1 || figures[0].Text != "fig3" {
			t.Fatalf("only lowercase 'figure' should match, got %d", len(figures))
		}
	})
}

// textBox implements Rectangular for testing.
type textBox struct{ x0, y0, x1, y1 float64 }

func (b textBox) Bounds() (float64, float64, float64, float64) {
	return b.x0, b.y0, b.x1, b.y1
}
