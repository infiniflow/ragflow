package parser

import (
	"context"
	"image"
	"testing"
)

// ── NormalizeLayoutType ────────────────────────────────────────────────

func TestNormalizeLayoutType(t *testing.T) {
	boxes := Boxes{
		{LayoutType: ""},
		{LayoutType: "  "},
		{LayoutType: "table"},
		{LayoutType: "  figure  "},
		{LayoutType: "text"},
	}
	NormalizeLayoutType(boxes)
	want := []string{"text", "text", "table", "figure", "text"}
	for i, b := range boxes {
		if b.LayoutType != want[i] {
			t.Errorf("boxes[%d]: got %q, want %q", i, b.LayoutType, want[i])
		}
	}
}

// ── FilterHeaderFooter ─────────────────────────────────────────────────

func TestFilterHeaderFooter(t *testing.T) {
	boxes := Boxes{
		{Text: "Page 1", LayoutType: "header"},
		{Text: "Chapter 1", LayoutType: "text"},
		{LayoutType: "footer"},
		{LayoutType: "number"},
		{Text: "Body", LayoutType: "text"},
		{Text: "reference item", LayoutType: "reference"},
	}
	result := FilterHeaderFooter(boxes)
	if len(result) != 2 {
		t.Errorf("expected 2 boxes, got %d: %+v", len(result), result)
	}
	if result[0].Text != "Chapter 1" || result[1].Text != "Body" {
		t.Errorf("wrong boxes kept: %+v", result)
	}
}

func TestFilterHeaderFooter_Empty(t *testing.T) {
	result := FilterHeaderFooter(Boxes{})
	if len(result) != 0 {
		t.Error("expected empty result")
	}
}

// ── AssignDocTypeKwd ───────────────────────────────────────────────────

func TestAssignDocTypeKwd(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	boxes := Boxes{
		{LayoutType: "table"},
		{LayoutType: "figure"},
		{LayoutType: "equation"},
		{LayoutType: "", Image: img},         // no layout but has image
		{LayoutType: "text"},
		{LayoutType: "", Image: nil},          // no layout, no image
	}
	AssignDocTypeKwd(boxes)
	want := []string{"table", "image", "text", "image", "text", "text"}
	for i, b := range boxes {
		if b.DocTypeKwd != want[i] {
			t.Errorf("boxes[%d]: got %q, want %q", i, b.DocTypeKwd, want[i])
		}
	}
}

// ── FlattenMediaToText ─────────────────────────────────────────────────

func TestFlattenMediaToText(t *testing.T) {
	boxes := Boxes{
		{DocTypeKwd: "table"},
		{DocTypeKwd: "image"},
		{DocTypeKwd: "text"},
	}
	FlattenMediaToText(boxes)
	for _, b := range boxes {
		if b.DocTypeKwd != "text" {
			t.Errorf("expected all 'text', got %q", b.DocTypeKwd)
		}
	}
}

// ── EnhanceWithVision ──────────────────────────────────────────────────

func TestEnhanceWithVision_NoOp(t *testing.T) {
	boxes := Boxes{
		{Text: "original", Image: newTestImage(100, 100), DocTypeKwd: "table"},
	}
	EnhanceWithVision(context.Background(), boxes, nil)
	if boxes[0].Text != "original" {
		t.Errorf("text changed when describer is nil: %q", boxes[0].Text)
	}
}

func TestEnhanceWithVision_Success(t *testing.T) {
	img := newTestImage(100, 100)
	want := "A table showing Q1 revenue."
	desc := &mockImageDescriber{describe: want}

	boxes := Boxes{
		{Text: "", Image: img, DocTypeKwd: "table"},
	}
	EnhanceWithVision(context.Background(), boxes, desc)
	if boxes[0].Text != want {
		t.Errorf("text not enhanced: got %q", boxes[0].Text)
	}
}

func TestEnhanceWithVision_SkipText(t *testing.T) {
	desc := &mockImageDescriber{describe: "should not be called"}

	boxes := Boxes{
		{Text: "plain text", DocTypeKwd: "text", Image: nil},
	}
	EnhanceWithVision(context.Background(), boxes, desc)
	// Should not call describer for text-type boxes (no image)
	if boxes[0].Text != "plain text" {
		t.Errorf("text changed: %q", boxes[0].Text)
	}
}

// ── Compose ────────────────────────────────────────────────────────────

func TestCompose_Order(t *testing.T) {
	boxes := Boxes{
		{Text: "H1", LayoutType: "header"},
		{Text: "T1", LayoutType: "", Image: nil},
	}
	pipe := Compose(
		NormalizeLayoutType, // "" → "text"
		FilterHeaderFooter,   // remove header
		AssignDocTypeKwd,     // text → "text"
	)
	result := pipe(boxes)
	if len(result) != 1 {
		t.Fatalf("expected 1 box, got %d", len(result))
	}
	if result[0].Text != "T1" || result[0].LayoutType != "text" || result[0].DocTypeKwd != "text" {
		t.Errorf("unexpected result: %+v", result[0])
	}
}

func TestCompose_Empty(t *testing.T) {
	result := Compose(NormalizeLayoutType, FilterHeaderFooter)(Boxes{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}
