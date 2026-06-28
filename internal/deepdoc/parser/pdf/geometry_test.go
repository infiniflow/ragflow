package parser

import (
	"strings"
	"testing"
)

func TestCharWidth(t *testing.T) {
	c := TextChar{X0: 50, X1: 58, Text: "A"}
	if w := CharWidth(c); w != 8.0 {
		t.Errorf("CharWidth = %v, want 8.0", w)
	}

	c2 := TextChar{X0: 50, X1: 70, Text: "hi"}
	if w := CharWidth(c2); w != 10.0 {
		t.Errorf("CharWidth = %v, want 10.0", w)
	}

	c3 := TextChar{X0: 50, X1: 50, Text: ""}
	if w := CharWidth(c3); w != 0 {
		t.Errorf("CharWidth empty = %v, want 0", w)
	}
}

func TestCharHeight(t *testing.T) {
	c := TextChar{Top: 200, Bottom: 212}
	if h := CharHeight(c); h != 12.0 {
		t.Errorf("CharHeight = %v, want 8.0", h)
	}
}

func TestXDis(t *testing.T) {
	a := TextChar{X0: 50, X1: 58}
	b := TextChar{X0: 60, X1: 68}
	d := XDis(a, b)
	expected := 2.0 // min(|58-60|=2, |50-68|=18, |108-128|/2=10)
	if d != expected {
		t.Errorf("XDis = %v, want %v", d, expected)
	}
}

func TestYDis(t *testing.T) {
	a := TextChar{Top: 100, Bottom: 112}
	b := TextChar{Top: 114, Bottom: 126}
	d := YDis(a, b)
	expected := (114.0 + 126.0 - 100.0 - 112.0) / 2 // 14
	if d != expected {
		t.Errorf("YDis = %v, want %v", d, expected)
	}
}

func TestSortXByPage(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 1, X0: 100, Top: 50, Text: "C"},
		{PageNumber: 1, X0: 50, Top: 100, Text: "A"},
		{PageNumber: 1, X0: 50, Top: 30, Text: "B"},
		{PageNumber: 0, X0: 0, Top: 0, Text: "D"},
	}
	result := SortXByPage(boxes, 3)
	if result[0].Text != "D" {
		t.Errorf("first should be page 0: got %q", result[0].Text)
	}
	if result[1].Text != "B" || result[2].Text != "A" {
		t.Errorf("page 1 ordering wrong: %q, %q", result[1].Text, result[2].Text)
	}
}

func TestOverlapX(t *testing.T) {
	b1 := TextBox{X0: 50, X1: 200}
	b2 := TextBox{X0: 100, X1: 250}
	overlap := OverlapX(&b1, &b2)
	if overlap <= 0.5 || overlap >= 0.8 {
		t.Errorf("OverlapX = %v, want ~0.667", overlap)
	}

	b3 := TextBox{X0: 50, X1: 100}
	b4 := TextBox{X0: 200, X1: 250}
	if overlap := OverlapX(&b3, &b4); overlap != 0 {
		t.Errorf("non-overlapping should be 0: got %v", overlap)
	}
}

func TestMedianCharHeight(t *testing.T) {
	chars := []TextChar{
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
	boxes := []TextBox{
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

func TestNaiveVerticalMerge(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, ColID: 0, X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "第一段", LayoutNo: "1", LayoutType: "text"},
		{PageNumber: 0, ColID: 0, X0: 50, X1: 550, Top: 114, Bottom: 126, Text: "续文", LayoutNo: "1", LayoutType: "text"},
	}
	meanH := map[int]float64{0: 12}
	meanW := map[int]float64{0: 5}
	result := NaiveVerticalMerge(boxes, meanH, meanW, false)
	// These should merge: small vertical gap, overlapping horizontally, same layout
	if len(result) != 1 {
		t.Errorf("expected 1 merged box, got %d: %v", len(result), result)
	}
	if len(result) > 0 && !strings.Contains(result[0].Text, "第一段") {
		t.Errorf("merged text should contain '第一段': got %q", result[0].Text)
	}
}

func TestNaiveVerticalMergeNonMerge(t *testing.T) {
	// Large gap — should not merge
	boxes := []TextBox{
		{PageNumber: 0, ColID: 0, X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "第一段。", LayoutNo: "1", LayoutType: "text"},
		{PageNumber: 0, ColID: 0, X0: 50, X1: 550, Top: 300, Bottom: 312, Text: "第二段。", LayoutNo: "1", LayoutType: "text"},
	}
	meanH := map[int]float64{0: 12}
	meanW := map[int]float64{0: 5}
	result := NaiveVerticalMerge(boxes, meanH, meanW, false)
	if len(result) != 2 {
		t.Errorf("expected 2 separate boxes (large gap), got %d", len(result))
	}
}

func TestBoxWidth(t *testing.T) {
	b := TextBox{X0: 50, X1: 200}
	if w := BoxWidth(b); w != 150 {
		t.Errorf("BoxWidth = %v, want 150", w)
	}
}

func TestBoxHeight(t *testing.T) {
	b := TextBox{Top: 100, Bottom: 130}
	if h := BoxHeight(b); h != 30 {
		t.Errorf("BoxHeight = %v, want 30", h)
	}
}

func TestBoxXDis(t *testing.T) {
	b1 := TextBox{X0: 50, X1: 100}
	b2 := TextBox{X0: 110, X1: 200}
	if d := BoxXDis(b1, b2); d != 10 {
		t.Errorf("BoxXDis = %v, want 10", d)
	}
}

func TestBoxYDis(t *testing.T) {
	b1 := TextBox{Top: 100, Bottom: 112}
	b2 := TextBox{Top: 114, Bottom: 126}
	d := BoxYDis(b1, b2)
	expected := (114.0 + 126.0 - 100.0 - 112.0) / 2
	if d != expected {
		t.Errorf("BoxYDis = %v, want %v", d, expected)
	}
}

func TestMedianCharWidth(t *testing.T) {
	chars := []TextChar{
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
