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
	"net/url"
	"strings"
	"time"
)

// NovaCode's public directory declares capabilities separately from the
// authenticated model list. Join it by ID; it must never add accessible models.
type novaCodePublicDirectory struct {
	Code int `json:"code"`
	Data struct {
		Groups []struct {
			Models []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Metadata struct {
					Mode           string `json:"mode"`
					SupportsVision *bool  `json:"supports_vision"`
				} `json:"metadata"`
			} `json:"models"`
		} `json:"groups"`
	} `json:"data"`
}

func (m *NovaCodeModel) fetchCapabilities(ctx context.Context, api *APIConfig) map[string][]string {
	base, err := m.baseModel.GetBaseURL(api)
	if err != nil {
		return nil
	}
	endpoint, err := url.Parse(base)
	if err != nil {
		return nil
	}
	endpoint.Path = "/api/v1/public/models"
	endpoint.RawPath, endpoint.RawQuery, endpoint.Fragment = "", "", ""
	endpoint.User = nil
	// This endpoint is public. Do not send the instance's API key, and keep
	// enrichment bounded so a directory outage cannot block model discovery.
	body, err := m.baseModel.doGetRequest(ctx, endpoint.String(), nil, 5*time.Second)
	if err != nil {
		return nil
	}
	var directory novaCodePublicDirectory
	if err := json.Unmarshal(body, &directory); err != nil || directory.Code != 0 {
		return nil
	}
	capabilities := make(map[string][]string)
	for _, group := range directory.Data.Groups {
		for _, model := range group.Models {
			key := strings.ToLower(strings.TrimSpace(model.ID))
			if key == "" {
				continue
			}
			var types []string
			switch {
			case model.Type == "image":
				types = []string{} // The driver has no image-generation endpoint.
			case model.Metadata.Mode == "chat" && model.Metadata.SupportsVision != nil:
				types = []string{modelTypeChat}
				if *model.Metadata.SupportsVision {
					types = append(types, modelTypeVision)
				}
			default:
				continue
			}
			if previous, exists := capabilities[key]; exists {
				// A key's group is not present in /v1/models. Advertise only
				// capabilities shared by every explicit declaration for this ID.
				shared := make([]string, 0, len(previous))
				for _, kind := range previous {
					if containsModelType(types, kind) {
						shared = append(shared, kind)
					}
				}
				types = shared
			}
			capabilities[key] = types
		}
	}
	return capabilities
}

func novaCodeCatalogModel(name string) *Model {
	manager := GetProviderManager()
	if manager == nil {
		return nil
	}
	provider := manager.FindProvider("NovaCode")
	if provider == nil {
		return nil
	}
	for _, model := range provider.Models {
		if strings.EqualFold(model.Name, name) {
			return model
		}
		for _, alias := range model.Alias {
			if strings.EqualFold(alias, name) {
				return model
			}
		}
	}
	return nil
}

func parseNovaCodeModels(list ModelList, capabilities map[string][]string) []ListModelResponse {
	models := make([]ListModelResponse, 0, len(list.Models))
	for _, item := range list.Models {
		name := strings.TrimSpace(item.ID)
		key := strings.ToLower(name)
		if name == "" || strings.Contains(key, "gpt-image") || strings.Contains(key, "dall-e") {
			continue
		}
		model := ListModelResponse{Name: name}
		if catalog := novaCodeCatalogModel(name); catalog != nil {
			model.ModelTypes = append([]string(nil), catalog.ModelTypes...)
			model.ContextLength = catalog.ContextLength
			model.MaxOutput = catalog.MaxOutput
			model.Thinking = catalog.Thinking
		}
		if declared, exists := capabilities[key]; exists {
			if len(declared) == 0 {
				continue
			}
			model.ModelTypes = append([]string(nil), declared...)
		}
		if len(model.ModelTypes) == 0 {
			model.ModelTypes = InferModelTypes(name)
			model.ModelTypesInferred = true
		}
		supported := make([]string, 0, len(model.ModelTypes))
		for _, kind := range model.ModelTypes {
			if kind == modelTypeChat || kind == modelTypeVision {
				supported = append(supported, kind)
			}
		}
		if len(supported) == 0 {
			continue
		}
		model.ModelTypes = supported
		if item.ContextLength != nil && *item.ContextLength > 0 {
			model.ContextLength = item.ContextLength
		} else if item.ContextWindow != nil && *item.ContextWindow > 0 {
			model.ContextLength = item.ContextWindow
		}
		if item.MaxTokens != nil && *item.MaxTokens > 0 {
			model.MaxOutput = item.MaxTokens
		}
		models = append(models, model)
	}
	return models
}
