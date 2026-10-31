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
	"net/url"
	"ragflow/internal/common"
	"strings"
)

// UnlimitedOCRModel wraps MinerULocalModel for Baidu's Unlimited-OCR-2 engine.
// It speaks the high-performance /file_parse and /tasks result extraction protocol
// returning clean Markdown directly into RAGFlow's ingestion pipeline.
type UnlimitedOCRModel struct {
	*MinerULocalModel
}

func NewUnlimitedOCRModel(baseURL map[string]string, urlSuffix URLSuffix) *UnlimitedOCRModel {
	return &UnlimitedOCRModel{MinerULocalModel: NewMinerLocalUModel(baseURL, urlSuffix)}
}

func (m *UnlimitedOCRModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewUnlimitedOCRModel(baseURL, m.baseModel.URLSuffix)
}

func (m *UnlimitedOCRModel) Name() string {
	return "unlimited_ocr"
}

// ResolveUnlimitedOCRServerURL extracts the server address from apiConfig or environment variables.
func ResolveUnlimitedOCRServerURL(apiConfig *APIConfig) string {
	if apiConfig != nil && apiConfig.BaseURL != nil {
		if u := strings.TrimRight(strings.TrimSpace(*apiConfig.BaseURL), "/"); u != "" {
			return u
		}
	}
	if apiConfig != nil && apiConfig.ApiKey != nil {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(*apiConfig.ApiKey), &cfg); err == nil {
			if u, ok := cfg["unlimited_ocr_server_url"].(string); ok && strings.TrimSpace(u) != "" {
				return strings.TrimRight(strings.TrimSpace(u), "/")
			}
			if u, ok := cfg["unlimited_ocr_apiserver"].(string); ok && strings.TrimSpace(u) != "" {
				return strings.TrimRight(strings.TrimSpace(u), "/")
			}
		}
	}
	if u := strings.TrimSpace(common.GetEnv(common.EnvUnlimitedOCRServerURL)); u != "" {
		return strings.TrimRight(u, "/")
	}
	if u := strings.TrimSpace(common.GetEnv(common.EnvUnlimitedOCRAPIServer)); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://127.0.0.1:18000"
}

// GetBaseURL resolves the effective server endpoint for Unlimited-OCR-2.
func (m *UnlimitedOCRModel) GetBaseURL(apiConfig *APIConfig) (string, error) {
	if u := ResolveUnlimitedOCRServerURL(apiConfig); u != "" {
		return u, nil
	}
	return m.baseModel.GetBaseURL(apiConfig)
}

// CheckConnection probes /v1/health and /health to verify service reachability.
func (m *UnlimitedOCRModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	resolvedBaseURL, err := m.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resolvedBaseURL, "http://") && !strings.HasPrefix(resolvedBaseURL, "https://") {
		resolvedBaseURL = "http://" + resolvedBaseURL
	}
	u, err := url.Parse(resolvedBaseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	v1URL := *u
	v1URL.Path = "/v1/health"
	v1Code, v1Body, v1Err := minerUGET(ctx, m.baseModel.httpClient, v1URL.String(), "")
	if v1Err == nil && minerUHealthOK(v1Code, v1Body) {
		return nil
	}

	legacy := *u
	legacy.Path = "/health"
	code, body, err := minerUGET(ctx, m.baseModel.httpClient, legacy.String(), "")
	if err == nil && minerUHealthOK(code, body) {
		return nil
	}
	if err != nil {
		if v1Err != nil {
			return fmt.Errorf("connection failed: %w", v1Err)
		}
		return fmt.Errorf("connection failed: %w", err)
	}
	return fmt.Errorf("not an Unlimited-OCR-2 service: HTTP %d", code)
}

// OCRFile exercises CheckConnection for provider model verification.
func (m *UnlimitedOCRModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	if err := m.CheckConnection(ctx, apiConfig); err != nil {
		return nil, err
	}
	return &OCRFileResponse{}, nil
}

// ListModels returns the catalogued OCR models for this provider instance.
func (m *UnlimitedOCRModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := m.CheckConnection(ctx, apiConfig); err != nil {
		return nil, err
	}
	return []ListModelResponse{{
		Name:       "Unlimited-OCR-2",
		ModelTypes: []string{"ocr"},
	}}, nil
}
