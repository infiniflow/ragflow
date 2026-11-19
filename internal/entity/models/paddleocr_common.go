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
	"bytes"
	"encoding/json"
	"strings"

	"ragflow/internal/common"
)

// paddleOCRDetectFileType determines the input file type code expected by
// the PaddleX layout parsing endpoint: 0 for PDF, 1 for images.
func paddleOCRDetectFileType(content []byte, fileURL *string) int {
	if bytes.HasPrefix(content, []byte("%PDF")) {
		return 0
	}
	if fileURL != nil && *fileURL != "" {
		clean := strings.ToLower(strings.Split(*fileURL, "?")[0])
		if strings.HasSuffix(clean, ".pdf") {
			return 0
		}
	}
	return 1
}

// paddleOCRConfigFromAPIKey parses the tenant PaddleOCR api_key payload:
// the api_key can be a JSON object carrying the server endpoint and access token,
// either with lower-case keys or upper-case env-style keys, optionally nested
// under an "api_key" key. A non-JSON api_key (such as the PaddleOCR.local plain
// bearer token) yields zero values and the caller falls back to plain-text semantics.
// base_url prefers paddleocr_base_url over paddleocr_api_url.
func paddleOCRConfigFromAPIKey(apiKey string) (baseURL, accessToken string) {
	if strings.TrimSpace(apiKey) == "" {
		return "", ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(apiKey), &raw); err != nil {
		return "", ""
	}
	if nested, ok := raw["api_key"].(map[string]interface{}); ok {
		raw = nested
	}
	get := func(key string) string {
		for _, k := range []string{key, strings.ToUpper(key)} {
			if value, _ := raw[k].(string); strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		return ""
	}
	baseURL = get("paddleocr_base_url")
	if baseURL == "" {
		baseURL = get("paddleocr_api_url")
	}
	return baseURL, get("paddleocr_access_token")
}

// paddleOCRResolvedAPIConfig resolves the wire config for the PaddleOCR
// drivers: the bearer token comes from the api_key JSON payload when present,
// and the base url falls back from the instance field through the payload to
// the PADDLEOCR_BASE_URL / PADDLEOCR_API_URL env vars. A plain-text api_key
// passes through untouched; an empty base url is left for the driver's catalog
// default.
func paddleOCRResolvedAPIConfig(apiConfig *APIConfig) *APIConfig {
	if apiConfig == nil {
		return nil
	}
	apiKey := ""
	if apiConfig.ApiKey != nil {
		apiKey = *apiConfig.ApiKey
	}
	baseURL := ""
	if apiConfig.BaseURL != nil {
		baseURL = strings.TrimSpace(*apiConfig.BaseURL)
	}
	keyBaseURL, keyAccessToken := paddleOCRConfigFromAPIKey(apiKey)
	if keyAccessToken != "" {
		apiKey = keyAccessToken
	}
	if baseURL == "" {
		baseURL = keyBaseURL
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRBaseUrl))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(common.GetEnv(common.EnvPaddleOCRAPIURL))
	}
	return &APIConfig{ApiKey: &apiKey, Region: apiConfig.Region, BaseURL: &baseURL}
}
