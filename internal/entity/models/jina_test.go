package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newJinaServer(t *testing.T, expectedPath string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected Content-Type=application/json, got %q", got)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			return
		}
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("invalid JSON body: %v\n%s", err, string(raw))
			return
		}
		handler(t, body, w)
	}))
}

func newJinaForTest(baseURL string) *JinaModel {
	return NewJinaModel(
		map[string]string{"default": baseURL},
		URLSuffix{
			Chat:      "chat/completions",
			Models:    "models",
			Embedding: "embeddings",
			Rerank:    "rerank",
		},
	)
}

func TestJinaChatHappyPath(t *testing.T) {
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "jina-vlm" {
			t.Errorf("expected model=jina-vlm, got %v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("expected stream=false, got %v", body["stream"])
		}
		msgs, ok := body["messages"].([]interface{})
		if !ok || len(msgs) != 1 {
			t.Errorf("expected 1 message, got %v", body["messages"])
			return
		}
		msg, ok := msgs[0].(map[string]interface{})
		if !ok || msg["role"] != "user" || msg["content"] != "ping" {
			t.Errorf("unexpected message payload: %v", msgs[0])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"content": "pong"}},
			},
		})
	})
	defer srv.Close()

	j := newJinaForTest(srv.URL)
	apiKey := "test-key"
	resp, err := j.ChatWithMessages("jina-vlm", []Message{{Role: "user", Content: "ping"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "pong" {
		t.Errorf("answer=%v, want pong", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "" {
		t.Errorf("expected empty reason content, got %v", resp.ReasonContent)
	}
}

func TestJinaChatPropagatesConfig(t *testing.T) {
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["max_tokens"] != float64(128) {
			t.Errorf("max_tokens=%v want 128", body["max_tokens"])
		}
		if body["temperature"] != 0.2 {
			t.Errorf("temperature=%v want 0.2", body["temperature"])
		}
		if body["top_p"] != 0.8 {
			t.Errorf("top_p=%v want 0.8", body["top_p"])
		}
		stop, ok := body["stop"].([]interface{})
		if !ok || len(stop) != 1 || stop[0] != "END" {
			t.Errorf("stop=%v want [END]", body["stop"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "ok"}}},
		})
	})
	defer srv.Close()

	j := newJinaForTest(srv.URL)
	apiKey := "test-key"
	maxTokens := 128
	temperature := 0.2
	topP := 0.8
	stop := []string{"END"}
	_, err := j.ChatWithMessages("jina-vlm", []Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &maxTokens, Temperature: &temperature, TopP: &topP, Stop: &stop},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestJinaChatValidation(t *testing.T) {
	j := newJinaForTest("http://unused")
	apiKey := "test-key"
	emptyKey := ""

	tests := []struct {
		name      string
		modelName string
		messages  []Message
		apiConfig *APIConfig
		want      string
	}{
		{
			name:      "missing api config",
			modelName: "jina-vlm",
			messages:  []Message{{Role: "user", Content: "x"}},
			want:      "api key is required",
		},
		{
			name:      "missing api key",
			modelName: "jina-vlm",
			messages:  []Message{{Role: "user", Content: "x"}},
			apiConfig: &APIConfig{},
			want:      "api key is required",
		},
		{
			name:      "empty api key",
			modelName: "jina-vlm",
			messages:  []Message{{Role: "user", Content: "x"}},
			apiConfig: &APIConfig{ApiKey: &emptyKey},
			want:      "api key is required",
		},
		{
			name:      "missing model",
			messages:  []Message{{Role: "user", Content: "x"}},
			apiConfig: &APIConfig{ApiKey: &apiKey},
			want:      "model name is required",
		},
		{
			name:      "missing messages",
			modelName: "jina-vlm",
			apiConfig: &APIConfig{ApiKey: &apiKey},
			want:      "messages is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := j.ChatWithMessages(tt.modelName, tt.messages, tt.apiConfig, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestJinaChatRejectsHTTPError(t *testing.T) {
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"invalid api key"}`))
	})
	defer srv.Close()

	j := newJinaForTest(srv.URL)
	apiKey := "test-key"
	_, err := j.ChatWithMessages("jina-vlm", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestJinaChatRejectsMalformedResponse(t *testing.T) {
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []map[string]interface{}{}})
	})
	defer srv.Close()

	j := newJinaForTest(srv.URL)
	apiKey := "test-key"
	_, err := j.ChatWithMessages("jina-vlm", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "no choices in response") {
		t.Errorf("expected malformed-response error, got %v", err)
	}
}

func TestJinaChatRejectsUnknownRegion(t *testing.T) {
	j := newJinaForTest("http://unused")
	apiKey := "test-key"
	region := "eu"
	_, err := j.ChatWithMessages("jina-vlm", []Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &region}, nil)
	if err == nil || !strings.Contains(err.Error(), "no base URL configured for region") {
		t.Errorf("expected region error, got %v", err)
	}
}

func TestJinaChatFallsBackToDefaultOnEmptyRegion(t *testing.T) {
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "ok"}}},
		})
	})
	defer srv.Close()

	j := newJinaForTest(srv.URL)
	apiKey := "test-key"
	emptyRegion := ""
	_, err := j.ChatWithMessages("jina-vlm", []Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &emptyRegion}, nil)
	if err != nil {
		t.Errorf("empty Region: expected fallback to default, got %v", err)
	}
}
