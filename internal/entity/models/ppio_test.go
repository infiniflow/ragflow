package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"strings"
	"testing"
)

func newPPIOServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		if got := r.Header.Get("Content-Type"); r.Method != http.MethodGet && !strings.HasPrefix(got, "application/json") {
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

func newPPIOForTest(baseURL string) *PPIOModel {
	return NewPPIOModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings", Rerank: "rerank"},
	)
}

func TestPPIOName(t *testing.T) {
	if got := newPPIOForTest("http://unused").Name(); got != "ppio" {
		t.Errorf("Name()=%q", got)
	}
}

func TestPPIOFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("PPIO", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*PPIOModel); !ok {
		t.Fatalf("driver type=%T, want *PPIOModel", driver)
	}
}

func TestPPIONewModelWithCustomDefaultTransport(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = original
	})

	if model := NewPPIOModel(map[string]string{"default": "http://unused"}, URLSuffix{}); model == nil {
		t.Fatal("NewPPIOModel returned nil")
	}
}

func TestPPIOChatHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "deepseek/deepseek-r1" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Errorf("reasoning_effort should not be sent: %v", body["reasoning_effort"])
		}
		if body["temperature"] != 0.3 {
			t.Errorf("temperature=%v", body["temperature"])
		}
		if body["top_p"] != 0.9 {
			t.Errorf("top_p=%v", body["top_p"])
		}
		stop, ok := body["stop"].([]interface{})
		if !ok || len(stop) != 1 || stop[0] != "END" {
			t.Errorf("stop=%#v", body["stop"])
		}
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) != 1 {
			t.Fatalf("messages=%#v", body["messages"])
		}
		first, ok := messages[0].(map[string]interface{})
		if !ok {
			t.Fatalf("message type=%T", messages[0])
		}
		if first["role"] != "user" || first["content"] != "ping" {
			t.Errorf("message=%#v", first)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chat-ppio",
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "thinking",
				},
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     3,
				"completion_tokens": 5,
				"total_tokens":      8,
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	mt := 32
	temp := 0.3
	topP := 0.9
	stop := []string{"END"}
	effort := "high"
	usage := &common.ModelUsage{}
	resp, err := newPPIOForTest(srv.URL).ChatWithMessages(
		ctx,
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt, Temperature: &temp, TopP: &topP, Stop: &stop, Effort: &effort},
		usage,
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
	if resp.Usage == nil || resp.Usage.PromptTokens != 3 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 8 {
		t.Fatalf("Usage=%#v, want prompt=3 completion=5 total=8", resp.Usage)
	}
	assertModelUsage(t, usage, 3, 5, 8)
}

func TestPPIOChatRequiresModelName(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	_, err := newPPIOForTest("http://unused").ChatWithMessages(ctx, "", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestPPIOChatRequiresMessages(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	_, err := newPPIOForTest("http://unused").ChatWithMessages(ctx, "deepseek/deepseek-r1", nil, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages error, got %v", err)
	}
}

func TestPPIOChatSurfacesHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newPPIOForTest(srv.URL).ChatWithMessages(ctx, "deepseek/deepseek-r1", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected HTTP status error, got %v", err)
	}
}

func TestPPIOChatRejectsProviderError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "invalid model"},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newPPIOForTest(srv.URL).ChatWithMessages(ctx, "deepseek/deepseek-r1", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "upstream error") {
		t.Errorf("expected upstream error, got %v", err)
	}
}

func TestPPIOStreamHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v want true", body["stream"])
		}
		streamOptions, ok := body["stream_options"].(map[string]interface{})
		if !ok || streamOptions["include_usage"] != true {
			t.Errorf("stream_options=%#v, want include_usage=true", body["stream_options"])
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Errorf("Accept=%q, want text/event-stream", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"think "}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n"+
				`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
				`data: {"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`+"\n",
		)
	})
	defer srv.Close()

	apiKey := "test-key"
	var content []string
	var reasoning []string
	cfg := &ChatConfig{}
	usage := &common.ModelUsage{}
	err := newPPIOForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, cfg, usage,
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
	if len(content) == 0 || content[len(content)-1] != "[DONE]" {
		t.Errorf("final content sentinel missing: %#v", content)
	}
	if cfg.UsageResult == nil || cfg.UsageResult.PromptTokens != 3 || cfg.UsageResult.CompletionTokens != 5 || cfg.UsageResult.TotalTokens != 8 {
		t.Fatalf("UsageResult=%#v, want prompt=3 completion=5 total=8", cfg.UsageResult)
	}
	if usage.InputTokens != 3 || usage.OutputTokens != 5 || usage.TotalTokens != 8 {
		t.Fatalf("stream usage=(%d,%d,%d), want (3,5,8)", usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	}
}

func TestPPIOEmbedRecordsUsage(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "embedding-model" {
			t.Errorf("model=%v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "embed-ppio",
			"data": []map[string]interface{}{{
				"embedding": []float64{0.1, 0.2},
				"index":     0,
			}},
			"usage": map[string]interface{}{
				"prompt_tokens": 7,
				"total_tokens":  7,
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	modelName := "embedding-model"
	usage := &common.ModelUsage{}
	embeddings, err := newPPIOForTest(srv.URL).Embed(ctx, &modelName, []string{"document"}, &APIConfig{ApiKey: &apiKey}, nil, usage)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 1 || len(embeddings[0].Embedding) != 2 {
		t.Fatalf("embeddings=%#v", embeddings)
	}
	assertModelUsage(t, usage, 7, 0, 7)
	if usage.Type != "embedding" {
		t.Fatalf("usage type=%q, want embedding", usage.Type)
	}
}

func TestPPIORerankRecordsUsage(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/rerank" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "rerank-model" {
			t.Errorf("model=%v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "rerank-ppio",
			"results": []map[string]interface{}{{
				"index":           0,
				"relevance_score": 0.9,
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     7,
				"completion_tokens": 2,
				"total_tokens":      9,
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	modelName := "rerank-model"
	usage := &common.ModelUsage{}
	reranked, err := newPPIOForTest(srv.URL).Rerank(ctx, &modelName, "query", []string{"document"}, &APIConfig{ApiKey: &apiKey}, &RerankConfig{TopN: 1}, usage)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(reranked.Data) != 1 || reranked.Data[0].Index != 0 {
		t.Fatalf("reranked=%#v", reranked)
	}
	assertModelUsage(t, usage, 7, 2, 9)
	if usage.Type != "rerank" {
		t.Fatalf("usage type=%q, want rerank", usage.Type)
	}
}

func TestPPIOStreamSurfacesHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newPPIOForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected HTTP status error, got %v", err)
	}
}

func TestPPIOStreamStopsOnSenderError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newPPIOForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(*string, *string) error { return io.ErrUnexpectedEOF },
	)
	if err == nil || !strings.Contains(err.Error(), "unexpected EOF") {
		t.Errorf("expected sender error, got %v", err)
	}
}

func TestPPIOStreamRejectsExplicitFalse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	stream := false
	err := newPPIOForTest("http://unused").ChatStreamlyWithSender(
		ctx,
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Errorf("expected stream guard, got %v", err)
	}
}

func TestPPIOStreamRequiresSender(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	err := newPPIOForTest("http://unused").ChatStreamlyWithSender(
		ctx,
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender error, got %v", err)
	}
}

func TestPPIOStreamRequiresTerminalEvent(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n")
	})
	defer srv.Close()

	apiKey := "test-key"
	err := newPPIOForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"deepseek/deepseek-r1",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(*string, *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected unterminated stream error, got %v", err)
	}
}

func TestPPIOListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "deepseek/deepseek-r1"},
				{"id": "qwen/qwen-2.5-72b-instruct"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	model := newPPIOForTest(srv.URL)
	models, err := model.ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(models, ",") != "deepseek/deepseek-r1,qwen/qwen-2.5-72b-instruct" {
		t.Errorf("models=%v", models)
	}
	if err := model.CheckConnection(ctx, &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

func TestPPIOListModelsRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	_, err := newPPIOForTest("http://unused").ListModels(ctx, &APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestPPIOListModelsRejectsProviderError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newPPIOServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "unauthorized"},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newPPIOForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "upstream error") {
		t.Errorf("expected upstream error, got %v", err)
	}
}

