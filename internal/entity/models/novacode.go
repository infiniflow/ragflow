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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"ragflow/internal/common"
)

// NovaCodeModel implements NovaCode's Responses and Chat Completions APIs.
// Credentials select the wire protocol independently of the model's reasoning effort.
type NovaCodeModel struct {
	*DummyModel
	baseModel BaseModel
}

type novaCodeCredentials struct {
	APIKey          string `json:"api_key"`
	WireAPI         string `json:"wire_api"`
	ReasoningEffort string `json:"reasoning_effort"`
}

func NewNovaCodeModel(baseURL map[string]string, suffix URLSuffix) *NovaCodeModel {
	if len(baseURL) == 0 {
		baseURL = map[string]string{"default": "https://ai.novacode.top/v1"}
	}
	if suffix.Chat == "" {
		suffix.Chat = "chat/completions"
	}
	if suffix.Responses == "" {
		suffix.Responses = "responses"
	}
	if suffix.Models == "" {
		suffix.Models = "models"
	}
	return &NovaCodeModel{DummyModel: NewDummyModel(baseURL, suffix), baseModel: BaseModel{BaseURL: baseURL, URLSuffix: suffix, httpClient: common.GetSSRFHTTPClient()}}
}

func (m *NovaCodeModel) Name() string { return "NovaCode" }
func (m *NovaCodeModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewNovaCodeModel(baseURL, m.baseModel.URLSuffix)
}

func novaCodeConfig(config *APIConfig) (*APIConfig, novaCodeCredentials, error) {
	credentials := novaCodeCredentials{WireAPI: "responses", ReasoningEffort: "medium"}
	if config == nil || config.ApiKey == nil {
		return nil, credentials, fmt.Errorf("NovaCode API key is required")
	}
	key := strings.TrimSpace(*config.ApiKey)
	if strings.HasPrefix(key, "{") {
		if err := json.Unmarshal([]byte(key), &credentials); err != nil {
			return nil, credentials, fmt.Errorf("invalid NovaCode configuration: %w", err)
		}
	} else {
		credentials.APIKey = key
	}
	credentials.APIKey = strings.TrimSpace(credentials.APIKey)
	if credentials.APIKey == "" {
		return nil, credentials, fmt.Errorf("NovaCode API key is required")
	}
	if credentials.WireAPI == "" {
		credentials.WireAPI = "responses"
	}
	if credentials.ReasoningEffort == "" {
		credentials.ReasoningEffort = "medium"
	}
	if credentials.WireAPI != "responses" && credentials.WireAPI != "chat_completions" {
		return nil, credentials, fmt.Errorf("unsupported NovaCode wire_api %q", credentials.WireAPI)
	}
	if err := validateNovaCodeEffort(credentials.ReasoningEffort); err != nil {
		return nil, credentials, err
	}
	normalized := *config
	normalized.ApiKey = &credentials.APIKey
	return &normalized, credentials, nil
}

func validateNovaCodeEffort(effort string) error {
	switch effort {
	case "low", "medium", "high", "xhigh", "max":
		return nil
	}
	return fmt.Errorf("unsupported NovaCode reasoning_effort %q", effort)
}

func (m *NovaCodeModel) prepare(apiConfig *APIConfig, cfg *ChatConfig, model string, messages []Message, stream bool) (*APIConfig, string, map[string]any, bool, error) {
	api, credentials, err := novaCodeConfig(apiConfig)
	if err != nil {
		return nil, "", nil, false, err
	}
	if strings.TrimSpace(model) == "" || len(messages) == 0 {
		return nil, "", nil, false, fmt.Errorf("NovaCode model and messages are required")
	}
	effort := credentials.ReasoningEffort
	if cfg != nil && cfg.Effort != nil {
		effort = strings.TrimSpace(*cfg.Effort)
	}
	if err := validateNovaCodeEffort(effort); err != nil {
		return nil, "", nil, false, err
	}
	responses := credentials.WireAPI == "responses"
	suffix := m.baseModel.URLSuffix.Chat
	var body map[string]any
	if responses {
		suffix = m.baseModel.URLSuffix.Responses
		body, err = buildNovaCodeResponsesRequest(cfg, model, messages, stream, effort)
		if err != nil {
			return nil, "", nil, false, err
		}
	} else {
		body = buildRequestBody(cfg, model, messages, stream)
		delete(body, "do_sample")
		body["reasoning_effort"] = effort
		if cfg != nil {
			setOpt(body, "verbosity", cfg.Verbosity)
		}
		if stream {
			body["stream_options"] = map[string]any{"include_usage": true}
		}
	}
	endpoint, err := m.baseModel.EndpointURL(api, suffix)
	return api, endpoint, body, responses, err
}

