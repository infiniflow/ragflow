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
	"strings"
)

var worldRouterNonChatModelPrefixes = []string{
	"gpt-image-",
	"kling-",
	"pixverse-",
	"seedance-",
}

// WorldRouterModel drives WorldRouter's OpenAI-compatible inference gateway.
// The gateway also exposes image and video models through /models, but RAGFlow
// cannot call those task APIs through its chat model interface, so ListModels
// only returns models that can use /chat/completions.
type WorldRouterModel struct {
	*OpenAIAPICompatibleModel
}

// NewWorldRouterModel creates a WorldRouter driver.
func NewWorldRouterModel(baseURL map[string]string, urlSuffix URLSuffix) *WorldRouterModel {
	return &WorldRouterModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the driver name.
func (m *WorldRouterModel) Name() string {
	return "worldrouter"
}

// NewInstance returns a driver bound to the given base URL.
func (m *WorldRouterModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewWorldRouterModel(baseURL, m.baseModel.URLSuffix)
}

// ListModels discovers the models available to the tenant's WorldRouter key.
func (m *WorldRouterModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	models, err := m.OpenAIAPICompatibleModel.ListModels(ctx, apiConfig)
	if err != nil {
		return nil, err
	}

	filtered := make([]ListModelResponse, 0, len(models))
	for _, model := range models {
		if worldRouterSupportsChatModel(model) {
			filtered = append(filtered, model)
		}
	}
	return filtered, nil
}

func worldRouterSupportsChatModel(model ListModelResponse) bool {
	name := strings.ToLower(strings.TrimSpace(model.Name))
	if name == "" || name == "mock-test" {
		return false
	}
	for _, prefix := range worldRouterNonChatModelPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	for _, modelType := range model.ModelTypes {
		if modelType == modelTypeChat {
			return true
		}
	}
	return false
}