func TestPPIOEndpointTrimsTrailingSlash(t *testing.T) {
	model := NewPPIOModel(map[string]string{"default": "https://example.test/base/"}, URLSuffix{Chat: "/chat/completions"})
	apiKey := "test-key"
	endpoint, err := model.endpoint(&APIConfig{ApiKey: &apiKey}, model.baseModel.URLSuffix.Chat)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://example.test/base/chat/completions" {
		t.Errorf("endpoint=%q", endpoint)
	}
}

func TestPPIODefaultEndpointUsesPPIOAPI(t *testing.T) {
	model := NewPPIOModel(map[string]string{"default": "https://api.ppio.com/openai/v1"}, URLSuffix{Chat: "chat/completions"})
	apiKey := "test-key"
	endpoint, err := model.endpoint(&APIConfig{ApiKey: &apiKey}, model.baseModel.URLSuffix.Chat)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://api.ppio.com/openai/v1/chat/completions" {
		t.Errorf("endpoint=%q", endpoint)
	}
}

func TestPPIOEmptyRegionCustomBaseURL(t *testing.T) {
	model := NewPPIOModel(map[string]string{"": "https://custom.example/openai/v1"}, URLSuffix{Models: "models"})
	apiKey := "test-key"
	region := ""
	endpoint, err := model.endpoint(&APIConfig{ApiKey: &apiKey, Region: &region}, model.baseModel.URLSuffix.Models)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://custom.example/openai/v1/models" {
		t.Errorf("endpoint=%q", endpoint)
	}
}

func TestPPIONamedRegionBaseURL(t *testing.T) {
	model := NewPPIOModel(map[string]string{
		"default": "https://api.ppio.com/openai/v1",
		"us":      "https://api.ppinfra.com/v3/openai",
	}, URLSuffix{Chat: "chat/completions"})
	apiKey := "test-key"
	region := "us"
	endpoint, err := model.endpoint(&APIConfig{ApiKey: &apiKey, Region: &region}, model.baseModel.URLSuffix.Chat)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if endpoint != "https://api.ppinfra.com/v3/openai/chat/completions" {
		t.Errorf("endpoint=%q", endpoint)
	}
}

func TestPPIOMissingRegionBaseURL(t *testing.T) {
	model := NewPPIOModel(map[string]string{"default": "https://api.ppinfra.com/v3/openai"}, URLSuffix{Models: "models"})
	apiKey := "test-key"
	region := "missing"
	_, err := model.endpoint(&APIConfig{ApiKey: &apiKey, Region: &region}, model.baseModel.URLSuffix.Models)
	if err == nil || !strings.Contains(err.Error(), "no base URL configured") {
		t.Errorf("expected base URL error, got %v", err)
	}
}

func TestPPIOUnsupportedMethods(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newPPIOForTest("http://unused")
	if _, err := m.Balance(ctx, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance error=%v", err)
	}
	if _, err := m.TranscribeAudio(ctx, nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio error=%v", err)
	}
	if err := m.TranscribeAudioWithSender(ctx, nil, nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudioWithSender error=%v", err)
	}
	if _, err := m.AudioSpeech(ctx, nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech error=%v", err)
	}
	if err := m.AudioSpeechWithSender(ctx, nil, nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeechWithSender error=%v", err)
	}
	if _, err := m.OCRFile(ctx, nil, nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile error=%v", err)
	}
	if _, err := m.ParseFile(ctx, nil, nil, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ParseFile error=%v", err)
	}
	if _, err := m.ListTasks(ctx, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListTasks error=%v", err)
	}
	if _, err := m.ShowTask(ctx, "", nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ShowTask error=%v", err)
	}
}
