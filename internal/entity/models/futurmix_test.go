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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newFuturMixForListModelsTest(baseURL string) *FuturMixModel {
	return NewFuturMixModel(map[string]string{"default": baseURL}, URLSuffix{Models: "v1/models"})
}

func TestFuturMixListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-5.5"},{"id":"claude-fable-5"}]}`)
	}))
	defer srv.Close()

	apiKey := "test-key"
	models, err := newFuturMixForListModelsTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models)=%d, want 2", len(models))
	}
	if models[0].Name != "gpt-5.5" || models[1].Name != "claude-fable-5" {
		t.Fatalf("names=%v, want [gpt-5.5 claude-fable-5]", []string{models[0].Name, models[1].Name})
	}
}

func TestFuturMixListModelsRequiresAPIKey(t *testing.T) {
	if _, err := newFuturMixForListModelsTest("http://unused").ListModels(&APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for missing api key, got nil")
	}
}

func TestFuturMixListModelsRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"forbidden"}`)
	}))
	defer srv.Close()

	apiKey := "bad-key"
	_, err := newFuturMixForListModelsTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("ListModels: expected 403 error, got %v", err)
	}
}

// TestFuturMixListModelsIntegration calls the real FuturMix models endpoint.
// Gated behind an env var so CI and offline runs skip it.
func TestFuturMixListModelsIntegration(t *testing.T) {
	if os.Getenv("FUTURMIX_LIST_MODELS_INTEGRATION") != "1" {
		t.Skip("set FUTURMIX_LIST_MODELS_INTEGRATION=1 to call the real FuturMix models endpoint")
	}
	apiKey := os.Getenv("FUTURMIX_API_KEY")
	if apiKey == "" {
		t.Skip("set FUTURMIX_API_KEY to run the FuturMix integration test")
	}
	m := NewFuturMixModel(map[string]string{"default": "https://futurmix.ai"}, URLSuffix{Models: "v1/models"})
	models, err := m.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("real FuturMix ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("real FuturMix ListModels returned no models")
	}
}
