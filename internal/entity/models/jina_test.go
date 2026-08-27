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
	withSSRFBypass(t)
	ctx := t.Context()
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
	resp, err := j.ChatWithMessages(ctx, "jina-vlm", []Message{{Role: "user", Content: "ping"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
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

func TestJinaChatStreamSeparatesThinkingContent(t *testing.T) {
	withSSRFBypass(t)
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		if _, ok := body["stream_options"]; ok {
			t.Error("stream_options should not be sent to Jina")
		}
		_, _ = io.WriteString(w, "data: {\"model\":\"jina-vlm\",\"choices\":[{\"delta\":{\"content\":\"reasoning\",\"type\":\"think\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"</thi\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"nk>\",\"type\":\"think\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\",\"type\":\"think\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	defer srv.Close()

	var content, reasoning strings.Builder
	apiKey := "test-key"
	err := newJinaForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(), "jina-vlm", []Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(delta, reason *string) error {
			if delta != nil && *delta != "[DONE]" {
				content.WriteString(*delta)
			}
			if reason != nil {
				reasoning.WriteString(*reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if got := reasoning.String(); got != "reasoning" {
		t.Errorf("reasoning=%q, want %q", got, "reasoning")
	}
	if got := content.String(); got != "answer" {
		t.Errorf("content=%q, want %q", got, "answer")
	}
}

func TestJinaChatStreamAcceptsNDJSONAndCleanEOF(t *testing.T) {
	withSSRFBypass(t)
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_, _ = io.WriteString(w, "{\"model\":\"jina-vlm\",\"choices\":[{\"delta\":{\"content\":\"plan\",\"type\":\"think\"}}]}\n")
		_, _ = io.WriteString(w, "{\"model\":\"jina-vlm\",\"choices\":[{\"delta\":{\"content\":\"</think>\"}}]}\n")
		_, _ = io.WriteString(w, "{\"model\":\"jina-vlm\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n")
	})
	defer srv.Close()

	var content, reasoning strings.Builder
	apiKey := "test-key"
	err := newJinaForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(), "jina-vlm", []Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(delta, reason *string) error {
			if delta != nil && *delta != "[DONE]" {
				content.WriteString(*delta)
			}
			if reason != nil {
				reasoning.WriteString(*reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if got := reasoning.String(); got != "plan" {
		t.Errorf("reasoning=%q, want %q", got, "plan")
	}
	if got := content.String(); got != "hello" {
		t.Errorf("content=%q, want %q", got, "hello")
	}
}

func TestParseJinaStreamAcceptsMultilineSSEData(t *testing.T) {
	var events []map[string]any
	done, count, err := parseJinaStream(strings.NewReader("data: {\"choices\":\n"+
		"data: [{\"delta\":{\"content\":\"hello\"}}]}\n\n"), func(event map[string]any) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("parseJinaStream: %v", err)
	}
	if done || count != 1 || len(events) != 1 {
		t.Fatalf("done=%v count=%d events=%d, want false/1/1", done, count, len(events))
	}
}

func TestJinaThinkingFlushPreservesUTF8(t *testing.T) {
	var reasoning strings.Builder
	err := handleJinaStreamingResponse(strings.NewReader("{"+
		"\"choices\":[{\"delta\":{\"content\":\"思考过程很长\",\"type\":\"think\"}}]}\n"), nil, nil,
		func(_ *string, reason *string) error {
			if reason != nil {
				reasoning.WriteString(*reason)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("handleJinaStreamingResponse: %v", err)
	}
	if !strings.Contains(reasoning.String(), "思考过程很长") {
		t.Errorf("reasoning=%q, want valid UTF-8 content", reasoning.String())
	}
}

func TestJinaChatStreamForwardsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"detail":{"message":"roles must alternate"}}`)
	})
	defer srv.Close()

	var chunks []string
	apiKey := "test-key"
	err := newJinaForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(), "jina-vlm", []Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(content, _ *string) error {
			if content != nil {
				chunks = append(chunks, *content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks=%q, want error and done chunks", chunks)
	}
	if !strings.Contains(chunks[0], `**ERROR**: API request failed with status 400`) || !strings.Contains(chunks[0], `roles must alternate`) {
		t.Errorf("error chunk=%q", chunks[0])
	}
	if chunks[1] != "[DONE]" {
		t.Errorf("terminal chunk=%q, want [DONE]", chunks[1])
	}
}

func TestPrepareJinaMessagesMergesSystemOnlyForAPIEndpoint(t *testing.T) {
	messages := []Message{
		{Role: "system", Content: "be helpful"},
		{Role: "user", Content: "hello"},
	}

	apiMessages := prepareJinaMessages("https://api.jina.ai/v1", messages)
	if len(apiMessages) != 1 || apiMessages[0].Role != "user" || apiMessages[0].Content != "be helpful\n\nhello" {
		t.Errorf("api.jina.ai messages=%#v", apiMessages)
	}
	deepSearchMessages := prepareJinaMessages("https://deepsearch.jina.ai/v1", messages)
	if len(deepSearchMessages) != 2 || deepSearchMessages[0].Role != "system" || deepSearchMessages[1].Role != "user" {
		t.Errorf("deepsearch.jina.ai messages=%#v", deepSearchMessages)
	}
}

func TestPrepareJinaMessagesPreservesMultimodalUserContent(t *testing.T) {
	image := map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,x"}}
	messages := prepareJinaMessages("https://api.jina.ai/v1", []Message{
		{Role: "system", Content: "describe the image"},
		{Role: "user", Content: []interface{}{image}},
	})
	parts, ok := messages[0].Content.([]interface{})
	if !ok || len(parts) != 2 {
		t.Fatalf("content=%#v, want text and image parts", messages[0].Content)
	}
	textPart, ok := parts[0].(map[string]interface{})
	if !ok || textPart["type"] != "text" || textPart["text"] != "describe the image" {
		t.Errorf("text part=%#v", parts[0])
	}
	if parts[1].(map[string]interface{})["type"] != "image_url" {
		t.Errorf("image part=%#v", parts[1])
	}
}

func TestJinaChatPreservesReasoningContent(t *testing.T) {
	withSSRFBypass(t)
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "jina-chat",
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "answer",
					"reasoning_content": "\nthought",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	thinking := true
	response, err := newJinaForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"jina-vlm",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if response.ReasonContent == nil || *response.ReasonContent != "thought" {
		t.Fatalf("ReasonContent=%v, want thought", response.ReasonContent)
	}
}

func TestJinaChatSupportsToolCalls(t *testing.T) {
	testNonStreamingToolCall(t, "jina-vlm", "/chat/completions", func(baseURL string) ModelDriver {
		return newJinaForTest(baseURL)
	})
}

func TestJinaChatPropagatesConfig(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
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
	_, err := j.ChatWithMessages(ctx, "jina-vlm", []Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &maxTokens, Temperature: &temperature, TopP: &topP, Stop: &stop},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
}

func TestJinaChatValidation(t *testing.T) {
	withSSRFBypass(t)
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
			ctx := t.Context()
			_, err := j.ChatWithMessages(ctx, tt.modelName, tt.messages, tt.apiConfig, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestJinaEmbedMeanPoolsMultivectorResponse(t *testing.T) {
	withSSRFBypass(t)
	srv := newJinaServer(t, "/embeddings", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{{
				"embeddings": [][]float64{{1, 3}, {3, 5}},
				"index":      0,
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	modelName := "jina-embeddings-v4"
	embeddings, err := newJinaForTest(srv.URL).Embed(
		t.Context(),
		&modelName,
		EmbedRequest{Texts: []string{"text"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 1 || len(embeddings[0].Embedding) != 2 || embeddings[0].Embedding[0] != 2 || embeddings[0].Embedding[1] != 4 {
		t.Fatalf("embeddings=%v, want [[2 4]]", embeddings)
	}
}

func TestJinaChatRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"invalid api key"}`))
	})
	defer srv.Close()

	j := newJinaForTest(srv.URL)
	apiKey := "test-key"
	_, err := j.ChatWithMessages(ctx, "jina-vlm", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestJinaChatRejectsMalformedResponse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"choices": []map[string]interface{}{}})
	})
	defer srv.Close()

	j := newJinaForTest(srv.URL)
	apiKey := "test-key"
	_, err := j.ChatWithMessages(ctx, "jina-vlm", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no choices in response") {
		t.Errorf("expected malformed-response error, got %v", err)
	}
}

func TestJinaChatRejectsUnknownRegion(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	j := newJinaForTest("http://unused")
	apiKey := "test-key"
	region := "eu"
	_, err := j.ChatWithMessages(ctx, "jina-vlm", []Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &region}, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "no base URL configured for region") {
		t.Errorf("expected region error, got %v", err)
	}
}

func TestJinaChatFallsBackToDefaultOnEmptyRegion(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newJinaServer(t, "/chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]interface{}{"content": "ok"}}},
		})
	})
	defer srv.Close()

	j := newJinaForTest(srv.URL)
	apiKey := "test-key"
	emptyRegion := ""
	_, err := j.ChatWithMessages(ctx, "jina-vlm", []Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey, Region: &emptyRegion}, nil, nil,
	)
	if err != nil {
		t.Errorf("empty Region: expected fallback to default, got %v", err)
	}
}

func TestJinaRerankDefaultsTopNToDocumentCount(t *testing.T) {
	withSSRFBypass(t)
	srv := newJinaServer(t, "/rerank", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["top_n"] != float64(2) {
			t.Errorf("top_n=%v, want 2", body["top_n"])
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	modelName := "jina-reranker-v3"
	_, err := newJinaForTest(srv.URL).Rerank(
		t.Context(),
		&modelName,
		RerankRequest{Query: "weather", Documents: []string{"sunny", "rainy"}},
		&APIConfig{ApiKey: &apiKey},
		&RerankConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
}
