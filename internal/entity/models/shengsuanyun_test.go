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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShengsuanyunListModelsSendsAttributionHeader(t *testing.T) {
	withSSRFBypass(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("request = %s %s, want GET /models", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Title"); got != "ragflow" {
			t.Errorf("X-Title = %q, want ragflow", got)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"gpt-4o-mini","owned_by":"shengsuanyun"}]}`)
	}))
	defer server.Close()

	driver, err := NewModelFactory().CreateModelDriver(
		"shengsuanyun",
		map[string]string{"default": server.URL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	listed, err := driver.ListModels(context.Background(), &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "gpt-4o-mini" {
		t.Fatalf("ListModels = %#v, want one gpt-4o-mini model", listed)
	}
}
