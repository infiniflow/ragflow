package util

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

func TestCharWidth(t *testing.T) {
	c := pdf.TextChar{X0: 50, X1: 58, Text: "A"}
	if w := CharWidth(c); w != 8.0 {
		t.Errorf("CharWidth = %v, want 8.0", w)
	}

	c2 := pdf.TextChar{X0: 50, X1: 70, Text: "hi"}
	if w := CharWidth(c2); w != 10.0 {
		t.Errorf("CharWidth = %v, want 10.0", w)
	}

	c3 := pdf.TextChar{X0: 50, X1: 50, Text: ""}
	if w := CharWidth(c3); w != 0 {
		t.Errorf("CharWidth empty = %v, want 0", w)
	}
}

func TestCharHeight(t *testing.T) {
	c := pdf.TextChar{Top: 200, Bottom: 212}
	if h := CharHeight(c); h != 12.0 {
		t.Errorf("CharHeight = %v, want 8.0", h)
	}
}

func TestXDis(t *testing.T) {
	a := pdf.TextChar{X0: 50, X1: 58}
	b := pdf.TextChar{X0: 60, X1: 68}
	d := XDis(a, b)
	expected := 2.0 // min(|58-60|=2, |50-68|=18, |108-128|/2=10)
	if d != expected {
		t.Errorf("XDis = %v, want %v", d, expected)
	}
}

func TestYDis(t *testing.T) {
	a := pdf.TextChar{Top: 100, Bottom: 112}
	b := pdf.TextChar{Top: 114, Bottom: 126}
	d := YDis(a, b)
	expected := (114.0 + 126.0 - 100.0 - 112.0) / 2 // 14
	if d != expected {
		t.Errorf("YDis = %v, want %v", d, expected)
	}
}

func TestOverlapX(t *testing.T) {
	b1 := pdf.TextBox{X0: 50, X1: 200}
	b2 := pdf.TextBox{X0: 100, X1: 250}
	overlap := OverlapX(&b1, &b2)
	if overlap <= 0.5 || overlap >= 0.8 {
		t.Errorf("OverlapX = %v, want ~0.667", overlap)
	}

	b3 := pdf.TextBox{X0: 50, X1: 100}
	b4 := pdf.TextBox{X0: 200, X1: 250}
	if overlap := OverlapX(&b3, &b4); overlap != 0 {
		t.Errorf("non-overlapping should be 0: got %v", overlap)
	}
}

func TestMedianCharHeight(t *testing.T) {
	chars := []pdf.TextChar{
		{Top: 0, Bottom: 10},
		{Top: 0, Bottom: 20},
	}
	h := MedianCharHeight(chars)
	if h != 15.0 {
		t.Errorf("MedianCharHeight = %v, want 15.0", h)
	}
	if h2 := MedianCharHeight(nil); h2 != 10.0 {
		t.Errorf("MedianCharHeight(empty) = %v, want 10.0", h2)
	}
}

func TestMedianHeight(t *testing.T) {
	boxes := []pdf.TextBox{
		{Top: 0, Bottom: 10},
		{Top: 0, Bottom: 20},
		{Top: 0, Bottom: 30},
	}
	if mh := MedianHeight(boxes); mh != 20.0 {
		t.Errorf("MedianHeight = %v, want 20.0", mh)
	}
	if mh2 := MedianHeight(nil); mh2 != 10.0 {
		t.Errorf("MedianHeight(empty) = %v, want 10.0", mh2)
	}
}

func TestBoxWidth(t *testing.T) {
	b := pdf.TextBox{X0: 50, X1: 200}
	if w := BoxWidth(b); w != 150 {
		t.Errorf("BoxWidth = %v, want 150", w)
	}
}

func TestBoxHeight(t *testing.T) {
	b := pdf.TextBox{Top: 100, Bottom: 130}
	if h := BoxHeight(b); h != 30 {
		t.Errorf("BoxHeight = %v, want 30", h)
	}
}

func TestBoxXDis(t *testing.T) {
	b1 := pdf.TextBox{X0: 50, X1: 100}
	b2 := pdf.TextBox{X0: 110, X1: 200}
	if d := BoxXDis(b1, b2); d != 10 {
		t.Errorf("BoxXDis = %v, want 10", d)
	}
}

func TestBoxYDis(t *testing.T) {
	b1 := pdf.TextBox{Top: 100, Bottom: 112}
	b2 := pdf.TextBox{Top: 114, Bottom: 126}
	d := BoxYDis(b1, b2)
	expected := (114.0 + 126.0 - 100.0 - 112.0) / 2
	if d != expected {
		t.Errorf("BoxYDis = %v, want %v", d, expected)
	}
}

func TestMedianCharWidth(t *testing.T) {
	chars := []pdf.TextChar{
		{X0: 0, X1: 8, Text: "A"},
		{X0: 0, X1: 16, Text: "AB"},
	}
	if w := MedianCharWidth(chars); w != 8 {
		t.Errorf("MedianCharWidth = %v, want 8", w)
	}
	if w := MedianCharWidth(nil); w != 5 {
		t.Errorf("MedianCharWidth(empty) = %v, want 5", w)
	}
}

// textBox implements Rectangular for testing.
type textBox struct{ x0, y0, x1, y1 float64 }

func (b textBox) Bounds() (float64, float64, float64, float64) {
	return b.x0, b.y0, b.x1, b.y1
}

func TestArea(t *testing.T) {
	tests := []struct {
		name string
		r    pdf.Rectangular
		want float64
	}{
		{"normal", textBox{0, 0, 10, 20}, 200},
		{"zero width", textBox{5, 0, 5, 10}, 0},
		{"zero height", textBox{0, 5, 10, 5}, 0},
		{"degenerate", textBox{10, 10, 5, 5}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Area(tt.r); got != tt.want {
				t.Errorf("Area = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOverlapInter(t *testing.T) {
	tests := []struct {
		name string
		a, b pdf.Rectangular
		want float64
	}{
		{"full overlap", textBox{0, 0, 10, 10}, textBox{0, 0, 10, 10}, 100},
		{"partial", textBox{0, 0, 10, 10}, textBox{5, 5, 15, 15}, 25},
		{"no overlap", textBox{0, 0, 10, 10}, textBox{20, 20, 30, 30}, 0},
		{"edge touching", textBox{0, 0, 10, 10}, textBox{10, 0, 20, 10}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OverlapInter(tt.a, tt.b); got != tt.want {
				t.Errorf("OverlapInter = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOverlapRatioA(t *testing.T) {
	a := textBox{0, 0, 10, 10} // area = 100
	b := textBox{5, 5, 15, 15} // overlap = 25
	if got := OverlapRatioA(a, b); got != 0.25 {
		t.Errorf("OverlapRatioA = %v, want 0.25", got)
	}
	// no overlap
	c := textBox{0, 0, 10, 10}
	d := textBox{20, 20, 30, 30}
	if got := OverlapRatioA(c, d); got != 0 {
		t.Errorf("OverlapRatioA no overlap = %v, want 0", got)
	}
}
