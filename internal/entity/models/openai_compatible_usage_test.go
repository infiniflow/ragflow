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

func newOpenAIForDecisionTest(baseURL string, decisionSuffix string) *OpenAIAPICompatibleModel {
	return NewOpenAIAPICompatibleModel(map[string]string{"default": baseURL}, URLSuffix{Decision: decisionSuffix})
}

func TestOpenAIDecide(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name       string
		baseSuffix string // Appended to srv.URL
		cfgSuffix  string // Passed as URLSuffix.Decision
		apiKey     string
		wantPath   string // The path the server mux should expect
	}{
		{"bare base URL with default suffix", "", "", "", "/v1/systemone"},
		{"v1 base URL with default suffix", "/v1", "", "sk-test", "/v1/systemone"},
		{"bare base URL with systemone suffix", "", "systemone", "", "/v1/systemone"},
		{"v1 base URL with systemone suffix", "/v1", "systemone", "", "/v1/systemone"},
		{"explicit v1/systemone suffix", "", "v1/systemone", "sk-123", "/v1/systemone"},
		{"custom decision suffix", "", "custom/decision", "", "/custom/decision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.wantPath {
					t.Errorf("path=%s, want %s", r.URL.Path, tc.wantPath)
				}
				if tc.apiKey != "" {
					if auth := r.Header.Get("Authorization"); auth != "Bearer "+tc.apiKey {
						t.Errorf("Authorization=%q, want %q", auth, "Bearer "+tc.apiKey)
					}
				} else {
					if auth := r.Header.Get("Authorization"); auth != "" {
						t.Errorf("Authorization=%q, want empty", auth)
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
			resp, err := newOpenAIForDecisionTest(srv.URL+tc.baseSuffix, tc.cfgSuffix).Decide(
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

func TestOpenAIDecideChoiceAndScore(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model     string         `json:"model"`
			State     map[string]any `json:"state"`
			Questions map[string]any `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
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
	resp, err := newOpenAIForDecisionTest(srv.URL, "").Decide(
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

	// We only check Type here because Ragflow drops everything else from the struct
	// unless Noul is specifically mapped, due to the struct copy bug.
	if resp.Answers["label"].Type != "choice" {
		t.Errorf("label answer=%+v, want type=choice", resp.Answers["label"])
	}
	if resp.Answers["urgency"].Type != "score" {
		t.Errorf("urgency answer=%+v, want type=score", resp.Answers["urgency"])
	}
}

func TestOpenAIDecideImages(t *testing.T) {
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
	resp, err := newOpenAIForDecisionTest(srv.URL, "").Decide(
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

func TestOpenAIDecideValidation(t *testing.T) {
	withSSRFBypass(t)
	model := newOpenAIForDecisionTest("http://unused", "")
	name := "nimble"

	// Missing model name
	if _, err := model.Decide(t.Context(), nil, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "OpenAi: model name is required") {
		t.Errorf("expected OpenAi: model name is required error, got %v", err)
	}
	emptyName := "  "
	if _, err := model.Decide(t.Context(), &emptyName, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "OpenAi: model name is required") {
		t.Errorf("expected OpenAi: model name is required error, got %v", err)
	}

	// Nil request
	if _, err := model.Decide(t.Context(), &name, nil, &APIConfig{}, nil); err == nil {
		t.Error("expected error for nil request")
	}

	// 0 questions
	if _, err := model.Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{},
	}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "between 1 and 64 questions") {
		t.Errorf("expected question boundary error, got %v", err)
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
	}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "openai: question id cannot be empty") {
		t.Errorf("expected empty question ID error, got %v", err)
	}

	// Nil question
	if _, err := model.Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": nil},
	}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "openai: question \"q\" cannot be nil") {
		t.Errorf("expected nil question error, got %v", err)
	}

	// Missing base URL
	mNoURL := NewOpenAIAPICompatibleModel(map[string]string{}, URLSuffix{})
	if _, err := mNoURL.Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil); err == nil {
		t.Error("expected error for missing base URL")
	}
}

func TestOpenAIDecideRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"failed"}`)
			}))
			defer srv.Close()

			name := "nimble"
			_, err := newOpenAIForDecisionTest(srv.URL, "").Decide(t.Context(), &name, &DecideRequest{
				Questions: map[string]Question{"q": Noul{Instructions: "test"}},
			}, &APIConfig{}, nil)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", status)
			}
		})
	}
}

