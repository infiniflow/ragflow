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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"ragflow/internal/common"
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
			httpClient: NewDriverHTTPClient(false),
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

// extractMinimaxAPIError checks a parsed MiniMax response for
// provider-specific or OpenAI-compatible error payloads.
//
// MiniMax can embed errors in the response body even when the HTTP
// status is 200 — the classic case is rate limiting, where the body
// carries a `base_resp` block with a non-zero `status_code` instead
// of the expected `choices` array. Returning the original message
// (which typically contains "rate limit" / "frequency limit" / "429")
// lets the upstream retry predicates match and retry appropriately.
//
// Returns the error description, or "" when no error is present.
func extractMinimaxAPIError(result map[string]interface{}) string {
	if br, ok := result["base_resp"].(map[string]interface{}); ok {
		if sc, _ := br["status_code"].(float64); sc != 0 {
			msg, _ := br["status_msg"].(string)
			if msg == "" {
				return fmt.Sprintf("status_code %v", sc)
			}
			return msg
		}
	}
	if e, ok := result["error"].(map[string]interface{}); ok {
		if msg, _ := e["message"].(string); msg != "" {
			return msg
		}
	}
	return ""
}

// extractMinimaxErrorBody parses a raw HTTP error response body and
// returns the human-readable error message when the body is a
// MiniMax or OpenAI-compatible error JSON. Falls back to the raw
// body string when parsing fails so no information is lost.
func extractMinimaxErrorBody(body []byte) string {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body)
	}
	if msg := extractMinimaxAPIError(result); msg != "" {
		return msg
	}
	return string(body)
}

// ChatWithMessages sends multiple messages with roles and returns response
func (m *MinimaxModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
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

	// Build request body
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {
		if chatModelConfig.Thinking != nil {
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

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	// MiniMax can embed errors in a base_resp block with HTTP 200.
	// Check for these before using the shared handler so the caller
	// sees the real error message instead of "no choices in response".
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if errMsg := extractMinimaxAPIError(result); errMsg != "" {
		return nil, fmt.Errorf("minimax API error: %s", errMsg)
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (m *MinimaxModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, modelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}
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

	// Build request body with streaming enabled
	reqBody := buildRequestBody(modelConfig, modelName, messages, true)

	if modelConfig != nil {
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

	reqBody["stream_options"] = map[string]interface{}{"include_usage": true}

	return m.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		// Pipe the response through a base_resp checker. MiniMax can send
		// error events (e.g. rate limits) without a choices array, and the
		// shared handler skips those silently. We surface them so the retry
		// predicates can match and the caller sees the real reason.
		pr, pw := io.Pipe()
		defer pr.Close()
		streamErr := make(chan error, 1)
		go func() {
			defer pw.Close()
			// resp.Body is owned by doStreamRequest — do NOT close it here.

			var scanErr error
			// Ensure streamErr always receives a result, on every exit
			// path, so the final receive below can never block.
			defer func() {
				select {
				case streamErr <- scanErr:
				default:
				}
			}()

			scanner := bufio.NewScanner(body)
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data:") {
					data := strings.TrimSpace(line[5:])
					if data != "" && data != "[DONE]" {
						var event map[string]any
						if json.Unmarshal([]byte(data), &event) == nil {
							if errMsg := extractMinimaxAPIError(event); errMsg != "" {
								pw.CloseWithError(fmt.Errorf("minimax API error: %s", errMsg))
								return
							}
						}
					}
				}
				if _, err := pw.Write([]byte(line + "\n")); err != nil {
					scanErr = err
					return
				}
			}
			scanErr = scanner.Err()
		}()

		if err := HandleStreamingResponse(pr, modelUsage, modelConfig, OpenAIParserConfig, sender); err != nil {
			return err
		}
		return <-streamErr
	})
}

// Embed embeds a list of texts into embeddings
func (m *MinimaxModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *MinimaxModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	apiKey := strings.TrimSpace(*apiConfig.ApiKey)

	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
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
		return nil, fmt.Errorf("minimax API error: status %d: %s", resp.StatusCode, extractMinimaxErrorBody(body))
	}

	// Parse response
	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}
	models := ParseListModel(modelList)
	if len(models) == 0 {
		return nil, fmt.Errorf("invalid models list format")
	}
	return models, nil
}

func (m *MinimaxModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MinimaxModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}

// Rerank calculates similarity scores between query and documents
func (m *MinimaxModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, Rerank not implemented", m.Name())
}

// TranscribeAudio transcribe audio
func (m *MinimaxModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MinimaxModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// AudioSpeech convert text to audio
func (m *MinimaxModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
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

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
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

func (m *MinimaxModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
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

	ctx, cancel := context.WithTimeout(ctx, streamCallTimeout)
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
func (m *MinimaxModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ParseFile parse file
func (m *MinimaxModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MinimaxModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *MinimaxModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
