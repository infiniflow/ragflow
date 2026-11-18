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
	"fmt"
	"io"
	"strings"

	"ragflow/internal/common"
)

// CrusoeModel drives Crusoe (https://api.inference.crusoecloud.com/v1), which
// serves open-weight models from its own GPU cloud under "owner/model" ids
// through OpenAI-compatible chat and model-list endpoints.
//
// The embedded OpenAIAPICompatibleModel supplies the shared plumbing. Chat
// requests and the model list are handled here: the thinking switch and the
// effort map onto reasoning_effort, the field the endpoint reads, and listed
// models take their context window, output limit and image input from the
// endpoint's own metadata instead of name inference.
type CrusoeModel struct {
	*OpenAIAPICompatibleModel
}

// NewCrusoeModel creates a Crusoe driver. The endpoint authenticates every
// call, so the driver does not keep the embedded driver's allowance for an
// empty API key.
func NewCrusoeModel(baseURL map[string]string, urlSuffix URLSuffix) *CrusoeModel {
	model := NewOpenAIAPICompatibleModel(baseURL, urlSuffix)
	model.baseModel.AllowEmptyAPIKey = false
	return &CrusoeModel{OpenAIAPICompatibleModel: model}
}

// Name returns the driver name.
func (m *CrusoeModel) Name() string {
	return "crusoe"
}

// NewInstance returns a driver bound to the given base URL.
func (m *CrusoeModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewCrusoeModel(baseURL, m.baseModel.URLSuffix)
}

// crusoeReasoningOffEffort returns the reasoning_effort value that asks
// modelName for the least reasoning it supports, and false when the model has
// no documented value for it.
//
// Probed against POST /chat/completions on 2026-08-13: gemma-4-31b-it,
// Nemotron-3-Nano-Omni and GLM-5.2 return no reasoning with "none", and the
// rule covers their families. gpt-oss rejects "none" with HTTP 400 ("Harmony
// does not support reasoning_effort='none'") and cannot stop reasoning, so its
// shortest level, "low", is used. Other models are left alone because "none"
// was a validation error on some of them.
func crusoeReasoningOffEffort(modelName string) (string, bool) {
	lower := strings.ToLower(modelName)
	if strings.Contains(lower, "gpt-oss") {
		return "low", true
	}
	for _, family := range []string{"gemma", "nemotron", "glm"} {
		if strings.Contains(lower, family) {
			return "none", true
		}
	}
	return "", false
}

// applyCrusoeReasoning maps the thinking switch and the effort onto
// reasoning_effort. A disabled switch wins over an effort. The effort passes
// through only as low, medium or high, the values every model on the endpoint
// accepts. An enabled switch without an effort sends "medium", which turns
// reasoning on for models that leave it off by default.
func applyCrusoeReasoning(reqBody map[string]any, modelName string, chatModelConfig *ChatConfig) {
	if chatModelConfig == nil {
		return
	}
	if chatModelConfig.Thinking != nil && !*chatModelConfig.Thinking {
		if effort, ok := crusoeReasoningOffEffort(modelName); ok {
			reqBody["reasoning_effort"] = effort
		}
		return
	}
	if chatModelConfig.Effort != nil {
		switch effort := strings.ToLower(strings.TrimSpace(*chatModelConfig.Effort)); effort {
		case "low", "medium", "high":
			reqBody["reasoning_effort"] = effort
			return
		}
	}
	if chatModelConfig.Thinking != nil {
		reqBody["reasoning_effort"] = "medium"
	}
}

// buildCrusoeRequestBody builds a Chat Completions request for the endpoint.
// It starts from the shared request builder, drops do_sample, which is not a
// parameter the endpoint lists, and adds the reasoning effort. Streaming
// requests ask for the usage of the final chunk.
func buildCrusoeRequestBody(chatModelConfig *ChatConfig, modelName string, messages []Message, stream bool) map[string]any {
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, stream)
	delete(reqBody, "do_sample")
	applyCrusoeReasoning(reqBody, modelName, chatModelConfig)
	if stream {
		reqBody["stream_options"] = map[string]any{"include_usage": true}
	}
	return reqBody
}

// ChatWithMessages sends multiple messages with roles and returns response
func (m *CrusoeModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
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

	reqBody := buildCrusoeRequestBody(chatModelConfig, modelName, messages, false)

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(ctx, body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams response via sender function
func (m *CrusoeModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
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

	reqBody := buildCrusoeRequestBody(chatModelConfig, modelName, messages, true)

	return m.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		// The endpoint streams reasoning text under delta.reasoning, the
		// field OpenRouterParserConfig reads.
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenRouterParserConfig, sender)
	})
}

// crusoeModelList is the part of the endpoint's GET /models response the
// driver reads.
type crusoeModelList struct {
	Models []crusoeModelListItem `json:"data"`
}

// crusoeModelListItem is one deployment of the list. context_length is the
// context window and top_provider.max_completion_tokens the output limit.
type crusoeModelListItem struct {
	ID            string `json:"id"`
	IsPublic      *bool  `json:"is_public"`
	ContextLength *int   `json:"context_length"`
	Architecture  struct {
		Modality string `json:"modality"`
	} `json:"architecture"`
	TopProvider struct {
		MaxCompletionTokens *int `json:"max_completion_tokens"`
	} `json:"top_provider"`
}

// ListModels lists the public chat models visible to the API key. The list
// also carries the account's private deployments and non-chat deployments
// such as embeddings, which are left out: a private deployment does not state
// what it serves, and chat/completions is the route this driver calls. Each
// model takes its context window from context_length and its output limit
// from top_provider.max_completion_tokens, and a model whose
// architecture.modality is "multimodal" is listed as a vision model.
func (m *CrusoeModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	modelsSuffix := strings.Trim(strings.TrimSpace(m.baseModel.URLSuffix.Models), "/")
	if modelsSuffix == "" {
		return nil, fmt.Errorf("crusoe: models URL suffix is not configured")
	}
	url, err := m.baseModel.EndpointURL(apiConfig, modelsSuffix)
	if err != nil {
		return nil, err
	}

	body, err := m.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var modelList crusoeModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	models := make([]ListModelResponse, 0, len(modelList.Models))
	for _, item := range modelList.Models {
		name := strings.TrimSpace(item.ID)
		if name == "" || !crusoeIsPublicChatModel(item) {
			continue
		}

		model := ListModelResponse{
			Name:       name,
			ModelTypes: []string{modelTypeChat},
		}
		if strings.EqualFold(strings.TrimSpace(item.Architecture.Modality), "multimodal") {
			model.ModelTypes = []string{modelTypeChat, modelTypeVision}
		}
		if item.ContextLength != nil && *item.ContextLength > 0 {
			model.ContextLength = item.ContextLength
		}
		if item.TopProvider.MaxCompletionTokens != nil && *item.TopProvider.MaxCompletionTokens > 0 {
			model.MaxOutput = item.TopProvider.MaxCompletionTokens
		}
		models = append(models, model)
	}
	return models, nil
}

// crusoeIsPublicChatModel reports whether a listed deployment is a public
// chat model. The modality decides it: the type field is "multimodal" on
// vision models and blank on some chat models, so it cannot. A deployment
// that does not name a modality is kept unless it is private.
func crusoeIsPublicChatModel(item crusoeModelListItem) bool {
	if item.IsPublic != nil && !*item.IsPublic {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(item.Architecture.Modality)) {
	case "", "text", "multimodal":
		return true
	}
	return false
}

// CheckConnection verifies the API key with a model-list call, which
// requires the key.
func (m *CrusoeModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}
