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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"strings"
	"testing"
	"time"
)

func newOllamaForListModelsTest(baseURL string) *OllamaModel {
	return NewOllamaModel(map[string]string{"default": baseURL}, URLSuffix{Models: "api/tags"})
}

func newOllamaForChatTest(baseURL string) *OllamaModel {
	return NewOllamaModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "api/chat", AsyncChat: "api/chat"})
}

func newOllamaForDecisionTest(baseURL string) *OllamaModel {
	return NewOllamaModel(map[string]string{"default": baseURL}, URLSuffix{})
}

func ollamaMultimodalTestMessages() []Message {
	return []Message{{Role: "user", Content: []interface{}{
		map[string]interface{}{"type": "text", "text": "describe"},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,aGVsbG8="}},
		map[string]interface{}{"type": "text", "text": "carefully"},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "cmF3LWltYWdl"}},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://example.com/cat.png"}},
	}}}
}

func assertOllamaMultimodalRequest(t *testing.T, body map[string]interface{}, stream bool) {
	t.Helper()
	if body["stream"] != stream {
		t.Errorf("stream=%v, want %v", body["stream"], stream)
	}
	messages, ok := body["messages"].([]interface{})
	if !ok || len(messages) != 1 {
		t.Errorf("messages=%v, want one message", body["messages"])
		return
	}
	message, ok := messages[0].(map[string]interface{})
	if !ok {
		t.Errorf("message=%v, want object", messages[0])
		return
	}
	if message["content"] != "describe\ncarefully" {
		t.Errorf("content=%v, want joined text parts", message["content"])
	}
	images, ok := message["images"].([]interface{})
	if !ok {
		t.Errorf("images=%v, want array", message["images"])
		return
	}
	want := []string{"aGVsbG8=", "cmF3LWltYWdl", "https://example.com/cat.png"}
	if len(images) != len(want) {
		t.Errorf("images=%v, want %v", images, want)
		return
	}
	for i, expected := range want {
		if images[i] != expected {
			t.Errorf("images[%d]=%v, want %q", i, images[i], expected)
		}
	}
}

func TestOllamaChatMapsMultimodalImages(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		assertOllamaMultimodalRequest(t, body, false)
		_, _ = io.WriteString(w, `{"message":{"content":"ok","thinking":""}}`)
	}))
	defer srv.Close()

	response, err := newOllamaForChatTest(srv.URL).ChatWithMessages(
		t.Context(),
		"llava",
		ollamaMultimodalTestMessages(),
		&APIConfig{},
		&ChatConfig{},
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if response.Answer == nil || *response.Answer != "ok" {
		t.Fatalf("answer=%v, want ok", response.Answer)
	}
}

func TestOllamaStreamingChatMapsMultimodalImages(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		assertOllamaMultimodalRequest(t, body, true)
		_, _ = io.WriteString(w, "{\"message\":{\"content\":\"ok\"},\"done\":false}\n{\"done\":true}\n")
	}))
	defer srv.Close()

	err := newOllamaForChatTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"llava",
		ollamaMultimodalTestMessages(),
		&APIConfig{},
		&ChatConfig{},
		&common.ModelUsage{},
		func(*string, *string) error { return nil },
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
}

func TestOllamaThinkingPayloadTriState(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name string
		cfg  *ChatConfig
		want any
	}{
		{name: "nil omitted"},
		{name: "false", cfg: &ChatConfig{Thinking: boolPointer(false)}, want: false},
		{name: "true", cfg: &ChatConfig{Thinking: boolPointer(true)}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]interface{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				_, _ = io.WriteString(w, `{"message":{"content":"ok","thinking":""}}`)
			}))
			defer srv.Close()

			if _, err := newOllamaForChatTest(srv.URL).ChatWithMessages(t.Context(), "llava", []Message{{Role: "user", Content: "hello"}}, &APIConfig{}, tc.cfg, nil); err != nil {
				t.Fatalf("ChatWithMessages: %v", err)
			}
			got, exists := body["think"]
			if tc.want == nil {
				if exists {
					t.Fatalf("think=%v, want omitted", got)
				}
				return
			}
			if !exists || got != tc.want {
				t.Fatalf("think=%v, want %v", got, tc.want)
			}
			if options, exists := body["options"]; exists {
				if optionsMap, ok := options.(map[string]interface{}); ok {
					if _, nested := optionsMap["think"]; nested {
						t.Fatal("think must not be nested in options")
					}
				}
			}
		})
	}
}

