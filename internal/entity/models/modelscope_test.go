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
	"time"
)

func newModelScopeForTest(baseURL string) *ModelScopeModel {
	return NewModelScopeModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:   "v1/chat/completions",
			Models: "v1/models",
		},
	)
}

func withModelScopeIdleTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	original := modelscopeStreamIdleTimeout
	modelscopeStreamIdleTimeout = d
	t.Cleanup(func() {
		modelscopeStreamIdleTimeout = original
	})
}

func TestModelScopeName(t *testing.T) {
	m := newModelScopeForTest("http://unused")
	if got := m.Name(); got != "modelscope" {
		t.Errorf("Name()=%q, want %q", got, "modelscope")
	}
}

func TestNormalizeModelScopeBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"http://127.0.0.1:8000", "http://127.0.0.1:8000"},
		{"http://127.0.0.1:8000/", "http://127.0.0.1:8000"},
		{"http://127.0.0.1:8000/v1", "http://127.0.0.1:8000"},
		{" http://127.0.0.1:8000/v1/ ", "http://127.0.0.1:8000"},
	}
	for _, tc := range cases {
		if got := normalizeModelScopeBaseURL(tc.in); got != tc.want {
			t.Errorf("normalizeModelScopeBaseURL(%q)=%q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestModelScopeFactoryRoute(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("modelscope", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if driver.Name() != "modelscope" {
		t.Errorf("driver.Name()=%q, want modelscope", driver.Name())
	}
}

func TestModelScopeNewModelWithCustomDefaultTransport(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = original
	})

	if model := NewModelScopeModel(map[string]string{"default": "http://unused"}, URLSuffix{}); model == nil {
		t.Fatal("NewModelScopeModel returned nil")
	}
}

func TestModelScopeChatHappyPathNormalizesBaseURLAndOmitsEmptyAuth(t *testing.T) {
	var seen map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected no Authorization header, got %q", got)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("unmarshal request: %v", err)
			http.Error(w, "unmarshal error", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"pong"}}]}`)
	}))
	defer srv.Close()

	m := newModelScopeForTest(srv.URL)
	maxTokens := 32
	temp := 0.2
	resp, err := m.ChatWithMessages("Qwen/Qwen2.5-7B-Instruct",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{},
		&ChatConfig{MaxTokens: &maxTokens, Temperature: &temp})
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Fatalf("Answer=%v, want pong", resp.Answer)
	}
	if seen["model"] != "Qwen/Qwen2.5-7B-Instruct" {
		t.Errorf("model=%v, want Qwen/Qwen2.5-7B-Instruct", seen["model"])
	}
	if seen["stream"] != false {
		t.Errorf("stream=%v, want false", seen["stream"])
	}
	if seen["max_tokens"] != float64(32) {
		t.Errorf("max_tokens=%v, want 32", seen["max_tokens"])
	}
	if seen["temperature"] != 0.2 {
		t.Errorf("temperature=%v, want 0.2", seen["temperature"])
	}
}

func TestModelScopeChatSendsAuthHeaderWhenKeyProvided(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ms-test" {
			t.Errorf("Authorization=%q, want Bearer ms-test", got)
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	m := newModelScopeForTest(srv.URL + "/v1")
	key := "ms-test"
	_, err := m.ChatWithMessages("Qwen/Qwen2.5-7B-Instruct",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &key}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestModelScopeChatExtractsReasoningFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[{"message":{
			"content":"12",
			"reasoning_content":"0.15 * 80 = 12"
		}}]}`)
	}))
	defer srv.Close()

	m := newModelScopeForTest(srv.URL)
	resp, err := m.ChatWithMessages("Qwen/Qwen3-8B",
		[]Message{{Role: "user", Content: "15% of 80?"}},
		&APIConfig{}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "0.15 * 80 = 12" {
		t.Errorf("ReasonContent=%v", resp.ReasonContent)
	}
}

func TestModelScopeStreamHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var seen map[string]interface{}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal(raw, &seen); err != nil {
			t.Errorf("unmarshal request: %v", err)
			http.Error(w, "unmarshal error", http.StatusBadRequest)
			return
		}
		if seen["stream"] != true {
			t.Errorf("stream=%v, want true", seen["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"step. "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	}))
	defer srv.Close()

	m := newModelScopeForTest(srv.URL)
	var content []string
	var reasoning []string
	var sawDone bool
	err := m.ChatStreamlyWithSender("Qwen/Qwen2.5-7B-Instruct",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{}, nil,
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
		})
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

func TestModelScopeStreamRejectsFalseStreamConfig(t *testing.T) {
	m := newModelScopeForTest("http://unused")
	stream := false
	err := m.ChatStreamlyWithSender("Qwen/Qwen2.5-7B-Instruct",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{},
		&ChatConfig{Stream: &stream},
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream-must-be-true error, got %v", err)
	}
}

func TestModelScopeStreamCancelsOnIdle(t *testing.T) {
	withModelScopeIdleTimeout(t, 200*time.Millisecond)

	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"hi"}}]}`+"\n")
			f.Flush()
		}
		select {
		case <-hold:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hold) })

	m := newModelScopeForTest(srv.URL)
	err := m.ChatStreamlyWithSender("Qwen/Qwen2.5-7B-Instruct",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream idle") {
		t.Errorf("expected stream-idle error, got %v", err)
	}
}

func TestModelScopeListModelsAndCheckConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ms-test" {
			t.Errorf("Authorization=%q, want Bearer ms-test", got)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"Qwen/Qwen2.5-7B-Instruct"},{"id":"Qwen/Qwen3-8B"}]}`)
	}))
	defer srv.Close()

	m := newModelScopeForTest(srv.URL)
	key := "ms-test"
	apiConfig := &APIConfig{ApiKey: &key}
	models, err := m.ListModels(apiConfig)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "Qwen/Qwen2.5-7B-Instruct,Qwen/Qwen3-8B" {
		t.Errorf("models=%v", models)
	}
	if err := m.CheckConnection(apiConfig); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestModelScopeMissingBaseURLFailsClearly(t *testing.T) {
	m := NewModelScopeModel(map[string]string{}, URLSuffix{Chat: "v1/chat/completions"})
	_, err := m.ChatWithMessages("Qwen/Qwen2.5-7B-Instruct",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing base URL") {
		t.Errorf("expected missing-base-URL error, got %v", err)
	}
}

func TestModelScopeUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	m := newModelScopeForTest("http://unused")
	model := "Qwen/Qwen2.5-7B-Instruct"

	if _, err := m.Embed(&model, []string{"x"}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: expected no such method, got %v", err)
	}
	if _, err := m.Rerank(&model, "q", []string{"d"}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: expected no such method, got %v", err)
	}
	if _, err := m.Balance(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: expected no such method, got %v", err)
	}
	if _, err := m.TranscribeAudio(&model, nil, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: expected no such method, got %v", err)
	}
	if err := m.TranscribeAudioWithSender(&model, nil, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudioWithSender: expected no such method, got %v", err)
	}
	if _, err := m.AudioSpeech(&model, nil, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: expected no such method, got %v", err)
	}
	if err := m.AudioSpeechWithSender(&model, nil, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeechWithSender: expected no such method, got %v", err)
	}
	if _, err := m.OCRFile(&model, nil, nil, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: expected no such method, got %v", err)
	}
}
