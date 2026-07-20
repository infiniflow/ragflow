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
	"os"
	"strings"
	"testing"
)

func newRAGconServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		var body map[string]interface{}
		if r.Method == http.MethodPost && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &body); err != nil {
					t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
					return
				}
			}
		}
		handler(t, r, body, w)
	}))
}

func newRAGconForTest(baseURL string) *RAGconModel {
	return NewRAGconModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Models:    "models",
			Embedding: "embeddings",
			Rerank:    "rerank",
			ASR:       "audio/transcriptions",
			TTS:       "audio/speech",
		},
	)
}

func TestRAGconName(t *testing.T) {
	if got := newRAGconForTest("http://unused").Name(); got != "ragcon" {
		t.Errorf("Name()=%q, want ragcon", got)
	}
}

func TestRAGconFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("RAGcon", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*RAGconModel); !ok {
		t.Fatalf("driver type=%T, want *RAGconModel", driver)
	}
}

func TestRAGconChatHappyPath(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s, want /chat/completions", r.URL.Path)
		}
		if body["model"] != "llama-4-maverick" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "thought about it",
				},
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     3,
				"completion_tokens": 1,
				"total_tokens":      4,
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newRAGconForTest(srv.URL).ChatWithMessages(
		"llama-4-maverick",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil, nil,
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
	if resp.Usage == nil || resp.Usage.TotalTokens != 4 {
		t.Errorf("Usage=%v, want total_tokens=4", resp.Usage)
	}
}

func TestRAGconChatRequiresAPIKey(t *testing.T) {
	_, err := newRAGconForTest("http://unused").ChatWithMessages(
		"llama-4-maverick",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{},
		nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key-required error, got %v", err)
	}
}

func TestRAGconChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newRAGconForTest("http://unused").ChatWithMessages(
		"llama-4-maverick",
		[]Message{},
		&APIConfig{ApiKey: &apiKey},
		nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestRAGconChatSurfacesHTTPError(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid key"}`)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newRAGconForTest(srv.URL).ChatWithMessages(
		"llama-4-maverick",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 status error, got %v", err)
	}
}

func TestRAGconStreamHappyPath(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
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
	err := newRAGconForTest(srv.URL).ChatStreamlyWithSender(
		"llama-4-maverick",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
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

func TestRAGconStreamRejectsNilSender(t *testing.T) {
	apiKey := "test-key"
	err := newRAGconForTest("http://unused").ChatStreamlyWithSender(
		"llama-4-maverick",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestRAGconEmbed(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path=%s, want /embeddings", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.1, 0.2}, "index": 0},
				{"embedding": []float64{0.3, 0.4}, "index": 1},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "text-embedding-3-small"
	embeddings, err := newRAGconForTest(srv.URL).Embed(&model, []string{"a", "b"}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 2 || embeddings[1].Index != 1 {
		t.Errorf("embeddings=%v", embeddings)
	}
}

func TestRAGconRerank(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/rerank" {
			t.Errorf("path=%s, want /rerank", r.URL.Path)
		}
		if body["top_n"] != float64(2) {
			t.Errorf("top_n=%v, want 2", body["top_n"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{"index": 1, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.2},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "rerank-v1"
	resp, err := newRAGconForTest(srv.URL).Rerank(&model, "q", []string{"doc0", "doc1"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{TopN: 2}, nil)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].Index != 1 || resp.Data[0].RelevanceScore != 0.9 {
		t.Errorf("Data=%v", resp.Data)
	}
}

func TestRAGconListModelsAndCheckConnection(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/models" {
			t.Errorf("path=%s, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"llama-4-maverick"},{"id":"gpt-oss-120b"}]}`)
	})
	defer srv.Close()

	apiKey := "test-key"
	cfg := &APIConfig{ApiKey: &apiKey}
	models, err := newRAGconForTest(srv.URL).ListModels(cfg)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "llama-4-maverick,gpt-oss-120b" {
		t.Errorf("models=%v", models)
	}
	if err := newRAGconForTest(srv.URL).CheckConnection(cfg); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestRAGconTranscribeAudioPostsMultipart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/transcriptions" {
			t.Errorf("path=%s, want /audio/transcriptions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
		}
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if got := r.FormValue("model"); got != "whisper-1" {
			t.Errorf("model=%q, want whisper-1", got)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		if string(content) != "audio-bytes" {
			t.Errorf("file content=%q, want audio-bytes", string(content))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "hello world"})
	}))
	defer srv.Close()

	audioPath := t.TempDir() + "/sample.wav"
	if err := os.WriteFile(audioPath, []byte("audio-bytes"), 0600); err != nil {
		t.Fatalf("write audio fixture: %v", err)
	}

	apiKey := "test-key"
	model := "whisper-1"
	resp, err := newRAGconForTest(srv.URL).TranscribeAudio(&model, &audioPath, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if resp.Text != "hello world" {
		t.Fatalf("Text=%q, want hello world", resp.Text)
	}
}

func TestRAGconAudioSpeechPostsJSON(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/audio/speech" {
			t.Errorf("path=%s, want /audio/speech", r.URL.Path)
		}
		if body["voice"] != "alloy" {
			t.Errorf("voice=%v, want alloy", body["voice"])
		}
		w.Write([]byte("raw-audio-bytes"))
	})
	defer srv.Close()

	apiKey := "test-key"
	model := "tts-1"
	text := "hello"
	resp, err := newRAGconForTest(srv.URL).AudioSpeech(&model, &text, &APIConfig{ApiKey: &apiKey}, &TTSConfig{Params: map[string]interface{}{"voice": "alloy"}}, nil)
	if err != nil {
		t.Fatalf("AudioSpeech: %v", err)
	}
	if string(resp.Audio) != "raw-audio-bytes" {
		t.Errorf("Audio=%q", string(resp.Audio))
	}
}

func TestRAGconUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	r := newRAGconForTest("http://unused")
	model := "llama-4-maverick"

	if _, err := r.Balance(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: expected no such method, got %v", err)
	}
	if _, err := r.OCRFile(&model, nil, nil, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: expected no such method, got %v", err)
	}
	if _, err := r.ParseFile(&model, nil, nil, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ParseFile: expected no such method, got %v", err)
	}
	if _, err := r.ListTasks(&APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListTasks: expected no such method, got %v", err)
	}
	if _, err := r.ShowTask("t1", &APIConfig{}); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ShowTask: expected no such method, got %v", err)
	}
}
