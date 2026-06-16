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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/common"
	"strings"
)

// VolcEngine implements ModelDriver for VolcEngine
type VolcEngine struct {
	baseModel BaseModel
}

// NewVolcEngine creates a new VolcEngine model instance
func NewVolcEngine(baseURL map[string]string, urlSuffix URLSuffix) *VolcEngine {
	return &VolcEngine{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (v *VolcEngine) NewInstance(baseURL map[string]string) ModelDriver {
	return NewVolcEngine(baseURL, v.baseModel.URLSuffix)
}

func (v *VolcEngine) Name() string {
	return "volcengine"
}

// ChatWithMessages sends multiple messages with roles and returns response
func (v *VolcEngine) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, v.baseModel.URLSuffix.Chat)

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

		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				var thinkingFlag string
				effort := "medium"
				if chatModelConfig.Effort != nil {
					effort = *chatModelConfig.Effort
				}
				switch effort {
				case "none", "minimal":
					thinkingFlag = "disabled"
					reqBody["reasoning_effort"] = "minimal"
				case "low":
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = "low"
				case "medium":
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = "medium"
				case "auto", "default":
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = "medium"
				case "high":
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = "high"
				default:
					thinkingFlag = "enabled"
					reqBody["reasoning_effort"] = effort
				}
				reqBody["thinking"] = map[string]interface{}{
					"type": thinkingFlag,
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

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := v.baseModel.httpClient.Do(req)
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

	// Parse response
	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
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
			return nil, fmt.Errorf("invalid reasonContent format")
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
func (v *VolcEngine) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/chat/completions", resolvedBaseURL)

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	// Build request body with streaming enabled
	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      true,
		"temperature": 1,
	}

	if modelConfig == nil {
		modelConfig = &ChatConfig{}
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

	if modelConfig.TopP != nil {
		reqBody["top_p"] = *modelConfig.TopP
	}

	if modelConfig.DoSample != nil {
		reqBody["do_sample"] = *modelConfig.DoSample
	}

	if modelConfig.Stop != nil {
		reqBody["stop"] = *modelConfig.Stop
	}

	// TODO VolcEngine has `auto` mode
	if modelConfig.Thinking != nil {
		if *modelConfig.Thinking {
			var thinkingFlag string
			effort := "medium"
			if modelConfig.Effort != nil {
				effort = *modelConfig.Effort
			}
			switch effort {
			case "none", "minimal":
				thinkingFlag = "disabled"
				reqBody["reasoning_effort"] = "minimal"
				break
			case "low":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "low"
				break
			case "medium":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "medium"
				break
			case "auto", "default":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "medium"
				break
			case "high":
				thinkingFlag = "enabled"
				reqBody["reasoning_effort"] = "high"
				break
			default:
				return fmt.Errorf("invalid effort level")
			}
			reqBody["thinking"] = map[string]interface{}{
				"type": thinkingFlag,
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

	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := v.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if _, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		common.Info(fmt.Sprintf("%v", event))

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return nil
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			return nil
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			return nil
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		reasoningContent, ok := delta["reasoning_content"].(string)
		if ok && reasoningContent != "" {
			if err := sender(nil, &reasoningContent); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	if err = sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

type volcengineEmbeddingResponse struct {
	Created int64                   `json:"created"`
	Data    volcengineEmbeddingData `json:"data"`
	ID      string                  `json:"id"`
	Model   string                  `json:"model"`
	Object  string                  `json:"object"`
	Usage   volcengineUsage         `json:"usage"`
}

type volcengineEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
}

type volcengineUsage struct {
	PromptTokens        int                            `json:"prompt_tokens"`
	TotalTokens         int                            `json:"total_tokens"`
	PromptTokensDetails *volcenginePromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

type volcenginePromptTokensDetails struct {
	ImageTokens int `json:"image_tokens"`
	TextTokens  int `json:"text_tokens"`
}

// Embed embeds a list of texts into embeddings
func (v *VolcEngine) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, v.baseModel.URLSuffix.Embedding)

	var embeddings []EmbeddingData

	for i, text := range texts {

		reqBody := map[string]interface{}{
			"model":           *modelName,
			"encoding_format": "float",
			"input": []map[string]interface{}{
				{
					"type": "text",
					"text": text,
				},
			},
		}
		if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
			reqBody["dimensions"] = embeddingConfig.Dimension
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to marshal request: %w",
				err,
			)
		}

		// Run each per-text request in its own scope so the context's
		// deadline is cancelled at the end of every iteration instead of
		// piling up deferred cancels until the whole batch finishes.
		parsed, err := func() (volcengineEmbeddingResponse, error) {
			var parsed volcengineEmbeddingResponse

			ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				return parsed, fmt.Errorf("failed to create request: %w", err)
			}

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

			resp, err := v.baseModel.httpClient.Do(req)
			if err != nil {
				return parsed, fmt.Errorf("failed to send request: %w", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return parsed, fmt.Errorf("failed to read response: %w", err)
			}

			if resp.StatusCode != http.StatusOK {
				return parsed, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
			}

			if err = json.Unmarshal(body, &parsed); err != nil {
				return parsed, fmt.Errorf("failed to parse response: %w", err)
			}
			return parsed, nil
		}()
		if err != nil {
			return nil, err
		}

		var embeddingData EmbeddingData
		embeddingData.Index = i
		embeddingData.Embedding = parsed.Data.Embedding
		embeddings = append(embeddings, embeddingData)
	}

	return embeddings, nil
}

// Rerank calculates similarity scores between query and documents
func (v *VolcEngine) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", v.Name())
}

// TranscribeAudio transcribe audio
func (v *VolcEngine) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VolcEngine) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", v.Name())
}

// AudioSpeech convert text to audio
func (v *VolcEngine) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VolcEngine) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", v.Name())
}

// OCRFile OCR file
func (v *VolcEngine) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

// ParseFile parse file
func (v *VolcEngine) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VolcEngine) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL
	if baseURL == "" {
		baseURL = resolvedBaseURL
	}
	modelsSuffix := strings.Trim(strings.TrimSpace(v.baseModel.URLSuffix.Models), "/")
	if modelsSuffix == "" {
		return nil, fmt.Errorf("volcengine: models URL suffix is not configured")
	}

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), modelsSuffix)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := v.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("VolcEngine models API error: %s, body: %s", resp.Status, string(body))
	}

	// Parse response
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return ParseListModel(modelList), nil
}

func (v *VolcEngine) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VolcEngine) CheckConnection(apiConfig *APIConfig) error {
	if err := v.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	resolvedBaseURL, err := v.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, v.baseModel.URLSuffix.Files)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := v.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (v *VolcEngine) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}

func (v *VolcEngine) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", v.Name())
}
