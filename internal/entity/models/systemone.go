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
	"math"
	"strings"

	"ragflow/internal/common"
)

// systemOneDefaultSuffix is the decision endpoint this protocol uses, declared
// in the provider catalog as url_suffix.decision. Decide builds its URL from it.
const systemOneDefaultSuffix = "systemone"

// systemOneDefaultModelsSuffix lists the models the configured deployment
// serves; it is the standard OpenAI-shaped path, which this API reuses.
const systemOneDefaultModelsSuffix = "models"

// SystemOneModel implements ModelDriver for endpoints speaking TypeSafe System
// One: one request carrying several questions, answered in a single round trip.
//
// It embeds OpenAIAPICompatibleModel only to satisfy the rest of the (wide)
// ModelDriver interface. Decide is one of the two methods it implements itself:
// a probe of the decision endpoint, which is what verification calls.
//
// What this driver is for is REGISTRATION: making the provider appear in the
// picker, making its model appear once a base URL has been shown to work, and
// making that model pass the verification the model form runs before saving.
type SystemOneModel struct {
	*OpenAIAPICompatibleModel
}

// NewSystemOneModel creates a System One provider instance.
//
// Missing URL suffixes are filled in here rather than at each call site: a
// catalog entry that forgets them would otherwise send the decide call to the
// bare host and the model list to the base URL, both of which answer 404 with no
// hint about which suffix was missing.
func NewSystemOneModel(baseURL map[string]string, urlSuffix URLSuffix) *SystemOneModel {
	if urlSuffix.Decision == "" {
		urlSuffix.Decision = systemOneDefaultSuffix
	}
	if urlSuffix.Models == "" {
		urlSuffix.Models = systemOneDefaultModelsSuffix
	}
	return &SystemOneModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the provider identifier.
func (m *SystemOneModel) Name() string {
	return "SystemOne"
}

// NewInstance creates an instance bound to another base URL.
func (m *SystemOneModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewSystemOneModel(baseURL, m.baseModel.URLSuffix)
}

// systemOneModelName is the model this protocol serves.
//
// It is declared here rather than discovered because the deployment's /models
// does not list it: that endpoint returns the host's general model catalog (178
// entries for the reference deployment, none of them a decision model). Asking
// for the catalog would therefore either surface no decision model at all or
// offer the whole chat catalog as decision models — both wrong.
const systemOneModelName = "decision-model-preview"

// ListModels proves the configured base URL actually works, then returns the
// model this provider can drive.
//
// The probe is the point: the callback is only called once a base URL is
// entered, so the model appears — and can be added — only after the address has
// been shown to answer with the supplied credential. Returning the model
// without probing would let a user add a model to a provider that cannot serve
// it, and the failure would surface much later, on the first decision call.
func (m *SystemOneModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if _, err := m.VllmModel.ListModels(ctx, apiConfig); err != nil {
		return nil, err
	}
	return []ListModelResponse{{
		Name:       systemOneModelName,
		ModelTypes: []string{"decision"},
	}}, nil
}

// ChatWithMessages refuses rather than forwarding to /chat/completions.
//
// The endpoint has no chat shape. Forwarding would surface the server's 404 as
// an opaque transport error, and a caller that ignores the error would read the
// empty body as an empty answer. Failing here names the actual mistake: this
// model belongs in the decision slot, not the chat one.
func (m *SystemOneModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	return nil, fmt.Errorf("systemone: %q is a decision model and cannot answer chat completions; select it as the decision model instead", modelName)
}

// Decide asks probability questions about one state in a single request.
func (m *SystemOneModel) Decide(ctx context.Context, modelName *string, request *DecideRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) (*DecideResponse, error) {
	if modelName == nil || strings.TrimSpace(*modelName) == "" {
		return nil, fmt.Errorf("systemone: model name is required")
	}
	if request == nil || len(request.Questions) == 0 || len(request.Questions) > MaxDecisionQuestions {
		return nil, fmt.Errorf("systemone: between 1 and %d questions are required", MaxDecisionQuestions)
	}
	for id, question := range request.Questions {
		if _, ok := question.(Noul); !ok {
			return nil, fmt.Errorf("systemone: question %q must be a noul question", id)
		}
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || strings.TrimSpace(*apiConfig.ApiKey) == "" {
		return nil, fmt.Errorf("systemone: api key is required")
	}
	url, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	suffix := strings.Trim(m.baseModel.URLSuffix.Decision, "/")
	if suffix != "" && !strings.HasSuffix(strings.ToLower(url), "/"+strings.ToLower(suffix)) {
		url = joinURL(url, suffix)
	}
	body, err := m.baseModel.doRequest(ctx, url, apiConfig, map[string]any{
		"model": *modelName, "state": request.State, "questions": request.Questions,
	}, nonStreamCallTimeout)
	if err != nil {
		return nil, fmt.Errorf("systemone: %w", err)
	}
	var response DecideResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("systemone: decode response: %w", err)
	}
	if len(response.Answers) == 0 {
		return nil, fmt.Errorf("systemone: no answers in response")
	}
	for id, answer := range response.Answers {
		if answer.Noul != nil && (math.IsNaN(*answer.Noul) || math.IsInf(*answer.Noul, 0) || *answer.Noul < 0 || *answer.Noul > 1) {
			return nil, fmt.Errorf("systemone: invalid probability for question %q", id)
		}
	}
	recordResponseUsage(modelUsage, response.ID, &TokenUsage{
		PromptTokens: response.Usage.InputTokens, CompletionTokens: response.Usage.OutputTokens,
		TotalTokens: response.Usage.InputTokens + response.Usage.OutputTokens,
	}, "decision")
	return &response, nil
}
