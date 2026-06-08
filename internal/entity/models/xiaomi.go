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

// XiaomiModel implements ModelDriver for Xiaomi MiMo chat models.
//
// Xiaomi MiMo documents an OpenAI-compatible chat completions endpoint at
// https://api.xiaomimimo.com/v1/chat/completions. The documented request
// sample uses api-key authentication and max_completion_tokens, so this
// driver follows that wire shape instead of blindly reusing max_tokens.
type XiaomiModel struct {
	baseModel BaseModel
}

func NewXiaomiModel(baseURL map[string]string, urlSuffix URLSuffix) *XiaomiModel {
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if ok {
		transport = defaultTransport.Clone()
	} else {
		transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}
	}
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &XiaomiModel{
		baseModel: BaseModel{
			BaseURL:   baseURL,
			URLSuffix: urlSuffix,
			httpClient: &http.Client{
				Transport: transport,
			},
		},
	}
}

func (m *XiaomiModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewXiaomiModel(baseURL, m.baseModel.URLSuffix)
}

func (m *XiaomiModel) Name() string {
	return "xiaomi"
}

func (m *XiaomiModel) baseURLForRegion(region string) (string, error) {
	keys := []string{region}
	if region != "" {
		keys = append(keys, "", "default")
	} else {
		keys = append(keys, "default")
	}
	for _, key := range keys {
		if base := strings.TrimRight(m.baseModel.BaseURL[key], "/"); base != "" {
			return base, nil
		}
	}
	return "", fmt.Errorf("xiaomi: no base URL configured for region %q", region)
}

func (m *XiaomiModel) endpointURL(apiConfig *APIConfig) (string, error) {
	if apiConfig != nil && apiConfig.BaseURL != nil && *apiConfig.BaseURL != "" {
		return fmt.Sprintf("%s/%s", strings.TrimRight(*apiConfig.BaseURL, "/"), strings.TrimLeft(m.baseModel.URLSuffix.Chat, "/")), nil
	}

	region := ""
	if apiConfig != nil && apiConfig.Region != nil {
		region = *apiConfig.Region
	}
	baseURL, err := m.baseURLForRegion(region)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(m.baseModel.URLSuffix.Chat, "/")), nil
}

func xiaomiAPIKey(apiConfig *APIConfig) (string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || strings.TrimSpace(*apiConfig.ApiKey) == "" {
		return "", fmt.Errorf("api key is required")
	}
	return *apiConfig.ApiKey, nil
}

type xiaomiAPIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type xiaomiThinking struct {
	Type string `json:"type"`
}

type xiaomiChatRequest struct {
	Model               string             `json:"model"`
	Messages            []xiaomiAPIMessage `json:"messages"`
	Stream              bool               `json:"stream"`
	MaxCompletionTokens *int               `json:"max_completion_tokens,omitempty"`
	Temperature         *float64           `json:"temperature,omitempty"`
	TopP                *float64           `json:"top_p,omitempty"`
	Stop                *[]string          `json:"stop,omitempty"`
	Thinking            *xiaomiThinking    `json:"thinking,omitempty"`
}

func buildXiaomiChatRequest(modelName string, messages []Message, stream bool, chatModelConfig *ChatConfig) xiaomiChatRequest {
	apiMessages := make([]xiaomiAPIMessage, len(messages))
	for i, msg := range messages {
		apiMessages[i] = xiaomiAPIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	reqBody := xiaomiChatRequest{
		Model:    modelName,
		Messages: apiMessages,
		Stream:   stream,
	}
	if chatModelConfig != nil {
		reqBody.MaxCompletionTokens = chatModelConfig.MaxTokens
		reqBody.Temperature = chatModelConfig.Temperature
		reqBody.TopP = chatModelConfig.TopP
		reqBody.Stop = chatModelConfig.Stop
		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody.Thinking = &xiaomiThinking{Type: "enabled"}
			} else {
				reqBody.Thinking = &xiaomiThinking{Type: "disabled"}
			}
		}
	}
	return reqBody
}

type xiaomiChatMessage struct {
	Content          *string `json:"content"`
	ReasoningContent string  `json:"reasoning_content"`
}

type xiaomiChatDelta struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

type xiaomiChatChoice struct {
	Message      xiaomiChatMessage `json:"message"`
	Delta        xiaomiChatDelta   `json:"delta"`
	FinishReason string            `json:"finish_reason"`
}

type xiaomiChatResponse struct {
	Choices []xiaomiChatChoice `json:"choices"`
	Error   interface{}        `json:"error"`
}

func newXiaomiJSONRequest(ctx context.Context, method, endpoint string, payload interface{}, apiKey string) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", apiKey)
	return req, nil
}

func (m *XiaomiModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	apiKey, err := xiaomiAPIKey(apiConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	endpoint, err := m.endpointURL(apiConfig)
	if err != nil {
		return nil, err
	}

	reqBody := buildXiaomiChatRequest(modelName, messages, false, chatModelConfig)
	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := newXiaomiJSONRequest(ctx, http.MethodPost, endpoint, reqBody, apiKey)
	if err != nil {
		return nil, err
	}

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
		return nil, fmt.Errorf("xiaomi chat API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed xiaomiChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("xiaomi: upstream error: %v", parsed.Error)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	if parsed.Choices[0].Message.Content == nil {
		return nil, fmt.Errorf("invalid content format")
	}

	content := *parsed.Choices[0].Message.Content
	reasonContent := parsed.Choices[0].Message.ReasoningContent
	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

func (m *XiaomiModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	apiKey, err := xiaomiAPIKey(apiConfig)
	if err != nil {
		return err
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

	endpoint, err := m.endpointURL(apiConfig)
	if err != nil {
		return err
	}
	reqBody := buildXiaomiChatRequest(modelName, messages, true, chatModelConfig)
	req, err := newXiaomiJSONRequest(context.Background(), http.MethodPost, endpoint, reqBody, apiKey)
	if err != nil {
		return err
	}

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("xiaomi chat stream API error: %s, body: %s", resp.Status, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	sawTerminal := false
	var dataLines []string
	dispatchEvent := func() (bool, error) {
		if len(dataLines) == 0 {
			return false, nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if payload == "[DONE]" {
			sawTerminal = true
			return true, nil
		}

		var event xiaomiChatResponse
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return false, fmt.Errorf("xiaomi: invalid SSE event: %w", err)
		}
		if event.Error != nil {
			return false, fmt.Errorf("xiaomi: upstream stream error: %v", event.Error)
		}
		if len(event.Choices) == 0 {
			return false, nil
		}
		choice := event.Choices[0]
		if choice.Delta.ReasoningContent != "" {
			r := choice.Delta.ReasoningContent
			if err := sender(nil, &r); err != nil {
				return false, err
			}
		}
		if choice.Delta.Content != "" {
			c := choice.Delta.Content
			if err := sender(&c, nil); err != nil {
				return false, err
			}
		}
		if choice.FinishReason != "" {
			sawTerminal = true
			return true, nil
		}
		return false, nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			stop, err := dispatchEvent()
			if err != nil {
				return err
			}
			if stop {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := line[5:]
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
			dataLines = append(dataLines, value)
		}
	}
	if !sawTerminal {
		if _, err := dispatchEvent(); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("xiaomi: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}
	return nil
}

func (m *XiaomiModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *XiaomiModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
