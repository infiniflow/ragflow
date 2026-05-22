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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAvianServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		// The Avian client sets Content-Type: application/json on every
		// request, including the GET to /v1/models (see avian.go ListModels),
		// so this assertion intentionally applies even when there's no body.
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("expected Content-Type to start with application/json, got %q", got)
			return
		}
		var body map[string]interface{}
		if r.Method == http.MethodPost {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
				return
			}
		}
		handler(t, r, body, w)
	}))
}

func newAvianForTest(baseURL string) *AvianModel {
	return NewAvianModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/chat/completions", Models: "v1/models"},
	)
}

func TestAvianName(t *testing.T) {
	if got := newAvianForTest("http://unused").Name(); got != "avian" {
		t.Errorf("Name()=%q, want avian", got)
	}
}

func TestAvianFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Avian", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*AvianModel); !ok {
		t.Fatalf("driver type=%T, want *AvianModel", driver)
	}
}

func TestAvianChatHappyPath(t *testing.T) {
	srv := newAvianServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s, want /v1/chat/completions", r.URL.Path)
		}
		if body["model"] != "deepseek/deepseek-v3.2" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		if body["max_tokens"] != float64(64) {
			t.Errorf("max_tokens=%v, want 64", body["max_tokens"])
		}
		if body["temperature"] != 0.3 {
			t.Errorf("temperature=%v, want 0.3", body["temperature"])
		}
		if body["top_p"] != 0.9 {
			t.Errorf("top_p=%v, want 0.9", body["top_p"])
		}
		if stopSlice, ok := body["stop"].([]interface{}); !ok || len(stopSlice) != 1 || stopSlice[0] != "END" {
			t.Errorf("stop=%v, want [END]", body["stop"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "thought about it",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	mt := 64
	temp := 0.3
	topP := 0.9
	stop := []string{"END"}
	resp, err := newAvianForTest(srv.URL).ChatWithMessages(
		"deepseek/deepseek-v3.2",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("Answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "thought about it" {
		t.Errorf("ReasonContent=%v, want 'thought about it'", resp.ReasonContent)
	}
}

func TestAvianChatFallsBackToReasoningField(t *testing.T) {
	srv := newAvianServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":   "12",
					"reasoning": "0.15 * 80 = 12",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newAvianForTest(srv.URL).ChatWithMessages(
		"moonshotai/kimi-k2.5",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "0.15 * 80 = 12" {
		t.Errorf("ReasonContent=%v", resp.ReasonContent)
	}
}

func TestAvianChatRequiresAPIKey(t *testing.T) {
	_, err := newAvianForTest("http://unused").ChatWithMessages(
		"deepseek/deepseek-v3.2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key-required error, got %v", err)
	}
}

func TestAvianChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newAvianForTest("http://unused").ChatWithMessages(
		"deepseek/deepseek-v3.2",
		[]Message{},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestAvianChatPropagatesUpstreamErrorStatus(t *testing.T) {
	srv := newAvianServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid key"}`)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAvianForTest(srv.URL).ChatWithMessages(
		"deepseek/deepseek-v3.2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 status error, got %v", err)
	}
}

func TestAvianStreamHappyPath(t *testing.T) {
	srv := newAvianServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"step. "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	var sawDone bool
	err := newAvianForTest(srv.URL).ChatStreamlyWithSender(
		"deepseek/deepseek-v3.2",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(c *string, r *string) error {
			if r != nil && *r != "" {
				reasoning = append(reasoning, *r)
			}
			if c != nil && *c == "[DONE]" {
				sawDone = true
			}
			if c != nil && *c != "" && *c != "[DONE]" {
				content = append(content, *c)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(reasoning, "") != "step. " {
		t.Errorf("reasoning=%q", strings.Join(reasoning, ""))
	}
	if strings.Join(content, "") != "Hello world" {
		t.Errorf("content=%q", strings.Join(content, ""))
	}
	if !sawDone {
		t.Error("expected [DONE] callback")
	}
}

func TestAvianStreamRejectsFalseStreamConfig(t *testing.T) {
	apiKey := "test-key"
	stream := false
	err := newAvianForTest("http://unused").ChatStreamlyWithSender(
		"deepseek/deepseek-v3.2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-must-be-true error, got %v", err)
	}
}

func TestAvianStreamRequiresSender(t *testing.T) {
	apiKey := "test-key"
	err := newAvianForTest("http://unused").ChatStreamlyWithSender(
		"deepseek/deepseek-v3.2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestAvianListModelsAndCheckConnection(t *testing.T) {
	srv := newAvianServer(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s, want /v1/models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `[{"id":"deepseek/deepseek-v3.2"},{"id":"moonshotai/kimi-k2.5"}]`)
	})
	defer srv.Close()

	apiKey := "test-key"
	cfg := &APIConfig{ApiKey: &apiKey}
	models, err := newAvianForTest(srv.URL).ListModels(cfg)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "deepseek/deepseek-v3.2,moonshotai/kimi-k2.5" {
		t.Errorf("models=%v", models)
	}
	if err := newAvianForTest(srv.URL).CheckConnection(cfg); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestAvianMissingBaseURLFailsClearly(t *testing.T) {
	a := NewAvianModel(map[string]string{}, URLSuffix{Chat: "v1/chat/completions"})
	apiKey := "test-key"
	_, err := a.ChatWithMessages(
		"deepseek/deepseek-v3.2",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "no base URL") {
		t.Errorf("expected no-base-URL error, got %v", err)
	}
}

func TestAvianUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	a := newAvianForTest("http://unused")
	model := "deepseek/deepseek-v3.2"

	if _, err := a.Embed(&model, []string{"x"}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: expected no such method, got %v", err)
	}
	if _, err := a.Rerank(&model, "q", []string{"d"}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: expected no such method, got %v", err)
	}
	if _, err := a.Balance(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: expected no such method, got %v", err)
	}
	if _, err := a.TranscribeAudio(&model, nil, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: expected no such method, got %v", err)
	}
	if _, err := a.AudioSpeech(&model, nil, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: expected no such method, got %v", err)
	}
	if _, err := a.OCRFile(&model, nil, nil, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: expected no such method, got %v", err)
	}
}
