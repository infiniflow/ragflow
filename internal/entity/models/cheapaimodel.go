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
	"strings"
)

// CheapaimodelModel drives cheapaimodel (https://api.cheapaimodel.ai/v1), a
// pay-per-token gateway for open-weight models (DeepSeek, GLM, Kimi) plus
// Grok. It serves OpenAI-compatible chat and model-list endpoints, so the
// embedded OpenAIAPICompatibleModel covers every call.
type CheapaimodelModel struct {
	*OpenAIAPICompatibleModel
}

// NewCheapaimodelModel creates a cheapaimodel driver.
func NewCheapaimodelModel(baseURL map[string]string, urlSuffix URLSuffix) *CheapaimodelModel {
	return &CheapaimodelModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *CheapaimodelModel) Name() string {
	return "cheapaimodel"
}

// NewInstance returns a driver bound to the given base URL.
func (m *CheapaimodelModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewCheapaimodelModel(baseURL, m.baseModel.URLSuffix)
}

// ListModels lists the models visible to the API key with a bodyless GET.
// The embedded vLLM driver sends a "{}" body on GET /models, which the Google
// load balancer in front of api.cheapaimodel.ai rejects with 400, so this
// driver issues the request itself, the same way the xAI driver does.
//
// Ids in the cheapaimodel catalog (conf/models/cheapaimodel.json) take its
// model types, context length and max output, so vision models keep their
// image input and the limits reach the UI; ParseListModel only consults the
// global catalog, which has no entry for most of these ids and may hold another
// provider's figures for the rest. A context length in the response itself
// still wins. Other ids keep the types ParseListModel resolved (global catalog,
// else name inference). The provider configures only the chat endpoint, so
// every type but chat and vision is removed, and ids left with none
// (embedding, rerank, audio) are not listed.
func (m *CheapaimodelModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	modelsSuffix := strings.Trim(strings.TrimSpace(m.baseModel.URLSuffix.Models), "/")
	if modelsSuffix == "" {
		return nil, fmt.Errorf("cheapaimodel: models URL suffix is not configured")
	}
	url, err := m.baseModel.EndpointURL(apiConfig, modelsSuffix)
	if err != nil {
		return nil, err
	}

	body, err := m.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	var provider *Provider
	pm := GetProviderManager()
	if pm != nil {
		provider = pm.FindProvider("cheapaimodel")
	}

	remoteContext := make(map[string]*int, len(modelList.Models))
	for _, item := range modelList.Models {
		if item.ContextLength != nil && *item.ContextLength > 0 {
			remoteContext[strings.TrimSpace(item.ID)] = item.ContextLength
		}
	}

	models := ParseListModel(modelList)
	filtered := make([]ListModelResponse, 0, len(models))
	for _, model := range models {
		if pm != nil && provider != nil {
			if known := pm.FindModel(provider, model.Name); known != nil {
				applyCatalogModel(&model, known)
			}
		}
		if contextLength, ok := remoteContext[model.Name]; ok {
			model.ContextLength = contextLength
		}
		model.ModelTypes = chatModelTypes(model.ModelTypes)
		if len(model.ModelTypes) == 0 {
			continue
		}
		filtered = append(filtered, model)
	}
	return filtered, nil
}

// applyCatalogModel overrides a listed model with the fields its provider
// catalog entry sets.
func applyCatalogModel(model *ListModelResponse, known *Model) {
	if len(known.ModelTypes) > 0 {
		model.ModelTypes = known.ModelTypes
	}
	if known.ContextLength != nil {
		model.ContextLength = known.ContextLength
	}
	if known.MaxOutput != nil {
		model.MaxOutput = known.MaxOutput
	}
}

// chatModelTypes keeps the types served by the chat endpoint, in a new slice.
func chatModelTypes(types []string) []string {
	kept := make([]string, 0, len(types))
	for _, t := range types {
		if t == modelTypeChat || t == modelTypeVision {
			kept = append(kept, t)
		}
	}
	return kept
}

// CheckConnection verifies the API key with a model-list call; GET /models
// requires the key on cheapaimodel.
func (m *CheapaimodelModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}