func TestOllamaThinkingOverridesEffortOnlyForGPTOSS(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name, model string
		want        any
	}{
		{name: "gpt-oss uses effort", model: "gpt-oss:20b", want: "high"},
		{name: "other model keeps thinking", model: "llava", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]interface{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				_, _ = io.WriteString(w, `{"message":{"content":"ok","thinking":""}}`)
			}))
			defer srv.Close()
			thinking := false
			effort := "high"
			if _, err := newOllamaForChatTest(srv.URL).ChatWithMessages(t.Context(), tc.model, []Message{{Role: "user", Content: "hello"}}, &APIConfig{}, &ChatConfig{Thinking: &thinking, Effort: &effort}, nil); err != nil {
				t.Fatalf("ChatWithMessages: %v", err)
			}
			if got := body["think"]; got != tc.want {
				t.Fatalf("think=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestOllamaListModels(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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

	models, err := newOllamaForListModelsTest(srv.URL).ListModels(ctx, &APIConfig{})
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
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Some entries may carry only the "model" field; it should be used as the name.
		_, _ = io.WriteString(w, `{"models":[{"model":"phi3:mini"},{"name":""},{"name":"  "}]}`)
	}))
	defer srv.Close()

	models, err := newOllamaForListModelsTest(srv.URL).ListModels(ctx, &APIConfig{})
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
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `boom`)
	}))
	defer srv.Close()

	if _, err := newOllamaForListModelsTest(srv.URL).ListModels(ctx, &APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for HTTP 500, got nil")
	}
}

func TestOllamaListModelsRequiresBaseURL(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := NewOllamaModel(map[string]string{}, URLSuffix{Models: "api/tags"})
	if _, err := m.ListModels(ctx, &APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for missing base URL, got nil")
	}
}

