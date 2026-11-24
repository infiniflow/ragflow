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

package component

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/entity/models"
)

func TestNewChatModelDriverPreservesProviderChatSuffix(t *testing.T) {
	setupAllowAnyHost(t, true)
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	driver, err := newChatModelDriver("Tongyi-Qianwen", server.URL)
	if err != nil {
		t.Fatalf("newChatModelDriver: %v", err)
	}
	apiKey := "test-key"
	modelName := "qwen-flash"
	chatModel := models.NewEinoChatModel(
		models.NewChatModel(driver, &modelName, &models.APIConfig{ApiKey: &apiKey}),
		nil,
	)
	response, err := chatModel.Generate(t.Context(), []*schema.Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if response.Content != "ok" {
		t.Errorf("content = %q, want ok", response.Content)
	}
	if got := <-requestPath; got != "/compatible-mode/v1/chat/completions" {
		t.Errorf("request path = %q, want /compatible-mode/v1/chat/completions", got)
	}
}
