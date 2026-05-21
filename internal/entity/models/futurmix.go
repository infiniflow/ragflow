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

// FuturMixModel implements ModelDriver for FuturMix
// (https://futurmix.ai/docs).
//
// FuturMix advertises itself as an "OpenAI-compatible API" aggregator
// (Claude, GPT, Gemini, DeepSeek, and others, ~22 models per their
// /models page) reachable at https://futurmix.ai. The public docs
// confirm three /v1 endpoints exist: /v1/chat/completions
// (OpenAI-compatible), /v1/messages (Anthropic-format), and
// /v1/responses (OpenAI Responses API). This driver implements only
// the OpenAI-compatible chat surface — the same path the FuturMix
// admin UI uses as its canonical example endpoint URL. The
// Anthropic-format and Responses-format surfaces require different
// request/response shapes than the ModelDriver interface currently
// models and are deferred to a follow-up.
//
// Per the maintainer's guidance on
// https://github.com/infiniflow/ragflow/pull/14809#pullrequestreview-4277917390
// ("there is no need to implement the interface that is not
// officially given"), endpoints FuturMix does not explicitly document
// (embeddings, rerank, audio, OCR, models list, balance) all return
// the standard `"<name>, no such method"` sentinel rather than guess.
type FuturMixModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewFuturMixModel creates a new FuturMix model instance.
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
func NewFuturMixModel(baseURL map[string]string, urlSuffix URLSuffix) *FuturMixModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &FuturMixModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (m *FuturMixModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewFuturMixModel(baseURL, m.URLSuffix)
}

func (m *FuturMixModel) Name() string {
	return "futurmix"
}

// baseURLForRegion returns the base URL for the given region, trimmed
// of any trailing slash so callers can append a suffix without
// producing "//" in the path.
func (m *FuturMixModel) baseURLForRegion(region string) (string, error) {
	base, ok := m.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("futurmix: no base URL configured for region %q", region)
	}
	return strings.TrimRight(base, "/"), nil
}

func (m *FuturMixModel) endpointURL(region, suffix string) (string, error) {
	baseURL, err := m.baseURLForRegion(region)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(suffix, "/")), nil
}

func futurmixRegion(apiConfig *APIConfig) string {
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		return *apiConfig.Region
	}
	return "default"
}

func futurmixValidateAPIKey(apiConfig *APIConfig) (string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return "", fmt.Errorf("api key is required")
	}
	return *apiConfig.ApiKey, nil
}

func newFuturMixJSONRequest(ctx context.Context, method, endpoint string, payload interface{}, apiKey string) (*http.Request, error) {
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
	if apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}
	return req, nil
}

type futurmixAPIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type futurmixChatRequest struct {
	Model       string               `json:"model"`
	Messages    []futurmixAPIMessage `json:"messages"`
	Stream      bool                 `json:"stream"`
	MaxTokens   *int                 `json:"max_tokens,omitempty"`
	Temperature *float64             `json:"temperature,omitempty"`
	TopP        *float64             `json:"top_p,omitempty"`
	Stop        *[]string            `json:"stop,omitempty"`
}

