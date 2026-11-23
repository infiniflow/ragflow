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

package service

import (
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

func TestBuildChatConfigPenaltiesReachModel(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	for _, tc := range []struct {
		name      string
		saved     entity.JSONMap
		request   map[string]interface{}
		presence  *float64
		frequency *float64
	}{
		{name: "unset omitted"},
		{name: "saved settings", saved: entity.JSONMap{"presence_penalty": 0.1, "frequency_penalty": 0.2}, presence: new(0.1), frequency: new(0.2)},
		{name: "request settings", request: map[string]interface{}{"presence_penalty": -0.5, "frequency_penalty": 0.75}, presence: new(-0.5), frequency: new(0.75)},
		{name: "request overrides saved", saved: entity.JSONMap{"presence_penalty": 0.1, "frequency_penalty": 0.2}, request: map[string]interface{}{"presence_penalty": 0.5, "frequency_penalty": -0.75}, presence: new(0.5), frequency: new(-0.75)},
		{name: "partial request inherits frequency", saved: entity.JSONMap{"presence_penalty": 0.1, "frequency_penalty": 0.2}, request: map[string]interface{}{"presence_penalty": 0.5}, presence: new(0.5), frequency: new(0.2)},
		{name: "partial request inherits presence", saved: entity.JSONMap{"presence_penalty": 0.1, "frequency_penalty": 0.2}, request: map[string]interface{}{"frequency_penalty": 0.5}, presence: new(0.1), frequency: new(0.5)},
		{name: "zero overrides saved", saved: entity.JSONMap{"presence_penalty": 0.1, "frequency_penalty": 0.2}, request: map[string]interface{}{"presence_penalty": 0, "frequency_penalty": 0.0}, presence: new(0.0), frequency: new(0.0)},
		{name: "saved zero preserved", saved: entity.JSONMap{"presence_penalty": 0, "frequency_penalty": 0.0}, presence: new(0.0), frequency: new(0.0)},
	} {
		for _, stream := range []bool{false, true} {
			name := tc.name + "/nonstream"
			if stream {
				name = tc.name + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				requestBody := make(chan map[string]interface{}, 1)
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode request: %v", err)
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					requestBody <- body
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
				}))
				defer srv.Close()

				savedBefore, requestBefore := maps.Clone(tc.saved), maps.Clone(tc.request)
				cfg := BuildChatConfig(&entity.Chat{LLMSetting: tc.saved}, tc.request)
				modelName, apiKey := "test-model", "test-key"
				model := modelModule.NewChatModel(
					modelModule.NewOpenAIModel(map[string]string{"default": srv.URL}, modelModule.URLSuffix{Chat: "chat/completions"}),
					&modelName, &modelModule.APIConfig{ApiKey: &apiKey},
				)
				messages := []modelModule.Message{{Role: "user", Content: "hello"}}
				if stream {
					if err := model.ChatStreamlyWithSender(t.Context(), messages, cfg, nil, func(*string, *string) error { return nil }); err != nil {
						t.Fatal(err)
					}
				} else if _, err := model.ChatWithMessages(t.Context(), messages, cfg, nil); err != nil {
					t.Fatal(err)
				}
				body := <-requestBody
				if body["stream"] != stream {
					t.Errorf("stream=%v, want %v", body["stream"], stream)
				}
				for key, want := range map[string]*float64{"presence_penalty": tc.presence, "frequency_penalty": tc.frequency} {
					got, exists := body[key]
					if want == nil {
						if exists {
							t.Errorf("%s=%v, want omitted", key, got)
						}
					} else if !exists || got != *want {
						t.Errorf("%s=%v, want %v", key, got, *want)
					}
				}
				if !reflect.DeepEqual(tc.saved, savedBefore) || !reflect.DeepEqual(tc.request, requestBefore) {
					t.Fatal("building the model request mutated the input settings")
				}
			})
		}
	}
}