func TestOllamaDecide(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name       string
		pathSuffix string
		apiKey     string
	}{
		{"bare base URL without key", "", ""},
		{"v1 base URL with key", "/v1", "ollama-secret"},
		{"full systemone path", "/v1/systemone", ""},
		{"trailing slash path", "/v1/systemone/", "key-123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/systemone" {
					t.Errorf("path=%s, want /v1/systemone", r.URL.Path)
				}
				if tc.apiKey != "" {
					if auth := r.Header.Get("Authorization"); auth != "Bearer "+tc.apiKey {
						t.Errorf("Authorization=%q, want %q", auth, "Bearer "+tc.apiKey)
					}
				} else {
					if auth := r.Header.Get("Authorization"); auth != "" {
						t.Errorf("Authorization=%q, want empty for local Ollama without key", auth)
					}
				}

				var req map[string]any
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatalf("failed to decode request: %v", err)
				}
				if req["model"] != "nimble" {
					t.Errorf("model=%v, want nimble", req["model"])
				}
				if req["state"] != "User requesting refund" {
					t.Errorf("state=%v, want User requesting refund", req["state"])
				}
				questions, ok := req["questions"].(map[string]any)
				if !ok || len(questions) != 1 {
					t.Fatalf("questions=%v, want 1 question", req["questions"])
				}
				q, ok := questions["is_refund"].(map[string]any)
				if !ok || q["type"] != "noul" || q["instructions"] != "Is a refund requested?" {
					t.Errorf("question is_refund=%v, want noul type with instructions", q)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{
					"model": "nimble",
					"answers": {
						"is_refund": {"type": "noul", "noul": 0.98}
					},
					"usage": {
						"input_tokens": 42,
						"output_tokens": 1
					}
				}`)
			}))
			defer srv.Close()

			modelName := "nimble"
			var apiCfg APIConfig
			if tc.apiKey != "" {
				apiCfg.ApiKey = &tc.apiKey
			}
			var usage common.ModelUsage
			resp, err := newOllamaForDecisionTest(srv.URL+tc.pathSuffix).Decide(
				t.Context(),
				&modelName,
				&DecideRequest{
					State: "User requesting refund",
					Questions: map[string]Question{
						"is_refund": Noul{Instructions: "Is a refund requested?"},
					},
				},
				&apiCfg,
				&usage,
			)
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if resp == nil || len(resp.Answers) != 1 {
				t.Fatalf("unexpected response: %+v", resp)
			}
			ans, ok := resp.Answers["is_refund"]
			if !ok || ans.Type != "noul" || ans.Noul == nil || *ans.Noul != 0.98 {
				t.Fatalf("ans=%+v, want noul=0.98", ans)
			}
			if resp.Usage.InputTokens != 42 || resp.Usage.OutputTokens != 1 {
				t.Errorf("usage=%+v, want 42 input, 1 output", resp.Usage)
			}
		})
	}
}

func TestOllamaDecideChoiceAndScore(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path=%s, want /v1/systemone", r.URL.Path)
		}

		var req struct {
			Model     string         `json:"model"`
			State     map[string]any `json:"state"`
			Questions map[string]any `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Model != "nimble" {
			t.Errorf("model=%v, want nimble", req.Model)
		}

		qMap := req.Questions
		labelQ, _ := qMap["label"].(map[string]any)
		if labelQ["type"] != "choice" {
			t.Errorf("label.type=%v, want choice", labelQ["type"])
		}
		urgencyQ, _ := qMap["urgency"].(map[string]any)
		if urgencyQ["type"] != "score" {
			t.Errorf("urgency.type=%v, want score", urgencyQ["type"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"model": "nimble",
			"answers": {
				"label": {
					"type": "choice",
					"choice": "billing",
					"probabilities": {"billing": 0.92, "tech": 0.08},
					"confidence": 0.85
				},
				"urgency": {
					"type": "score",
					"score": 1.4,
					"legend": {"0": "Routine", "1": "Soon", "2": "Immediate"},
					"probabilities": {"0": 0.1, "1": 0.4, "2": 0.5},
					"confidence": 0.75
				}
			},
			"usage": {"input_tokens": 80, "output_tokens": 2}
		}`)
	}))
	defer srv.Close()

	modelName := "nimble"
	resp, err := newOllamaForDecisionTest(srv.URL).Decide(
		t.Context(),
		&modelName,
		&DecideRequest{
			State: map[string]any{"ticket": "Payment charged twice"},
			Questions: map[string]Question{
				"label": Choice{
					Instructions: "Which department?",
					Criteria:     map[string]string{"billing": "Billing", "tech": "Tech support"},
				},
				"urgency": Score{
					Instructions: "How urgent?",
					Criteria:     []string{"Routine", "Soon", "Immediate"},
				},
			},
		},
		&APIConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("Decide choice and score: %v", err)
	}

	labelAns := resp.Answers["label"]
	if labelAns.Type != "choice" {
		t.Errorf("label answer=%+v, want type=choice", labelAns)
	}

	urgencyAns := resp.Answers["urgency"]
	if urgencyAns.Type != "score" {
		t.Errorf("urgency answer=%+v, want type=score", urgencyAns)
	}
}

func TestOllamaDecideImages(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model     string         `json:"model"`
			State     string         `json:"state"`
			Images    []string       `json:"images"`
			Questions map[string]any `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Model != "clef" {
			t.Errorf("model=%v, want clef", req.Model)
		}
		if len(req.Images) != 1 || req.Images[0] != "base64-image-data" {
			t.Errorf("images=%v, want [base64-image-data]", req.Images)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"model": "clef",
			"answers": {
				"is_receipt": {"type": "noul", "noul": 0.99}
			},
			"usage": {"input_tokens": 150, "output_tokens": 1}
		}`)
	}))
	defer srv.Close()

	modelName := "clef"
	resp, err := newOllamaForDecisionTest(srv.URL).Decide(
		t.Context(),
		&modelName,
		&DecideRequest{
			State: map[string]any{
				"state":  "A photo of a document",
				"images": []string{"base64-image-data"},
			},
			Questions: map[string]Question{
				"is_receipt": Noul{Instructions: "Is this a receipt?"},
			},
		},
		&APIConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("Decide images: %v", err)
	}
	if resp.Answers["is_receipt"].Noul == nil || *resp.Answers["is_receipt"].Noul != 0.99 {
		t.Errorf("is_receipt ans=%+v", resp.Answers["is_receipt"])
	}
}

func TestOllamaDecideValidation(t *testing.T) {
	withSSRFBypass(t)
	model := newOllamaForDecisionTest("http://unused")
	name := "nimble"

	// Missing model name
	if _, err := model.Decide(t.Context(), nil, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil); err == nil {
		t.Error("expected error for nil model name")
	}
	emptyName := "  "
	if _, err := model.Decide(t.Context(), &emptyName, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil); err == nil {
		t.Error("expected error for whitespace model name")
	}

	// Nil request
	if _, err := model.Decide(t.Context(), &name, nil, &APIConfig{}, nil); err == nil {
		t.Error("expected error for nil request")
	}

	// 0 questions
	if _, err := model.Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{},
	}, &APIConfig{}, nil); err == nil {
		t.Error("expected error for empty questions")
	}

	// Exceeding 64 questions
	manyQuestions := make(map[string]Question, 65)
	for i := 0; i < 65; i++ {
		manyQuestions[fmt.Sprintf("q_%d", i)] = Noul{Instructions: "test"}
	}
	if _, err := model.Decide(t.Context(), &name, &DecideRequest{
		Questions: manyQuestions,
	}, &APIConfig{}, nil); err == nil {
		t.Error("expected error for >64 questions")
	}

	// Empty question ID
	if _, err := model.Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{" ": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil); err == nil {
		t.Error("expected error for blank question ID")
	}

	// Nil question
	if _, err := model.Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": nil},
	}, &APIConfig{}, nil); err == nil {
		t.Error("expected error for nil question")
	}

	// Missing base URL
	mNoURL := NewOllamaModel(map[string]string{}, URLSuffix{})
	if _, err := mNoURL.Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil); err == nil {
		t.Error("expected error for missing base URL")
	}
}

func TestOllamaDecideRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"failed"}`)
			}))
			defer srv.Close()

			name := "nimble"
			_, err := newOllamaForDecisionTest(srv.URL).Decide(t.Context(), &name, &DecideRequest{
				Questions: map[string]Question{"q": Noul{Instructions: "test"}},
			}, &APIConfig{}, nil)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", status)
			}
		})
	}
}

func TestOllamaDecideUpstreamError(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"error": "model 'nonexistent' not found"}`)
	}))
	defer srv.Close()

	name := "nonexistent"
	_, err := newOllamaForDecisionTest(srv.URL).Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for upstream error payload, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("error=%v, want nonexistent mentioned", err)
	}
}

func TestOllamaDecideInvalidProbability(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"nimble","answers":{"q":{"type":"noul","noul":1.5}}}`)
	}))
	defer srv.Close()

	name := "nimble"
	_, err := newOllamaForDecisionTest(srv.URL).Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for noul probability > 1, got nil")
	}
}

func TestOllamaDecideEmptyAnswers(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"nimble","answers":{}}`)
	}))
	defer srv.Close()

	name := "nimble"
	_, err := newOllamaForDecisionTest(srv.URL).Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for empty answers map, got nil")
	}
}

func TestOllamaDecideDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	name := "nimble"
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := newOllamaForDecisionTest(srv.URL).Decide(ctx, &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Decide = %v; want deadline exceeded", err)
	}
}
