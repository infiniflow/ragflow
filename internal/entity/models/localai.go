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

// localAIStreamIdleTimeout bounds how long ChatStreamlyWithSender will
// wait between SSE chunks before assuming the upstream has stalled and
// aborting the request. A local LLM normally emits at least one token
// every few seconds; 60s is generous enough to never break a working
// stream but tight enough to bound a worst-case mid-body hang.
//
// var (not const) so tests can lower it without waiting a real minute.
var localAIStreamIdleTimeout = 60 * time.Second

// LocalAIModel implements ModelDriver for LocalAI, a self-hosted
// OpenAI-compatible inference server (https://localai.io).
//
// Unlike cloud providers, LocalAI runs on a tenant-supplied base URL
// (for example http://127.0.0.1:8080/v1). The driver therefore reads
// the base URL from the per-instance map at call time and does not
// assume a "default" entry. The API key is optional: LocalAI accepts
// an empty key by default, and the driver only sets the Authorization
// header when a non-empty key was supplied.
type LocalAIModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewLocalAIModel creates a new LocalAI model instance.
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
func NewLocalAIModel(baseURL map[string]string, urlSuffix URLSuffix) *LocalAIModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &LocalAIModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (l *LocalAIModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewLocalAIModel(baseURL, l.URLSuffix)
}

func (l *LocalAIModel) Name() string {
	return "localai"
}

// resolveBaseURL returns the tenant-supplied base URL for the given
// region, falling back to the "default" entry, and fails with a clear
// message when nothing is configured. LocalAI is self-hosted so the
// driver cannot fall back to a public endpoint.
func (l *LocalAIModel) resolveBaseURL(region string) (string, error) {
	if base, ok := l.BaseURL[region]; ok && base != "" {
		return strings.TrimSuffix(base, "/"), nil
	}
	if base, ok := l.BaseURL["default"]; ok && base != "" {
		return strings.TrimSuffix(base, "/"), nil
	}
	return "", fmt.Errorf("localai: missing base URL, configure the local access address (e.g., http://127.0.0.1:8080/v1)")
}

// setAuth sets the Authorization header only when a non-empty API key
// is supplied. LocalAI accepts an empty key by default, so sending
// "Bearer " (with an empty value) would be wrong in both directions:
// some local proxies reject it, and it leaks the fact that the
// driver was misconfigured.
func setLocalAIAuth(req *http.Request, apiConfig *APIConfig) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))
}

// localAIReasoningFields lists the JSON field names that different
// upstream models put their chain-of-thought into. LocalAI is a proxy
// that can route to any of these, so the driver tries each in turn:
//
//   - reasoning_content: OpenAI o-series, kimi-k2.6, DeepSeek-R1,
//     magistral when proxied through an OpenAI-shim
//   - reasoning:         Upstage solar-pro3 (and its proxies)
//   - thinking:          Qwen3 (Ollama-style) and some local llama-r1
//     variants exposed through LocalAI's OpenAI shim
//
// The first non-empty match wins. Order matters: reasoning_content is
// the OpenAI-conformant name and the most widely used, so it's tried
// first.
var localAIReasoningFields = []string{"reasoning_content", "reasoning", "thinking"}

