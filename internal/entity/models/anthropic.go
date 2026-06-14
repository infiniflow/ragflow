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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const anthropicVersion = "2023-06-01"

// AnthropicModel implements ModelDriver for Claude models through the
// Anthropic Messages API.
type AnthropicModel struct {
	baseModel BaseModel
}

func NewAnthropicModel(baseURL map[string]string, urlSuffix URLSuffix) *AnthropicModel {
	return &AnthropicModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (a *AnthropicModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAnthropicModel(baseURL, a.baseModel.URLSuffix)
}

func (a *AnthropicModel) Name() string {
	return "anthropic"
}

func (a *AnthropicModel) region(apiConfig *APIConfig) string {
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		return *apiConfig.Region
	}
	return "default"
}

func (a *AnthropicModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(*apiConfig.ApiKey)
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	apiMessages, systemPrompt, err := anthropicMessages(messages)
	if err != nil {
		return nil, err
	}

	baseURLRegion := a.region(apiConfig)
	baseURLConfig := &APIConfig{Region: &baseURLRegion}
	if apiConfig != nil {
		baseURLConfig.BaseURL = apiConfig.BaseURL
	}
	baseURL, err := a.baseModel.GetBaseURL(baseURLConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSpace(strings.TrimSuffix(baseURL, "/"))
	url := fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(a.baseModel.URLSuffix.Chat, "/"))

	reqBody := map[string]interface{}{
		"model":      modelName,
		"messages":   apiMessages,
		"max_tokens": 1024,
	}
	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}
	applyAnthropicChatConfig(reqBody, chatModelConfig)

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	setAnthropicHeaders(req, apiKey)

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic messages API error: %s, body: %s", resp.Status, string(body))
	}

	answer, reasoning, err := parseAnthropicChatResponse(body)
	if err != nil {
		return nil, err
	}
	return &ChatResponse{
		Answer:        &answer,
		ReasonContent: &reasoning,
	}, nil
}

func applyAnthropicChatConfig(reqBody map[string]interface{}, chatModelConfig *ChatConfig) {
	if chatModelConfig == nil {
		return
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
		reqBody["stop_sequences"] = *chatModelConfig.Stop
	}
}

func setAnthropicHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
}

func anthropicMessages(messages []Message) ([]map[string]interface{}, string, error) {
	apiMessages := make([]map[string]interface{}, 0, len(messages))
	systemPrompts := make([]string, 0)
	for _, msg := range messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content, err := anthropicContent(msg.Content)
		if err != nil {
			return nil, "", err
		}
		switch role {
		case "system":
			if text, ok := anthropicSystemText(content); ok && text != "" {
				systemPrompts = append(systemPrompts, text)
			}
		case "user", "assistant":
			apiMessages = append(apiMessages, map[string]interface{}{
				"role":    role,
				"content": content,
			})
		default:
			return nil, "", fmt.Errorf("anthropic: unsupported message role %q", msg.Role)
		}
	}
	if len(apiMessages) == 0 {
		return nil, "", fmt.Errorf("messages is empty")
	}
	return apiMessages, strings.Join(systemPrompts, "\n\n"), nil
}

func anthropicSystemText(content interface{}) (string, bool) {
	switch value := content.(type) {
	case string:
		return value, true
	case []map[string]interface{}:
		parts := make([]string, 0, len(value))
		for _, block := range value {
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n"), true
	default:
		return "", false
	}
}

func anthropicContent(content interface{}) (interface{}, error) {
	switch value := content.(type) {
	case string:
		return value, nil
	case []interface{}:
		return anthropicContentBlocks(value)
	case []map[string]interface{}:
		blocks := make([]interface{}, 0, len(value))
		for _, block := range value {
			blocks = append(blocks, block)
		}
		return anthropicContentBlocks(blocks)
	default:
		return nil, fmt.Errorf("anthropic: unsupported message content type %T", content)
	}
}

func anthropicContentBlocks(blocks []interface{}) ([]map[string]interface{}, error) {
	apiBlocks := make([]map[string]interface{}, 0, len(blocks))
	for _, item := range blocks {
		block, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("anthropic: invalid content block %T", item)
		}
		converted, err := anthropicContentBlock(block)
		if err != nil {
			return nil, err
		}
		apiBlocks = append(apiBlocks, converted)
	}
	return apiBlocks, nil
}

