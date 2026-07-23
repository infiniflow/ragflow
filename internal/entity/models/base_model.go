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
	"sort"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
)

type BaseModel struct {
	BaseURL          map[string]string
	URLSuffix        URLSuffix
	httpClient       *http.Client
	AllowEmptyAPIKey bool
}

// chatResponseParts is the provider-normalized result of a non-streaming chat
// completion. Provider response structs remain provider-specific; only the
// common ChatResponse and model-usage handling is shared.
type chatResponseParts struct {
	RequestID     string
	Content       *string
	ReasonContent *string
	ToolCalls     []map[string]interface{}
	Usage         *TokenUsage
}

// chatResponseExtractor maps one provider-specific response type into the
// common result used by RAGFlow's chat model abstraction.
type chatResponseExtractor[T any] func(*T, *ChatConfig) (chatResponseParts, error)

// parseChatCompletionResponse decodes a provider-specific non-streaming chat
// response, then applies the shared ChatResponse and usage-accounting flow.
func parseChatCompletionResponse[T any](body []byte, chatConfig *ChatConfig, modelUsage *common.ModelUsage, extract chatResponseExtractor[T]) (*ChatResponse, error) {
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	parts, err := extract(&result, chatConfig)
	if err != nil {
		return nil, err
	}

	if err := collectChatModelUsage(modelUsage, parts.RequestID, parts.Usage); err != nil {
		return nil, fmt.Errorf("failed to collect model usage: %w", err)
	}

	return &ChatResponse{
		Answer:        parts.Content,
		ReasonContent: parts.ReasonContent,
		ToolCalls:     parts.ToolCalls,
		Usage:         parts.Usage,
	}, nil
}

// collectChatModelUsage records one completed chat response when the caller
// supplied a usage sink.
func collectChatModelUsage(modelUsage *common.ModelUsage, requestID string, usage *TokenUsage) error {
	if modelUsage == nil {
		return nil
	}
	modelUsage.RequestID = requestID
	return collectModelUsage(modelUsage, usage)
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
	modelUsage.ResponseTimeMS = time.Since(modelUsage.StartAt).Milliseconds()
	return clickhouse.GetDriver().CollectModelUsage(modelUsage)
}

// decodeOpenAICompatibleStreamUsage extracts aggregate token usage from one
// OpenAI-compatible streaming event. A missing usage field is not an error.
func decodeOpenAICompatibleStreamUsage(event map[string]any) (*TokenUsage, bool, error) {
	rawUsage, ok := event["usage"].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	usage := &TokenUsage{}
	if err := mapstructure.Decode(rawUsage, usage); err != nil {
		return nil, false, err
	}
	return usage, true, nil
}

// applyStreamUsage exposes streamed token usage to the caller and records it
// for model-usage analytics when a usage event is received.
func applyStreamUsage(chatConfig *ChatConfig, modelUsage *common.ModelUsage, usage *TokenUsage) error {
	if usage == nil {
		return nil
	}
	if chatConfig != nil {
		chatConfig.UsageResult = usage
	}
	return collectModelUsage(modelUsage, usage)
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
		if model.OwnedBy != "" {
			modelName = modelName + "@" + model.OwnedBy
		}
		modelResponse.Name = modelName
		if modelEntity != nil {
			modelResponse.MaxDimension = modelEntity.MaxDimension
			modelResponse.Dimensions = modelEntity.Dimensions
			modelResponse.MaxTokens = modelEntity.MaxTokens
			modelResponse.ModelTypes = modelEntity.ModelTypes
			modelResponse.Thinking = modelEntity.Thinking
			modelResponse.Dimensions = modelEntity.Dimensions
		}

		models = append(models, modelResponse)
	}
	return models
}

// NewDriverHTTPClient returns an *http.Client with the standard connection-pool
func NewDriverHTTPClient() *http.Client {
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
	t.ResponseHeaderTimeout = 2 * 60 * time.Second
	t.TLSHandshakeTimeout = 30 * time.Second
	return &http.Client{Transport: t}
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

// buildChatMessages converts internal messages to chat API payload items.
func buildChatMessages(messages []Message) []map[string]any {
	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMsg := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if msg.ToolCallID != "" {
			apiMsg["tool_call_id"] = msg.ToolCallID
		}
		if len(msg.ToolCalls) > 0 {
			apiMsg["tool_calls"] = msg.ToolCalls
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
