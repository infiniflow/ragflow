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

// N1NModel implements ModelDriver for n1n.ai
// (https://docs.n1n.ai).
//
// n1n.ai is an aggregator that exposes an OpenAI-compatible REST API
// at https://api.n1n.ai/v1. The chat, embeddings, rerank, and models
// endpoints are documented; audio/image/video endpoints are listed in
// the docs but out of scope for this driver, which sticks to the
// ModelDriver methods backed by documented JSON surfaces.
type N1NModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewN1NModel creates a new n1n.ai model instance.
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
func NewN1NModel(baseURL map[string]string, urlSuffix URLSuffix) *N1NModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &N1NModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (m *N1NModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewN1NModel(baseURL, m.URLSuffix)
}

func (m *N1NModel) Name() string {
	return "n1n"
}

// baseURLForRegion returns the base URL for the given region, trimmed
// of any trailing slash so callers can append a suffix without
// producing "//" in the path.
func (m *N1NModel) baseURLForRegion(region string) (string, error) {
	base, ok := m.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("n1n: no base URL configured for region %q", region)
	}
	return strings.TrimRight(base, "/"), nil
}

func (m *N1NModel) endpointURL(region, suffix string) (string, error) {
	baseURL, err := m.baseURLForRegion(region)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", baseURL, strings.TrimLeft(suffix, "/")), nil
}

func n1nRegion(apiConfig *APIConfig) string {
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		return *apiConfig.Region
	}
	return "default"
}

func n1nValidateAPIKey(apiConfig *APIConfig) (string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return "", fmt.Errorf("api key is required")
	}
	return *apiConfig.ApiKey, nil
}

func newN1NJSONRequest(ctx context.Context, method, endpoint string, payload interface{}, apiKey string) (*http.Request, error) {
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

type n1nAPIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type n1nThinking struct {
	Type string `json:"type"`
}

type n1nChatRequest struct {
	Model       string          `json:"model"`
	Messages    []n1nAPIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	TopP        *float64        `json:"top_p,omitempty"`
	Stop        *[]string       `json:"stop,omitempty"`
	Thinking    *n1nThinking    `json:"thinking,omitempty"`
}

func buildN1NChatRequest(modelName string, messages []Message, stream bool, chatModelConfig *ChatConfig) n1nChatRequest {
	apiMessages := make([]n1nAPIMessage, len(messages))
	for i, msg := range messages {
		apiMessages[i] = n1nAPIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	reqBody := n1nChatRequest{
		Model:    modelName,
		Messages: apiMessages,
		Stream:   stream,
	}
	if chatModelConfig != nil {
		reqBody.MaxTokens = chatModelConfig.MaxTokens
		reqBody.Temperature = chatModelConfig.Temperature
		reqBody.TopP = chatModelConfig.TopP
		reqBody.Stop = chatModelConfig.Stop
		// Map ChatConfig.Thinking *bool -> n1n.ai's documented
		// `thinking: {type: "enabled"|"disabled"}` body field
		// (per maintainer review on PR #15010, with example curl
		// against deepseek-v3-1-250821). Models that don't support
		// the field ignore it silently; the reasoning-capable
		// variants (e.g. deepseek-v3-1-think-250821) surface the
		// chain-of-thought via message.reasoning_content on the
		// non-stream path and delta.reasoning_content on the
		// streaming path, both of which this driver already reads.
		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody.Thinking = &n1nThinking{Type: "enabled"}
			} else {
				reqBody.Thinking = &n1nThinking{Type: "disabled"}
			}
		}
	}
	return reqBody
}

type n1nChatChoice struct {
	Message      n1nChatMessage `json:"message"`
	Delta        n1nChatDelta   `json:"delta"`
	FinishReason string         `json:"finish_reason"`
}

type n1nChatMessage struct {
	Content          *string `json:"content"`
	ReasoningContent string  `json:"reasoning_content"`
}

type n1nChatDelta struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

type n1nChatResponse struct {
	Choices []n1nChatChoice `json:"choices"`
}

