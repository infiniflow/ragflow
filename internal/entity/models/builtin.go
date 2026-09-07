package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// builtinHTTPClient is a shared client with timeouts for all Builtin model
// requests. http.DefaultClient has no timeout, so a hung or slow TEI server
// would otherwise block goroutines indefinitely.
var builtinHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost:   10,
		ResponseHeaderTimeout: 10 * time.Second,
	},
}

// BuiltinModel implements ModelDriver for Builtin (local embedding models via TEI)
type BuiltinModel struct {
	baseURL string
	model   string
}

// NewBuiltinModel creates a new Builtin model instance
func NewBuiltinModel(baseURL, model string) *BuiltinModel {
	return &BuiltinModel{
		baseURL: baseURL,
		model:   model,
	}
}

func (b *BuiltinModel) Name() string {
	return "builtin"
}

func (b *BuiltinModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &BuiltinModel{
		baseURL: b.baseURL,
		model:   b.model,
	}
}

func (b *BuiltinModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf("builtin model does not support chat")
}

func (b *BuiltinModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("builtin model does not support chat")
}

// Embed sends texts to a TEI (Text Embeddings Inference) server and returns embeddings
func (b *BuiltinModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	baseURL := b.baseURL
	if baseURL == "" {
		baseURL = "http://localhost:6380"
	}

	url := fmt.Sprintf("%s/embed", baseURL)

	reqBody := map[string]interface{}{
		"inputs": texts,
	}

	// Add dimension if specified
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	// Note: TEI server typically doesn't require auth for local deployments

	resp, err := builtinHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Builtin embeddings API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// TEI returns a simple array of embeddings by default
	var embeddings [][]float64
	if err = json.Unmarshal(body, &embeddings); err != nil {
		return nil, fmt.Errorf("failed to parse TEI response: %w, body: %s", err, string(body))
	}

	result := make([]EmbeddingData, len(embeddings))
	for i, emb := range embeddings {
		result[i] = EmbeddingData{
			Embedding: emb,
			Index:     i,
		}
	}

	return result, nil
}

func (b *BuiltinModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("builtin model does not support rerank")
}

func (b *BuiltinModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("builtin model does not support transcription")
}

func (b *BuiltinModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("builtin model does not support transcription")
}

func (b *BuiltinModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("builtin model does not support TTS")
}

func (b *BuiltinModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("builtin model does not support TTS")
}

func (b *BuiltinModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("builtin model does not support OCR")
}

func (b *BuiltinModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("builtin model does not support parse file")
}

func (b *BuiltinModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	return []ListModelResponse{
		{
			Name: b.model,
		},
	}, nil
}

func (b *BuiltinModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("builtin model does not support balance")
}

func (b *BuiltinModel) CheckConnection(apiConfig *APIConfig) error {
	// Try to get model info to verify connection
	_, err := b.Embed(nil, []string{"test"}, apiConfig, nil)
	return err
}

func (b *BuiltinModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("builtin model does not support tasks")
}

func (b *BuiltinModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("builtin model does not support tasks")
}

// GetBuiltinEmbeddingModel returns a Builtin model driver for the given model name
func GetBuiltinEmbeddingModel(modelName string) ModelDriver {
	// Get TEI base URL from environment or config
	// Default to port 6380 where docker-tei-cpu-1 maps internal port 80
	teiBaseURL := getEnv("TEI_BASE_URL", "http://localhost:6380")

	// Create a builtin model instance with TEI endpoint
	driver := NewBuiltinModel(teiBaseURL, modelName)
	return driver
}

// getEnv is a helper to get environment variable with default
func getEnv(key, defaultValue string) string {
	if value := strings.TrimSpace(strings.Replace(os.Getenv(key), "\\", "/", -1)); value != "" {
		return value
	}
	return defaultValue
}