func (m *NovaCodeModel) ChatWithMessages(ctx context.Context, model string, messages []Message, apiConfig *APIConfig, cfg *ChatConfig, usage *common.ModelUsage) (*ChatResponse, error) {
	api, endpoint, request, responses, err := m.prepare(apiConfig, cfg, model, messages, false)
	if err != nil {
		return nil, err
	}
	body, err := m.baseModel.doRequest(ctx, endpoint, api, request, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}
	if !responses {
		return HandleNonStreamingResponse(ctx, body, usage, cfg, OpenAIParserConfig)
	}
	var response novaCodeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("invalid NovaCode response: %w", err)
	}
	if err := response.validate(); err != nil {
		return nil, err
	}
	result := response.chatResponse()
	if result.Answer == nil && len(result.ToolCalls) == 0 {
		return nil, fmt.Errorf("NovaCode response contains no text or tool calls")
	}
	if cfg != nil {
		cfg.UsageResult = result.Usage
		cfg.ProviderStateResult = &result.ProviderState
	}
	if result.Usage != nil {
		common.ChargeTokens(ctx, result.Usage.TotalTokens)
	}
	recordResponseUsage(usage, response.ID, result.Usage, "chat")
	return result, nil
}

func (m *NovaCodeModel) ChatStreamlyWithSender(ctx context.Context, model string, messages []Message, apiConfig *APIConfig, cfg *ChatConfig, usage *common.ModelUsage, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if err := validateStreamConfig(cfg); err != nil {
		return err
	}
	api, endpoint, request, responses, err := m.prepare(apiConfig, cfg, model, messages, true)
	if err != nil {
		return err
	}
	return m.baseModel.doStreamRequest(ctx, endpoint, api, request, streamCallTimeout, func(body io.ReadCloser) error {
		if !responses {
			return HandleStreamingResponse(body, usage, cfg, OpenAIParserConfig, sender)
		}
		return handleNovaCodeResponseStream(body, usage, cfg, sender)
	})
}

func (m *NovaCodeModel) ListModels(ctx context.Context, config *APIConfig) ([]ListModelResponse, error) {
	api, _, err := novaCodeConfig(config)
	if err != nil {
		return nil, err
	}
	endpoint, err := m.baseModel.EndpointURL(api, m.baseModel.URLSuffix.Models)
	if err != nil {
		return nil, err
	}
	body, err := m.baseModel.doGetRequest(ctx, endpoint, api, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}
	var list ModelList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("invalid NovaCode model list: %w", err)
	}
	capabilities := m.fetchCapabilities(ctx, api)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return parseNovaCodeModels(list, capabilities), nil
}
func (m *NovaCodeModel) CheckConnection(ctx context.Context, config *APIConfig) error {
	_, err := m.ListModels(ctx, config)
	return err
}

func buildNovaCodeResponsesRequest(cfg *ChatConfig, model string, messages []Message, stream bool, effort string) (map[string]any, error) {
	input := make([]any, 0, len(messages))
	for _, message := range messages {
		if message.Role == "assistant" {
			if output := novaCodeStateOutput(message.ProviderState); len(output) > 0 {
				input = append(input, output...)
				continue
			}
		}
		if message.Role == "tool" {
			if message.ToolCallID == "" {
				return nil, fmt.Errorf("NovaCode tool result requires tool_call_id")
			}
			output, ok := message.Content.(string)
			if !ok {
				encoded, err := json.Marshal(message.Content)
				if err != nil {
					return nil, err
				}
				output = string(encoded)
			}
			input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": output})
			continue
		}
		if message.Content != nil {
			content, err := novaCodeInputContent(message.Content, message.Role)
			if err != nil {
				return nil, err
			}
			input = append(input, map[string]any{"role": message.Role, "content": content})
		}
		for _, call := range message.ToolCalls {
			function, ok := call["function"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid NovaCode function call")
			}
			input = append(input, map[string]any{"type": "function_call", "call_id": call["id"], "name": function["name"], "arguments": function["arguments"]})
		}
	}
	body := map[string]any{"model": model, "input": input, "stream": stream, "store": false, "reasoning": map[string]any{"effort": effort}, "include": []string{"reasoning.encrypted_content"}}
	if cfg == nil {
		return body, nil
	}
	setOpt(body, "max_output_tokens", cfg.MaxTokens)
	setOpt(body, "temperature", cfg.Temperature)
	setOpt(body, "top_p", cfg.TopP)
	if cfg.Verbosity != nil {
		body["text"] = map[string]any{"verbosity": *cfg.Verbosity}
	}
	if cfg.Tools != nil {
		raw, err := json.Marshal(cfg.Tools)
		if err != nil {
			return nil, err
		}
		var tools []map[string]any
		if err := json.Unmarshal(raw, &tools); err != nil {
			return nil, fmt.Errorf("invalid NovaCode tools: %w", err)
		}
		converted := make([]map[string]any, 0, len(tools))
		for _, tool := range tools {
			function, ok := tool["function"].(map[string]any)
			if tool["type"] != "function" || !ok {
				return nil, fmt.Errorf("NovaCode supports function tools")
			}
			convertedTool := map[string]any{"type": "function"}
			for _, key := range []string{"name", "description", "parameters", "strict"} {
				if value, ok := function[key]; ok {
					convertedTool[key] = value
				}
			}
			converted = append(converted, convertedTool)
		}
		body["tools"] = converted
	}
	if cfg.ToolChoiceValue != nil {
		raw, err := json.Marshal(cfg.ToolChoiceValue)
		if err != nil {
			return nil, err
		}
		var choice map[string]any
		if err := json.Unmarshal(raw, &choice); err != nil {
			return nil, err
		}
		if function, ok := choice["function"].(map[string]any); ok {
			choice = map[string]any{"type": "function", "name": function["name"]}
		}
		body["tool_choice"] = choice
	} else if cfg.ToolChoice != nil {
		body["tool_choice"] = *cfg.ToolChoice
	}
	return body, nil
}

