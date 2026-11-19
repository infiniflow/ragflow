// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// The public NovaCode directory declares vision and function calling for these
// exact IDs (https://ai.novacode.top/api/v1/public/models, 2026-11-18).
// Bare model IDs from /models must retain those declared abilities;
// a future, unknown GPT name must not inherit them just from its prefix.
func TestNovaCodeCatalogCapabilities(t *testing.T) {
	withSSRFBypass(t)
	dir, restore := setupProviderTestDir(t, "novacode.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatal(err)
	}
	manager := GetProviderManager()
	provider := manager.FindProvider("NovaCode")
	if provider == nil {
		t.Fatal("NovaCode provider not found")
	}
	modelIDs := []string{
		"codex-auto-review", "gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra",
		"gpt-6-astra", "gpt-6-luna", "gpt-6-sol", "gpt-6.1-sol", "gpt-6.1-terra",
		"gpt-6.2-astra", "gpt-6.2-sol",
	}
	for _, id := range modelIDs {
		t.Run(id, func(t *testing.T) {
			model := manager.FindModel(provider, id)
			if model == nil || model.Tools == nil || !model.Tools.Support {
				t.Fatal("documented tool capability missing")
			}
			list := listNovaCodeCatalogModel(t, id)
			if len(list) != 1 || !slices.Contains(list[0].ModelTypes, "chat") || !slices.Contains(list[0].ModelTypes, "vision") {
				t.Fatalf("declared chat and vision capabilities lost: %+v", list)
			}
		})
	}
	unknown := listNovaCodeCatalogModel(t, "gpt-99-unknown")
	if len(unknown) != 1 || slices.Contains(unknown[0].ModelTypes, "vision") {
		t.Fatalf("unknown model inherited unsupported vision capability: %+v", unknown)
	}
}

func listNovaCodeCatalogModel(t *testing.T, id string) []ListModelResponse {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": id}}})
	}))
	defer server.Close()
	key := "test-key"
	list, err := NewNovaCodeModel(map[string]string{"default": server.URL}, URLSuffix{}).ListModels(t.Context(), &APIConfig{ApiKey: &key})
	if err != nil {
		t.Fatal(err)
	}
	return list
}
