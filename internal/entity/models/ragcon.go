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

// RAGconModel implements ModelDriver for RAGcon (https://connect.ragcon.com).
//
// RAGcon is a SaaS inference platform fronting a LiteLLM proxy with an
// OpenAI-compatible REST surface (chat completions at /v1/chat/completions,
// list models at /v1/models). The Python side already supports the chat
// path at rag/llm/chat_model.py::RAGconChat with the same default base URL,
// so this driver brings the new Go API server to parity for the LLM tag.
//
// The default base URL is https://connect.ragcon.com; the tenant may
// override per-instance. Authentication is always required: every call
// sets Authorization: Bearer <api_key>. Reasoning models surface their
// thinking in either `reasoning_content` (preferred) or `reasoning`; the
// driver extracts whichever is non-empty and routes it to
// ChatResponse.ReasonContent (non-stream) or the sender's second argument
// (stream).
type RAGconModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewRAGconModel creates a new RAGcon model instance.
//
// Same transport convention as other Go SaaS drivers in this package:
// clone http.DefaultTransport, override the connection-pool fields, no
// client-level Timeout so SSE streams are not capped at the client layer
// (per-request contexts handle that).
func NewRAGconModel(baseURL map[string]string, urlSuffix URLSuffix) *RAGconModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &RAGconModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (r *RAGconModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewRAGconModel(baseURL, r.URLSuffix)
}

func (r *RAGconModel) Name() string {
	return "ragcon"
}

func (r *RAGconModel) baseURLForRegion(region string) (string, error) {
	base, ok := r.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("ragcon: no base URL configured for region %q", region)
	}
	// Tenants may paste in a base URL that already includes the API
	// version (".../v1" or ".../v1/"); callers append "v1/..." so we
	// strip those plus any trailing "/" to avoid "/v1/v1/..." paths.
	base = strings.TrimSuffix(base, "/")
	base = strings.TrimSuffix(base, "/v1")
	return strings.TrimSuffix(base, "/"), nil
}

func (r *RAGconModel) chatPayload(modelName string, messages []Message, stream bool, chatModelConfig *ChatConfig) map[string]interface{} {
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

func (r *RAGconModel) chatURL(apiConfig *APIConfig) (string, error) {
	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := r.baseURLForRegion(region)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", baseURL, r.URLSuffix.Chat), nil
}

type ragconChatMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
	Reasoning        string `json:"reasoning"`
}

type ragconChatChoice struct {
	Message      ragconChatMessage `json:"message"`
	Delta        ragconChatMessage `json:"delta"`
	FinishReason string            `json:"finish_reason"`
}

type ragconChatResponse struct {
	Choices      []ragconChatChoice `json:"choices"`
	Error        interface{}        `json:"error"`
	FinishReason string             `json:"finish_reason"`
}

func (r *RAGconModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, err := r.chatURL(apiConfig)
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(r.chatPayload(modelName, messages, false, chatModelConfig))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := r.httpClient.Do(req)
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

	var result ragconChatResponse
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("ragcon: upstream error: %v", result.Error)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := result.Choices[0].Message.Content
	reasonContent := result.Choices[0].Message.ReasoningContent
	if reasonContent == "" {
		reasonContent = result.Choices[0].Message.Reasoning
	}
	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

func (r *RAGconModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return fmt.Errorf("api key is required")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if chatModelConfig != nil && chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}

	url, err := r.chatURL(apiConfig)
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(r.chatPayload(modelName, messages, true, chatModelConfig))
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// ResponseHeaderTimeout caps the initial header wait. This context
	// also caps the body-read phase so a stalled SSE stream cannot hold
	// the caller's goroutine and connection indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), streamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := r.httpClient.Do(req)
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

		var event ragconChatResponse
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("ragcon: invalid SSE event: %w", err)
		}
		if event.Error != nil {
			return fmt.Errorf("ragcon: upstream stream error: %v", event.Error)
		}
		if len(event.Choices) == 0 {
			continue
		}

		choice := event.Choices[0]
		if reasoning := choice.Delta.ReasoningContent; reasoning != "" {
			if err := sender(nil, &reasoning); err != nil {
				return err
			}
		} else if reasoning := choice.Delta.Reasoning; reasoning != "" {
			if err := sender(nil, &reasoning); err != nil {
				return err
			}
		}
		if choice.Delta.Content != "" {
			if err := sender(&choice.Delta.Content, nil); err != nil {
				return err
			}
		}
		if choice.FinishReason != "" || event.FinishReason != "" {
			sawTerminal = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("ragcon: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	return sender(&endOfStream, nil)
}

type ragconModelInfo struct {
	ID string `json:"id"`
}

type ragconModelsEnvelope struct {
	Data []ragconModelInfo `json:"data"`
}

func (r *RAGconModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := r.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, r.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := r.httpClient.Do(req)
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

	// LiteLLM proxies (which back RAGcon) emit the OpenAI envelope
	// {"data":[{"id":"..."}]} rather than a bare list, so try the
	// envelope first and fall back to the legacy bare-list shape.
	var envelope ragconModelsEnvelope
	if err = json.Unmarshal(body, &envelope); err == nil && envelope.Data != nil {
		models := make([]string, 0, len(envelope.Data))
		for _, m := range envelope.Data {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		return models, nil
	}

	var bare []ragconModelInfo
	if err = json.Unmarshal(body, &bare); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]string, 0, len(bare))
	for _, m := range bare {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

func (r *RAGconModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := r.ListModels(apiConfig)
	return err
}

func (r *RAGconModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}

func (r *RAGconModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", r.Name())
}
