package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveSparkModel(t *testing.T) {
	cases := map[string]string{
		"Spark-Max":       "generalv3.5",
		"Spark-Max-32K":   "max-32k",
		"Spark-Lite":      "lite",
		"Spark-Pro":       "generalv3",
		"Spark-Pro-128K":  "pro-128k",
		"Spark-4.0-Ultra": "4.0Ultra",
		// Unknown names pass through unchanged (e.g. "spark-x").
		"spark-x": "spark-x",
	}
	for name, want := range cases {
		if got := resolveSparkModel(name); got != want {
			t.Errorf("resolveSparkModel(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestResolveBearerToken(t *testing.T) {
	bundle := `{"spark_api_password":"pwd","spark_app_id":"app","spark_api_secret":"secret","spark_api_key":"key"}`
	cases := []struct {
		name string
		key  *string
		want string
	}{
		{"nil key", nil, ""},
		{"plain key", strPtr("sk-plain"), "sk-plain"},
		{"bundle uses password", strPtr(bundle), "pwd"},
		{"bundle without password falls back to raw", strPtr(`{"spark_app_id":"app"}`), `{"spark_app_id":"app"}`},
		{"malformed json falls back to raw", strPtr(`{not-json`), `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveBearerToken(&APIConfig{ApiKey: tc.key})
			if got != tc.want {
				t.Errorf("resolveBearerToken() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestXunFeiCheckConnectionRequiresAPIKey(t *testing.T) {
	driver := NewXunFeiModel(map[string]string{"default": "http://unused"}, URLSuffix{}).
		NewInstance(map[string]string{"default": "http://unused"})
	err := driver.CheckConnection(t.Context(), &APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("CheckConnection with empty key = %v, want 'api key is required'", err)
	}
}

func strPtr(s string) *string { return &s }

func TestXunFeiUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	driver := NewXunFeiModel(map[string]string{"default": "http://unused"}, URLSuffix{}).
		NewInstance(map[string]string{"default": "http://unused"})
	modelName := "spark"
	text := "hello"

	checks := []struct {
		name string
		call func() error
	}{
		{"Embed", func() error {
			_, err := driver.Embed(ctx, &modelName, EmbedRequest{Texts: []string{text}}, &APIConfig{}, nil, nil)
			return err
		}},
		{"Rerank", func() error {
			_, err := driver.Rerank(ctx, &modelName, RerankRequest{Query: text, Documents: []string{text}}, &APIConfig{}, nil, nil)
			return err
		}},
		{"TranscribeAudio", func() error {
			_, err := driver.TranscribeAudio(ctx, &modelName, &text, &APIConfig{}, nil, nil)
			return err
		}},
		{"TranscribeAudioWithSender", func() error {
			return driver.TranscribeAudioWithSender(ctx, &modelName, &text, &APIConfig{}, nil, nil, nil)
		}},
		{"AudioSpeech", func() error {
			_, err := driver.AudioSpeech(ctx, &modelName, &text, &APIConfig{}, nil, nil)
			return err
		}},
		{"AudioSpeechWithSender", func() error {
			return driver.AudioSpeechWithSender(ctx, &modelName, &text, &APIConfig{}, nil, nil, nil)
		}},
		{"OCRFile", func() error {
			_, err := driver.OCRFile(ctx, &modelName, nil, &text, &APIConfig{}, nil, nil)
			return err
		}},
		{"ParseFile", func() error {
			_, err := driver.ParseFile(ctx, &modelName, nil, &text, &APIConfig{}, nil, nil)
			return err
		}},
		{"Balance", func() error {
			_, err := driver.Balance(ctx, &APIConfig{})
			return err
		}},
		{"ListTasks", func() error {
			_, err := driver.ListTasks(ctx, &APIConfig{})
			return err
		}},
		{"ShowTask", func() error {
			_, err := driver.ShowTask(ctx, "task-id", &APIConfig{})
			return err
		}},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			requireNoSuchMethod(t, check.name, check.call())
		})
	}
}

func newXunFeiForTest(baseURL string) *XunFeiModel {
	return NewXunFeiModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/chat/completions", Models: "v1/models"},
	)
}

func TestXunFeiChatUsesResolvedBearerToken(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
		}
		// The Spark credential bundle is stored as JSON; the request must
		// authenticate with the extracted spark_api_password.
		if got := r.Header.Get("Authorization"); got != "Bearer pwd" {
			t.Errorf("Authorization=%q, want Bearer pwd", got)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if body["model"] != "lite" {
			t.Errorf("model=%v, want lite (resolved Spark-Lite)", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v, want false", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "\nthought",
				},
			}},
		})
	}))
	defer srv.Close()

	bundle := `{"spark_api_password":"pwd","spark_app_id":"app","spark_api_secret":"secret","spark_api_key":"key"}`
	resp, err := newXunFeiForTest(srv.URL).ChatWithMessages(
		ctx,
		"Spark-Lite",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &bundle},
		nil,
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

func TestXunFeiStreamHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-plain" {
			t.Errorf("Authorization=%q, want Bearer sk-plain", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"choices":[{"delta":{"reasoning_content":"step "}}]}`,
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":" world"},"finish_reason":"stop"}]}`,
			// XunFei carries usage in the final chunk without requiring
			// stream_options.include_usage.
			`data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`,
			`data: [DONE]`,
			``,
		}, "\n"))
	}))
	defer srv.Close()

	apiKey := "sk-plain"
	var content, reasoning []string
	config := &ChatConfig{}
	err := newXunFeiForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"Spark-Lite",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		config,
		nil,
		func(answer, reason *string) error {
			if answer != nil {
				content = append(content, *answer)
			}
			if reason != nil {
				reasoning = append(reasoning, *reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(reasoning, "") != "step " {
		t.Errorf("reasoning=%q", strings.Join(reasoning, ""))
	}
	if got := strings.Join(content, ""); got != "Hello world[DONE]" {
		t.Errorf("content=%q, want Hello world[DONE]", got)
	}
	if config.UsageResult == nil || config.UsageResult.TotalTokens != 8 {
		t.Errorf("UsageResult=%#v, want total tokens 8", config.UsageResult)
	}
}
