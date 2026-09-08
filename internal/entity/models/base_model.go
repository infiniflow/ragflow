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
	"ragflow/internal/common"
	"ragflow/internal/engine/clickhouse"
	"ragflow/internal/utility"
	"sort"
	"strings"
	"time"
)

type BaseModel struct {
	BaseURL          map[string]string
	URLSuffix        URLSuffix
	httpClient       *http.Client
	AllowEmptyAPIKey bool
	// authHeader, when non-nil, supplies the (name, value) pair used for
	// authentication instead of the default "Authorization: Bearer <key>".
	// Drivers with non-standard auth (e.g. Xiaomi's api-key header, Xunfei's
	// spark_api_password bundle) set it in their constructor.
	authHeader func(*APIConfig) (string, string)
}

// chatResponseParts is the provider-normalized result of a non-streaming chat
// completion. Provider response structs remain provider-specific; only the
// common ChatResponse and model-usage handling is shared.
type chatResponseParts struct {
	RequestID     string
	Content       *string
	ReasonContent *string
	ToolCalls     []map[string]any
	Usage         *TokenUsage
}

// chatResponseExtractor parses one provider-specific response and maps it
// into the common result used by RAGFlow's chat model abstraction.
type chatResponseExtractor func([]byte, *ChatConfig) (chatResponseParts, error)

// parseChatCompletionResponse applies the shared ChatResponse and
// usage-accounting flow after the provider-specific response is parsed.
func parseChatCompletionResponse(body []byte, chatConfig *ChatConfig, modelUsage *common.ModelUsage, extract chatResponseExtractor) (*ChatResponse, error) {
	parts, err := extract(body, chatConfig)
	if err != nil {
		return nil, err
	}

	recordResponseUsage(modelUsage, parts.RequestID, parts.Usage, "chat")

	return &ChatResponse{
		Answer:        parts.Content,
		ReasonContent: parts.ReasonContent,
		ToolCalls:     parts.ToolCalls,
		Usage:         parts.Usage,
	}, nil
}

// recordResponseUsage records the request ID and token usage returned by a
// completed model response.
//
// When modelUsage is nil (caller did not pass a usage context) but
// usage is non-nil, we still surface a single CollectModelUsage call
// against a synthetic empty ModelUsage so the analytics path can
// observe the provider's reported token counts. Without this, drivers
// whose upstream service layer passes nil — common in the current
// model_chat / generator code paths — would never reach the stats
// driver and the token usage would be invisible. The synthetic record
// carries zero UserID/TenantID; production callers should pass a
// populated *common.ModelUsage to attribute usage to a tenant.
func recordResponseUsage(modelUsage *common.ModelUsage, requestID string, usage *TokenUsage, modelType string) {
	if usage == nil {
		return
	}
	if modelUsage == nil {
		modelUsage = &common.ModelUsage{}
	}
	if modelUsage.Type == "" {
		modelUsage.Type = modelType
	}
	modelUsage.RequestID = requestID
	if err := collectModelUsage(modelUsage, usage); err != nil {
		common.Error("Failed to collect model usage", err)
	}
}

// collectModelUsage records token usage and response time for one model call.
// The caller owns setting RequestID because streaming providers can receive it
// in a different event from usage.
func collectModelUsage(modelUsage *common.ModelUsage, usage *TokenUsage) error {
	if modelUsage == nil {
		return nil
	}
	if usage != nil {
		modelUsage.InputTokens = usage.PromptTokens
		modelUsage.OutputTokens = usage.CompletionTokens
		modelUsage.TotalTokens = usage.TotalTokens
	}
	// StartAt may be zero when the synthetic ModelUsage came from
	// recordResponseUsage's nil-caller path. In that case we cannot
	// compute a meaningful response time; leave it at zero instead
	// of reporting a 50-year epoch delta.
	if !modelUsage.StartAt.IsZero() {
		modelUsage.ResponseTimeMS = time.Since(modelUsage.StartAt).Milliseconds()
	}
	return clickhouse.GetDriver().CollectModelUsage(modelUsage)
}

