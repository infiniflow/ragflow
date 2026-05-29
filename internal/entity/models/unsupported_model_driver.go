package models

import "fmt"

// UnsupportedModelDriver provides safe defaults for model capabilities that a
// provider does not implement.
type UnsupportedModelDriver struct {
	// ProviderName should match the provider's Name() value.
	ProviderName string
}

func (u UnsupportedModelDriver) noSuchMethod() error {
	if u.ProviderName == "" {
		return fmt.Errorf("no such method")
	}
	return fmt.Errorf("%s, no such method", u.ProviderName)
}

func (u UnsupportedModelDriver) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, u.noSuchMethod()
}

func (u UnsupportedModelDriver) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, u.noSuchMethod()
}

func (u UnsupportedModelDriver) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, u.noSuchMethod()
}

func (u UnsupportedModelDriver) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return u.noSuchMethod()
}

func (u UnsupportedModelDriver) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, u.noSuchMethod()
}

func (u UnsupportedModelDriver) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return u.noSuchMethod()
}

func (u UnsupportedModelDriver) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, u.noSuchMethod()
}

func (u UnsupportedModelDriver) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, u.noSuchMethod()
}

func (u UnsupportedModelDriver) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, u.noSuchMethod()
}

func (u UnsupportedModelDriver) CheckConnection(apiConfig *APIConfig) error {
	return u.noSuchMethod()
}

func (u UnsupportedModelDriver) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, u.noSuchMethod()
}

func (u UnsupportedModelDriver) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, u.noSuchMethod()
}
