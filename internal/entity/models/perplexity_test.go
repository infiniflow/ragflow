package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newPerplexityServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
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

func newPerplexityForTest(baseURL string) *PerplexityModel {
	return NewPerplexityModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/sonar", Embedding: "v1/embeddings", Models: "v1/models"},
	)
}

func TestPerplexityName(t *testing.T) {
	if got := newPerplexityForTest("http://unused").Name(); got != "perplexity" {
		t.Errorf("Name()=%q", got)
	}
}

func TestPerplexityFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Perplexity", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*PerplexityModel); !ok {
		t.Fatalf("driver type=%T, want *PerplexityModel", driver)
	}
}

func TestPerplexityChatHappyPath(t *testing.T) {
	srv := newPerplexityServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/sonar" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "sonar-reasoning-pro" {
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
	resp, err := newPerplexityForTest(srv.URL).ChatWithMessages(
		"sonar-reasoning-pro",
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

func TestPerplexityChatSkipsReasoningEffortForNonReasoningModel(t *testing.T) {
	srv := newPerplexityServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "sonar" {
			t.Errorf("model=%v", body["model"])
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Errorf("reasoning_effort should not be sent for non-reasoning model: %v", body["reasoning_effort"])
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
	effort := "high"
	resp, err := newPerplexityForTest(srv.URL).ChatWithMessages(
		"sonar",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Effort: &effort},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
}

func TestPerplexityChatRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newPerplexityForTest("http://unused").ChatWithMessages("", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestPerplexityChatRequiresApiKey(t *testing.T) {
	_, err := newPerplexityForTest("http://unused").ChatWithMessages("sonar", []Message{{Role: "user", Content: "x"}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestPerplexityStreamHappyPath(t *testing.T) {
	srv := newPerplexityServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/sonar" {
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
	err := newPerplexityForTest(srv.URL).ChatStreamlyWithSender(
		"sonar",
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

func TestPerplexityStreamStopsOnDoneMarker(t *testing.T) {
	srv := newPerplexityServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"content":"Done"}}]}`+"\n"+
				`data: [DONE]`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var chunks []string
	err := newPerplexityForTest(srv.URL).ChatStreamlyWithSender(
		"sonar",
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

func TestPerplexityListModelsAndCheckConnection(t *testing.T) {
	srv := newPerplexityServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "sonar"},
				{"id": "sonar-pro"},
				{"id": "pplx-embed-v1-0.6b"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := newPerplexityForTest(srv.URL)
	models, err := model.ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "sonar,sonar-pro,pplx-embed-v1-0.6b" {
		t.Errorf("models=%v", models)
	}
	if err := model.CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestPerplexityListModelsAcceptsBareArray(t *testing.T) {
	srv := newPerplexityServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "sonar"},
			{"id": "sonar-pro"},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newPerplexityForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "sonar,sonar-pro" {
		t.Errorf("models=%v", models)
	}
}

func TestPerplexityEmbedHappyPath(t *testing.T) {
	srv := newPerplexityServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "pplx-embed-v1-0.6b" {
			t.Errorf("model=%v", body["model"])
		}
		inputs, ok := body["input"].([]interface{})
		if !ok || len(inputs) != 2 {
			t.Fatalf("input=%v", body["input"])
		}
		if body["dimensions"] != float64(16) {
			t.Errorf("dimensions=%v", body["dimensions"])
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
	modelName := "pplx-embed-v1-0.6b"
	out, err := newPerplexityForTest(srv.URL).Embed(
		&modelName,
		[]string{"hello", "world"},
		&APIConfig{ApiKey: &apiKey},
		&EmbeddingConfig{Dimension: 16},
	)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out)=%d", len(out))
	}
	if out[0].Index != 0 || out[1].Index != 1 {
		t.Errorf("indices=%v,%v", out[0].Index, out[1].Index)
	}
	if len(out[0].Embedding) != 2 || out[0].Embedding[0] != 0.1 {
		t.Errorf("out[0].Embedding=%v", out[0].Embedding)
	}
}

func TestPerplexityEmbedEmptyTextsReturnsEmpty(t *testing.T) {
	modelName := "pplx-embed-v1-0.6b"
	apiKey := "test-key"
	out, err := newPerplexityForTest("http://unused").Embed(&modelName, nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestPerplexityEmbedRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newPerplexityForTest("http://unused").Embed(nil, []string{"x"}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestPerplexityUnsupportedMethods(t *testing.T) {
	m := newPerplexityForTest("http://unused")
	if _, err := m.Rerank(nil, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank error=%v", err)
	}
	if _, err := m.Balance(nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance error=%v", err)
	}
}
