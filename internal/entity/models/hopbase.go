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

import "context"

const hopBaseProviderName = "HopBase"

// HopBaseModel drives HopBase (https://api.hop-base.com/v1), an enterprise AI
// gateway that serves many model families behind one key through
// OpenAI-compatible chat and model-list endpoints, so the embedded
// OpenAIAPICompatibleModel covers every call. The model list is scoped to the
// API key.
type HopBaseModel struct {
	*OpenAIAPICompatibleModel
}

// NewHopBaseModel creates a HopBase driver.
func NewHopBaseModel(baseURL map[string]string, urlSuffix URLSuffix) *HopBaseModel {
	return &HopBaseModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *HopBaseModel) Name() string {
	return "hopbase"
}

// NewInstance returns a driver bound to the given base URL.
func (m *HopBaseModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewHopBaseModel(baseURL, m.baseModel.URLSuffix)
}

// ListModels lists the models the API key can call. The OpenAI-compatible
// listing infers types from model names, which reports vision models as
// chat-only, so models in the HopBase catalog keep the catalog's types.
func (m *HopBaseModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	models, err := m.OpenAIAPICompatibleModel.ListModels(ctx, apiConfig)
	if err != nil {
		return nil, err
	}
	pm := GetProviderManager()
	if pm == nil {
		return models, nil
	}
	provider := pm.FindProvider(hopBaseProviderName)
	if provider == nil {
		return models, nil
	}
	for i := range models {
		if entry := pm.FindModel(provider, models[i].Name); entry != nil && len(entry.ModelTypes) > 0 {
			models[i].ModelTypes = append([]string(nil), entry.ModelTypes...)
		}
	}
	return models, nil
}