// extractLocalAIReasoning pulls the chain-of-thought out of a message
// or delta object regardless of which field name the upstream model
// chose. Returns "" when no reasoning field is present or non-string.
func extractLocalAIReasoning(m map[string]interface{}) string {
	for _, k := range localAIReasoningFields {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// addLocalAIReasoningRequestParams propagates the caller's request-side
// reasoning intent into the body. Different upstream models behind
// LocalAI accept different parameters:
//
//   - reasoning_effort: OpenAI-compatible reasoning APIs (kimi, magistral,
//     solar-pro2/pro3, gpt-o-series, R1 proxies)
//   - enable_thinking:  Qwen3 explicit thinking toggle
//
// Both are emitted when the caller opts in, so the request works
// against whichever family the LocalAI instance routes to. A non-
// supporting upstream simply ignores the extra field.
func addLocalAIReasoningRequestParams(reqBody map[string]interface{}, cfg *ChatConfig) {
	if cfg == nil {
		return
	}
	if cfg.Effort != nil && *cfg.Effort != "" {
		reqBody["reasoning_effort"] = *cfg.Effort
	}
	if cfg.Thinking != nil {
		reqBody["enable_thinking"] = *cfg.Thinking
	}
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (l *LocalAIModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := l.resolveBaseURL(region)
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
		// LocalAI is a proxy; emit both reasoning_effort and
		// enable_thinking so the request works regardless of which
		// model family the LocalAI instance routes to. See
		// addLocalAIReasoningRequestParams.
		addLocalAIReasoningRequestParams(reqBody, chatModelConfig)
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
	setLocalAIAuth(req, apiConfig)

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

	// Pull the chain-of-thought from whichever field the upstream model
	// used. See localAIReasoningFields for the priority order.
	reasonContent := extractLocalAIReasoning(messageMap)

	return &ChatResponse{
		Answer:        &content,
		ReasonContent: &reasonContent,
	}, nil
}

// ChatStreamlyWithSender sends messages and streams the response via the
// sender function. The LocalAI SSE stream uses the same shape as OpenAI:
// "data:" lines carrying JSON events, with a final "[DONE]" line.
func (l *LocalAIModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := l.resolveBaseURL(region)
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
		// LocalAI is a proxy; emit both reasoning_effort and
		// enable_thinking so the streaming request works regardless of
		// which model family the LocalAI instance routes to.
		addLocalAIReasoningRequestParams(reqBody, chatModelConfig)
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// SSE streams are long-lived, so we cannot attach a hard deadline:
	// a legitimate response may take many minutes to finish on a busy
	// local model. Instead, wrap the request with WithCancel and run
	// an idle watchdog below that calls cancel() if no new data has
	// arrived for streamIdleTimeout. That bounds the worst-case stall
	// to a known finite window without breaking working long streams.
	//
	// Threading a real caller-supplied ctx through the ModelDriver
	// interface remains a wider follow-up; this is the contained fix.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	setLocalAIAuth(req, apiConfig)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Idle watchdog: every successful Scan resets lastActive. If
	// streamIdleTimeout passes without a reset, the watchdog calls
	// cancel(), which closes the underlying connection. The blocking
	// scanner.Scan() then returns false with the context error in
	// scanner.Err(), and we surface it to the caller instead of
	// hanging the goroutine forever.
	lastActive := time.Now()
	var lastActiveMu sync.Mutex
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(localAIStreamIdleTimeout / 4)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case now := <-ticker.C:
				lastActiveMu.Lock()
				idle := now.Sub(lastActive)
				lastActiveMu.Unlock()
				if idle >= localAIStreamIdleTimeout {
					cancel()
					return
				}
			}
		}
	}()

	// SSE parsing: bump the scanner buffer from the 64KB default to 1MB
	// so we never silently truncate a long data: line.
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

		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			continue
		}

		// Reasoning chunk first, content second. When an SSE event
		// carries both, callers that pipe them to a UI render the
		// chain-of-thought before the answer for that token, matching
		// the wire ordering Upstage solar-pro3 and kimi-k2.6 emit.
		// extractLocalAIReasoning tries reasoning_content, reasoning,
		// and thinking in that order so this works against whichever
		// model family LocalAI routes to.
		if reasoning := extractLocalAIReasoning(delta); reasoning != "" {
			if err := sender(nil, &reasoning); err != nil {
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
		// If the watchdog fired, the context is done; surface that as
		// a clearer "idle" error instead of leaking the raw
		// "context canceled" string.
		if ctx.Err() != nil {
			return fmt.Errorf("localai: stream idle for more than %s, aborted", localAIStreamIdleTimeout)
		}
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("localai: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

type localAIEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
	Object    string    `json:"object"`
	Index     int       `json:"index"`
}

type localAIEmbeddingResponse struct {
	Data   []localAIEmbeddingData `json:"data"`
	Model  string                 `json:"model"`
	Object string                 `json:"object"`
}

// Embed turns a list of texts into embedding vectors using the LocalAI
// /v1/embeddings endpoint. The output has one vector per input, in the
// same order the inputs were given.
func (l *LocalAIModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	if len(texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := l.resolveBaseURL(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, l.URLSuffix.Embedding)

	reqBody := map[string]interface{}{
		"model": *modelName,
		"input": texts,
	}
	if embeddingConfig != nil && embeddingConfig.Dimension > 0 {
		reqBody["dimensions"] = embeddingConfig.Dimension
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
	setLocalAIAuth(req, apiConfig)

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
		return nil, fmt.Errorf("LocalAI embeddings API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed localAIEmbeddingResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Reorder by the reported index so the output always lines up with
	// the input texts, even if the upstream API ever returns items out
	// of order. A nil slot at the end indicates the upstream did not
	// return an embedding for that input.
	embeddings := make([]EmbeddingData, len(texts))
	filled := make([]bool, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, fmt.Errorf("localai: response index %d out of range for %d inputs", item.Index, len(texts))
		}
		if filled[item.Index] {
			// A malformed response that repeats the same index would
			// silently overwrite the earlier vector. Fail loudly so
			// the caller never uses ambiguous output.
			return nil, fmt.Errorf("localai: duplicate embedding index %d in response", item.Index)
		}
		embeddings[item.Index] = EmbeddingData{
			Embedding: item.Embedding,
			Index:     item.Index,
		}
		filled[item.Index] = true
	}
	for i, ok := range filled {
		if !ok {
			return nil, fmt.Errorf("localai: missing embedding for input index %d", i)
		}
	}

	return embeddings, nil
}

type localAIRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type localAIRerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Rerank calculates similarity scores between a query and a list of documents
// using LocalAI's /v1/rerank endpoint. The response shape is Cohere-style:
// {results: [{index, relevance_score}]}. The output is copied into the shared
// RerankResponse{Data: []RerankResult{Index, RelevanceScore}} shape that the
// rest of the codebase already consumes.
func (l *LocalAIModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := l.resolveBaseURL(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, l.URLSuffix.Rerank)

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 {
		topN = rerankConfig.TopN
	}

	reqBody := localAIRerankRequest{
		Model:     *modelName,
		Query:     query,
		Documents: documents,
		TopN:      topN,
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
	setLocalAIAuth(req, apiConfig)

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
		return nil, fmt.Errorf("LocalAI rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed localAIRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	rerankResponse := &RerankResponse{}
	for _, r := range parsed.Results {
		if r.Index < 0 || r.Index >= len(documents) {
			return nil, fmt.Errorf("localai: rerank result index %d out of range for %d documents", r.Index, len(documents))
		}
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		})
	}

	return rerankResponse, nil
}

// ListModels returns the list of model ids the running LocalAI instance has
// loaded. There is no fixed model list at the SaaS level because LocalAI is
// self-hosted; the answer depends on what the tenant has configured.
func (l *LocalAIModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	region := "default"
	if apiConfig != nil && apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := l.resolveBaseURL(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, l.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	setLocalAIAuth(req, apiConfig)

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

	data, ok := result["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid models list format")
	}

	models := make([]string, 0)
	for _, model := range data {
		modelMap, ok := model.(map[string]interface{})
		if !ok {
			continue
		}
		modelName, ok := modelMap["id"].(string)
		if !ok {
			continue
		}
		models = append(models, modelName)
	}

	return models, nil
}

// Balance is not exposed by LocalAI (it is self-hosted and free), so this
// returns "no such method".
func (l *LocalAIModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("no such method")
}

// CheckConnection runs a lightweight ListModels call to verify the LocalAI
// base URL is reachable.
func (l *LocalAIModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := l.ListModels(apiConfig)
	if err != nil {
		return err
	}
	return nil
}

// TranscribeAudio (ASR): LocalAI can route audio to a Whisper backend
// when one is loaded, but the wire shape and driver-side plumbing for
// streaming audio uploads is separate from this PR's scope. Stub here
// to satisfy the ModelDriver interface; follow-up PR welcome.
func (l *LocalAIModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

func (l *LocalAIModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", l.Name())
}

// AudioSpeech (TTS): same story as TranscribeAudio above.
func (l *LocalAIModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}

func (l *LocalAIModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", l.Name())
}

// OCRFile: LocalAI has no OCR pipeline in its OpenAI-compatible surface;
// document parsing belongs to a different interface entirely.
func (l *LocalAIModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", l.Name())
}