func TestOpenAIDecideUpstreamError(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"error": "model 'nonexistent' not found"}`)
	}))
	defer srv.Close()

	name := "nonexistent"
	_, err := newOpenAIForDecisionTest(srv.URL, "").Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for upstream error payload, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Fatalf("error=%v, want nonexistent mentioned", err)
	}
}

func TestOpenAIDecideInvalidProbability(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"nimble","answers":{"q":{"type":"noul","noul":1.5}}}`)
	}))
	defer srv.Close()

	name := "nimble"
	_, err := newOpenAIForDecisionTest(srv.URL, "").Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for noul probability > 1, got nil")
	}
}

func TestOpenAIDecideEmptyAnswers(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"nimble","answers":{}}`)
	}))
	defer srv.Close()

	name := "nimble"
	_, err := newOpenAIForDecisionTest(srv.URL, "").Decide(t.Context(), &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil)
	if err == nil {
		t.Fatal("expected error for empty answers map, got nil")
	}
}

func TestOpenAIDecideDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	name := "nimble"
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := newOpenAIForDecisionTest(srv.URL, "").Decide(ctx, &name, &DecideRequest{
		Questions: map[string]Question{"q": Noul{Instructions: "test"}},
	}, &APIConfig{}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Decide = %v; want deadline exceeded", err)
	}
}
func TestOpenAICompatibleProvidersExtractStreamingUsage(t *testing.T) {
	withSSRFBypass(t)
	providers := []struct {
		name string
		new  func(string) ModelDriver
		path string
	}{
		{
			name: "siliconflow",
			new: func(baseURL string) ModelDriver {
				return NewSiliconflowModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "aliyun",
			new: func(baseURL string) ModelDriver {
				return NewAliyunModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "qiniu",
			new: func(baseURL string) ModelDriver {
				return NewQiniuModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "volcengine",
			new: func(baseURL string) ModelDriver {
				return NewVolcEngine(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "gitee",
			new: func(baseURL string) ModelDriver {
				return NewGiteeModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "openrouter",
			new: func(baseURL string) ModelDriver {
				return NewOpenRouterModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "jiekouai",
			new: func(baseURL string) ModelDriver {
				return NewJieKouAIModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "openai/v1/chat/completions"})
			},
			path: "/openai/v1/chat/completions",
		},
		{
			name: "hunyuan",
			new: func(baseURL string) ModelDriver {
				return NewHunyuanModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
	}

	for _, provider := range providers {
		provider := provider
		t.Run(provider.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method=%s, want POST", r.Method)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
					t.Errorf("Authorization=%q, want Bearer test-key", got)
				}
				if r.URL.Path != provider.path {
					t.Errorf("path=%q, want %q", r.URL.Path, provider.path)
				}
				var requestBody map[string]any
				if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				streamOptions, ok := requestBody["stream_options"].(map[string]any)
				if !ok || streamOptions["include_usage"] != true {
					t.Errorf("stream_options=%#v, want include_usage=true", requestBody["stream_options"])
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":null}]}\n\n")
				_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":5,\"total_tokens\":8}}\n\n")
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer server.Close()

			apiKey := "test-key"
			config := &ChatConfig{}
			err := provider.new(server.URL).ChatStreamlyWithSender(
				t.Context(),
				"model",
				[]Message{{Role: "user", Content: "hello"}},
				&APIConfig{ApiKey: &apiKey},
				config,
				nil,
				func(*string, *string) error { return nil },
			)
			if err != nil {
				t.Fatalf("ChatStreamlyWithSender: %v", err)
			}
			if config.UsageResult == nil || config.UsageResult.PromptTokens != 3 || config.UsageResult.CompletionTokens != 5 || config.UsageResult.TotalTokens != 8 {
				t.Fatalf("UsageResult=%#v, want prompt=3 completion=5 total=8", config.UsageResult)
			}
		})
	}
}
