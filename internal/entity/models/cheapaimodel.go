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
// Ids in the cheapaimodel catalog (conf/models/cheapaimodel.json) report the
// catalogued model types, so vision models keep their image-input capability.
// Other ids fall back to name-based inference; ids with no inferable type are
// dropped, as in the embedded OpenAI-compatible driver.
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

	models := ParseListModel(modelList)
	filtered := make([]ListModelResponse, 0, len(models))
	for _, model := range models {
		model.ModelTypes = m.modelTypes(pm, provider, model.Name)
		if len(model.ModelTypes) == 0 {
			continue
		}
		filtered = append(filtered, model)
	}
	return filtered, nil
}

// modelTypes returns the catalogued types for a known id, else the types
// inferred from its name.
func (m *CheapaimodelModel) modelTypes(pm *ProviderManager, provider *Provider, name string) []string {
	if pm != nil && provider != nil {
		if known := pm.FindModel(provider, name); known != nil && len(known.ModelTypes) > 0 {
			return append([]string(nil), known.ModelTypes...)
		}
	}
	return InferModelTypes(name)
}

// CheckConnection verifies the API key with a model-list call; GET /models
// requires the key on cheapaimodel.
func (m *CheapaimodelModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := m.ListModels(ctx, apiConfig)
	return err
}
