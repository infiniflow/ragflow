package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/common"
)

func newCohereForTest(baseURL string) *CoHereModel {
	return NewCoHereModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v2/chat", Embedding: "v2/embed", Rerank: "v2/rerank"},
	)
}

func TestCohereChatAllowsToolCallOnlyResponse(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/chat" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cohere-chat-tool",
			"message": map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{
					{
						"id":   "tool-1",
						"type": "function",
						"function": map[string]any{
							"name":      "lookup",
							"arguments": `{"q":"ragflow"}`,
						},
					},
				},
			},
			"usage": map[string]any{
				"tokens": map[string]any{
					"input_tokens":  4,
					"output_tokens": 2,
				},
			},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	usage := &common.ModelUsage{}
	resp, err := newCohereForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"command-a",
		[]Message{{Role: "user", Content: "use a tool"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		usage,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "" {
		t.Fatalf("Answer=%v, want empty content", resp.Answer)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0]["id"] != "tool-1" {
		t.Fatalf("ToolCalls=%#v, want tool-1", resp.ToolCalls)
	}
	assertModelUsage(t, usage, 4, 2, 6)
}

func TestCohereStreamRecordsDeltaUsage(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/chat" {
			t.Errorf("path=%s", r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if body["stream"] != true {
			t.Fatalf("stream=%v, want true", body["stream"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"type":"content-delta","delta":{"message":{"content":{"text":"hello"}}}}`+"\n"+
				`data: {"type":"message-end","delta":{"usage":{"tokens":{"input_tokens":4,"output_tokens":2}}}}`+"\n",
		)
	}))
	defer srv.Close()

	apiKey := "test-key"
	cfg := &ChatConfig{}
	usage := &common.ModelUsage{}
	var chunks []string
	err := newCohereForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"command-a",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		cfg,
		usage,
		func(content *string, reasoning *string) error {
			if content != nil {
				chunks = append(chunks, *content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(chunks, "") != "hello[DONE]" {
		t.Fatalf("chunks=%q, want hello[DONE]", strings.Join(chunks, ""))
	}
	if cfg.UsageResult == nil || cfg.UsageResult.PromptTokens != 4 || cfg.UsageResult.CompletionTokens != 2 || cfg.UsageResult.TotalTokens != 6 {
		t.Fatalf("UsageResult=%#v, want prompt=4 completion=2 total=6", cfg.UsageResult)
	}
	if usage.InputTokens != 4 || usage.OutputTokens != 2 || usage.TotalTokens != 6 {
		t.Fatalf("model usage=(%d,%d,%d), want (4,2,6)", usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	}
}

// TestCohereEmbedInputTypeFollowsQuery pins Python CoHereEmbed: encode sends
// input_type="search_document", encode_queries sends "search_query".
func TestCohereEmbedInputTypeFollowsQuery(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	baseURL, bodies := captureEmbedBodies(t, `{"embeddings":{"float":[[0.1]]}}`)
	m := newCohereForTest(baseURL)
	apiKey := "test-key"
	model := "embed-v4.0"

	if _, err := m.Embed(ctx, &model, EmbedRequest{Texts: []string{"a"}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if _, err := m.Embed(ctx, &model, EmbedRequest{Texts: []string{"a"}, Query: true}, &APIConfig{ApiKey: &apiKey}, nil, nil); err != nil {
		t.Fatalf("Embed(query): %v", err)
	}
	if got := (*bodies)[0]["input_type"]; got != "search_document" {
		t.Errorf("document input_type = %v, want search_document", got)
	}
	if got := (*bodies)[1]["input_type"]; got != "search_query" {
		t.Errorf("query input_type = %v, want search_query", got)
	}
}

func TestCohereChatRequestUsesNativeSamplingAndStopFields(t *testing.T) {
	withSSRFBypass(t)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				for _, key := range []string{"top_p", "stop", "do_sample"} {
					if _, exists := body[key]; exists {
						http.Error(w, "unsupported parameter: "+key, http.StatusBadRequest)
						return
					}
				}
				if body["p"] != 0.8 || !reflect.DeepEqual(body["stop_sequences"], []any{"END"}) {
					t.Errorf("request parameters: %#v", body)
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					io.WriteString(w, "data: {\"type\":\"content-delta\",\"delta\":{\"message\":{\"content\":{\"text\":\"hello\"}}}}\n\ndata: {\"type\":\"message-end\"}\n\n")
					return
				}
				io.WriteString(w, `{"message":{"role":"assistant","content":[{"type":"text","text":"hello"}]}}`)
			}))
			defer srv.Close()
			key, topP, sample := "test-key", 0.8, true
			stop := []string{"END"}
			cfg := &ChatConfig{TopP: &topP, Stop: &stop, DoSample: &sample}
			driver := newCohereForTest(srv.URL)
			messages := []Message{{Role: "user", Content: "hello"}}
			api := &APIConfig{ApiKey: &key}
			if stream {
				var answer strings.Builder
				err := driver.ChatStreamlyWithSender(t.Context(), "command-a", messages, api, cfg, &common.ModelUsage{}, func(content, _ *string) error {
					if content != nil {
						answer.WriteString(*content)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if answer.String() != "hello[DONE]" {
					t.Fatalf("answer=%q", answer.String())
				}
			} else {
				resp, err := driver.ChatWithMessages(t.Context(), "command-a", messages, api, cfg, &common.ModelUsage{})
				if err != nil {
					t.Fatal(err)
				}
				if resp.Answer == nil || *resp.Answer != "hello" {
					t.Fatalf("response=%#v", resp)
				}
			}
			if topP != 0.8 || !sample || !reflect.DeepEqual(stop, []string{"END"}) {
				t.Fatal("caller config changed")
			}
		})
	}
}

func TestCohereRequestDefaultsAndProviderIsolation(t *testing.T) {
	messages := []Message{{Role: "user", Content: "hello"}}
	for _, cfg := range []*ChatConfig{nil, {}} {
		body := buildCohereRequestBody(cfg, "command-a", messages, false)
		if len(body) != 3 || body["model"] != "command-a" || body["stream"] != false {
			t.Fatalf("default body=%#v", body)
		}
	}
	topP, sample := 0.8, false
	stop := []string{"END"}
	cfg := &ChatConfig{TopP: &topP, Stop: &stop, DoSample: &sample}
	body := buildRequestBody(cfg, "other-provider", messages, true)
	if body["top_p"] != topP || body["do_sample"] != sample || !reflect.DeepEqual(body["stop"], stop) {
		t.Fatalf("shared provider body=%#v", body)
	}
	for _, key := range []string{"p", "stop_sequences"} {
		if _, exists := body[key]; exists {
			t.Fatalf("Cohere field %s leaked to shared builder", key)
		}
	}
}

// TestVoyageEmbedInputTypeFollowsQuery pins Python VoyageEmbed: encode sends
// input_type="document", encode_queries sends "query".
