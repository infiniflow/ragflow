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
	"fmt"
	"io"
	"strings"

	"ragflow/internal/common"
)

// OnomeoModel drives onomeo (https://onomeo.com/v1), a gateway that serves
// Claude, GPT, DeepSeek and other models through OpenAI-compatible chat and
// model-list endpoints.
//
// The embedded OpenAIAPICompatibleModel supplies the shared plumbing and the
// model list. Chat requests are built here: the gateway forwards each request
// to the model's upstream, so the vLLM thinking payloads and do_sample that the
// embedded driver adds are not sent, and the effort reaches the gateway as
// reasoning_effort, the field it maps onto each upstream.
type OnomeoModel struct {
	*OpenAIAPICompatibleModel
}

// NewOnomeoModel creates an onomeo driver. The gateway authenticates every
// call, so the driver does not keep the embedded driver's allowance for an
// empty API key.
func NewOnomeoModel(baseURL map[string]string, urlSuffix URLSuffix) *OnomeoModel {
	model := NewOpenAIAPICompatibleModel(baseURL, urlSuffix)
	model.baseModel.AllowEmptyAPIKey = false
	return &OnomeoModel{OpenAIAPICompatibleModel: model}
}

// Name returns the driver name.
func (m *OnomeoModel) Name() string {
	return "onomeo"
}

// NewInstance returns a driver bound to the given base URL.
func (m *OnomeoModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewOnomeoModel(baseURL, m.baseModel.URLSuffix)
}

// onomeoParserConfig reads usage like OpenAIParserConfig. The gateway passes
// each upstream's streamed reasoning through unchanged, as
// delta.reasoning_content (DeepSeek and most direct upstreams) or as
// delta.reasoning (upstreams that follow OpenRouter), so both are read.
var onomeoParserConfig = &ParserConfig{
	Protocol:               "openai",
	ResponseParser:         extractOpenAIUsage,
	StreamParser:           extractOpenAIStreamUsage,
	ExtractStreamReasoning: extractOnomeoStreamReasoning,
}

// extractOnomeoStreamReasoning reads delta.reasoning_content, then
// delta.reasoning.
func extractOnomeoStreamReasoning(delta map[string]any) string {
	if r := extractDefaultStreamReasoning(delta); r != "" {
		return r
	}
	if r, ok := delta["reasoning"].(string); ok {
		return r
	}
	return ""
}

// applyOnomeoReasoning passes the effort through as reasoning_effort when it
// is low, medium or high, the levels the gateway maps onto every upstream that
// takes an effort. The gateway has no switch that turns reasoning off for
// every model, so the thinking switch alone sends nothing.
func applyOnomeoReasoning(reqBody map[string]any, chatModelConfig *ChatConfig) {
	if chatModelConfig == nil || chatModelConfig.Effort == nil {
		return
	}
	switch effort := strings.ToLower(strings.TrimSpace(*chatModelConfig.Effort)); effort {
	case "low", "medium", "high":
		reqBody["reasoning_effort"] = effort
	}
}

// buildOnomeoRequestBody builds a Chat Completions request for the gateway.
// It starts from the shared request builder, drops do_sample, which is not a
// Chat Completions parameter, and adds the reasoning effort. Streaming
// requests ask for the usage of the final chunk.
func buildOnomeoRequestBody(chatModelConfig *ChatConfig, modelName string, messages []Message, stream bool) map[string]any {
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, stream)
	delete(reqBody, "do_sample")
	applyOnomeoReasoning(reqBody, chatModelConfig)
	if stream {
		reqBody["stream_options"] = map[string]any{"include_usage": true}
	}
	return reqBody
}

// ChatWithMessages sends multiple messages with roles and returns response
func (m *OnomeoModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Chat)
	if err != nil {
		return nil, err
	}

	reqBody := buildOnomeoRequestBody(chatModelConfig, modelName, messages, false)

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(ctx, body, modelUsage, chatModelConfig, onomeoParserConfig)
}

// ChatStreamlyWithSender sends messages and streams response via sender function
func (m *OnomeoModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if chatModelConfig != nil {
		chatModelConfig.ToolCallsResult = nil
		chatModelConfig.UsageResult = nil
	}

	url, err := m.baseModel.EndpointURL(apiConfig, m.baseModel.URLSuffix.Chat)
	if err != nil {
		return err
	}

	reqBody := buildOnomeoRequestBody(chatModelConfig, modelName, messages, true)

	return m.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, onomeoParserConfig, sender)
	})
}

// ListModels lists the models the API key can call. The gateway answers
// GET /models only for a valid key, and the list depends on the account: an
// account that has not topped up sees the free models only.
func (m *OnomeoModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	return m.OpenAIAPICompatibleModel.ListModels(ctx, apiConfig)
}

// CheckConnection verifies the API key with a model-list call, which
// requires the key.
func (m *OnomeoModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}