func novaCodeInputContent(content any, role string) (any, error) {
	if text, ok := content.(string); ok {
		return text, nil
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("unsupported NovaCode message content: %w", err)
	}
	converted := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part["type"] {
		case "text":
			kind := "input_text"
			if role == "assistant" {
				kind = "output_text"
			}
			converted = append(converted, map[string]any{"type": kind, "text": part["text"]})
		case "image_url":
			image, ok := part["image_url"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid NovaCode image_url content")
			}
			item := map[string]any{"type": "input_image", "image_url": image["url"]}
			if detail, ok := image["detail"]; ok {
				item["detail"] = detail
			}
			converted = append(converted, item)
		default:
			return nil, fmt.Errorf("unsupported NovaCode content type %v", part["type"])
		}
	}
	return converted, nil
}

type novaCodeOutput struct {
	Raw       map[string]any `json:"-"`
	Type      string         `json:"type"`
	ID        string         `json:"id"`
	CallID    string         `json:"call_id"`
	Name      string         `json:"name"`
	Arguments string         `json:"arguments"`
	Content   []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Refusal string `json:"refusal"`
	} `json:"content"`
	Summary []struct {
		Text string `json:"text"`
	} `json:"summary"`
}

func (output *novaCodeOutput) UnmarshalJSON(data []byte) error {
	type decodedOutput novaCodeOutput
	var decoded decodedOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := json.Unmarshal(data, &decoded.Raw); err != nil {
		return err
	}
	*output = novaCodeOutput(decoded)
	return nil
}

