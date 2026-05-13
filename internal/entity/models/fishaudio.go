package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 208cc2d0e4594ca896a600c43c9497aa

type FishAudioModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

func (f *FishAudioModel) ParseFile() {
	//TODO implement me
	panic("implement me")
}

func NewFishAudioModel(baseURL map[string]string, urlSuffix URLSuffix) *FishAudioModel {
	return &FishAudioModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (f *FishAudioModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &FishAudioModel{
		BaseURL:   baseURL,
		URLSuffix: f.URLSuffix,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (f *FishAudioModel) Name() string {
	return "fishaudio"
}

func (f *FishAudioModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	return nil, fmt.Errorf(f.Name() + " no such method")
}

func (f *FishAudioModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf(f.Name() + " no such method")
}

func (f *FishAudioModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("no such method")
}

func (f *FishAudioModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("no such method")
}

// TranscribeAudio transcribe audio
func (f *FishAudioModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

func (z *FishAudioModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// AudioSpeech convert audio to text
func (f *FishAudioModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

func (z *FishAudioModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// OCRFile OCR file
func (f *FishAudioModel) OCRFile(modelName *string, fileContent *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", f.Name())
}

func (f *FishAudioModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", f.BaseURL[region], f.URLSuffix.Models)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if apiConfig != nil && apiConfig.ApiKey != nil && *apiConfig.ApiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	} else {
		return nil, fmt.Errorf("Fish Audio API key is missing")
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Fish Audio API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Items []struct {
			ID    string `json:"_id"`
			Title string `json:"title"`
		} `json:"items"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		models = append(models, item.Title)
	}

	return models, nil
}

func (f *FishAudioModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL := f.BaseURL[region]
	if baseURL == "" {
		baseURL = f.BaseURL["default"]
	}

	url := fmt.Sprintf("%s/wallet/self/api-credit", strings.TrimSuffix(baseURL, "/"))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Fish Audio balance API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

func (f *FishAudioModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := f.ListModels(apiConfig)
	return err
}
