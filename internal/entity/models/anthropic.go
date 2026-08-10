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
	"ragflow/internal/common"
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
			httpClient: NewDriverHTTPClient(false),
			// Anthropic authenticates with the "x-api-key" header instead
			// of the default "Authorization: Bearer".
			authHeader: func(cfg *APIConfig) (string, string) {
				return "x-api-key", strings.TrimSpace(*cfg.ApiKey)
			},
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

func (a *AnthropicModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	apiMessages, systemPrompt, err := anthropicMessages(messages)
	if err != nil {
		return nil, err
	}

	baseURLRegion := a.region(apiConfig)
	baseURLConfig := &APIConfig{Region: &baseURLRegion}
	baseURLConfig.BaseURL = apiConfig.BaseURL

	baseURL, err := a.baseModel.GetBaseURL(baseURLConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSpace(strings.TrimSuffix(baseURL, "/"))
	url := fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(a.baseModel.URLSuffix.Chat, "/"))

	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
	}
	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}
	applyAnthropicChatConfig(reqBody, chatModelConfig)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := a.baseModel.newJSONPostRequest(ctx, url, apiConfig, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Accept", "application/json")

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
		return nil, fmt.Errorf("anthropic messages API error: %s, body: %s", resp.Status, string(body))
	}

	return parseChatCompletionResponse(body, chatModelConfig, modelUsage, parseAnthropicChatResponse)
}

func applyAnthropicChatConfig(reqBody map[string]interface{}, chatModelConfig *ChatConfig) {
	if chatModelConfig == nil {
		return
	}
	if chatModelConfig.MaxTokens != nil {
		reqBody["max_tokens"] = *chatModelConfig.MaxTokens
	} else {
		reqBody["max_tokens"] = 1024 // default when not configured
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

func setAnthropicHeaders(req *http.Request, apiKey string, streaming bool) {
	req.Header.Set("Content-Type", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
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

func parseAnthropicChatResponse(body []byte, _ *ChatConfig) (chatResponseParts, error) {
	var result struct {
		ID      string `json:"id"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return chatResponseParts{}, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Content) == 0 {
		return chatResponseParts{}, fmt.Errorf("no content in Anthropic response")
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
		return chatResponseParts{}, fmt.Errorf("no text content in Anthropic response")
	}

	usage := &TokenUsage{
		PromptTokens:     result.Usage.InputTokens,
		CompletionTokens: result.Usage.OutputTokens,
		TotalTokens:      result.Usage.InputTokens + result.Usage.OutputTokens,
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		usage = nil
	}

	ans := answer.String()
	reason := reasoning.String()
	return chatResponseParts{
		RequestID:     result.ID,
		Content:       &ans,
		ReasonContent: &reason,
		Usage:         usage,
	}, nil
}

func (a *AnthropicModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(*apiConfig.ApiKey)

	baseURLRegion := a.region(apiConfig)
	baseURLConfig := &APIConfig{Region: &baseURLRegion}
	baseURLConfig.BaseURL = apiConfig.BaseURL

	baseURL, err := a.baseModel.GetBaseURL(baseURLConfig)
	if err != nil {
		return nil, err
	}
	baseURL = strings.TrimSpace(strings.TrimSuffix(baseURL, "/"))
	url := fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(a.baseModel.URLSuffix.Models, "/"))

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	setAnthropicHeaders(req, apiKey, false)

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
		return nil, fmt.Errorf("anthropic models API error: %s, body: %s", resp.Status, string(body))
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

func (a *AnthropicModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := a.ListModels(ctx, apiConfig)
	return err
}

func (a *AnthropicModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
	apiKey := strings.TrimSpace(*apiConfig.ApiKey)
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	apiMessages, systemPrompt, err := anthropicMessages(messages)
	if err != nil {
		return err
	}

	baseURLRegion := a.region(apiConfig)
	baseURLConfig := &APIConfig{Region: &baseURLRegion}
	baseURLConfig.BaseURL = apiConfig.BaseURL

	baseURL, err := a.baseModel.GetBaseURL(baseURLConfig)
	if err != nil {
		return err
	}
	baseURL = strings.TrimSpace(strings.TrimSuffix(baseURL, "/"))
	url := fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(a.baseModel.URLSuffix.Chat, "/"))

	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   true,
	}
	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}
	applyAnthropicChatConfig(reqBody, modelConfig)

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	setAnthropicHeaders(req, apiKey, true)

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("anthropic messages API error: %s, body: %s", resp.Status, string(body))
	}

	sawTerminal := false
	var streamUsage TokenUsage
	sawUsage := false
	done, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		eventType, _ := event["type"].(string)
		switch eventType {
		case "content_block_delta":
			delta, ok := event["delta"].(map[string]interface{})
			if !ok {
				return nil
			}
			deltaType, _ := delta["type"].(string)
			switch deltaType {
			case "text_delta":
				if text, ok := delta["text"].(string); ok && text != "" {
					if err := sender(&text, nil); err != nil {
						return err
					}
				}
			case "thinking_delta":
				if thinking, ok := delta["thinking"].(string); ok && thinking != "" {
					if err := sender(nil, &thinking); err != nil {
						return err
					}
				}
			}
		case "message_start":
			message, ok := event["message"].(map[string]interface{})
			if !ok {
				return nil
			}
			if usage, ok := message["usage"].(map[string]interface{}); ok {
				if inputTokens, ok := usage["input_tokens"].(float64); ok {
					streamUsage.PromptTokens = int(inputTokens)
					sawUsage = true
				}
			}
		case "message_delta":
			// message_delta carries the running total of output tokens
			// generated so far; the last event before message_stop is
			// authoritative.
			if usage, ok := event["usage"].(map[string]interface{}); ok {
				if outputTokens, ok := usage["output_tokens"].(float64); ok {
					streamUsage.CompletionTokens = int(outputTokens)
					sawUsage = true
				}
			}
		case "message_stop":
			sawTerminal = true
		case "error":
			errInfo, _ := event["error"].(map[string]interface{})
			message, _ := errInfo["message"].(string)
			return fmt.Errorf("anthropic stream error: %s", message)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !done && !sawTerminal {
		return fmt.Errorf("anthropic: stream ended before message_stop")
	}

	if sawUsage {
		streamUsage.TotalTokens = streamUsage.PromptTokens + streamUsage.CompletionTokens
		applyStreamUsage(modelConfig, modelUsage, &streamUsage)
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

func (a *AnthropicModel) Embed(ctx context.Context, modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) Rerank(ctx context.Context, modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AnthropicModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}
