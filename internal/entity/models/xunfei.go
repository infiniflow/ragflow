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
)

type XunFeiModel struct {
	baseModel BaseModel
}

func NewXunFeiModel(baseURL map[string]string, urlSuffix URLSuffix) *XunFeiModel {
	return &XunFeiModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (x *XunFeiModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewXunFeiModel(baseURL, x.baseModel.URLSuffix)
}

func (x *XunFeiModel) Name() string {
	return "xunfei"
}

func (x *XunFeiModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, x.baseModel.URLSuffix.Chat)

	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      false,
		"temperature": 1,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}

		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}

		if chatModelConfig.Stream != nil {
			reqBody["stream"] = *chatModelConfig.Stream
		}

		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
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
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := x.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse Response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("no choices in response")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no choices in response")
	}

	messageMap, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no message in response")
	}

	content, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("no message in response")
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

func (x *XunFeiModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, x.baseModel.URLSuffix.Chat)

	// Convert messages to API format
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":       modelName,
		"messages":    apiMessages,
		"stream":      true,
		"temperature": 1,
	}

	if modelConfig != nil {
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

	resp, err := x.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("invalid status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	if _, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
		if data, marshalErr := json.Marshal(event); marshalErr == nil {
			common.Info(string(data))
		}

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

func (x *XunFeiModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := x.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := x.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, x.baseModel.URLSuffix.Models)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := x.baseModel.httpClient.Do(req)
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
	// Parse response
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	return ParseListModel(modelList), nil
}

func (x *XunFeiModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XunFeiModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}
