package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTogetherAIServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
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

func newTogetherAIForTest(baseURL string) *TogetherAIModel {
	return NewTogetherAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestTogetherAIName(t *testing.T) {
	if got := newTogetherAIForTest("http://unused").Name(); got != "togetherai" {
		t.Errorf("Name()=%q", got)
	}
}

func TestTogetherAIFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("TogetherAI", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*TogetherAIModel); !ok {
		t.Fatalf("driver type=%T, want *TogetherAIModel", driver)
	}
}

func TestTogetherAIChatHappyPath(t *testing.T) {
	srv := newTogetherAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "openai/gpt-oss-20b" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		if body["reasoning_effort"] != "high" {
			t.Errorf("reasoning_effort=%v", body["reasoning_effort"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":   "pong",
					"reasoning": "thinking",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	mt := 32
	temp := 0.3
	topP := 0.9
	stop := []string{"END"}
	effort := "high"
	resp, err := newTogetherAIForTest(srv.URL).ChatWithMessages(
		"openai/gpt-oss-20b",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop, Effort: &effort},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "thinking" {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

func TestTogetherAIChatForwardsReasoningEnabled(t *testing.T) {
	srv := newTogetherAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "Qwen/Qwen3.5-9B" {
			t.Errorf("model=%v", body["model"])
		}
		reasoning, ok := body["reasoning"].(map[string]interface{})
		if !ok {
			t.Fatalf("reasoning=%T, want object", body["reasoning"])
		}
		if reasoning["enabled"] != false {
			t.Errorf("reasoning.enabled=%v, want false", reasoning["enabled"])
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Errorf("reasoning_effort should not be sent for non-GPT-OSS model: %v", body["reasoning_effort"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": "pong",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	thinking := false
	resp, err := newTogetherAIForTest(srv.URL).ChatWithMessages(
		"Qwen/Qwen3.5-9B",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
}

func TestTogetherAIChatRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newTogetherAIForTest("http://unused").ChatWithMessages("", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestTogetherAIStreamHappyPath(t *testing.T) {
	srv := newTogetherAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v want true", body["stream"])
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept=%q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning":"think "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content []string
	var reasoning []string
	err := newTogetherAIForTest(srv.URL).ChatStreamlyWithSender(
		"meta-llama/Llama-3.3-70B-Instruct-Turbo",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, r *string) error {
			if c != nil {
				content = append(content, *c)
			}
			if r != nil {
				reasoning = append(reasoning, *r)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(content, "") != "Hello world[DONE]" {
		t.Errorf("content=%q", strings.Join(content, ""))
	}
	if strings.Join(reasoning, "") != "think " {
		t.Errorf("reasoning=%q", strings.Join(reasoning, ""))
	}
}

func TestTogetherAIStreamStopsOnRootFinishReason(t *testing.T) {
	srv := newTogetherAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"Done"}}],"finish_reason":"stop"}`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var chunks []string
	err := newTogetherAIForTest(srv.URL).ChatStreamlyWithSender(
		"meta-llama/Llama-3.3-70B-Instruct-Turbo",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil,
		func(c *string, _ *string) error {
			if c != nil {
				chunks = append(chunks, *c)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(chunks, "") != "Done[DONE]" {
		t.Errorf("chunks=%q", strings.Join(chunks, ""))
	}
}

func TestTogetherAIListModelsAndCheckConnection(t *testing.T) {
	srv := newTogetherAIServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "openai/gpt-oss-20b"},
			{"id": "meta-llama/Llama-3.3-70B-Instruct-Turbo"},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := newTogetherAIForTest(srv.URL)
	models, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if strings.Join(models, ",") != "openai/gpt-oss-20b,meta-llama/Llama-3.3-70B-Instruct-Turbo" {
		t.Errorf("models=%v", models)
	}
	if err := model.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestTogetherAIUnsupportedMethods(t *testing.T) {
	m := newTogetherAIForTest("http://unused")
	if _, err := m.Embed(nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed error=%v", err)
	}
	if _, err := m.Rerank(nil, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank error=%v", err)
	}
	if _, err := m.Balance(nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance error=%v", err)
	}
}
