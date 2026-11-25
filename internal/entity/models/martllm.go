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

const martLLMProviderName = "MartLLM"

// MartLLMModel drives MartLLM (https://api.martllm.com/v1), which serves
// many model families through OpenAI-compatible chat and model-list endpoints,
// so the embedded OpenAIAPICompatibleModel covers every call.
type MartLLMModel struct {
	*OpenAIAPICompatibleModel
}

// NewMartLLMModel creates a MartLLM driver.
func NewMartLLMModel(baseURL map[string]string, urlSuffix URLSuffix) *MartLLMModel {
	return &MartLLMModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *MartLLMModel) Name() string {
	return "martllm"
}

// NewInstance returns a driver bound to the given base URL.
func (m *MartLLMModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMartLLMModel(baseURL, m.baseModel.URLSuffix)
}

// martLLMModelList is the part of MartLLM's GET /models response the driver
// reads. Each entry names its endpoints and input modalities.
type martLLMModelList struct {
	Models []martLLMModelListItem `json:"data"`
}

type martLLMModelListItem struct {
	ID              string   `json:"id"`
	ContextLength   *int     `json:"context_length"`
	MaxOutputTokens *int     `json:"max_output_tokens"`
	Endpoints       []string `json:"endpoints"`
	InputModalities []string `json:"input_modalities"`
}

// ListModels lists MartLLM's models. A model whose endpoints include
// chat/completions is a chat model, and a vision model too when its
// input_modalities include "image", so models outside the shipped catalogue
// get accurate types. A model that names no endpoints falls back to
// name-inferred types. Models in the MartLLM catalogue keep the catalogue's
// types.
func (m *MartLLMModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	modelsSuffix := strings.Trim(strings.TrimSpace(m.baseModel.URLSuffix.Models), "/")
	if modelsSuffix == "" {
		return nil, fmt.Errorf("martllm: models URL suffix is not configured")
	}
	url, err := m.baseModel.EndpointURL(apiConfig, modelsSuffix)
	if err != nil {
		return nil, err
	}
	body, err := m.baseModel.doGetRequest(ctx, url, apiConfig, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}
	var modelList martLLMModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if modelList.Models == nil {
		return nil, fmt.Errorf("invalid models list format")
	}

	var provider *Provider
	pm := GetProviderManager()
	if pm != nil {
		provider = pm.FindProvider(martLLMProviderName)
	}

	models := make([]ListModelResponse, 0, len(modelList.Models))
	for _, item := range modelList.Models {
		name := strings.TrimSpace(item.ID)
		if name == "" {
			continue
		}
		model := ListModelResponse{Name: name, ModelTypes: martLLMModelTypes(item)}
		if provider != nil {
			if entry := pm.FindModel(provider, name); entry != nil && len(entry.ModelTypes) > 0 {
				model.ModelTypes = append([]string(nil), entry.ModelTypes...)
			}
		}
		if len(model.ModelTypes) == 0 {
			continue
		}
		if item.ContextLength != nil && *item.ContextLength > 0 {
			model.ContextLength = item.ContextLength
		}
		if item.MaxOutputTokens != nil && *item.MaxOutputTokens > 0 {
			model.MaxOutput = item.MaxOutputTokens
		}
		models = append(models, model)
	}
	return models, nil
}

// martLLMModelTypes derives a listed model's types from its endpoints and
// input modalities. A model that names no endpoints gets name-inferred types,
// and one whose endpoints do not include chat/completions gets none.
func martLLMModelTypes(item martLLMModelListItem) []string {
	if len(item.Endpoints) == 0 {
		return InferModelTypes(item.ID)
	}
	chat := false
	for _, endpoint := range item.Endpoints {
		if strings.HasSuffix(strings.TrimRight(endpoint, "/"), "chat/completions") {
			chat = true
			break
		}
	}
	if !chat {
		return nil
	}
	for _, modality := range item.InputModalities {
		if strings.EqualFold(modality, "image") {
			return []string{modelTypeChat, modelTypeVision}
		}
	}
	return []string{modelTypeChat}
}
