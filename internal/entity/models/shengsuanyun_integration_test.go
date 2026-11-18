//go:build integration

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

package models

import (
	"context"
	"os"
	"testing"
)

func TestShengsuanyunListModelsIntegration(t *testing.T) {
	withSSRFBypass(t)
	if os.Getenv("SHENGSUANYUN_LIST_MODELS_INTEGRATION") != "1" {
		t.Skip("set SHENGSUANYUN_LIST_MODELS_INTEGRATION=1 to call Shengsuanyun's live models endpoint")
	}

	baseURL := "https://router.shengsuanyun.com/api/v1"
	driver, err := NewModelFactory().CreateModelDriver(
		"shengsuanyun",
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}

	apiConfig := &APIConfig{}
	if apiKey := os.Getenv("SHENGSUANYUN_API_KEY"); apiKey != "" {
		apiConfig.ApiKey = &apiKey
	}
	listed, err := driver.ListModels(context.Background(), apiConfig)
	if err != nil {
		t.Fatalf("live Shengsuanyun ListModels: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("live Shengsuanyun ListModels returned no RAGFlow-compatible models")
	}
	for _, model := range listed {
		if model.Name == "" || len(model.ModelTypes) == 0 {
			t.Fatalf("live Shengsuanyun model lacks a name or inferred RAGFlow model type: %#v", model)
		}
	}
	t.Logf("Shengsuanyun provider returned %d RAGFlow-compatible models; first model: %s (%v)", len(listed), listed[0].Name, listed[0].ModelTypes)
}
