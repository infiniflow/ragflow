package post

import (
	"context"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"

	modelModule "ragflow/internal/entity/models"
)

// ── mock ModelDriver ───────────────────────────────────────────────────

type mockModelDriver struct {
	answer string
	err    error
}

func (m *mockModelDriver) ChatWithMessages(_ string, _ []modelModule.Message, _ *modelModule.APIConfig, _ *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	a := m.answer
	return &modelModule.ChatResponse{Answer: &a}, nil
}

// Stubs for the rest of ModelDriver.
func (m *mockModelDriver) Name() string                                       { return "mock" }
func (m *mockModelDriver) NewInstance(_ map[string]string) modelModule.ModelDriver { return m }
func (m *mockModelDriver) ChatStreamlyWithSender(_ string, _ []modelModule.Message, _ *modelModule.APIConfig, _ *modelModule.ChatConfig, _ func(*string, *string) error) error {
	return nil
}
func (m *mockModelDriver) Embed(_ *string, _ []string, _ *modelModule.APIConfig, _ *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, error) {
	return nil, nil
}
func (m *mockModelDriver) Rerank(_ *string, _ string, _ []string, _ *modelModule.APIConfig, _ *modelModule.RerankConfig) (*modelModule.RerankResponse, error) {
	return nil, nil
}
func (m *mockModelDriver) TranscribeAudio(_ *string, _ *string, _ *modelModule.APIConfig, _ *modelModule.ASRConfig) (*modelModule.ASRResponse, error) {
	return nil, nil
}
func (m *mockModelDriver) TranscribeAudioWithSender(_ *string, _ *string, _ *modelModule.APIConfig, _ *modelModule.ASRConfig, _ func(*string, *string) error) error {
	return nil
}
func (m *mockModelDriver) AudioSpeech(_ *string, _ *string, _ *modelModule.APIConfig, _ *modelModule.TTSConfig) (*modelModule.TTSResponse, error) {
	return nil, nil
}
func (m *mockModelDriver) AudioSpeechWithSender(_ *string, _ *string, _ *modelModule.APIConfig, _ *modelModule.TTSConfig, _ func(*string, *string) error) error {
	return nil
}
func (m *mockModelDriver) OCRFile(_ *string, _ []byte, _ *string, _ *modelModule.APIConfig, _ *modelModule.OCRConfig) (*modelModule.OCRFileResponse, error) {
	return nil, nil
}
func (m *mockModelDriver) ParseFile(_ *string, _ []byte, _ *string, _ *modelModule.APIConfig, _ *modelModule.ParseFileConfig) (*modelModule.ParseFileResponse, error) {
	return nil, nil
}
func (m *mockModelDriver) ListModels(_ *modelModule.APIConfig) ([]modelModule.ListModelResponse, error) {
	return nil, nil
}
func (m *mockModelDriver) Balance(_ *modelModule.APIConfig) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockModelDriver) CheckConnection(_ *modelModule.APIConfig) error { return nil }
func (m *mockModelDriver) ListTasks(_ *modelModule.APIConfig) ([]modelModule.ListTaskStatus, error) {
	return nil, nil
}
func (m *mockModelDriver) ShowTask(_ string, _ *modelModule.APIConfig) (*modelModule.TaskResponse, error) {
	return nil, nil
}

// ── ModelImageDescriber tests ──────────────────────────────────────────

func TestModelImageDescriber_Success(t *testing.T) {
	img := newTestImage(100, 100)
	want := "A chart showing revenue growth."
	driver := &mockModelDriver{answer: want}
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
	driver := &mockModelDriver{err: errors.New("API rate limited")}
	desc := NewModelImageDescriber(driver, "gpt-4o", nil, 0)

	_, err := desc.DescribeImage(context.Background(), img, "prompt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestModelImageDescriber_EmptyAnswer(t *testing.T) {
	img := newTestImage(100, 100)
	driver := &mockModelDriver{answer: ""}
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
