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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MinimaxModel implements ModelDriver for Minimax
type MinimaxModel struct {
	baseModel BaseModel
}

// NewMinimaxModel creates a new Minimax model instance
func NewMinimaxModel(baseURL map[string]string, urlSuffix URLSuffix) *MinimaxModel {
	return &MinimaxModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(),
		},
	}
}

func (m *MinimaxModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMinimaxModel(baseURL, m.baseModel.URLSuffix)
}

func (m *MinimaxModel) Name() string {
	return "minimax"
}

func validateMinimaxModelName(modelName string) (string, error) {
	if strings.TrimSpace(modelName) == "" {
		return "", fmt.Errorf("model name is required")
	}
	return strings.TrimSpace(modelName), nil
}

// ChatWithMessages sends multiple messages with roles and returns response
func (m *MinimaxModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(*apiConfig.ApiKey)
	modelName, err := validateMinimaxModelName(modelName)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.Chat)

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
		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}

		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}

		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}

		if chatModelConfig.DoSample != nil {
			reqBody["do_sample"] = *chatModelConfig.DoSample
		}

		if chatModelConfig != nil && chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{
					"type": "adaptive",
				}
				reqBody["reasoning_split"] = true
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to send request: %d %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
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

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (m *MinimaxModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, sender func(*string, *string) error) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
	apiKey := strings.TrimSpace(*apiConfig.ApiKey)
	modelName, err := validateMinimaxModelName(modelName)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.Chat)

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

	if modelConfig != nil {
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

		if modelConfig.Thinking != nil {
			if *modelConfig.Thinking {
				reqBody["thinking"] = map[string]interface{}{
					"type": "adaptive",
				}
				reqBody["reasoning_split"] = true
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Accept", "text/event-stream")

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: read line by line
	sawTerminal := false
	done, err := ParseSSEStream[map[string]interface{}](resp.Body, func(event map[string]interface{}) error {
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

		finishReason, ok := firstChoice["finish_reason"].(string)
		if ok && finishReason != "" {
			sawTerminal = true
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !done && !sawTerminal {
		return fmt.Errorf("minimax: stream ended before [DONE] or finish_reason")
	}

	// Send [DONE] marker for OpenAI compatibility
	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

// Embed embeds a list of texts into embeddings
func (m *MinimaxModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MinimaxModel) ListModels(apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(*apiConfig.ApiKey)

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Accept", "application/json")

	resp, err := m.baseModel.httpClient.Do(req)
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
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return ParseListModel(modelList), nil
}

func (m *MinimaxModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MinimaxModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := m.ListModels(apiConfig)
	return err
}

// Rerank calculates similarity scores between query and documents
func (m *MinimaxModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", m.Name())
}

// TranscribeAudio transcribe audio
func (m *MinimaxModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MinimaxModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// AudioSpeech convert text to audio
func (m *MinimaxModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("text content is empty")
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.TTS)

	reqBody := map[string]interface{}{
		"model": modelName,
		"text":  audioContent,
	}
	if ttsConfig != nil && ttsConfig.Params != nil {
		for key, value := range ttsConfig.Params {
			reqBody[key] = value
		}
	}
	if ttsConfig != nil && ttsConfig.Format != "" {
		reqBody["audio_setting"] = map[string]interface{}{
			"format": ttsConfig.Format,
		}
	}
	reqBody["stream"] = false

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MiniMax TTS API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
		Data struct {
			Audio string `json:"audio"` // HEX
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("MiniMax TTS returned error: %d - %s", result.BaseResp.StatusCode, result.BaseResp.StatusMsg)
	}

	// format HEX
	audioBytes, err := hex.DecodeString(result.Data.Audio)
	if err != nil {
		return nil, fmt.Errorf("failed to decode MiniMax hex audio: %w", err)
	}

	return &TTSResponse{
		Audio: audioBytes,
	}, nil
}

func (m *MinimaxModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
	if audioContent == nil || *audioContent == "" {
		return fmt.Errorf("text content is empty")
	}

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	baseURL := strings.TrimSuffix(resolvedBaseURL, "/")
	if baseURL == "" {
		baseURL = strings.TrimSuffix(resolvedBaseURL, "/")
	}
	suffix := strings.TrimPrefix(m.baseModel.URLSuffix.TTS, "/")
	if suffix == "" {
		suffix = "v1/t2a_v2"
	}
	url := fmt.Sprintf("%s/%s", baseURL, suffix)

	reqBody := map[string]interface{}{
		"model": modelName,
		"text":  audioContent,
	}
	if ttsConfig != nil && ttsConfig.Params != nil {
		for key, value := range ttsConfig.Params {
			reqBody[key] = value
		}
	}
	reqBody["stream"] = false

	if ttsConfig != nil && ttsConfig.Format != "" {
		reqBody["audio_setting"] = map[string]interface{}{
			"format": ttsConfig.Format,
		}
	}

	reqBody["stream"] = true

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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", strings.TrimSpace(*apiConfig.ApiKey)))

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MiniMax stream TTS API error: %d, body: %s", resp.StatusCode, string(body))
	}

	type minimaxTTSEvent struct {
		Data struct {
			Audio  string `json:"audio"`
			Status int    `json:"status"`
		} `json:"data"`
	}

	if _, err := ParseSSEStream[minimaxTTSEvent](resp.Body, func(event minimaxTTSEvent) error {
		if event.Data.Audio != "" {
			audioBytes, err := hex.DecodeString(event.Data.Audio)
			if err == nil && len(audioBytes) > 0 {
				chunk := string(audioBytes)
				if errSend := sender(&chunk, nil); errSend != nil {
					return errSend
				}
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}

	return nil
}

// OCRFile OCR file
func (m *MinimaxModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ParseFile parse file
func (m *MinimaxModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MinimaxModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MinimaxModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
