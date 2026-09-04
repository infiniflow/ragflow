// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
package models

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ragflow/internal/common"
)

// MonkeyOCRv2Model reuses the local document-parser model surface while
// identifying the provider independently from MinerU. The ingestion dispatch
// owns MonkeyOCRv2's native synchronous ZIP protocol.
type MonkeyOCRv2Model struct {
	*MinerULocalModel
}

func NewMonkeyOCRv2Model(baseURL map[string]string, urlSuffix URLSuffix) *MonkeyOCRv2Model {
	return &MonkeyOCRv2Model{MinerULocalModel: NewMinerLocalUModel(baseURL, urlSuffix)}
}

func (m *MonkeyOCRv2Model) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMonkeyOCRv2Model(baseURL, m.baseModel.URLSuffix)
}

func (m *MonkeyOCRv2Model) Name() string {
	return "monkeyocrv2"
}

// OCRFile performs the lightweight capability check expected by the generic
// OCR-provider verification path. Document parsing itself is owned by the
// ingestion dispatch because MonkeyOCRv2 returns a ZIP archive.
func (m *MonkeyOCRv2Model) OCRFile(ctx context.Context, _ *string, _ []byte, _ *string, apiConfig *APIConfig, _ *OCRConfig, _ *common.ModelUsage) (*OCRFileResponse, error) {
	if err := m.CheckConnection(ctx, apiConfig); err != nil {
		return nil, err
	}
	return &OCRFileResponse{}, nil
}

func (m *MonkeyOCRv2Model) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	baseURL, err := m.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		baseURL = monkeyOCRv2BaseURLFromAPIKey(apiConfig)
		if baseURL == "" {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/openapi.json", nil)
	if err != nil {
		return err
	}
	resp, err := m.baseModel.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("MonkeyOCRv2 connection failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MonkeyOCRv2 OpenAPI returned HTTP %d", resp.StatusCode)
	}
	var spec struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		return fmt.Errorf("invalid MonkeyOCRv2 OpenAPI response: %w", err)
	}
	if _, ok := spec.Paths["/parse"]; !ok {
		return fmt.Errorf("MonkeyOCRv2 server does not expose /parse")
	}
	return nil
}

func monkeyOCRv2BaseURLFromAPIKey(apiConfig *APIConfig) string {
	if apiConfig == nil || apiConfig.ApiKey == nil || strings.TrimSpace(*apiConfig.ApiKey) == "" {
		return ""
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(*apiConfig.ApiKey), &config); err != nil {
		return ""
	}
	if nested, ok := config["api_key"].(map[string]any); ok {
		config = nested
	}
	for _, key := range []string{"monkeyocrv2_server_url", common.EnvMonkeyOCRv2ServerURL} {
		if value, ok := config[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimRight(strings.TrimSpace(value), "/")
		}
	}
	return ""
}
