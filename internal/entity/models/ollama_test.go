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
	"testing"
)

func newOllamaForListModelsTest(baseURL string) *OllamaModel {
	return NewOllamaModel(map[string]string{"default": baseURL}, URLSuffix{Models: "api/tags"})
}

func TestOllamaListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/api/tags" {
			t.Errorf("path=%s, want /api/tags", r.URL.Path)
		}
		// Ollama's /api/tags response shape (name + model fields).
		_, _ = io.WriteString(w, `{"models":[{"name":"llama3:latest","model":"llama3:latest"},{"name":"qwen3:8b","model":"qwen3:8b"}]}`)
	}))
	defer srv.Close()

	models, err := newOllamaForListModelsTest(srv.URL).ListModels(&APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models)=%d, want 2", len(models))
	}
	if models[0].Name != "llama3:latest" || models[1].Name != "qwen3:8b" {
		t.Fatalf("names=%v, want [llama3:latest qwen3:8b]", []string{models[0].Name, models[1].Name})
	}
}

func TestOllamaListModelsFallsBackToModelField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Some entries may carry only the "model" field; it should be used as the name.
		_, _ = io.WriteString(w, `{"models":[{"model":"phi3:mini"},{"name":""},{"name":"  "}]}`)
	}))
	defer srv.Close()

	models, err := newOllamaForListModelsTest(srv.URL).ListModels(&APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models)=%d, want 1 (blank names skipped)", len(models))
	}
	if models[0].Name != "phi3:mini" {
		t.Fatalf("Name=%q, want phi3:mini", models[0].Name)
	}
}

func TestOllamaListModelsRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `boom`)
	}))
	defer srv.Close()

	if _, err := newOllamaForListModelsTest(srv.URL).ListModels(&APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for HTTP 500, got nil")
	}
}

func TestOllamaListModelsRequiresBaseURL(t *testing.T) {
	m := NewOllamaModel(map[string]string{}, URLSuffix{Models: "api/tags"})
	if _, err := m.ListModels(&APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for missing base URL, got nil")
	}
}
