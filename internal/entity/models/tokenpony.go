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
	"time"
)

// TokenPonyModel implements ModelDriver for TokenPony. TokenPony is a
// SaaS, OpenAI-compatible inference gateway hosting Qwen, DeepSeek,
// Kimi, GLM, MiniMax, and Hunyuan families behind a single Bearer-token
// API at https://ragflow.vip-api.tokenpony.cn/v1.
//
// Wire shape matches the OpenAI convention exactly:
//   - POST /v1/chat/completions with {model, messages, stream, ...}
//   - GET  /v1/models for the catalog
//   - Authorization: Bearer <api-key> on every call
//   - SSE response with `data:` lines and a [DONE] terminator
//
// Reasoning models surface chain-of-thought in `reasoning_content`
// (OpenAI o-series shape), so the same handling as LongCat /
// DeepSeek-R1 applies and there's no need for an inline <think>...
// extractor like Novita's.
type TokenPonyModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewTokenPonyModel creates a new TokenPony model instance.
//
// Same transport convention as the other Go drivers in this package:
// clone http.DefaultTransport to keep ProxyFromEnvironment, DialContext,
// HTTP/2, and TLS defaults, and only override the connection-pool
// fields. No client-level Timeout so SSE streams aren't capped mid-flight.
func NewTokenPonyModel(baseURL map[string]string, urlSuffix URLSuffix) *TokenPonyModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &TokenPonyModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (a *TokenPonyModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewTokenPonyModel(baseURL, a.URLSuffix)
}

func (a *TokenPonyModel) Name() string {
	return "tokenpony"
}

func (a *TokenPonyModel) baseURLForRegion(region string) (string, error) {
	base, ok := a.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("tokenpony: no base URL configured for region %q", region)
	}
	return strings.TrimSuffix(base, "/"), nil
}

// ChatWithMessages sends a non-streaming chat request and returns the
// full response. Forwards documented OpenAI-shaped parameters when the
// caller supplies them; reasoning_content is surfaced separately so the
// visible Answer is never polluted by chain-of-thought.
func (a *TokenPonyModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := a.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, a.URLSuffix.Chat)

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
		"stream":   false,
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

	resp, err := a.httpClient.Do(req)
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

	// Reasoning models (deepseek-r1 / kimi / glm-thinking) put
	// chain-of-thought in a separate `reasoning_content` field with
	// `content` already cleaned. Absent or non-string means no reasoning
	// was emitted; leave it empty rather than synthesizing one.
	reasonContent := ""
	if r, ok := messageMap["reasoning_content"].(string); ok {
		reasonContent = r
	}

	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

// ChatStreamlyWithSender opens the SSE chat-completions endpoint and
// forwards each delta through the supplied sender. Reasoning chunks go
// to the sender's second argument, content chunks to the first; the
// stream is terminated by either `[DONE]` or a delta with finish_reason.
func (a *TokenPonyModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return fmt.Errorf("api key is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := a.baseURLForRegion(region)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", baseURL, a.URLSuffix.Chat)

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
		"stream":   true,
	}

	if chatModelConfig != nil {
		// Guard against the caller asking for stream=false on a code path
		// that only knows how to read SSE. Without this, a non-SSE JSON
		// body would parse as zero chunks and look like a silent timeout.
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
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
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// SSE is long-lived; rely on the transport's ResponseHeaderTimeout
	// to cap connection-establishment instead of a hard deadline.
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	sawTerminal := false
	for scanner.Scan() {
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
			// A malformed frame usually means a truncated event or an
			// upstream incident. Surface it instead of silently producing
			// partial output.
			return fmt.Errorf("tokenpony: invalid SSE event: %w", err)
		}

		// TokenPony can emit a terminal `{"error": ...}` frame when the
		// upstream model rejects mid-stream (rate limit, content policy).
		// Surface it verbatim instead of falling through to "no choices".
		if apiErr, ok := event["error"]; ok {
			return fmt.Errorf("tokenpony: upstream stream error: %v", apiErr)
		}

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}
		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}
		// Reasoning first, content second — matches the wire ordering
		// for reasoning models and lets UIs render the chain-of-thought
		// before the visible token. A terminal frame may carry
		// finish_reason without a delta, so don't skip when delta is absent.
		if delta, ok := firstChoice["delta"].(map[string]interface{}); ok {
			if r, ok := delta["reasoning_content"].(string); ok && r != "" {
				rr := r
				if err := sender(nil, &rr); err != nil {
					return err
				}
			}
			if c, ok := delta["content"].(string); ok && c != "" {
				cc := c
				if err := sender(&cc, nil); err != nil {
					return err
				}
			}
		}
		if finish, ok := firstChoice["finish_reason"].(string); ok && finish != "" {
			sawTerminal = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("tokenpony: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}
	return nil
}

// ListModels returns the model ids visible to the API key by calling
// /v1/models. Used by Add-Provider's connection check and by the UI's
// model picker.
func (a *TokenPonyModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := a.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, a.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.httpClient.Do(req)
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

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid models list format")
	}

	models := make([]string, 0, len(data))
	for _, m := range data {
		modelMap, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		id, ok := modelMap["id"].(string)
		if !ok {
			continue
		}
		models = append(models, id)
	}
	return models, nil
}

// CheckConnection verifies the API key by calling ListModels. The /v1/models
// endpoint is the documented lightweight way to validate credentials on
// OpenAI-compatible gateways without burning chat-completion quota.
func (a *TokenPonyModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := a.ListModels(apiConfig)
	return err
}

// Embed is not implemented for TokenPony in this initial driver; the
// factory entry only registers chat models. Mirrors how LongCat /
// Astraflow landed chat-only.
func (a *TokenPonyModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *TokenPonyModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}
