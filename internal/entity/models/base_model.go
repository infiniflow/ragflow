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
	"fmt"
	"net/http"
	"strings"
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
