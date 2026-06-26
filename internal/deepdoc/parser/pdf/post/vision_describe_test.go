package post

import (
	"context"
	"errors"
	"image"
	"image/color"
	"testing"
)

// ── mock image describer ───────────────────────────────────────────────

type mockImageDescriber struct {
	describe string
	err      error
}

func (m *mockImageDescriber) DescribeImage(_ context.Context, _ image.Image, _ string) (string, error) {
	return m.describe, m.err
}

// ── DescribeImage tests ────────────────────────────────────────────────

func TestDescribeImage_Success(t *testing.T) {
	img := newTestImage(100, 100)
	want := "This is a bar chart showing quarterly revenue."
	client := &mockImageDescriber{describe: want}

	got, err := DescribeImage(context.Background(), img, "Describe this image", client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("DescribeImage() = %q, want %q", got, want)
	}
}

func TestDescribeImage_VLMError(t *testing.T) {
	img := newTestImage(100, 100)
	client := &mockImageDescriber{err: errors.New("VLM timeout")}

	got, err := DescribeImage(context.Background(), img, "Describe this image", client)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != "" {
		t.Errorf("expected empty string on error, got %q", got)
	}
}

func TestDescribeImage_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	img := newTestImage(100, 100)
	client := &mockImageDescriber{describe: "should not be reached"}

	got, err := DescribeImage(ctx, img, "prompt", client)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestDescribeImage_NilImage(t *testing.T) {
	client := &mockImageDescriber{describe: "should not be reached"}

	got, err := DescribeImage(context.Background(), nil, "prompt", client)
	if err == nil {
		t.Fatal("expected error for nil image, got nil")
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestDescribeImage_EmptyImage(t *testing.T) {
	img := newTestImage(0, 0)
	client := &mockImageDescriber{describe: "should not be reached"}

	_, err := DescribeImage(context.Background(), img, "prompt", client)
	if err == nil {
		t.Fatal("expected error for empty image, got nil")
	}
}

func TestDescribeImage_TinyImage(t *testing.T) {
	img := newTestImage(5, 5) // below minSide=11
	client := &mockImageDescriber{describe: "should not be reached"}

	got, err := DescribeImage(context.Background(), img, "prompt", client)
	if err != nil {
		t.Fatal("tiny images should be silently skipped, not error")
	}
	if got != "" {
		t.Errorf("expected empty string for tiny image, got %q", got)
	}
}

// ── helpers ────────────────────────────────────────────────────────────

func newTestImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Fill with a recognizable pattern.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	return img
}
