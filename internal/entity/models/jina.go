package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type JinaModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

func NewJinaModel(baseURL map[string]string, urlSuffix URLSuffix) *JinaModel {
	return &JinaModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Timeout: time.Second * 90,
		},
	}
}

func (j *JinaModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &JinaModel{
		BaseURL:   baseURL,
		URLSuffix: j.URLSuffix,
		httpClient: &http.Client{
			Timeout: time.Second * 90,
		},
	}
}

func (j *JinaModel) Name() string {
	return "jina"
}

func (j *JinaModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	//TODO implement me: https://api.jina.ai/docs#/Search%20Foundation%20Models/chat_completions_v1_chat_completions_post
	return nil, fmt.Errorf("jina does not implement ChatWithMessages(not available for now)")
}

func (j *JinaModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	//TODO implement me: https://api.jina.ai/docs#/Search%20Foundation%20Models/chat_completions_v1_chat_completions_post
	return fmt.Errorf("jina does not implement ChatStreamlyWithSender(not available for now)")
}

func (j *JinaModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", j.BaseURL[region], j.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jina embedding API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var parsedResponse struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}

	if err = json.Unmarshal(body, &parsedResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(parsedResponse.Data) == 0 {
		return nil, fmt.Errorf("Jina embedding response contains no data: %s", string(body))
	}

	var embeddings []EmbeddingData
	for _, dataElem := range parsedResponse.Data {
		embeddings = append(embeddings, EmbeddingData{
			Embedding: dataElem.Embedding,
			Index:     dataElem.Index,
		})
	}

	return embeddings, nil
}

func (j *JinaModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", j.BaseURL[region], j.URLSuffix.Rerank)

	var topN = rerankConfig.TopN
	if rerankConfig.TopN != 0 {
		topN = rerankConfig.TopN
	}

	reqBody := map[string]interface{}{
		"model":     *modelName,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Jina Rerank API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var rerankResp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}

	if err = json.Unmarshal(body, &rerankResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var rerankResponse RerankResponse
	for _, result := range rerankResp.Results {
		rerankResult := RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
		}
		rerankResponse.Data = append(rerankResponse.Data, rerankResult)
	}

	return &rerankResponse, nil
}

func (j *JinaModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", j.BaseURL[region], j.URLSuffix.Models)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// convert result["data"] to []map[string]interface{}
	models := make([]string, 0)
	for _, model := range result["data"].([]interface{}) {
		modelMap := model.(map[string]interface{})
		modelName := modelMap["name"].(string)
		models = append(models, modelName)
	}

	return models, nil
}

func (j *JinaModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

func (j *JinaModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := j.ListModels(apiConfig)
	return err
}

// TranscribeAudio transcribe audio
func (z *JinaModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *JinaModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// AudioSpeech convert audio to text
func (z *JinaModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *JinaModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// OCRFile OCR file
func (z *JinaModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}
