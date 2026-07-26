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
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSiliconflowToolCalls(t *testing.T) {
	newDriver := func(baseURL string) ModelDriver {
		return NewSiliconflowModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
	}
	t.Run("non-streaming", func(t *testing.T) {
		testNonStreamingToolCall(t, "Pro/deepseek-ai/DeepSeek-V4-Pro", "/chat/completions", newDriver)
	})
	t.Run("streaming", func(t *testing.T) {
		testStreamingToolCall(t, "Pro/deepseek-ai/DeepSeek-V4-Pro", "/chat/completions", newDriver)
	})
}

func TestSiliconflowChatRejectsMissingContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{}}]}`))
	}))
	defer server.Close()

	apiKey := "test-key"
	model := NewSiliconflowModel(map[string]string{"default": server.URL}, URLSuffix{Chat: "chat/completions"})
	ctx := t.Context()
	if _, err := model.ChatWithMessages(ctx, "model", []Message{{Role: "user", Content: "hi"}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil {
		t.Fatal("expected missing content error")
	}
}