type novaCodeResponse struct {
	ID                string           `json:"id"`
	Status            string           `json:"status"`
	Error             json.RawMessage  `json:"error"`
	IncompleteDetails json.RawMessage  `json:"incomplete_details"`
	Output            []novaCodeOutput `json:"output"`
	Usage             *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (r *novaCodeResponse) validate() error {
	if len(r.Error) > 0 && string(r.Error) != "null" {
		return fmt.Errorf("NovaCode response error: %s", r.Error)
	}
	switch r.Status {
	case "", "completed":
		return nil
	case "incomplete":
		return fmt.Errorf("NovaCode response incomplete: %s", r.IncompleteDetails)
	default:
		return fmt.Errorf("NovaCode response ended with status %q", r.Status)
	}
}
func (r *novaCodeResponse) chatResponse() *ChatResponse {
	var text, reason strings.Builder
	calls := []map[string]any{}
	for _, output := range r.Output {
		switch output.Type {
		case "message":
			for _, content := range output.Content {
				if content.Type == "output_text" {
					text.WriteString(content.Text)
				}
				if content.Type == "refusal" {
					text.WriteString(content.Refusal)
				}
			}
		case "reasoning":
			for _, summary := range output.Summary {
				reason.WriteString(summary.Text)
			}
		case "function_call":
			calls = append(calls, novaCodeToolCall(output))
		}
	}
	result := &ChatResponse{ToolCalls: calls, ProviderState: novaCodeState(r.Output)}
	if text.Len() > 0 || len(calls) > 0 {
		s := text.String()
		result.Answer = &s
	}
	if reason.Len() > 0 {
		s := reason.String()
		result.ReasonContent = &s
	}
	if r.Usage != nil {
		result.Usage = &TokenUsage{PromptTokens: r.Usage.InputTokens, CompletionTokens: r.Usage.OutputTokens, TotalTokens: r.Usage.TotalTokens}
	}
	return result
}
func novaCodeToolCall(output novaCodeOutput) map[string]any {
	return map[string]any{"id": output.CallID, "type": "function", "function": map[string]any{"name": output.Name, "arguments": output.Arguments}}
}

// novaCodeState keeps the original output items, including encrypted reasoning,
// next to the assistant turn. A subsequent stateless request must replay these
// items before sending the matching function_call_output items.
func novaCodeState(outputs []novaCodeOutput) []map[string]any {
	raw := make([]any, 0, len(outputs))
	for _, output := range outputs {
		if output.Raw != nil {
			raw = append(raw, output.Raw)
		}
	}
	if len(raw) == 0 {
		return nil
	}
	return []map[string]any{{"provider": "NovaCode", "output": raw}}
}

func novaCodeStateOutput(state []map[string]any) []any {
	for _, entry := range state {
		if entry["provider"] != "NovaCode" {
			continue
		}
		if output, ok := entry["output"].([]any); ok {
			return output
		}
	}
	return nil
}

type novaCodeStreamEvent struct {
	Type        string           `json:"type"`
	Delta       string           `json:"delta"`
	OutputIndex int              `json:"output_index"`
	Item        novaCodeOutput   `json:"item"`
	Arguments   string           `json:"arguments"`
	Response    novaCodeResponse `json:"response"`
	Error       json.RawMessage  `json:"error"`
	Message     string           `json:"message"`
}

func handleNovaCodeResponseStream(body io.Reader, usage *common.ModelUsage, cfg *ChatConfig, sender func(*string, *string) error) error {
	completed := errors.New("NovaCode response completed")
	calls := map[int]map[string]any{}
	outputs := map[int]novaCodeOutput{}
	complete := false
	var final novaCodeResponse
	_, err := ParseSSEStream[novaCodeStreamEvent](body, func(event novaCodeStreamEvent) error {
		switch event.Type {
		case "error":
			return fmt.Errorf("NovaCode stream error: %s %s", event.Message, event.Error)
		case "response.failed", "response.incomplete", "response.cancelled":
			if err := event.Response.validate(); err != nil {
				return err
			}
			return fmt.Errorf("NovaCode stream ended with %s", event.Type)
		case "response.output_text.delta", "response.refusal.delta":
			if event.Delta != "" {
				return sender(&event.Delta, nil)
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if event.Delta != "" {
				return sender(nil, &event.Delta)
			}
		case "response.output_item.added", "response.output_item.done":
			outputs[event.OutputIndex] = event.Item
			if event.Item.Type == "function_call" {
				calls[event.OutputIndex] = novaCodeToolCall(event.Item)
			}
		case "response.function_call_arguments.delta":
			call, ok := calls[event.OutputIndex]
			if !ok {
				return fmt.Errorf("NovaCode tool arguments arrived before function call")
			}
			function := call["function"].(map[string]any)
			function["arguments"] = function["arguments"].(string) + event.Delta
			output := outputs[event.OutputIndex]
			output.Arguments = function["arguments"].(string)
			if output.Raw != nil {
				output.Raw["arguments"] = output.Arguments
			}
			outputs[event.OutputIndex] = output
		case "response.function_call_arguments.done":
			if call, ok := calls[event.OutputIndex]; ok {
				call["function"].(map[string]any)["arguments"] = event.Arguments
				output := outputs[event.OutputIndex]
				output.Arguments = event.Arguments
				if output.Raw != nil {
					output.Raw["arguments"] = event.Arguments
				}
				outputs[event.OutputIndex] = output
			}
		case "response.completed":
			if err := event.Response.validate(); err != nil {
				return err
			}
			complete = true
			final = event.Response
			for index, item := range final.Output {
				if item.Type == "function_call" {
					calls[index] = novaCodeToolCall(item)
				}
			}
			return completed
		}
		return nil
	})
	if err != nil && !errors.Is(err, completed) {
		return fmt.Errorf("NovaCode stream: %w", err)
	}
	if !complete {
		return fmt.Errorf("NovaCode stream ended before response.completed")
	}
	if len(final.Output) == 0 {
		indices := make([]int, 0, len(outputs))
		for index := range outputs {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			final.Output = append(final.Output, outputs[index])
		}
	}
	result := final.chatResponse()
	if cfg != nil {
		cfg.UsageResult = result.Usage
		cfg.ProviderStateResult = &result.ProviderState
		setSortedToolCallsResult(cfg, calls)
	}
	recordResponseUsage(usage, final.ID, result.Usage, "chat")
	done := "[DONE]"
	return sender(&done, nil)
}
