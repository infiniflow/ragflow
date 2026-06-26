package post

import (
	"context"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"
)

// ── mock ChatDriver ────────────────────────────────────────────────────

type mockChatDriver struct {
	answer string
	err    error
}

func (m *mockChatDriver) ChatWithMessages(_ string, _ []ChatMessage, _ *ChatAPIConfig, _ *ChatConfig) (*ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	a := m.answer
	return &ChatResponse{Answer: &a}, nil
}

// ── ModelImageDescriber tests ──────────────────────────────────────────

func TestModelImageDescriber_Success(t *testing.T) {
	img := newTestImage(100, 100)
	want := "A chart showing revenue growth."
	driver := &mockChatDriver{answer: want}
	desc := NewModelImageDescriber(driver, "gpt-4o", nil, 0)

	got, err := desc.DescribeImage(context.Background(), img, "Describe this chart")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestModelImageDescriber_DriverError(t *testing.T) {
	img := newTestImage(100, 100)
	driver := &mockChatDriver{err: errors.New("API rate limited")}
	desc := NewModelImageDescriber(driver, "gpt-4o", nil, 0)

	_, err := desc.DescribeImage(context.Background(), img, "prompt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestModelImageDescriber_EmptyAnswer(t *testing.T) {
	img := newTestImage(100, 100)
	driver := &mockChatDriver{answer: ""}
	desc := NewModelImageDescriber(driver, "gpt-4o", nil, 0)

	_, err := desc.DescribeImage(context.Background(), img, "prompt")
	if err == nil {
		t.Fatal("expected error for empty answer, got nil")
	}
}

// ── encodeImageToBase64DataURL tests ───────────────────────────────────

func TestEncodeImageToBase64DataURL(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	url, err := encodeImageToBase64DataURL(img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("missing data URL prefix: %s...", url[:min(50, len(url))])
	}
}
