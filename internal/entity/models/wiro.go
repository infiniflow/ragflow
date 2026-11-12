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

// WiroModel drives Wiro AI (https://llm.wiro.ai/v1), an LLM gateway that
// serves models from several providers under "owner/model" ids through
// OpenAI-compatible chat and model-list endpoints.
//
// The embedded OpenAIAPICompatibleModel supplies the shared plumbing. Chat
// requests and the model list are handled here: requests carry the Chat
// Completions fields the gateway documents, and listed models take their
// context window, output limit and image input from the gateway's own
// metadata instead of name inference.
type WiroModel struct {
	*OpenAIAPICompatibleModel
}

// NewWiroModel creates a Wiro AI driver. The gateway authenticates every
// call, so the driver does not keep the embedded driver's allowance for an
// empty API key.
func NewWiroModel(baseURL map[string]string, urlSuffix URLSuffix) *WiroModel {
	model := NewOpenAIAPICompatibleModel(baseURL, urlSuffix)
	model.baseModel.AllowEmptyAPIKey = false
	return &WiroModel{OpenAIAPICompatibleModel: model}
}

// Name returns the driver name.
func (m *WiroModel) Name() string {
	return "wiro"
}

// NewInstance returns a driver bound to the given base URL.
func (m *WiroModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewWiroModel(baseURL, m.baseModel.URLSuffix)
}

// buildWiroRequestBody builds a Chat Completions request for the gateway. It
// starts from the shared request builder and keeps the OpenAI fields:
// do_sample is dropped, stop is sent only when it holds a non-empty sequence,
// and the vLLM chat-template and thinking payloads that the embedded driver
// adds are left out.
func buildWiroRequestBody(chatModelConfig *ChatConfig, modelName string, messages []Message, stream bool) map[string]any {
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, stream)
	delete(reqBody, "do_sample")

	if stops, ok := reqBody["stop"].([]string); ok {
		sequences := make([]string, 0, len(stops))
		for _, stop := range stops {
			if stop != "" {
				sequences = append(sequences, stop)
			}
		}
		if len(sequences) == 0 {
			delete(reqBody, "stop")
		} else {
			reqBody["stop"] = sequences
		}
	}

	if stream {
		reqBody["stream_options"] = map[string]any{"include_usage": true}
	}
	return reqBody
}

// ChatWithMessages sends multiple messages with roles and returns response
func (m *WiroModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
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

	reqBody := buildWiroRequestBody(chatModelConfig, modelName, messages, false)

	body, err := m.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(ctx, body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams response via sender function
func (m *WiroModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
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

	reqBody := buildWiroRequestBody(chatModelConfig, modelName, messages, true)

	return m.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		// The gateway streams reasoning text under delta.reasoning, the
		// field OpenRouterParserConfig reads.
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenRouterParserConfig, sender)
	})
}

// wiroModelList is the part of the gateway's GET /models response the driver
// reads.
type wiroModelList struct {
	Models []wiroModelListItem `json:"data"`
}

// wiroModelListItem is one model of the list. context_length is the context
// window and max_tokens the output limit.
type wiroModelListItem struct {
	ID            string `json:"id"`
	ContextLength *int   `json:"context_length"`
	MaxTokens     *int   `json:"max_tokens"`
	Capabilities  struct {
		Endpoints       []string `json:"endpoints"`
		InputModalities []string `json:"input_modalities"`
	} `json:"capabilities"`
}

// ListModels lists the models visible to the API key. Each model takes its
// context window from context_length and its output limit from max_tokens,
// and a model whose capabilities.input_modalities include "image" is listed
// as a vision model. A model that names its endpoints is listed only when
// "chat" is one of them, since chat/completions is the route this driver
// calls.
func (m *WiroModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	modelsSuffix := strings.Trim(strings.TrimSpace(m.baseModel.URLSuffix.Models), "/")
	if modelsSuffix == "" {
		return nil, fmt.Errorf("wiro: models URL suffix is not configured")
	}
	url, err := m.baseModel.EndpointURL(apiConfig, modelsSuffix)
	if err != nil {
		return nil, err
	}

	body, err := m.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var modelList wiroModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	models := make([]ListModelResponse, 0, len(modelList.Models))
	for _, item := range modelList.Models {
		name := strings.TrimSpace(item.ID)
		if name == "" || !wiroHasChatEndpoint(item) {
			continue
		}

		model := ListModelResponse{
			Name:       name,
			ModelTypes: []string{modelTypeChat},
		}
		if wiroHasImageInput(item) {
			model.ModelTypes = []string{modelTypeChat, modelTypeVision}
		}
		if item.ContextLength != nil && *item.ContextLength > 0 {
			model.ContextLength = item.ContextLength
		}
		if item.MaxTokens != nil && *item.MaxTokens > 0 {
			model.MaxOutput = item.MaxTokens
		}
		models = append(models, model)
	}
	return models, nil
}

// wiroHasChatEndpoint reports whether a listed model can be called through
// chat/completions. A model that does not name its endpoints is kept.
func wiroHasChatEndpoint(item wiroModelListItem) bool {
	if len(item.Capabilities.Endpoints) == 0 {
		return true
	}
	for _, endpoint := range item.Capabilities.Endpoints {
		if strings.EqualFold(endpoint, "chat") {
			return true
		}
	}
	return false
}

// wiroHasImageInput reports whether a listed model takes image input.
func wiroHasImageInput(item wiroModelListItem) bool {
	for _, modality := range item.Capabilities.InputModalities {
		if strings.EqualFold(modality, "image") {
			return true
		}
	}
	return false
}

// CheckConnection verifies the API key with a model-list call, which
// requires the key.
func (m *WiroModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}
