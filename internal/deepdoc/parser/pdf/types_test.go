package parser

import (
	"testing"
)

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
			t.Errorf("first figure: expected (fig1, img1), got (%s, %s)", figures[0].Text, figures[0].Image)
		}
		if figures[1].Text != "fig2" || figures[1].Image != "img2" {
			t.Errorf("second figure: expected (fig2, img2), got (%s, %s)", figures[1].Text, figures[1].Image)
		}
	})

	t.Run("no figures", func(t *testing.T) {
		sections := []Section{
			{LayoutType: "text", Text: "text1"},
			{LayoutType: "table", Text: "tbl1"},
			{LayoutType: "title", Text: "title1"},
		}
		figures := CollectFigures(sections)
		if len(figures) != 0 {
			t.Fatalf("expected 0 figures, got %d", len(figures))
		}
	})

	t.Run("nil input", func(t *testing.T) {
		figures := CollectFigures(nil)
		if figures != nil {
			t.Fatalf("expected nil for nil input, got %d elements", len(figures))
		}
	})

	t.Run("empty input", func(t *testing.T) {
		figures := CollectFigures([]Section{})
		if figures == nil {
			t.Fatal("expected empty slice (not nil) for empty input")
		}
		if len(figures) != 0 {
			t.Fatalf("expected 0 figures, got %d", len(figures))
		}
	})

	t.Run("all figures", func(t *testing.T) {
		sections := []Section{
			{LayoutType: "figure", Text: "fig1"},
			{LayoutType: "figure", Text: "fig2"},
			{LayoutType: "figure", Text: "fig3"},
		}
		figures := CollectFigures(sections)
		if len(figures) != 3 {
			t.Fatalf("expected 3 figures, got %d", len(figures))
		}
	})

	t.Run("figure with empty image", func(t *testing.T) {
		sections := []Section{
			{LayoutType: "figure", Text: "fig1", Image: ""},
			{LayoutType: "figure", Text: "fig2", Image: "img2"},
		}
		figures := CollectFigures(sections)
		if len(figures) != 2 {
			t.Fatalf("expected 2 figures, got %d", len(figures))
		}
		// Figure with empty image is still collected — downstream should handle.
		if figures[0].Image != "" {
			t.Errorf("first figure: expected empty Image, got %s", figures[0].Image)
		}
	})

	t.Run("single section, figure", func(t *testing.T) {
		figures := CollectFigures([]Section{
			{LayoutType: "figure", Text: "only", Image: "img"},
		})
		if len(figures) != 1 {
			t.Fatalf("expected 1 figure, got %d", len(figures))
		}
	})

	t.Run("single section, not figure", func(t *testing.T) {
		figures := CollectFigures([]Section{
			{LayoutType: "text", Text: "only"},
		})
		if len(figures) != 0 {
			t.Fatalf("expected 0 figures, got %d", len(figures))
		}
	})

	t.Run("case sensitive", func(t *testing.T) {
		sections := []Section{
			{LayoutType: "Figure", Text: "fig1"},
			{LayoutType: "FIGURE", Text: "fig2"},
			{LayoutType: "figure", Text: "fig3"},
		}
		figures := CollectFigures(sections)
		if len(figures) != 1 {
			t.Fatalf("only lowercase 'figure' should match, got %d", len(figures))
		}
		if figures[0].Text != "fig3" {
			t.Errorf("expected fig3, got %s", figures[0].Text)
		}
	})
}