// applyStreamUsage exposes streamed token usage to the caller and records it
// for model-usage analytics when a usage event is received. Analytics failures
// are logged but do not interrupt the stream.
//
// Like recordResponseUsage, a nil modelUsage (the common case from the
// model_chat / generator service layer) still surfaces a synthetic
// CollectModelUsage call so streaming usage is not silently dropped. The
// synthetic record carries zero UserID/TenantID; production callers should
// pass a populated *common.ModelUsage to attribute usage to a tenant.
func applyStreamUsage(chatConfig *ChatConfig, modelUsage *common.ModelUsage, usage *TokenUsage) {
	if usage == nil {
		return
	}
	if chatConfig != nil {
		chatConfig.UsageResult = usage
	}
	if modelUsage == nil {
		modelUsage = &common.ModelUsage{}
	}
	if modelUsage.Type == "" {
		modelUsage.Type = "chat"
	}
	if err := collectModelUsage(modelUsage, usage); err != nil {
		common.Error("Failed to collect model usage", err)
	}
}

func (b *BaseModel) APIConfigCheck(apiConfig *APIConfig) error {
	if b.AllowEmptyAPIKey {
		return nil
	}

	if apiConfig == nil || apiConfig.ApiKey == nil || strings.TrimSpace(*apiConfig.ApiKey) == "" {
		return fmt.Errorf("api key is required")
	}

	return nil
}

// applyAuth sets the authentication header on req. Drivers with a custom
// authHeader hook (e.g. Xiaomi's api-key header, Xunfei's spark_api_password
// bundle) use it; the default is "Authorization: Bearer <key>".
func (b *BaseModel) applyAuth(req *http.Request, apiConfig *APIConfig) {
	if b.authHeader != nil {
		name, value := b.authHeader(apiConfig)
		req.Header.Set(name, value)
		return
	}
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}
}

func (b *BaseModel) newJSONPostRequest(ctx context.Context, url string, apiConfig *APIConfig, reqBody map[string]any) (*http.Request, error) {
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	b.applyAuth(req, apiConfig)

	return req, nil
}

