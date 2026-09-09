//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

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
)

// OllamaModel implements ModelDriver for Ollama AI
type OllamaModel struct {
	baseModel BaseModel
}

// NewOllamaModel creates a new Ollama AI model instance
func NewOllamaModel(baseURL map[string]string, urlSuffix URLSuffix) *OllamaModel {
	return &OllamaModel{
		baseModel: BaseModel{
			BaseURL:          baseURL,
			URLSuffix:        urlSuffix,
			AllowEmptyAPIKey: true,
			httpClient:       NewDriverHTTPClient(true),
		},
	}
}

func (o *OllamaModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewOllamaModel(baseURL, o.baseModel.URLSuffix)
}

func (o *OllamaModel) Name() string {
	return "Ollama"
}

func buildOllamaRequestBody(cfg *ChatConfig, modelName string, messages []Message, stream bool) map[string]any {
	reqBody := buildRequestBody(cfg, modelName, messages, stream)
	reqBody["messages"] = buildOllamaMessages(messages)
	return reqBody
}

func buildOllamaMessages(messages []Message) []map[string]any {
	apiMessages := buildChatMessages(messages)
	for i, message := range messages {
		content, images, ok := ollamaMultimodalContent(message.Content)
		if !ok {
			continue
		}
		apiMessages[i]["content"] = content
		if len(images) > 0 {
			apiMessages[i]["images"] = images
		}
	}
	return apiMessages
}

func ollamaMultimodalContent(content interface{}) (string, []string, bool) {
	var parts []interface{}
	switch value := content.(type) {
	case []interface{}:
		parts = value
	case []map[string]interface{}:
		parts = make([]interface{}, len(value))
		for i := range value {
			parts[i] = value[i]
		}
	default:
		return "", nil, false
	}

	var textParts []string
	var images []string
	for _, part := range parts {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			if text, ok := part.(string); ok {
				textParts = append(textParts, text)
			}
			continue
		}

		partType, _ := partMap["type"].(string)
		switch partType {
		case "text", "input_text":
			if text, ok := partMap["text"].(string); ok {
				textParts = append(textParts, text)
			}
		case "image_url":
			if imageURL := ollamaImageURL(partMap["image_url"]); imageURL != "" {
				images = append(images, cleanOllamaImageData(imageURL))
			}
		}
	}

	return strings.Join(textParts, "\n"), images, true
}

func ollamaImageURL(value interface{}) string {
	switch image := value.(type) {
	case string:
		return image
	case map[string]interface{}:
		url, _ := image["url"].(string)
		return url
	case map[string]string:
		return image["url"]
	default:
		return ""
	}
}

func cleanOllamaImageData(image string) string {
	const base64Marker = ";base64,"
	if strings.HasPrefix(image, "data:") {
		if marker := strings.Index(image, base64Marker); marker >= 0 {
			return image[marker+len(base64Marker):]
		}
	}
	return image
}

func (o *OllamaModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("message is nil")
	}

	resolvedBaseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, o.baseModel.URLSuffix.Chat)

	// For qwen/glm models, use async chat endpoint
	modelType := strings.Split(modelName, "_")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = fmt.Sprintf("%s/%s", resolvedBaseURL, o.baseModel.URLSuffix.AsyncChat)
	}

	// Build request body
	reqBody := buildOllamaRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {
		if chatModelConfig.Effort != nil && *chatModelConfig.Effort != "" {
			if strings.HasPrefix(strings.ToLower(modelName), "gpt-oss") {
				reqBody["think"] = *chatModelConfig.Effort
			}
		} else if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["think"] = true
			}
		}
	}

	body, err := o.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return handleOllamaNonStreamingResponse(body, modelUsage, chatModelConfig)
}

func (o *OllamaModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, o.baseModel.URLSuffix.Chat)
	modelType := strings.Split(modelName, "-")[0]
	if modelType == "qwen" || modelType == "glm" {
		url = fmt.Sprintf("%s/%s", resolvedBaseURL, o.baseModel.URLSuffix.AsyncChat)
	}

	// Build request body with streaming enabled
	reqBody := buildOllamaRequestBody(modelConfig, modelName, messages, true)

	if modelConfig != nil {
		if modelConfig.Effort != nil && *modelConfig.Effort != "" {
			if strings.HasPrefix(strings.ToLower(modelName), "gpt-oss") {
				reqBody["think"] = *modelConfig.Effort
			}
		} else if modelConfig.Thinking != nil {
			if *modelConfig.Thinking {
				reqBody["think"] = true
			}
		}
	}

	reqBody["stream_options"] = map[string]interface{}{"include_usage": true}

	return o.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return handleOllamaStreamingResponse(body, modelUsage, modelConfig, sender)
	})
}

