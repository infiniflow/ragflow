package task

import (
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