// doRequest sends a JSON POST request and returns the response body.
func (b *BaseModel) doRequest(ctx context.Context, url string, apiConfig *APIConfig, reqBody map[string]any, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := b.newJSONPostRequest(ctx, url, apiConfig, reqBody)
	if err != nil {
		return nil, err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("API request failed with status %d; failed to read error response: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := readModelResponseBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

// doGetRequest sends a GET request and returns the response body.
func (b *BaseModel) doGetRequest(ctx context.Context, url string, apiConfig *APIConfig, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	b.applyAuth(req, apiConfig)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("API request failed with status %d; failed to read error response: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := readModelResponseBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

// doStreamRequest sends a JSON POST request and calls handler with the response body.
func (b *BaseModel) doStreamRequest(ctx context.Context, url string, apiConfig *APIConfig, reqBody map[string]any, timeout time.Duration, handler func(io.ReadCloser) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := b.newJSONPostRequest(ctx, url, apiConfig, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := readModelErrorBody(resp.Body)
		if err != nil {
			return fmt.Errorf("API request failed with status %d; failed to read error response: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return handler(resp.Body)
}

// BearerAuth returns the Bearer token for Authorization header,
// or empty string if apiConfig or its ApiKey is nil/empty.
func BearerAuth(apiConfig *APIConfig) string {
	if apiConfig == nil || apiConfig.ApiKey == nil {
		return ""
	}
	key := strings.TrimSpace(*apiConfig.ApiKey)
	if key == "" {
		return ""
	}
	return fmt.Sprintf("Bearer %s", key)
}

func (b *BaseModel) GetBaseURL(apiConfig *APIConfig) (string, error) {
	if apiConfig != nil && apiConfig.BaseURL != nil && *apiConfig.BaseURL != "" {
		return strings.TrimSuffix(*apiConfig.BaseURL, "/"), nil
	}

	region := "default"
	hasRegion := false
	if apiConfig != nil && apiConfig.Region != nil {
		hasRegion = true
		region = *apiConfig.Region
	}

	baseURL, ok := b.BaseURL[region]
	if !ok || baseURL == "" {
		if (!hasRegion || region == "") && b.BaseURL != nil {
			if defaultBaseURL, ok := b.BaseURL["default"]; ok && defaultBaseURL != "" {
				return defaultBaseURL, nil
			}
		}
		return "", fmt.Errorf("no base URL configured for region %q", region)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return baseURL, nil
}

// ParseSSEStream reads the body of an OpenAI-compatible Server-Sent Events
// response and calls onEvent for each successfully-parsed JSON payload.
// A malformed JSON payload after "data:" returns an error wrapped as
// "invalid SSE event" so the caller cannot silently swallow truncated or
// corrupted streams.
func ParseSSEStream[T any](r io.Reader, onEvent func(event T) error) (done bool, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return true, nil
		}
		var event T
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return false, fmt.Errorf("invalid SSE event: %w", err)
		}
		if err := onEvent(event); err != nil {
			return false, err
		}
	}
	return false, scanner.Err()
}

// ParseSSEStreamTolerant is like ParseSSEStream but silently skips
// malformed JSON payloads. Use this only for drivers whose upstream is
// known to interleave invalid frames the test suite documents as safe
// to ignore.
func ParseSSEStreamTolerant[T any](r io.Reader, onEvent func(event T) error) (done bool, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			return true, nil
		}
		var event T
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if err := onEvent(event); err != nil {
			return false, err
		}
	}
	return false, scanner.Err()
}

// ParseListModel Parse model list. Empty/whitespace IDs are skipped so
// upstream typos do not surface as blank entries in the UI.
//
// Entries the catalog cannot type fall back to name-based inference
// (InferModelTypes), mirroring Python's
// OpenAIAPICompatible._format_model_list (rag/llm/model_meta.py) so remote
// entries never surface type-less.
func ParseListModel(modelList ModelList) []ListModelResponse {
	var models []ListModelResponse
	pm := GetProviderManager()
	for _, model := range modelList.Models {
		modelName := strings.TrimSpace(model.ID)
		if modelName == "" {
			continue
		}
		var modelResponse ListModelResponse
		var modelEntity *Model
		if pm != nil {
			modelEntity = pm.GetModelByNameOrAlias(modelName)
		}

		modelResponse.Name = modelName
		if modelEntity != nil {
			modelResponse.MaxDimension = modelEntity.MaxDimension
			modelResponse.MaxBatchSize = modelEntity.MaxBatchSize
			modelResponse.Dimensions = modelEntity.Dimensions
			modelResponse.ContextLength = modelEntity.ContextLength
			modelResponse.MaxOutput = modelEntity.MaxOutput
			modelResponse.ModelTypes = modelEntity.ModelTypes
			modelResponse.Thinking = modelEntity.Thinking
		}

		if model.ContextLength != nil && *model.ContextLength > 0 {
			modelResponse.ContextLength = model.ContextLength
		}

		// The provider-list merge treats remote entries as authoritative
		// (internal/handler/providers.go) and the instance save path
		// persists whatever types this list carries, so a catalog miss
		// must not leave ModelTypes empty — the UI renders type-less
		// models with an LLM-only badge. Infer types from the model name
		// (vision models like qwen-vl-plus keep their VLM tag even before
		// the catalog knows them); InferModelTypes always returns at
		// least ["chat"].
		if len(modelResponse.ModelTypes) == 0 {
			modelResponse.ModelTypes = InferModelTypes(modelName)
		}
		models = append(models, modelResponse)
	}
	return FillMissingModelTypes(models)
}

// NewDriverHTTPClient returns an *http.Client with the standard connection-pool
// settings and an SSRF guard wired into its Transport.
//
// allowPrivate selects the guard strictness:
//   - false (cloud-hosted drivers): every request is validated with
//     utility.AssertURLSafe — scheme + host must be present and every resolved
//     IP must be globally routable (private/loopback/link-local/metadata are
//     rejected). This is the default and closes the go/request-forgery sink.
//   - true (local-inference drivers): requests are validated with
//     utility.AssertURLSchemeSafe — only the scheme and a non-empty host are
//     enforced, so self-hosted backends on private networks or loopback work.
func NewDriverHTTPClient(allowPrivate bool) *http.Client {
	var t *http.Transport
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		t = dt.Clone()
	} else {
		t = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	t.MaxIdleConns = 100
	t.MaxIdleConnsPerHost = 10
	t.IdleConnTimeout = 90 * time.Second
	t.DisableCompression = false
	t.ResponseHeaderTimeout = 5 * 60 * time.Second
	t.TLSHandshakeTimeout = 30 * time.Second

	var rt http.RoundTripper = t
	if allowPrivate {
		rt = &schemeSafeTransport{base: rt}
	} else {
		rt = &strictSSRFTransport{base: rt}
	}
	return &http.Client{Transport: rt}
}

// schemeSafeTransport wraps an http.RoundTripper so every outgoing request is
// validated by the lenient SSRF guard (http/https scheme + non-empty host).
// Private and loopback hosts are permitted. Used only by local-inference
// drivers that may target a user's own network.
type schemeSafeTransport struct{ base http.RoundTripper }

func (t *schemeSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := utility.AssertURLSchemeSafe(req.URL.String()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// strictSSRFTransport wraps an http.RoundTripper so every outgoing request is
// validated by the strict SSRF guard (scheme + host + globally routable IP).
// This is the default for cloud-hosted model drivers and closes the
// go/request-forgery data flow: the user-controllable BaseURL cannot be made to
// point at private hosts, loopback, link-local, or cloud metadata endpoints.
type strictSSRFTransport struct{ base http.RoundTripper }

func (t *strictSSRFTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if _, _, err := utility.AssertURLSafe(req.URL.String()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// PostJSONRequest marshals body to JSON, creates a POST request to url
func PostJSONRequest(ctx context.Context, client *http.Client, url, auth string, body map[string]interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return client.Do(req)
}

// ReadErrorBody reads all bytes from r and returns them as a string suitable
func ReadErrorBody(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}

func buildRequestBody(cfg *ChatConfig, modelName string, messages []Message, stream bool) map[string]any {
	reqBody := map[string]any{
		"model":    modelName,
		"messages": buildChatMessages(messages),
		"stream":   stream,
	}

	if cfg != nil {
		if cfg.Temperature != nil {
			reqBody["temperature"] = *cfg.Temperature
		}

		if cfg.DoSample != nil {
			reqBody["do_sample"] = *cfg.DoSample
		}

		if cfg.TopP != nil {
			reqBody["top_p"] = *cfg.TopP
		}

		if cfg.MaxTokens != nil {
			reqBody["max_tokens"] = *cfg.MaxTokens
		}

		if cfg.Stop != nil {
			reqBody["stop"] = *cfg.Stop
		}

		if cfg.Tools != nil {
			reqBody["tools"] = cfg.Tools
			toolChoice := "auto"
			if cfg.ToolChoice != nil {
				toolChoice = *cfg.ToolChoice
			}
			reqBody["tool_choice"] = toolChoice
		}
	}

	return reqBody
}

func validateStreamConfig(cfg *ChatConfig) error {
	if cfg != nil && cfg.Stream != nil && !*cfg.Stream {
		return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
	}
	return nil
}

// buildChatMessages converts internal messages to chat API payload items.
func buildChatMessages(messages []Message) []map[string]any {
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMsg := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.Name != nil {
			apiMsg["name"] = msg.Name
		}
		if msg.ToolCallID != "" {
			apiMsg["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			apiMsg["tool_calls"] = msg.ToolCalls
		}
		if msg.FunctionCall != nil {
			apiMsg["function_call"] = msg.FunctionCall
		}
		if msg.Refusal != nil {
			apiMsg["refusal"] = msg.Refusal
		}
		if msg.Audio != nil {
			apiMsg["audio"] = msg.Audio
		}
		apiMessages[i] = apiMsg
	}
	return apiMessages
}

// applyChatToolConfig adds OpenAI-compatible tool configuration to a request.
func applyChatToolConfig(reqBody map[string]interface{}, chatConfig *ChatConfig) {
	if chatConfig == nil || chatConfig.Tools == nil {
		return
	}
	reqBody["tools"] = chatConfig.Tools
	if chatConfig.ToolChoice != nil {
		reqBody["tool_choice"] = *chatConfig.ToolChoice
	}
}

// extractToolCalls converts an OpenAI-compatible message's tool calls.
func extractToolCalls(message map[string]interface{}) []map[string]interface{} {
	rawToolCalls, ok := message["tool_calls"].([]interface{})
	if !ok {
		return nil
	}
	toolCalls := make([]map[string]interface{}, 0, len(rawToolCalls))
	for _, rawToolCall := range rawToolCalls {
		if toolCall, ok := rawToolCall.(map[string]interface{}); ok {
			toolCalls = append(toolCalls, toolCall)
		}
	}
	return toolCalls
}

// setSortedToolCallsResult stores accumulated tool calls in index order.
func setSortedToolCallsResult(chatConfig *ChatConfig, accumulatedToolCalls map[int]map[string]any) {
	if chatConfig == nil || len(accumulatedToolCalls) == 0 {
		return
	}
	indices := make([]int, 0, len(accumulatedToolCalls))
	for idx := range accumulatedToolCalls {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	toolCalls := make([]map[string]interface{}, 0, len(accumulatedToolCalls))
	for _, idx := range indices {
		toolCalls = append(toolCalls, accumulatedToolCalls[idx])
	}
	chatConfig.ToolCallsResult = &toolCalls
}

// accumulateToolCallDeltas merges streaming tool-call deltas by index.
func accumulateToolCallDeltas(delta map[string]interface{}, accumulatedToolCalls map[int]map[string]any) bool {
	toolCallDeltas, ok := delta["tool_calls"].([]interface{})
	if !ok {
		return false
	}
	for _, toolCallDelta := range toolCallDeltas {
		toolCall, ok := toolCallDelta.(map[string]interface{})
		if !ok {
			continue
		}
		idxF, ok := toolCall["index"].(float64)
		if !ok {
			continue
		}
		idx := int(idxF)
		existing, hasExisting := accumulatedToolCalls[idx]
		if !hasExisting {
			accumulatedToolCalls[idx] = cloneMap(toolCall)
			continue
		}
		appendStringField(existing, toolCall, "id")
		if typ, ok := toolCall["type"].(string); ok && typ != "" {
			existing["type"] = typ
		}
		mergeToolCallFunction(existing, toolCall)
	}
	return true
}

// appendStringField appends a non-empty string field from src into dst.
func appendStringField(dst, src map[string]interface{}, key string) {
	value, ok := src[key].(string)
	if !ok || value == "" {
		return
	}
	if existing, ok := dst[key].(string); ok {
		dst[key] = existing + value
	} else {
		dst[key] = value
	}
}

// mergeToolCallFunction merges streamed function name and arguments.
func mergeToolCallFunction(existing, delta map[string]interface{}) {
	fn, ok := delta["function"].(map[string]interface{})
	if !ok {
		return
	}
	existingFn, ok := existing["function"].(map[string]interface{})
	if !ok {
		existingFn = make(map[string]interface{})
		existing["function"] = existingFn
	}
	appendStringField(existingFn, fn, "name")
	appendStringField(existingFn, fn, "arguments")
}

// CloneMap returns a shallow copy of m.
func cloneMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
