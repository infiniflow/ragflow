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

func newFishAudioForListModelsTest(baseURL string) *FishAudioModel {
	return NewFishAudioModel(map[string]string{"default": baseURL}, URLSuffix{Models: "model"})
}

func TestFishAudioListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/model" {
			t.Errorf("path=%s, want /model", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		// Fish Audio voice catalog: each item has _id and a human-readable title.
		_, _ = io.WriteString(w, `{"items":[{"_id":"abc123","title":"Energetic Male"},{"_id":"def456","title":"Calm Female"},{"_id":"ghi789","title":""}]}`)
	}))
	defer srv.Close()

	apiKey := "test-key"
	models, err := newFishAudioForListModelsTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	// Blank title falls back to _id; all three are returned.
	if len(models) != 3 {
		t.Fatalf("len(models)=%d, want 3", len(models))
	}
	if models[0].Name != "Energetic Male" || models[1].Name != "Calm Female" {
		t.Fatalf("titles=%v, want [Energetic Male, Calm Female]", []string{models[0].Name, models[1].Name})
	}
	if models[2].Name != "ghi789" {
		t.Fatalf("models[2].Name=%q, want fallback to _id ghi789", models[2].Name)
	}
}

func TestFishAudioListModelsRequiresAPIKey(t *testing.T) {
	if _, err := newFishAudioForListModelsTest("http://unused").ListModels(&APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for missing api key, got nil")
	}
}

func TestFishAudioListModelsRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"unauthorized"}`)
	}))
	defer srv.Close()

	apiKey := "bad-key"
	if _, err := newFishAudioForListModelsTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey}); err == nil {
		t.Fatal("ListModels: expected error for HTTP 401, got nil")
	}
}
