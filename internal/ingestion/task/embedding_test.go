package task

import (
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity/models"
)

// stubDriver records the texts passed to Embed for verification.
type stubDriver struct {
	capturedTexts []string
}

func (d *stubDriver) Embed(modelName *string, texts []string, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig) ([]models.EmbeddingData, error) {
	d.capturedTexts = texts
	result := make([]models.EmbeddingData, len(texts))
	for i := range texts {
		result[i] = models.EmbeddingData{
			Embedding:  []float64{float64(i), 0.1},
			Index:      i,
			TokenCount: len(texts[i]),
		}
	}
	return result, nil
}
func (d *stubDriver) NewInstance(baseURL map[string]string) models.ModelDriver { return d }
func (d *stubDriver) Name() string                                             { return "stub" }
func (d *stubDriver) ChatWithMessages(modelName string, messages []models.Message, apiConfig *models.APIConfig, chatModelConfig *models.ChatConfig) (*models.ChatResponse, error) {
	return nil, nil
}
func (d *stubDriver) ChatStreamlyWithSender(modelName string, messages []models.Message, apiConfig *models.APIConfig, modelConfig *models.ChatConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) Rerank(modelName *string, query string, documents []string, apiConfig *models.APIConfig, rerankConfig *models.RerankConfig) (*models.RerankResponse, error) {
	return nil, nil
}
func (d *stubDriver) TranscribeAudio(modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig) (*models.ASRResponse, error) {
	return nil, nil
}
func (d *stubDriver) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) AudioSpeech(modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig) (*models.TTSResponse, error) {
	return nil, nil
}
func (d *stubDriver) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *stubDriver) OCRFile(modelName *string, content []byte, url *string, apiConfig *models.APIConfig, ocrConfig *models.OCRConfig) (*models.OCRFileResponse, error) {
	return nil, nil
}
func (d *stubDriver) ParseFile(modelName *string, content []byte, url *string, apiConfig *models.APIConfig, parseFileConfig *models.ParseFileConfig) (*models.ParseFileResponse, error) {
	return nil, nil
}
func (d *stubDriver) ListModels(apiConfig *models.APIConfig) ([]models.ListModelResponse, error) {
	return nil, nil
}
func (d *stubDriver) Balance(apiConfig *models.APIConfig) (map[string]interface{}, error) {
	return nil, nil
}
func (d *stubDriver) CheckConnection(apiConfig *models.APIConfig) error { return nil }
func (d *stubDriver) ListTasks(apiConfig *models.APIConfig) ([]models.ListTaskStatus, error) {
	return nil, nil
}
func (d *stubDriver) ShowTask(taskID string, apiConfig *models.APIConfig) (*models.TaskResponse, error) {
	return nil, nil
}
func (d *stubDriver) ToolCall(name string, arguments map[string]interface{}) (string, error) {
	return "", nil
}

func makeTestEmbeddingModel(stub *stubDriver, maxTokens int) *models.EmbeddingModel {
	return &models.EmbeddingModel{
		ModelDriver: stub,
		ModelName:   strPtr("test-model"),
		APIConfig:   &models.APIConfig{},
		MaxTokens:   maxTokens,
	}
}

// =============================================================================
// encodeTexts — must truncate texts before passing to ModelDriver.Embed
// Python: truncated = truncate_texts(txts, model.max_length); model.encode(truncated)
// =============================================================================

func TestEncodeTexts_TruncatesBeforeEmbed(t *testing.T) {
	stub := &stubDriver{}
	model := makeTestEmbeddingModel(stub, 12)
	// "hello world" is 2 tokens, safeMax = 12-10 = 2, fits
	texts := []string{"hello world"}
	_, _, err := encodeTexts(model, texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.capturedTexts) != 1 {
		t.Fatalf("captured %d texts, want 1", len(stub.capturedTexts))
	}
	if stub.capturedTexts[0] != "hello world" {
		t.Errorf("texts should pass through when within limit, got %q", stub.capturedTexts[0])
	}
}

func TestEncodeTexts_TruncatesLongText(t *testing.T) {
	stub := &stubDriver{}
	model := makeTestEmbeddingModel(stub, 12)
	// safeMax = 12-10 = 2 tokens. Text > 2 tokens should be truncated.
	longText := strings.Repeat("hello world ", 20) // 40 tokens
	texts := []string{longText}
	_, _, err := encodeTexts(model, texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.capturedTexts) != 1 {
		t.Fatalf("captured %d texts, want 1", len(stub.capturedTexts))
	}
	// The text passed to Embed must be SHORTER than the original
	if len(stub.capturedTexts[0]) >= len(longText) {
		t.Errorf("text should be truncated before embed: original=%d, truncated=%d",
			len(longText), len(stub.capturedTexts[0]))
	}
}

