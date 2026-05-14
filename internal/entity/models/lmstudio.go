package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/common"
	"strings"
	"time"
)

// LmStudioModel implements ModelDriver for lm-studio
type LmStudioModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

func (l *LmStudioModel) ParseFile() {
	//TODO implement me
	panic("implement me")
}

// NewLmStudioModel
func NewLmStudioModel(baseURL map[string]string, urlSuffix URLSuffix) *LmStudioModel {
	return &LmStudioModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

func (l *LmStudioModel) NewInstance(baseURL map[string]string) ModelDriver {
	return &LmStudioModel{
		BaseURL:   baseURL,
		URLSuffix: l.URLSuffix,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
	}
}

func (l *LmStudioModel) Name() string {
	return "lmstudio"
}

// ChatWithMessages sends multiple messages with roles and returns response
func (l *LmStudioModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	var region = "default"
	if apiConfig.Region != nil {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", l.BaseURL[region], l.URLSuffix.Chat)

	// For qwen/glm models, use async chat endpoint
	modelType := strings.Split(modelName, "-")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = fmt.Sprintf("%s/%s", l.BaseURL[region], l.URLSuffix.AsyncChat)
	}

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil {
			reqBody["stream"] = *chatModelConfig.Stream
		}

		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}

		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}

		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}

		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}

		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{
					"type": "enabled",
				}
			} else {
				reqBody["thinking"] = map[string]interface{}{
					"type": "disabled",
				}
			}
		}
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

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s :%s", resp.StatusCode, string(body), messages[0].Content)
	}

	// Parse response
	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid choice format")
	}

	messageMap, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	content, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	var reasonContent string
	if chatModelConfig != nil && chatModelConfig.Thinking != nil && *chatModelConfig.Thinking {
		reasonContent, ok = messageMap["reasoning_content"].(string)
		if !ok {
			return nil, fmt.Errorf("invalid content format")
		}
		if reasonContent != "" && reasonContent[0] == '\n' {
			reasonContent = reasonContent[1:]
		}
	}

	chatResponse := &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}

	return chatResponse, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (l *LmStudioModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	url := fmt.Sprintf("%s/%s", l.BaseURL[region], l.URLSuffix.Chat)
	modelType := strings.Split(modelName, "-")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = fmt.Sprintf("%s/%s", l.BaseURL[region], l.URLSuffix.AsyncChat)
	}

	// Convert messages to API format (supporting multimodal content)
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   true,
	}

	if modelConfig.Stream != nil {
		reqBody["stream"] = *modelConfig.Stream
	}

	if modelConfig.MaxTokens != nil {
		reqBody["max_tokens"] = *modelConfig.MaxTokens
	}

	if modelConfig.Temperature != nil {
		reqBody["temperature"] = *modelConfig.Temperature
	}

	if modelConfig.DoSample != nil {
		reqBody["do_sample"] = *modelConfig.DoSample
	}

	if modelConfig.TopP != nil {
		reqBody["top_p"] = *modelConfig.TopP
	}

	if modelConfig.Stop != nil {
		reqBody["stop"] = *modelConfig.Stop
	}

	if modelConfig.Thinking != nil {
		if *modelConfig.Thinking {
			reqBody["thinking"] = map[string]interface{}{
				"type": "enabled",
			}
		} else {
			reqBody["thinking"] = map[string]interface{}{
				"type": "disabled",
			}
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		common.Info(line)

		// SSE data line starts with "data:"
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON after "data:"
		data := strings.TrimSpace(line[5:])

		// [DONE] marks the end of stream
		if data == "[DONE]" {
			break
		}

		// Parse the JSON event
		var event map[string]interface{}
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		finishReason, ok := firstChoice["finish_reason"].(string)
		if ok && finishReason != "" {
			break
		}
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return scanner.Err()
}

func (l *LmStudioModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL := l.BaseURL[region]
	if baseURL == "" {
		baseURL = l.BaseURL["default"]
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for LM Studio (e.g., http://127.0.0.1:1234/v1)")
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), l.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiConfig != nil && apiConfig.ApiKey != nil && *apiConfig.ApiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	}

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LM Studio embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed openaiEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var embeddings []EmbeddingData
	for _, dataElem := range parsed.Data {
		var embeddingData EmbeddingData
		embeddingData.Embedding = dataElem.Embedding
		embeddingData.Index = dataElem.Index
		embeddings = append(embeddings, embeddingData)
	}

	return embeddings, nil
}

func (l *LmStudioModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("no such method")
}

// TranscribeAudio transcribe audio
func (z *LmStudioModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *LmStudioModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// AudioSpeech convert audio to text
func (z *LmStudioModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", z.Name())
}

func (z *LmStudioModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", z.Name())
}

// OCRFile OCR file
func (l *LmStudioModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// ListModels list supported models
func (l *LmStudioModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	var region = "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL := l.BaseURL[region]
	if baseURL == "" {
		baseURL = l.BaseURL["default"]
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for LM Studio (e.g., http://127.0.0.1:1234/v1)")
	}

	url := fmt.Sprintf("%s/%s", baseURL, l.URLSuffix.Models)

	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// LM Studio is a local provider and the API key is optional. Only
	// set the Authorization header when a non-empty key was supplied.
	// This also avoids a nil-pointer dereference on apiConfig or ApiKey.
	if apiConfig != nil && apiConfig.ApiKey != nil && *apiConfig.ApiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
	}

	resp, err := l.httpClient.Do(req)
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

	// convert result["data"] 2 []map[string]interface{}
	models := make([]string, 0)
	for _, model := range result["data"].([]interface{}) {
		modelMap := model.(map[string]interface{})
		modelName := modelMap["id"].(string)
		models = append(models, modelName)
	}

	return models, nil
}

func (l *LmStudioModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection verifies that the configured LM Studio base URL is reachable
func (l *LmStudioModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := l.ListModels(apiConfig)
	return err
}
