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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type BaseModel struct {
	BaseURL          map[string]string
	URLSuffix        URLSuffix
	httpClient       *http.Client
	AllowEmptyAPIKey bool
}

func (b *BaseModel) APIConfigCheck(apiConfig *APIConfig) error {
	if b.AllowEmptyAPIKey {
		return nil
	}

	if apiConfig == nil || apiConfig.ApiKey == nil || strings.TrimSpace(*apiConfig.ApiKey) == "" {
		return fmt.Errorf("api key is required")
	}

	return nil
}

// BearerAuth returns the Bearer token for Authorization header,
// or empty string if apiConfig or its ApiKey is nil/empty.
func BearerAuth(apiConfig *APIConfig) string {
	if apiConfig == nil || apiConfig.ApiKey == nil {
		return ""
	}
	key := strings.TrimSpace(*apiConfig.ApiKey)
	if key == "" {
		return ""
	}
	return fmt.Sprintf("Bearer %s", key)
}

func (b *BaseModel) GetBaseURL(apiConfig *APIConfig) (string, error) {
	if apiConfig != nil && apiConfig.BaseURL != nil && *apiConfig.BaseURL != "" {
		return strings.TrimSuffix(*apiConfig.BaseURL, "/"), nil
	}

	region := "default"
	hasRegion := false
	if apiConfig != nil && apiConfig.Region != nil {
		hasRegion = true
		region = *apiConfig.Region
	}

	baseURL, ok := b.BaseURL[region]
	if !ok || baseURL == "" {
		if (!hasRegion || region == "") && b.BaseURL != nil {
			if defaultBaseURL, ok := b.BaseURL["default"]; ok && defaultBaseURL != "" {
				return defaultBaseURL, nil
			}
		}
		return "", fmt.Errorf("no base URL configured for region %q", region)
	}

	return baseURL, nil
}

// ParseSSEStream reads the body of an OpenAI-compatible Server-Sent Events
// response and calls onEvent for each successfully-parsed JSON payload.
func ParseSSEStream[T any](r io.Reader, onEvent func(event T) error) (done bool, err error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "[DONE]" {
			return true, nil
		}
		var event T
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if err := onEvent(event); err != nil {
			return false, err
		}
	}
	return false, scanner.Err()
}

// ParseListModel Parse model list
func ParseListModel(modelList ModelList) []ListModelResponse {
	var models []ListModelResponse
	pm := GetProviderManager()
	for _, model := range modelList.Models {
		modelName := model.ID
		var modelResponse ListModelResponse
		var modelEntity *Model
		if pm != nil {
			modelEntity = pm.GetModelByNameOrAlias(modelName)
		}
		if model.OwnedBy != "" {
			modelName = model.ID + "@" + model.OwnedBy
		}
		modelResponse.Name = modelName
		if modelEntity != nil {
			modelResponse.MaxDimension = modelEntity.MaxDimension
			modelResponse.Dimensions = modelEntity.Dimensions
			modelResponse.MaxTokens = modelEntity.MaxTokens
			modelResponse.ModelTypes = modelEntity.ModelTypes
			modelResponse.Thinking = modelEntity.Thinking
			modelResponse.Dimensions = modelEntity.Dimensions
		}

		models = append(models, modelResponse)
	}
	return models
}

// NewDriverHTTPClient returns an *http.Client with the standard connection-pool
func NewDriverHTTPClient() *http.Client {
	var t *http.Transport
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		t = dt.Clone()
	} else {
		t = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	t.MaxIdleConns = 100
	t.MaxIdleConnsPerHost = 10
	t.IdleConnTimeout = 90 * time.Second
	t.DisableCompression = false
	t.ResponseHeaderTimeout = 60 * time.Second
	return &http.Client{Transport: t}
}

// PostJSONRequest marshals body to JSON, creates a POST request to url
func PostJSONRequest(ctx context.Context, client *http.Client, url, auth string, body map[string]interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return client.Do(req)
}

// ReadErrorBody reads all bytes from r and returns them as a string suitable
func ReadErrorBody(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}
