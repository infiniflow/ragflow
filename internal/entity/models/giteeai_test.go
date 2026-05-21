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

func newGiteeAIServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		var body map[string]interface{}
		if r.Method == http.MethodPost {
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("expected Content-Type to start with application/json, got %q", got)
				return
			}
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

func newGiteeAIForTest(baseURL string) *GiteeAIModel {
	return NewGiteeAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestGiteeAINameAndDefaultURL(t *testing.T) {
	model := NewGiteeAIModel(nil, URLSuffix{})
	if got := model.Name(); got != "giteeai" {
		t.Errorf("Name()=%q, want giteeai", got)
	}
	if got := model.BaseURL["default"]; got != giteeAIDefaultBaseURL {
		t.Errorf("default base URL=%q, want %q", got, giteeAIDefaultBaseURL)
	}
	if model.URLSuffix.Chat != "chat/completions" || model.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", model.URLSuffix)
	}
}

func TestGiteeAIFactoryIsSeparateFromGitee(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("GiteeAI", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*GiteeAIModel); !ok {
		t.Fatalf("driver type=%T, want *GiteeAIModel", driver)
	}

	giteeDriver, err := NewModelFactory().CreateModelDriver("gitee", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver(gitee): %v", err)
	}
	if _, ok := giteeDriver.(*GiteeModel); !ok {
		t.Fatalf("gitee driver type=%T, want *GiteeModel", giteeDriver)
	}
}

func TestGiteeAIChatWithMessages(t *testing.T) {
	srv := newGiteeAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s, want /chat/completions", r.URL.Path)
		}
		if body["model"] != "DeepSeek-V3" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) != 1 {
			t.Errorf("messages=%v", body["messages"])
			return
		}
		first, ok := messages[0].(map[string]interface{})
		if !ok || first["role"] != "user" || first["content"] != "ping" {
			t.Errorf("message=%v", messages[0])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "thought",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newGiteeAIForTest(srv.URL).ChatWithMessages(
		"DeepSeek-V3",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("Answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "thought" {
		t.Errorf("ReasonContent=%v, want thought", resp.ReasonContent)
	}
}

func TestGiteeAIChatExtractsThinkTags(t *testing.T) {
	srv := newGiteeAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": "<think>private reasoning</think>visible answer",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newGiteeAIForTest(srv.URL).ChatWithMessages(
		"Qwen3-30B-A3B",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "visible answer" {
		t.Errorf("Answer=%v, want visible answer", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "private reasoning" {
		t.Errorf("ReasonContent=%v, want private reasoning", resp.ReasonContent)
	}
}

func TestGiteeAIStreamChatSSEParsing(t *testing.T) {
	srv := newGiteeAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s, want /chat/completions", r.URL.Path)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept=%q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"step. "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	var sawDone bool
	err := newGiteeAIForTest(srv.URL).ChatStreamlyWithSender(
		"DeepSeek-R1",
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

func TestGiteeAIStreamHandlesThinkTagsAcrossSSEEvents(t *testing.T) {
	srv := newGiteeAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"<thi"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"nk>hidden"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"</think>shown"}}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content, reasoning []string
	err := newGiteeAIForTest(srv.URL).ChatStreamlyWithSender(
		"Qwen3-235B-A22B",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(c *string, r *string) error {
			if r != nil && *r != "" {
				reasoning = append(reasoning, *r)
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
	if strings.Join(reasoning, "") != "hidden" {
		t.Errorf("reasoning=%q, want hidden", strings.Join(reasoning, ""))
	}
	if strings.Join(content, "") != "shown" {
		t.Errorf("content=%q, want shown", strings.Join(content, ""))
	}
}

func TestGiteeAIStreamDoneEventHandling(t *testing.T) {
	srv := newGiteeAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"ok"}}]}`+"\n"+
				`data: [DONE]`+"\n"+
				`data: {"choices":[{"delta":{"content":"ignored"}}]}`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var got []string
	err := newGiteeAIForTest(srv.URL).ChatStreamlyWithSender(
		"DeepSeek-V3",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(c *string, r *string) error {
			if c != nil && *c != "" {
				got = append(got, *c)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(got, "|") != "ok|[DONE]" {
		t.Errorf("got=%v, want [ok [DONE]]", got)
	}
}

func TestGiteeAIListModelsAndCheckConnection(t *testing.T) {
	srv := newGiteeAIServer(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"DeepSeek-R1"},{"id":"Qwen3-30B-A3B"}]}`)
	})
	defer srv.Close()

	apiKey := "test-key"
	cfg := &APIConfig{ApiKey: &apiKey}
	models, err := newGiteeAIForTest(srv.URL).ListModels(cfg)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "DeepSeek-R1,Qwen3-30B-A3B" {
		t.Errorf("models=%v", models)
	}
	if err := newGiteeAIForTest(srv.URL).CheckConnection(cfg); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestGiteeAIBaseURLOverride(t *testing.T) {
	srv := newGiteeAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/custom/chat/completions" {
			t.Errorf("path=%s, want /custom/chat/completions", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "ok"},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := NewGiteeAIModel(
		map[string]string{"default": srv.URL + "/custom"},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	).ChatWithMessages("DeepSeek-V3", []Message{{Role: "user", Content: "hi"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestGiteeAIUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	model := "DeepSeek-V3"
	g := newGiteeAIForTest("http://unused")

	if _, err := g.Embed(&model, []string{"x"}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed: expected no such method, got %v", err)
	}
	if _, err := g.Rerank(&model, "q", []string{"d"}, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: expected no such method, got %v", err)
	}
	if _, err := g.TranscribeAudio(&model, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: expected no such method, got %v", err)
	}
	if _, err := g.AudioSpeech(&model, &model, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech: expected no such method, got %v", err)
	}
	if _, err := g.OCRFile(&model, nil, nil, &APIConfig{}, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: expected no such method, got %v", err)
	}
}