// ChatWithMessages sends a single, non-streaming chat completion
// against n1n.ai's /v1/chat/completions endpoint.
func (m *N1NModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	apiKey, err := n1nValidateAPIKey(apiConfig)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	endpoint, err := m.endpointURL(n1nRegion(apiConfig), m.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	// Force stream=false here; ChatWithMessages reads a single JSON
	// response body, so a streaming SSE response would be parsed as
	// truncated JSON and produce a confusing error.
	reqBody := buildN1NChatRequest(modelName, messages, false, chatModelConfig)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := newN1NJSONRequest(ctx, "POST", endpoint, reqBody, apiKey)
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
		return nil, fmt.Errorf("n1n chat API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed n1nChatResponse
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
	chatResp := &ChatResponse{
		Answer: &content,
	}
	// Preserve a nil pointer when the upstream omitted reasoning, so
	// downstream callers can distinguish "no reasoning emitted" from
	// "reasoning present but empty". Matches the streaming path,
	// which already suppresses empty reasoning chunks.
	if parsed.Choices[0].Message.ReasoningContent != "" {
		reasonContent := parsed.Choices[0].Message.ReasoningContent
		chatResp.ReasonContent = &reasonContent
	}
	return chatResp, nil
}

// ChatStreamlyWithSender sends a streaming chat completion. The n1n.ai
// SSE stream uses the standard OpenAI shape: "data:" lines carrying
// JSON events with delta.content (and delta.reasoning_content for
// reasoning-capable models), terminated by a "[DONE]" line.
func (m *N1NModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	apiKey, err := n1nValidateAPIKey(apiConfig)
	if err != nil {
		return err
	}

	endpoint, err := m.endpointURL(n1nRegion(apiConfig), m.URLSuffix.Chat)
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

	reqBody := buildN1NChatRequest(modelName, messages, true, chatModelConfig)

	// SSE streams are long-lived; rely on the transport's
	// ResponseHeaderTimeout to cap the connection-establishment phase
	// instead of attaching a hard deadline here.
	req, err := newN1NJSONRequest(context.Background(), "POST", endpoint, reqBody, apiKey)
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
		return fmt.Errorf("n1n chat stream API error: %s, body: %s", resp.Status, string(body))
	}

	// Bump the scanner buffer from the 64KB default to 1MB so we
	// never silently truncate a long data: line. n1n.ai tags some
	// chunks with a long obfuscation suffix that can push individual
	// SSE lines well past 64KB on large contexts.
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

		var event n1nChatResponse
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			// A malformed frame can mean a truncated SSE event or an
			// upstream incident; the caller is better served by a
			// hard failure than by silent partial output.
			return fmt.Errorf("n1n: invalid SSE event: %w", err)
		}
		if len(event.Choices) == 0 {
			continue
		}
		choice := event.Choices[0]
		if choice.Delta.ReasoningContent != "" {
			r := choice.Delta.ReasoningContent
			if err := sender(nil, &r); err != nil {
				return err
			}
		}
		if choice.Delta.Content != "" {
			c := choice.Delta.Content
			if err := sender(&c, nil); err != nil {
				return err
			}
		}
		if choice.FinishReason != "" {
			sawTerminal = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("n1n: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}
	return nil
}

type n1nEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
}

type n1nEmbeddingResponse struct {
	Data  []n1nEmbeddingData `json:"data"`
	Model string             `json:"model"`
}

type n1nEmbeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

// Embed turns a list of texts into embedding vectors using the
// n1n.ai /v1/embeddings endpoint. Output is one vector per input, in
// the same order the inputs were given.
func (m *N1NModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}
	apiKey, err := n1nValidateAPIKey(apiConfig)
	if err != nil {
		return nil, err
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	endpoint, err := m.endpointURL(n1nRegion(apiConfig), m.URLSuffix.Embedding)
	if err != nil {
		return nil, err
	}

	reqBody := n1nEmbeddingRequest{
		Model: *modelName,
		Input: texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody.Dimensions = embeddingConfig.Dimension
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := newN1NJSONRequest(ctx, "POST", endpoint, reqBody, apiKey)
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
		return nil, fmt.Errorf("n1n embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed n1nEmbeddingResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Reorder the returned vectors by their reported index so the
	// output always lines up with the input texts, even if the
	// upstream returns items out of order. Reject duplicates and
	// out-of-range indices so a malformed response fails loudly
	// rather than silently overwriting earlier slots.
	embeddings := make([]EmbeddingData, len(texts))
	filled := make([]bool, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("n1n: embedding response index %d out of range for %d inputs", item.Index, len(texts))
		}
		if filled[item.Index] {
			return nil, fmt.Errorf("n1n: duplicate embedding index %d in response", item.Index)
		}
		embeddings[item.Index] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     item.Index,
		}
		filled[item.Index] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("n1n: missing embedding for input index %d", i)
		}
	}
	return embeddings, nil
}