func anthropicContentBlock(block map[string]interface{}) (map[string]interface{}, error) {
	blockType, _ := block["type"].(string)
	switch blockType {
	case "text":
		text, ok := block["text"].(string)
		if !ok {
			return nil, fmt.Errorf("anthropic: text block missing or invalid text field %T", block["text"])
		}
		return map[string]interface{}{"type": "text", "text": text}, nil
	case "image":
		return validateAnthropicImageBlock(block)
	case "image_url":
		return anthropicImageURLBlock(block)
	default:
		return nil, fmt.Errorf("anthropic: unsupported content block type %q", blockType)
	}
}

func validateAnthropicImageBlock(block map[string]interface{}) (map[string]interface{}, error) {
	source, ok := block["source"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("anthropic: image block missing source object")
	}
	sourceType, ok := source["type"].(string)
	if !ok || sourceType == "" {
		return nil, fmt.Errorf("anthropic: image source missing type")
	}
	switch sourceType {
	case "url":
		if url, ok := source["url"].(string); !ok || url == "" {
			return nil, fmt.Errorf("anthropic: image url source missing url")
		}
	case "base64":
		mediaType, ok := source["media_type"].(string)
		if !ok || mediaType == "" {
			return nil, fmt.Errorf("anthropic: image base64 source missing media_type")
		}
		data, ok := source["data"].(string)
		if !ok || data == "" {
			return nil, fmt.Errorf("anthropic: image base64 source missing data")
		}
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			return nil, fmt.Errorf("anthropic: invalid base64 image data: %w", err)
		}
	default:
		return nil, fmt.Errorf("anthropic: unsupported image source type %q", sourceType)
	}
	return block, nil
}

func anthropicImageURLBlock(block map[string]interface{}) (map[string]interface{}, error) {
	imageURL, ok := block["image_url"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("anthropic: image_url block missing image_url object")
	}
	url, _ := imageURL["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("anthropic: image_url block missing url")
	}
	source := map[string]interface{}{
		"type": "url",
		"url":  url,
	}
	if strings.HasPrefix(url, "data:") {
		mediaType, data, err := parseDataImageURL(url)
		if err != nil {
			return nil, err
		}
		source = map[string]interface{}{
			"type":       "base64",
			"media_type": mediaType,
			"data":       data,
		}
	}
	return map[string]interface{}{
		"type":   "image",
		"source": source,
	}, nil
}

func parseDataImageURL(url string) (string, string, error) {
	const marker = ";base64,"
	if !strings.HasPrefix(url, "data:") || !strings.Contains(url, marker) {
		return "", "", fmt.Errorf("anthropic: unsupported data image url")
	}
	trimmed := strings.TrimPrefix(url, "data:")
	parts := strings.SplitN(trimmed, marker, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("anthropic: invalid data image url")
	}
	if _, err := base64.StdEncoding.DecodeString(parts[1]); err != nil {
		return "", "", fmt.Errorf("anthropic: invalid base64 image data: %w", err)
	}
	return parts[0], parts[1], nil
}

func parseAnthropicChatResponse(body []byte) (string, string, error) {
	var result struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Content) == 0 {
		return "", "", fmt.Errorf("no content in Anthropic response")
	}

	var answer strings.Builder
	var reasoning strings.Builder
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			answer.WriteString(block.Text)
		case "thinking":
			reasoning.WriteString(block.Thinking)
		}
	}
	if answer.Len() == 0 {
		return "", "", fmt.Errorf("no text content in Anthropic response")
	}
	return answer.String(), reasoning.String(), nil
}

func (a *AnthropicModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(*apiConfig.ApiKey)

	baseURLRegion := a.region(apiConfig)
	baseURLConfig := &APIConfig{Region: &baseURLRegion}
	if apiConfig != nil {
		baseURLConfig.BaseURL = apiConfig.BaseURL
	}
	baseURL, err := a.baseModel.GetBaseURL(baseURLConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSpace(strings.TrimSuffix(baseURL, "/"))
	url := fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(a.baseModel.URLSuffix.Models, "/"))

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	setAnthropicHeaders(req, apiKey)

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic models API error: %s, body: %s", resp.Status, string(body))
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	models := make([]ListModelResponse, 0, len(result.Data))
	for _, item := range result.Data {
		if item.ID != "" {
			models = append(models, ListModelResponse{
				Name: item.ID,
			})
		}
	}
	return models, nil
}

func (a *AnthropicModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := a.ListModels(apiConfig)
	return err
}

func (a *AnthropicModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}