func buildFuturMixChatRequest(modelName string, messages []Message, stream bool, chatModelConfig *ChatConfig) futurmixChatRequest {
	apiMessages := make([]futurmixAPIMessage, len(messages))
	for i, msg := range messages {
		apiMessages[i] = futurmixAPIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	reqBody := futurmixChatRequest{
		Model:    modelName,
		Messages: apiMessages,
		Stream:   stream,
	}
	if chatModelConfig != nil {
		reqBody.MaxTokens = chatModelConfig.MaxTokens
		reqBody.Temperature = chatModelConfig.Temperature
		reqBody.TopP = chatModelConfig.TopP
		reqBody.Stop = chatModelConfig.Stop
	}
	return reqBody
}

type futurmixChatChoice struct {
	Message      futurmixChatMessage `json:"message"`
	Delta        futurmixChatDelta   `json:"delta"`
	FinishReason string              `json:"finish_reason"`
}

type futurmixChatMessage struct {
	Content          *string `json:"content"`
	ReasoningContent string  `json:"reasoning_content"`
}

type futurmixChatDelta struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

type futurmixChatResponse struct {
	Choices []futurmixChatChoice `json:"choices"`
}

// ChatWithMessages sends a non-streaming chat completion against
// FuturMix's /v1/chat/completions endpoint. Wire shape follows the
// OpenAI Chat Completions contract since FuturMix is documented as
// "OpenAI-compatible".
func (m *FuturMixModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	apiKey, err := futurmixValidateAPIKey(apiConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	endpoint, err := m.endpointURL(futurmixRegion(apiConfig), m.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	// Force stream=false here; ChatWithMessages reads a single JSON
	// response body, so a streaming SSE response would be parsed as
	// truncated JSON and produce a confusing error.
	reqBody := buildFuturMixChatRequest(modelName, messages, false, chatModelConfig)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := newFuturMixJSONRequest(ctx, "POST", endpoint, reqBody, apiKey)
	if err != nil {
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("futurmix chat API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed futurmixChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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

// ChatStreamlyWithSender sends a streaming chat completion. The
// FuturMix SSE stream is assumed to use the standard OpenAI shape:
// "data:" lines carrying JSON events with delta.content (and
// delta.reasoning_content for reasoning-capable models routed
// through FuturMix's aggregator), terminated by a "[DONE]" line.
// Without live testing access this is taken on faith from the
// "OpenAI-compatible API" marketing language; if a future test
// reveals divergence (e.g. routed Claude responses surfacing in
// /v1/messages-style chunks) the SSE event parser is where to
// intervene.
func (m *FuturMixModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	apiKey, err := futurmixValidateAPIKey(apiConfig)
	if err != nil {
		return err
	}

	endpoint, err := m.endpointURL(futurmixRegion(apiConfig), m.URLSuffix.Chat)
	if err != nil {
		return err
	}

	if chatModelConfig != nil && chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
		// Caller explicitly asked for stream=false. The body of this
		// method only knows how to read SSE, so a non-SSE JSON
		// response would be parsed as if it were a stream and produce
		// no chunks. Fail clearly.
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}

	reqBody := buildFuturMixChatRequest(modelName, messages, true, chatModelConfig)

	// SSE streams are long-lived; rely on the transport's
	// ResponseHeaderTimeout to cap the connection-establishment phase
	// instead of attaching a hard deadline here.
	req, err := newFuturMixJSONRequest(context.Background(), "POST", endpoint, reqBody, apiKey)
	if err != nil {
		return err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("futurmix chat stream API error: %s, body: %s", resp.Status, string(body))
	}

	// Bump the scanner buffer from the 64KB default to 1MB so we
	// never silently truncate a long data: line.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	sawTerminal := false
	// SSE allows a single event to span multiple `data:` lines that
	// the consumer must join with newlines (separator), then parse
	// the result as one payload — see the HTML Living Standard
	// "Server-sent events" section. A blank line terminates the
	// event. The previous implementation parsed each `data:` line as
	// a standalone JSON document, which broke streaming whenever the
	// upstream emitted a wrapped event (multi-line JSON or a deltas
	// payload too wide for the upstream's single-line buffer).
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

		var event futurmixChatResponse
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			// A malformed frame can mean a truncated SSE event or an
			// upstream incident; the caller is better served by a
			// hard failure than by silent partial output.
			return false, fmt.Errorf("futurmix: invalid SSE event: %w", err)
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
		line := scanner.Text()
		if line == "" {
			// Blank line == event terminator. Flush accumulated `data:`
			// lines as a single JSON payload.
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
			// Trim only the single optional space after the colon — any
			// further leading whitespace is part of the payload (the SSE
			// spec strips at most one space after the field name).
			value := line[5:]
			if strings.HasPrefix(value, " ") {
				value = value[1:]
			}
			dataLines = append(dataLines, value)
		}
		// All other field lines (event:, id:, retry:, comments) are
		// intentionally ignored — only `data:` carries the payload
		// the OpenAI-compatible /v1/chat/completions stream uses.
	}
	// Streams that end without a trailing blank line still leave a
	// pending event in the buffer; flush it so we don't drop the
	// final delta on partially-conforming upstreams.
	if !sawTerminal {
		if _, err := dispatchEvent(); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("futurmix: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}
	return nil
}

// Embed is not exposed by the FuturMix API per the public docs.
func (m *FuturMixModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// Rerank is not exposed by the FuturMix API per the public docs.
func (m *FuturMixModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ListModels is not documented as a public endpoint by FuturMix.
// The shipped catalog in conf/models/futurmix.json is the source of
// truth for which models RAGFlow knows about; this method does not
// invent a fake live listing.
func (m *FuturMixModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// CheckConnection is not exposed by the FuturMix API. With no
// documented /models or /health endpoint, the only way to verify
// credentials would be to burn a real chat completion against
// tenant quota — return the documented sentinel rather than pretend.
func (m *FuturMixModel) CheckConnection(apiConfig *APIConfig) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// Balance is not exposed by the FuturMix public API.
func (m *FuturMixModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// TranscribeAudio is not exposed by the FuturMix API per the docs.
func (m *FuturMixModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *FuturMixModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// AudioSpeech is not exposed by the FuturMix API per the docs.
func (m *FuturMixModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *FuturMixModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// OCRFile is not exposed by the FuturMix API per the docs.
func (m *FuturMixModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ParseFile is not exposed by the FuturMix API per the docs.
func (m *FuturMixModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ListTasks is not exposed by the FuturMix API per the docs.
func (m *FuturMixModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ShowTask is not exposed by the FuturMix API per the docs.
func (m *FuturMixModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