func TestEncodeTexts_TokenCountSum(t *testing.T) {
	stub := &stubDriver{}
	model := makeTestEmbeddingModel(stub, 100)
	texts := []string{"hello", "world"}
	_, totalTokens, err := encodeTexts(model, texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalTokens <= 0 {
		t.Errorf("expected positive token count, got %d", totalTokens)
	}
}

func TestEncodeTexts_ReturnsVectors(t *testing.T) {
	stub := &stubDriver{}
	model := makeTestEmbeddingModel(stub, 100)
	texts := []string{"a", "b", "c"}
	vecs, _, err := encodeTexts(model, texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
}

// =============================================================================
// GetEmbeddingDimension
// =============================================================================

func TestTestEncodeForDim_ReturnsDimension(t *testing.T) {
	stub := &stubDriver{}
	model := makeTestEmbeddingModel(stub, 100)
	dim, err := getEmbeddingDimension(model)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dim != 2 {
		t.Errorf("dim = %d, want 2 (stubDriver returns 2-element vectors)", dim)
	}
}

func TestTestEncodeForDim_ModelError(t *testing.T) {
	stub := &stubDriver{}
	model := makeTestEmbeddingModel(stub, 100)
	model.ModelDriver = &errDriver{}
	_, err := getEmbeddingDimension(model)
	if err == nil {
		t.Error("expected error from failing driver")
	}
}

type errDriver struct{}

func (d *errDriver) Embed(modelName *string, texts []string, apiConfig *models.APIConfig, embeddingConfig *models.EmbeddingConfig) ([]models.EmbeddingData, error) {
	return nil, errors.New("embed error")
}
func (d *errDriver) NewInstance(baseURL map[string]string) models.ModelDriver { return d }
func (d *errDriver) Name() string                                             { return "err" }
func (d *errDriver) ChatWithMessages(modelName string, messages []models.Message, apiConfig *models.APIConfig, chatModelConfig *models.ChatConfig) (*models.ChatResponse, error) {
	return nil, nil
}
func (d *errDriver) ChatStreamlyWithSender(modelName string, messages []models.Message, apiConfig *models.APIConfig, modelConfig *models.ChatConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *errDriver) Rerank(modelName *string, query string, documents []string, apiConfig *models.APIConfig, rerankConfig *models.RerankConfig) (*models.RerankResponse, error) {
	return nil, nil
}
func (d *errDriver) TranscribeAudio(modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig) (*models.ASRResponse, error) {
	return nil, nil
}
func (d *errDriver) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *models.APIConfig, asrConfig *models.ASRConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *errDriver) AudioSpeech(modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig) (*models.TTSResponse, error) {
	return nil, nil
}
func (d *errDriver) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *models.APIConfig, ttsConfig *models.TTSConfig, sender func(*string, *string) error) error {
	return nil
}
func (d *errDriver) OCRFile(modelName *string, content []byte, url *string, apiConfig *models.APIConfig, ocrConfig *models.OCRConfig) (*models.OCRFileResponse, error) {
	return nil, nil
}
func (d *errDriver) ParseFile(modelName *string, content []byte, url *string, apiConfig *models.APIConfig, parseFileConfig *models.ParseFileConfig) (*models.ParseFileResponse, error) {
	return nil, nil
}
func (d *errDriver) ListModels(apiConfig *models.APIConfig) ([]models.ListModelResponse, error) {
	return nil, nil
}
func (d *errDriver) Balance(apiConfig *models.APIConfig) (map[string]interface{}, error) {
	return nil, nil
}
func (d *errDriver) CheckConnection(apiConfig *models.APIConfig) error { return nil }
func (d *errDriver) ListTasks(apiConfig *models.APIConfig) ([]models.ListTaskStatus, error) {
	return nil, nil
}
func (d *errDriver) ShowTask(taskID string, apiConfig *models.APIConfig) (*models.TaskResponse, error) {
	return nil, nil
}
func (d *errDriver) ToolCall(name string, arguments map[string]interface{}) (string, error) {
	return "", nil
}
