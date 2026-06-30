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

func newBaichuanForListModelsTest(baseURL string) *BaichuanModel {
	return NewBaichuanModel(map[string]string{"default": baseURL}, URLSuffix{Models: "models"})
}

func TestBaichuanListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type=%q, want application/json", got)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"Baichuan4-Turbo","owned_by":"baichuan"},{"id":"Baichuan4-Air"}]}`)
	}))
	defer srv.Close()

	apiKey := "test-key"
	models, err := newBaichuanForListModelsTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models)=%d, want 2", len(models))
	}
	// owned_by is appended to the model id as "<id>@<owned_by>".
	if models[0].Name != "Baichuan4-Turbo@baichuan" {
		t.Fatalf("models[0].Name=%q, want Baichuan4-Turbo@baichuan", models[0].Name)
	}
	if models[1].Name != "Baichuan4-Air" {
		t.Fatalf("models[1].Name=%q, want Baichuan4-Air", models[1].Name)
	}
}

func TestBaichuanListModelsRequiresAPIKey(t *testing.T) {
	if _, err := newBaichuanForListModelsTest("http://unused").ListModels(&APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for missing api key, got nil")
	}
}

func TestBaichuanListModelsRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()

	apiKey := "bad-key"
	_, err := newBaichuanForListModelsTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("ListModels: expected 401 error, got %v", err)
	}
}

// TestBaichuanListModelsIntegration calls the real Baichuan models endpoint.
// Gated behind an env var so CI and offline runs skip it.
func TestBaichuanListModelsIntegration(t *testing.T) {
	if os.Getenv("BAICHUAN_LIST_MODELS_INTEGRATION") != "1" {
		t.Skip("set BAICHUAN_LIST_MODELS_INTEGRATION=1 to call the real Baichuan models endpoint")
	}
	apiKey := os.Getenv("BAICHUAN_API_KEY")
	if apiKey == "" {
		t.Skip("set BAICHUAN_API_KEY to run the Baichuan integration test")
	}
	m := NewBaichuanModel(map[string]string{"default": "https://api.baichuan-ai.com/v1"}, URLSuffix{Models: "models"})
	models, err := m.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("real Baichuan ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("real Baichuan ListModels returned no models")
	}
}
