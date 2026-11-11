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

package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
)

// A Jina stream failure is forwarded as answer text. The chat answer adds the
// API key hint when that text says "invalid api"/"invalid key"; a platform
// model failing the same way must lead to the same decision.
func TestJinaPlatformStreamErrorKeepsAPIKeyHint(t *testing.T) {
	t.Setenv(common.EnvAllowAnyHost, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"invalid api key"}`))
	}))
	defer srv.Close()

	answer := func(driver modelModule.ModelDriver) string {
		key := "k"
		var b strings.Builder
		err := driver.ChatStreamlyWithSender(t.Context(), "jina-model",
			[]modelModule.Message{{Role: "user", Content: "hi"}},
			&modelModule.APIConfig{ApiKey: &key}, nil, nil,
			func(text, _ *string) error {
				if text != nil && *text != "[DONE]" {
					b.WriteString(*text)
				}
				return nil
			})
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		timer, _ := newTimerAndPrompt()
		return (&ChatPipelineService{}).decorateAnswer(t.Context(), b.String(), map[string]interface{}{}, "", []string{"q"}, 0, timer, nil, 0, false, nil, "", nil, "", nil, false).Answer
	}
	base := func() modelModule.ModelDriver {
		return modelModule.NewJinaModel(map[string]string{"default": srv.URL}, modelModule.URLSuffix{Chat: "chat/completions"})
	}
	tenant := answer(base())
	platform := answer(modelModule.NewPlatformSafeDriver(base()))
	t.Logf("tenant answer:   %q", tenant)
	t.Logf("platform answer: %q", platform)
	const hint = "Please set LLM API-Key"
	if !strings.Contains(tenant, hint) || !strings.Contains(platform, hint) {
		t.Fatalf("API key hint differs: tenant=%v platform=%v", strings.Contains(tenant, hint), strings.Contains(platform, hint))
	}
}
