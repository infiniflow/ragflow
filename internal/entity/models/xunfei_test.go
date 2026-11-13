package models

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
