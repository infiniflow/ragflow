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
	"strings"
	"sync"
	"time"
)

// xinferenceStreamIdleTimeout bounds how long a stream can go without
// receiving any SSE line. Self-hosted models can be slow, but a stream
// that stays silent for a full minute is more useful as a surfaced error
// than as a stuck goroutine.
var xinferenceStreamIdleTimeout = 60 * time.Second

// XinferenceModel implements ModelDriver for Xinference chat models.
//
// Xinference exposes an OpenAI-compatible API under <endpoint>/v1. The
// tenant may configure either the root endpoint (http://127.0.0.1:9997)
// or the OpenAI-compatible endpoint (http://127.0.0.1:9997/v1); the
// driver normalizes both to the root endpoint before adding URLSuffix
// values that match Xinference docs, such as v1/chat/completions.
// Authentication is optional: no-auth deployments ignore API keys, while
// auth-enabled deployments require Authorization: Bearer <api_key>.
type XinferenceModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

type xinferenceChatChoice struct {
	Message struct {
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content"`
		Reasoning        string `json:"reasoning"`
		Thinking         string `json:"thinking"`
	} `json:"message"`
}

type xinferenceChatResponse struct {
	Choices []xinferenceChatChoice `json:"choices"`
}

type xinferenceModelListResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// NewXinferenceModel creates a new Xinference model instance.
func NewXinferenceModel(baseURL map[string]string, urlSuffix URLSuffix) *XinferenceModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &XinferenceModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (x *XinferenceModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewXinferenceModel(baseURL, x.URLSuffix)
}

func (x *XinferenceModel) Name() string {
	return "xinference"
}

func (x *XinferenceModel) baseURLForRegion(region string) (string, error) {
	if base, ok := x.BaseURL[region]; ok && strings.TrimSpace(base) != "" {
		return normalizeXinferenceBaseURL(base), nil
	}
	if base, ok := x.BaseURL["default"]; ok && strings.TrimSpace(base) != "" {
		return normalizeXinferenceBaseURL(base), nil
	}
	return "", fmt.Errorf("xinference: missing base URL, configure the Xinference endpoint (e.g., http://127.0.0.1:9997 or http://127.0.0.1:9997/v1)")
}

func normalizeXinferenceBaseURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		return trimmed
	}
	if strings.HasSuffix(trimmed, "/v1") {
		return strings.TrimSuffix(trimmed, "/v1")
	}
	return trimmed
}

func setXinferenceAuth(req *http.Request, apiConfig *APIConfig) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
}

func xinferenceRegion(apiConfig *APIConfig) string {
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		return *apiConfig.Region
	}
	return "default"
}

func xinferenceReasoningFromStrings(reasoningContent string, reasoning string, thinking string) string {
	switch {
	case reasoningContent != "":
		return reasoningContent
	case reasoning != "":
		return reasoning
	case thinking != "":
		return thinking
	default:
		return ""
	}
}

func xinferenceReasoningFromMap(value map[string]interface{}) string {
	for _, field := range []string{"reasoning_content", "reasoning", "thinking"} {
		if text, ok := value[field].(string); ok && text != "" {
			return text
		}
	}
	return ""
}

func buildXinferenceChatBody(modelName string, messages []Message, stream bool, chatModelConfig *ChatConfig) map[string]interface{} {
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   stream,
	}

	if chatModelConfig != nil {
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
	}

	return reqBody
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (x *XinferenceModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	baseURL, err := x.baseURLForRegion(xinferenceRegion(apiConfig))
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, x.URLSuffix.Chat)

	reqBody := buildXinferenceChatBody(modelName, messages, false, chatModelConfig)
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
	setXinferenceAuth(req, apiConfig)

	resp, err := x.httpClient.Do(req)
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

	var result xinferenceChatResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := result.Choices[0].Message.Content
	reasonContent := xinferenceReasoningFromStrings(
		result.Choices[0].Message.ReasoningContent,
		result.Choices[0].Message.Reasoning,
		result.Choices[0].Message.Thinking,
	)

	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

// ChatStreamlyWithSender sends messages and streams response via sender.
func (x *XinferenceModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if chatModelConfig != nil && chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}

	baseURL, err := x.baseURLForRegion(xinferenceRegion(apiConfig))
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", baseURL, x.URLSuffix.Chat)

	reqBody := buildXinferenceChatBody(modelName, messages, true, chatModelConfig)
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	setXinferenceAuth(req, apiConfig)

	resp, err := x.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	lastActive := time.Now()
	var lastActiveMu sync.Mutex
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(xinferenceStreamIdleTimeout / 4)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				lastActiveMu.Lock()
				idle := now.Sub(lastActive)
				lastActiveMu.Unlock()
				if idle >= xinferenceStreamIdleTimeout {
					cancel()
					return
				}
			}
		}
	}()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	sawTerminal := false
	for scanner.Scan() {
		lastActiveMu.Lock()
		lastActive = time.Now()
		lastActiveMu.Unlock()

		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "[DONE]" {
			sawTerminal = true
			break
		}

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

		if delta, ok := firstChoice["delta"].(map[string]interface{}); ok {
			if reasoning := xinferenceReasoningFromMap(delta); reasoning != "" {
				if err := sender(nil, &reasoning); err != nil {
					return err
				}
			}
			if content, ok := delta["content"].(string); ok && content != "" {
				if err := sender(&content, nil); err != nil {
					return err
				}
			}
		}

		if finishReason, ok := firstChoice["finish_reason"].(string); ok && finishReason != "" {
			sawTerminal = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("xinference: stream idle for more than %s, aborted", xinferenceStreamIdleTimeout)
		}
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("xinference: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

func (x *XinferenceModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

// ListModels returns the model IDs exposed by Xinference's OpenAI-compatible
// /v1/models endpoint.
func (x *XinferenceModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	baseURL, err := x.baseURLForRegion(xinferenceRegion(apiConfig))
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, x.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	setXinferenceAuth(req, apiConfig)

	resp, err := x.httpClient.Do(req)
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

	var result xinferenceModelListResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]string, 0, len(result.Data))
	for _, model := range result.Data {
		if model.ID != "" {
			models = append(models, model.ID)
		}
	}
	return models, nil
}

func (x *XinferenceModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := x.ListModels(apiConfig)
	return err
}

func (x *XinferenceModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}

func (x *XinferenceModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", x.Name())
}
