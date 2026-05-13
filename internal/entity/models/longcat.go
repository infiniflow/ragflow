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

// LongCatModel implements ModelDriver for LongCat (Meituan).
//
// LongCat exposes an OpenAI-compatible chat completions endpoint at
// https://api.longcat.chat/openai/v1/chat/completions. The official
// docs (https://longcat.chat/platform/docs/APIDocs.html) only describe
// the chat-completions surface — no /models, /embeddings, /rerank,
// /audio, or /ocr endpoints are advertised. The wire shape matches the
// OpenAI convention: response/delta carry reasoning_content alongside
// content for thinking models.
//
// Documented request fields are limited to: model, messages, stream,
// max_tokens, temperature, top_p. Sending other OpenAI-style fields
// (stop, reasoning_effort, etc.) is not documented and is therefore
// omitted to avoid relying on undocumented upstream behavior.
type LongCatModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewLongCatModel creates a new LongCat model instance.
//
// We clone http.DefaultTransport so we keep Go's defaults for
// ProxyFromEnvironment, DialContext (with KeepAlive), HTTP/2,
// TLSHandshakeTimeout, and ExpectContinueTimeout, and only override
// the connection-pool fields we care about.
//
// The Client itself has no Timeout. http.Client.Timeout would also
// cap the time spent reading the response body, which would cut off
// long-lived SSE streams in ChatStreamlyWithSender. Non-streaming
// callers wrap each request with context.WithTimeout instead.
func NewLongCatModel(baseURL map[string]string, urlSuffix URLSuffix) *LongCatModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &LongCatModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (l *LongCatModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewLongCatModel(baseURL, l.URLSuffix)
}

func (l *LongCatModel) Name() string {
	return "longcat"
}

// baseURLForRegion returns the base URL for the given region, or an
// error if no entry exists. This makes a misconfigured region fail
// fast with a clear message, instead of silently producing a relative
// URL that the HTTP transport then rejects.
func (l *LongCatModel) baseURLForRegion(region string) (string, error) {
	base, ok := l.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("longcat: no base URL configured for region %q", region)
	}
	return base, nil
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (l *LongCatModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
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

	baseURL, err := l.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, l.URLSuffix.Chat)

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

	// Note: do NOT propagate chatModelConfig.Stream into the request body
	// here. ChatWithMessages parses a single JSON response, so stream must
	// always be off for this code path.
	//
	// Only the fields documented at
	// https://longcat.chat/platform/docs/APIDocs.html are forwarded.
	// Other ChatConfig fields (Stop, Effort, ...) are dropped on the
	// floor because the upstream behavior is undefined.
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

	resp, err := l.httpClient.Do(req)
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

	// LongCat-Flash-Thinking returns the chain-of-thought in a
	// `reasoning_content` field on the message (OpenAI o-series shape,
	// also used by kimi-k2.6 and DeepSeek-R1). Pass it through when
	// present so callers can surface reasoning to the UI. Absent or
	// non-string means no reasoning was emitted — leave it empty.
	reasonContent := ""
	if r, ok := messageMap["reasoning_content"].(string); ok {
		reasonContent = r
	}

	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

// ChatStreamlyWithSender sends messages and streams the response via the
// sender function. The LongCat SSE stream uses the same shape as the
// OpenAI o-series: "data:" lines carrying JSON events with
// delta.content for the visible answer and delta.reasoning_content for
// the chain-of-thought (LongCat-Flash-Thinking only), terminated by
// a [DONE] line.
func (l *LongCatModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
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

	baseURL, err := l.baseURLForRegion(region)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", baseURL, l.URLSuffix.Chat)

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
		// Refuse to run if the caller explicitly asked for stream=false.
		// The body of this method only knows how to read SSE, so a
		// non-SSE JSON response would be parsed as if it were a stream
		// and produce no chunks. Better to fail clearly.
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
		}

		// Only documented fields are forwarded; see ChatWithMessages.
		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}
		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}
		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// SSE streams are long-lived. Rely on the transport's
	// ResponseHeaderTimeout to cap the connection-establishment phase
	// instead of attaching a hard deadline here.
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// SSE parsing: bump the scanner buffer from the 64KB default to 1MB
	// so we never silently truncate a long data: line.
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
			// A malformed frame can mean a truncated SSE event or an
			// upstream incident; either way, the caller is better
			// served by a hard failure than by silent partial output.
			return fmt.Errorf("longcat: invalid SSE event: %w", err)
		}

		// LongCat (like other OpenAI-compatible upstreams) can emit a
		// terminal `{"error": ...}` frame instead of a normal choices
		// chunk when something goes wrong mid-stream. Surface it
		// instead of falling through to the choices-missing branch.
		if apiErr, ok := event["error"]; ok {
			return fmt.Errorf("longcat: upstream stream error: %v", apiErr)
		}

		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}

		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		// Reasoning chunks first, content second. When an SSE event
		// carries both, callers that pipe them to a UI render the
		// chain-of-thought before the answer for that token, matching
		// the wire ordering LongCat-Flash-Thinking emits.
		if r, ok := delta["reasoning_content"].(string); ok && r != "" {
			if err := sender(nil, &r); err != nil {
				return err
			}
		}

		content, ok := delta["content"].(string)
		if ok && content != "" {
			if err := sender(&content, nil); err != nil {
				return err
			}
		}

		finishReason, ok := firstChoice["finish_reason"].(string)
		if ok && finishReason != "" {
			sawTerminal = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("longcat: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

// ListModels is not exposed by the LongCat platform. The official
// docs at https://longcat.chat/platform/docs/APIDocs.html only
// document /openai/v1/chat/completions and /anthropic/v1/messages;
// no /models endpoint exists. The shipped catalog lives in
// conf/models/longcat.json; this driver method does not invent a
// fake one.
func (l *LongCatModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// CheckConnection is not exposed by the LongCat platform. With no
// documented /models or /health endpoint, there is no cheap way to
// verify the API key without burning a real chat completion against
// a tenant's quota. Return the documented sentinel rather than
// pretend.
func (l *LongCatModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", l.Name())
}

// Embed is not exposed by the LongCat API. The /v1/embeddings endpoint
// does not exist on api.longcat.chat; this returns the documented
// sentinel.
func (l *LongCatModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// Rerank is not exposed by the LongCat API.
func (l *LongCatModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// Balance is not exposed by the LongCat API.
func (l *LongCatModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

// TranscribeAudio (ASR) is not exposed by the LongCat API.
func (l *LongCatModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

func (l *LongCatModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", l.Name())
}

// AudioSpeech (TTS) is not exposed by the LongCat API.
func (l *LongCatModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

func (l *LongCatModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", l.Name())
}

// OCRFile is not exposed by the LongCat API.
func (l *LongCatModel) OCRFile(modelName *string, fileContent *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}
