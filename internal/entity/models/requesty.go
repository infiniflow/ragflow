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
	"net/http"
	"strings"
	"time"
	"unicode"
)

// requestyListModelsTimeout bounds each model-list request so a slow catalog
// endpoint does not hold the provider settings page for the full chat timeout.
const requestyListModelsTimeout = 15 * time.Second

// requestyMaxListBody caps the size of a model-list response body.
const requestyMaxListBody = 16 << 20

// RequestyModel implements Requesty's OpenAI-compatible chat and model-list
// APIs. Chat goes through the embedded OpenAIAPICompatibleModel unchanged.
//
// ListModels is overridden because Requesty reports its catalog with its own
// field names (context_window, max_output_tokens, supports_vision) and exposes
// a curated list of managed models at models/managed next to the full catalog
// at models.
type RequestyModel struct {
	*OpenAIAPICompatibleModel
}

// NewRequestyModel creates a Requesty model driver.
func NewRequestyModel(baseURL map[string]string, urlSuffix URLSuffix) *RequestyModel {
	return &RequestyModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

// Name returns the model driver name.
func (m *RequestyModel) Name() string {
	return "Requesty"
}

// NewInstance creates a new Requesty driver bound to the given base URL.
func (m *RequestyModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewRequestyModel(baseURL, m.baseModel.URLSuffix)
}

type requestyModelItem struct {
	ID              string `json:"id"`
	API             string `json:"api"`
	ContextWindow   *int   `json:"context_window"`
	MaxOutputTokens *int   `json:"max_output_tokens"`
	SupportsVision  bool   `json:"supports_vision"`
}

type requestyModelList struct {
	Data []requestyModelItem `json:"data"`
}

// ListModels returns Requesty's managed models first, followed by the rest of
// the chat catalog. The two lists are fetched independently: if one request
// fails the other is still returned. The API key is optional here because
// Requesty serves the public catalog without one.
func (m *RequestyModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	resolvedBaseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	modelsURL := fmt.Sprintf("%s/%s", resolvedBaseURL, m.baseModel.URLSuffix.Models)

	managed, managedErr := m.fetchRequestyModels(ctx, modelsURL+"/managed", apiConfig)
	catalog, catalogErr := m.fetchRequestyModels(ctx, modelsURL, apiConfig)
	if managedErr != nil && catalogErr != nil {
		return nil, fmt.Errorf("list requesty models: %w", catalogErr)
	}

	seen := make(map[string]struct{}, len(managed)+len(catalog))
	models := make([]ListModelResponse, 0, len(managed)+len(catalog))
	for _, item := range append(managed, catalog...) {
		name := strings.TrimSpace(item.ID)
		if !isValidRequestyModelID(name) {
			continue
		}
		if item.API != "" && item.API != modelTypeChat {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}

		modelTypes := []string{modelTypeChat}
		if item.SupportsVision {
			modelTypes = append(modelTypes, modelTypeVision)
		}
		model := ListModelResponse{Name: name, ModelTypes: modelTypes}
		if item.ContextWindow != nil && *item.ContextWindow > 0 {
			model.ContextLength = item.ContextWindow
		}
		if item.MaxOutputTokens != nil && *item.MaxOutputTokens > 0 {
			model.MaxOutput = item.MaxOutputTokens
		}
		models = append(models, model)
	}
	return models, nil
}

func (m *RequestyModel) fetchRequestyModels(ctx context.Context, url string, apiConfig *APIConfig) ([]requestyModelItem, error) {
	ctx, cancel := context.WithTimeout(ctx, requestyListModelsTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if auth := BearerAuth(apiConfig); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, requestyMaxListBody))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var list requestyModelList
	if err = json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return list.Data, nil
}

// isValidRequestyModelID rejects empty IDs and IDs carrying control characters
// (including ANSI escape sequences), since model names are rendered in the UI.
func isValidRequestyModelID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