type ollamaChatMessage struct {
	Content  string `json:"content"`
	Thinking string `json:"thinking"`
}

type ollamaChatResponse struct {
	Model           string            `json:"model"`
	Message         ollamaChatMessage `json:"message"`
	Done            bool              `json:"done"`
	PromptEvalCount int               `json:"prompt_eval_count"`
	EvalCount       int               `json:"eval_count"`
	TotalDuration   int64             `json:"total_duration"`
	Error           any               `json:"error"`
}

func handleOllamaNonStreamingResponse(body []byte, modelUsage *common.ModelUsage, chatConfig *ChatConfig) (*ChatResponse, error) {
	var result ollamaChatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("upstream error: %v", result.Error)
	}
	content := result.Message.Content
	reasonContent := result.Message.Thinking
	usage := ollamaTokenUsage(result)
	if usage != nil {
		recordResponseUsage(modelUsage, "", usage, "chat")
		if chatConfig != nil {
			chatConfig.UsageResult = usage
		}
	}
	if content == "" && reasonContent == "" {
		return nil, fmt.Errorf("no message in response")
	}
	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
		Usage:         usage,
	}, nil
}

func handleOllamaStreamingResponse(body io.Reader, modelUsage *common.ModelUsage, chatConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	var usage *TokenUsage
	sawDone := false
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event ollamaChatResponse
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("invalid Ollama stream event: %w", err)
		}
		if event.Error != nil {
			return fmt.Errorf("upstream stream error: %v", event.Error)
		}
		if event.Message.Thinking != "" {
			if err := sender(nil, &event.Message.Thinking); err != nil {
				return err
			}
		}
		if event.Message.Content != "" {
			if err := sender(&event.Message.Content, nil); err != nil {
				return err
			}
		}
		if event.Done {
			sawDone = true
			usage = ollamaTokenUsage(event)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawDone {
		return fmt.Errorf("stream ended before done")
	}
	if usage != nil {
		recordResponseUsage(modelUsage, "", usage, "chat")
		if chatConfig != nil {
			chatConfig.UsageResult = usage
		}
	}
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

func ollamaTokenUsage(result ollamaChatResponse) *TokenUsage {
	if result.PromptEvalCount == 0 && result.EvalCount == 0 {
		return nil
	}
	return &TokenUsage{
		PromptTokens:     result.PromptEvalCount,
		CompletionTokens: result.EvalCount,
		TotalTokens:      result.PromptEvalCount + result.EvalCount,
	}
}

func (o *OllamaModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := o.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL
	if baseURL == "" {
		baseURL = resolvedBaseURL
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for Ollama (e.g., http://127.0.0.1:11434/v1)")
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), o.baseModel.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": request.Texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var embedResp struct {
		Model      string      `json:"model"`
		Embeddings [][]float64 `json:"embeddings"`
	}

	if err = json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(embedResp.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	embeddings := make([]EmbeddingData, 0, len(embedResp.Embeddings))

	for i, emb := range embedResp.Embeddings {
		if len(emb) == 0 {
			return nil, fmt.Errorf("empty embedding at index %d", i)
		}

		embeddings = append(embeddings, EmbeddingData{
			Embedding: emb,
			Index:     i,
		})
	}

	return embeddings, nil
}

func (o *OllamaModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("no such method")
}

// TranscribeAudio transcribe audio
func (o *OllamaModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OllamaModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", o.Name())
}

// AudioSpeech convert text to audio
func (o *OllamaModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OllamaModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", o.Name())
}

// OCRFile OCR file
func (o *OllamaModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

// ParseFile parse file
func (o *OllamaModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OllamaModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {

	baseURL, err := o.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing base URL: please configure the local access address for Ollama (e.g., http://127.0.0.1:11434)")
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), o.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := o.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Ollama's /api/tags returns {"models":[{"name":...,"model":...}]}, a shape
	// that differs from the OpenAI list. Decode it into a local struct, map the
	// names into ModelList, then enrich through the shared ParseListModel helper
	// (issue #15853). Using a typed struct also avoids the previous unchecked
	// type assertions, which panicked when "models" was absent or malformed.
	var result struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	modelList := ModelList{Object: "list"}
	for _, m := range result.Models {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = strings.TrimSpace(m.Model)
		}
		if name == "" {
			continue
		}
		modelList.Models = append(modelList.Models, ModelListItem{ID: name})
	}

	return ParseListModel(modelList), nil
}

func (o *OllamaModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection verifies that the configured Ollama base URL is reachable
func (o *OllamaModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := o.ListModels(ctx, apiConfig)
	return err
}

func (o *OllamaModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}

func (o *OllamaModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", o.Name())
}