type n1nRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

type n1nRerankResponse struct {
	Results []n1nRerankResult `json:"results"`
}

type n1nRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// Rerank scores a query against a list of documents using
// n1n.ai's /v1/rerank endpoint (Cohere-shaped response).
func (m *N1NModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	apiKey, err := n1nValidateAPIKey(apiConfig)
	if err != nil {
		return nil, err
	}
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	endpoint, err := m.endpointURL(n1nRegion(apiConfig), m.URLSuffix.Rerank)
	if err != nil {
		return nil, err
	}

	reqBody := n1nRerankRequest{
		Model:     *modelName,
		Query:     query,
		Documents: documents,
	}
	if rerankConfig != nil && rerankConfig.TopN > 0 {
		reqBody.TopN = rerankConfig.TopN
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := newN1NJSONRequest(ctx, "POST", endpoint, reqBody, apiKey)
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
		return nil, fmt.Errorf("n1n rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed n1nRerankResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	rerankResponse := &RerankResponse{}
	seen := make(map[int]bool, len(parsed.Results))
	for _, r := range parsed.Results {
		if r.Index < 0 || r.Index >= len(documents) {
			return nil, fmt.Errorf("n1n: rerank result index %d out of range for %d documents", r.Index, len(documents))
		}
		if seen[r.Index] {
			return nil, fmt.Errorf("n1n: duplicate rerank index %d in response", r.Index)
		}
		seen[r.Index] = true
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		})
	}
	return rerankResponse, nil
}

type n1nModelCatalogItem struct {
	ID string `json:"id"`
}

type n1nModelCatalogResponse struct {
	Data []n1nModelCatalogItem `json:"data"`
}

// ListModels returns the live n1n.ai model catalog from
// GET /v1/models. The shipped catalog in conf/models/n1n.json is a
// representative subset; this method surfaces the full upstream list
// (hundreds of models routed through the aggregator).
func (m *N1NModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	apiKey, err := n1nValidateAPIKey(apiConfig)
	if err != nil {
		return nil, err
	}

	endpoint, err := m.endpointURL(n1nRegion(apiConfig), m.URLSuffix.Models)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := newN1NJSONRequest(ctx, "GET", endpoint, nil, apiKey)
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
		return nil, fmt.Errorf("n1n models API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed n1nModelCatalogResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]string, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.ID != "" {
			models = append(models, item.ID)
		}
	}
	return models, nil
}

// CheckConnection verifies the API key by querying the documented
// /v1/models endpoint — the cheapest auth check on the documented
// surface, with no per-call charge against tenant quota.
func (m *N1NModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := m.ListModels(apiConfig)
	return err
}

// Balance is not exposed by the n1n.ai API. Account balance and
// quota are available only via the web console at
// https://api.n1n.ai/console; the public API surface does not
// publish them.
func (m *N1NModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// TranscribeAudio: n1n.ai exposes /v1/audio/transcriptions but the
// driver does not currently implement the multipart upload flow.
func (m *N1NModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *N1NModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// AudioSpeech: n1n.ai exposes /v1/audio/speech but the driver does
// not currently implement the binary audio response handling.
func (m *N1NModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *N1NModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", m.Name())
}

// OCRFile is not exposed by the n1n.ai API.
func (m *N1NModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ParseFile is not exposed by the n1n.ai API.
func (m *N1NModel) ParseFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

// ListTasks: n1n.ai has /v1/contents/generations/tasks for async
// image/video jobs, but that surface is not modeled by this driver.
func (m *N1NModel) ListTasks(apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}

func (m *N1NModel) ShowTask(taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", m.Name())
}
