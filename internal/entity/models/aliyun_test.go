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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAliyunChatWithMessagesSupportsToolCalls(t *testing.T) {
	withSSRFBypass(t)
	requestBody := make(chan map[string]interface{}, 1)
	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestPath <- r.URL.Path
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"retrieval","arguments":"{\"query\":\"ragflow\"}"}}]}}]}`))
	}))
	defer server.Close()
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{Chat: "compatible-mode/v1/chat/completions"},
	)
	apiKey := "test-key"
	toolChoice := "auto"
	tools := []map[string]interface{}{{
		"type": "function",
		"function": map[string]interface{}{
			"name":       "retrieval",
			"parameters": map[string]interface{}{"type": "object"},
		},
	}}
	messages := []Message{{Role: "user", Content: "find ragflow"}}

	response, err := model.ChatWithMessages(
		ctx,
		"qwen-flash",
		messages,
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Tools: tools, ToolChoice: &toolChoice},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if len(response.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(response.ToolCalls))
	}
	if response.ToolCalls[0]["id"] != "call-1" {
		t.Errorf("tool call id = %v, want call-1", response.ToolCalls[0]["id"])
	}

	if got := <-requestPath; got != "/compatible-mode/v1/chat/completions" {
		t.Errorf("request path = %q, want /compatible-mode/v1/chat/completions", got)
	}
	body := <-requestBody
	if body["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v, want auto for initial qwen-flash call", body["tool_choice"])
	}
	if _, ok := body["tools"].([]interface{}); !ok {
		t.Fatalf("tools = %T, want JSON array", body["tools"])
	}
}

func TestAliyunChatWithMessagesStopsQwenFlashAfterToolResult(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	requestBody := make(chan map[string]interface{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"final answer"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{Chat: "compatible-mode/v1/chat/completions"},
	)
	apiKey := "test-key"
	auto := "auto"
	messages := []Message{
		{Role: "user", Content: "find ragflow"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []map[string]interface{}{{
				"id":   "previous-call",
				"type": "function",
				"function": map[string]interface{}{
					"name":      "retrieval",
					"arguments": `{"query":"ragflow"}`,
				},
			}},
		},
		{Role: "tool", Content: "retrieved text", ToolCallID: "previous-call"},
	}
	response, err := model.ChatWithMessages(
		ctx,
		"qwen-flash",
		messages,
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{
			Tools:      []map[string]interface{}{{"type": "function"}},
			ToolChoice: &auto,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if response.Answer == nil || *response.Answer != "final answer" {
		t.Fatalf("answer = %#v, want final answer", response.Answer)
	}

	body := <-requestBody
	if body["tool_choice"] != "none" {
		t.Fatalf("tool_choice = %v, want none after qwen-flash tool result", body["tool_choice"])
	}
	gotMessages, ok := body["messages"].([]interface{})
	if !ok || len(gotMessages) != 3 {
		t.Fatalf("messages = %T len=%d, want 3", body["messages"], len(gotMessages))
	}
	assistantMessage, _ := gotMessages[1].(map[string]interface{})
	if _, ok := assistantMessage["tool_calls"].([]interface{}); !ok {
		t.Errorf("assistant tool_calls = %T, want JSON array", assistantMessage["tool_calls"])
	}
	toolMessage, _ := gotMessages[2].(map[string]interface{})
	if toolMessage["tool_call_id"] != "previous-call" {
		t.Errorf("tool_call_id = %v, want previous-call", toolMessage["tool_call_id"])
	}
}

func TestAliyunToolChoiceStopsOnlyQwenFlashAfterToolResult(t *testing.T) {
	auto := "auto"
	required := "required"
	toolResult := []Message{{Role: "tool", Content: "result", ToolCallID: "call-1"}}

	tests := []struct {
		name       string
		model      string
		messages   []Message
		configured *string
		want       string
	}{
		{name: "qwen initial call", model: "qwen-flash", configured: &auto, want: "auto"},
		{name: "qwen after tool result", model: "qwen-flash", messages: toolResult, configured: &auto, want: "none"},
		{name: "versioned qwen after tool result", model: "qwen-flash-2025-07-28", messages: toolResult, configured: &auto, want: "none"},
		{name: "other aliyun model", model: "qwen-plus", messages: toolResult, configured: &auto, want: "auto"},
		{name: "explicit choice", model: "qwen-flash", messages: toolResult, configured: &required, want: "required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aliyunToolChoice(tt.model, tt.messages, tt.configured); got != tt.want {
				t.Fatalf("aliyunToolChoice() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAliyunChatStreamlyWithSenderSupportsToolCalls(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	requestBody := make(chan map[string]interface{}, 1)
	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestPath <- r.URL.Path
		requestBody <- body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call-2","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"retrieval","arguments":"{\"query\":\""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ragflow\"}"}}]},"finish_reason":"tool_calls"}]}

`))
	}))
	defer server.Close()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{Chat: "compatible-mode/v1/chat/completions"},
	)
	apiKey := "test-key"
	config := &ChatConfig{
		Tools: []map[string]interface{}{{
			"type": "function",
			"function": map[string]interface{}{
				"name":       "retrieval",
				"parameters": map[string]interface{}{"type": "object"},
			},
		}},
	}
	var streamed []string
	err := model.ChatStreamlyWithSender(
		ctx,
		"qwen-flash",
		[]Message{{Role: "user", Content: "find ragflow"}},
		&APIConfig{ApiKey: &apiKey},
		config,
		nil,
		func(content, reasoning *string) error {
			if reasoning != nil {
				return errors.New("unexpected reasoning content")
			}
			if content != nil {
				streamed = append(streamed, *content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}

	if got := <-requestPath; got != "/compatible-mode/v1/chat/completions" {
		t.Errorf("request path = %q, want /compatible-mode/v1/chat/completions", got)
	}
	body := <-requestBody
	if body["stream"] != true {
		t.Errorf("stream = %v, want true", body["stream"])
	}
	if body["tool_choice"] != "auto" {
		t.Errorf("tool_choice = %v, want auto", body["tool_choice"])
	}
	if _, ok := body["tools"].([]interface{}); !ok {
		t.Fatalf("tools = %T, want JSON array", body["tools"])
	}
	if len(streamed) != 1 || streamed[0] != "[DONE]" {
		t.Errorf("streamed content = %#v, want only [DONE]", streamed)
	}
	if config.ToolCallsResult == nil {
		t.Fatal("ToolCallsResult is nil")
	}
	toolCalls := *config.ToolCallsResult
	if len(toolCalls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(toolCalls))
	}
	if toolCalls[0]["id"] != "call-1" || toolCalls[1]["id"] != "call-2" {
		t.Fatalf("tool call order = [%v, %v], want [call-1, call-2]", toolCalls[0]["id"], toolCalls[1]["id"])
	}
	function, _ := toolCalls[0]["function"].(map[string]interface{})
	if function["name"] != "retrieval" {
		t.Errorf("function name = %v, want retrieval", function["name"])
	}
	if function["arguments"] != `{"query":"ragflow"}` {
		t.Errorf("function arguments = %v, want complete JSON", function["arguments"])
	}
}

func TestAliyunChatStreamlyWithSenderRejectsStreamFalse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	model := NewAliyunModel(
		map[string]string{"default": "https://dashscope.example"},
		URLSuffix{Chat: "compatible-mode/v1/chat/completions"},
	)
	apiKey := "test-key"
	stream := false
	err := model.ChatStreamlyWithSender(
		ctx,
		"qwen-flash",
		[]Message{{Role: "user", Content: "hello"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Stream: &stream},
		nil,
		func(_, _ *string) error { return nil },
	)
	if err == nil || err.Error() != "stream must be true in ChatStreamlyWithSender" {
		t.Fatalf("error = %v, want stream validation error", err)
	}
}

// newAliyunTTSTestServer stubs the DashScope multimodal-generation endpoint:
// POST returns a JSON body whose output.audio.url points back at the same
// server, GET returns the synthesized WAV bytes.
func newAliyunTTSTestServer(t *testing.T, requestBody chan<- map[string]interface{}, requestPath chan<- string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write([]byte("fake-wav-bytes"))
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if requestPath != nil {
			requestPath <- r.URL.Path
		}
		if requestBody != nil {
			requestBody <- body
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"output":{"audio":{"url":%q},"finish_reason":"stop"},"request_id":"req-1"}`, server.URL+"/audio.wav"); err != nil {
			t.Errorf("failed to write TTS response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

const aliyunTTSTestSuffix = "api/v1/services/aigc/multimodal-generation/generation"

func TestAliyunAudioSpeechSynthesizesViaNativeEndpoint(t *testing.T) {
	withSSRFBypass(t)
	requestBody := make(chan map[string]interface{}, 1)
	requestPath := make(chan string, 1)
	server := newAliyunTTSTestServer(t, requestBody, requestPath)
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{TTS: aliyunTTSTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-tts-flash"
	text := "你好，世界"

	response, err := model.AudioSpeech(
		ctx,
		&modelName,
		&text,
		&APIConfig{ApiKey: &apiKey},
		&TTSConfig{Format: "mp3"},
		nil,
	)
	if err != nil {
		t.Fatalf("AudioSpeech: %v", err)
	}
	if string(response.Audio) != "fake-wav-bytes" {
		t.Errorf("audio = %q, want fake-wav-bytes", string(response.Audio))
	}
	if response.MediaType != "audio/wav" {
		t.Errorf("media type = %q, want audio/wav", response.MediaType)
	}

	if got := <-requestPath; got != "/"+aliyunTTSTestSuffix {
		t.Errorf("request path = %q, want /%s", got, aliyunTTSTestSuffix)
	}
	body := <-requestBody
	if body["model"] != "qwen-tts-flash" {
		t.Errorf("model = %v, want qwen-tts-flash", body["model"])
	}
	input, ok := body["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("input = %T, want JSON object", body["input"])
	}
	if input["text"] != "你好，世界" {
		t.Errorf("input.text = %v, want 你好，世界", input["text"])
	}
	if input["voice"] != aliyunTTSDefaultVoice {
		t.Errorf("input.voice = %v, want default %s", input["voice"], aliyunTTSDefaultVoice)
	}
	if _, ok := input["language_type"]; ok {
		t.Errorf("language_type = %v, want omitted by default", input["language_type"])
	}
}

func TestAliyunAudioSpeechHonorsExplicitVoiceAndLanguage(t *testing.T) {
	withSSRFBypass(t)
	requestBody := make(chan map[string]interface{}, 1)
	server := newAliyunTTSTestServer(t, requestBody, nil)
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{TTS: aliyunTTSTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-tts-flash"
	text := "hello"

	if _, err := model.AudioSpeech(
		ctx,
		&modelName,
		&text,
		&APIConfig{ApiKey: &apiKey},
		&TTSConfig{Params: map[string]any{"voice": "Serena", "language_type": "English"}},
		nil,
	); err != nil {
		t.Fatalf("AudioSpeech: %v", err)
	}
	input := (<-requestBody)["input"].(map[string]interface{})
	if got := input["voice"]; got != "Serena" {
		t.Errorf("voice = %v, want Serena", got)
	}
	if got := input["language_type"]; got != "English" {
		t.Errorf("language_type = %v, want English", got)
	}
}

func TestAliyunAudioSpeechRejectsOversizedAudio(t *testing.T) {
	withSSRFBypass(t)
	oversized := make([]byte, aliyunTTSAudioMaxBytes+1)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(oversized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"output":{"audio":{"url":%q},"finish_reason":"stop"},"request_id":"req-1"}`, server.URL+"/audio.wav"); err != nil {
			t.Errorf("failed to write TTS response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{TTS: aliyunTTSTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-tts-flash"
	text := "hello"

	_, err := model.AudioSpeech(
		ctx,
		&modelName,
		&text,
		&APIConfig{ApiKey: &apiKey},
		&TTSConfig{Format: "mp3"},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want oversized audio error", err)
	}
}

func TestAliyunAudioSpeechSurfacesAPIError(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"code":"InvalidApiKey","message":"Invalid API-key provided."}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{TTS: aliyunTTSTestSuffix},
	)
	apiKey := "bad-key"
	modelName := "qwen-tts-flash"
	text := "hello"

	_, err := model.AudioSpeech(ctx, &modelName, &text, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil {
		t.Fatal("error = nil, want API error")
	}
}

func TestAliyunAudioSpeechRejectsMissingAudioURL(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"finish_reason":"stop"},"request_id":"req-1"}`))
	}))
	defer server.Close()
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{TTS: aliyunTTSTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-tts-flash"
	text := "hello"

	_, err := model.AudioSpeech(ctx, &modelName, &text, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil {
		t.Fatal("error = nil, want missing audio url error")
	}
}

func TestAliyunAudioSpeechRequiresTTSSuffix(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	model := NewAliyunModel(
		map[string]string{"default": "https://dashscope.example"},
		URLSuffix{Chat: "compatible-mode/v1/chat/completions"},
	)
	apiKey := "test-key"
	modelName := "qwen-tts-flash"
	text := "hello"

	_, err := model.AudioSpeech(ctx, &modelName, &text, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || err.Error() != "aliyun TTS URL suffix is required" {
		t.Fatalf("error = %v, want missing TTS suffix error", err)
	}
}

func TestAliyunAudioSpeechWithSenderSendsSingleChunk(t *testing.T) {
	withSSRFBypass(t)
	server := newAliyunTTSTestServer(t, nil, nil)
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{TTS: aliyunTTSTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-tts-flash"
	text := "hello"

	var chunks []string
	err := model.AudioSpeechWithSender(
		ctx,
		&modelName,
		&text,
		&APIConfig{ApiKey: &apiKey},
		nil,
		nil,
		func(content, _ *string) error {
			if content != nil {
				chunks = append(chunks, *content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("AudioSpeechWithSender: %v", err)
	}
	if len(chunks) != 1 || chunks[0] != "fake-wav-bytes" {
		t.Fatalf("chunks = %v, want [fake-wav-bytes]", chunks)
	}
}

// TestAliyunNativeEmbeddingRootMapping pins Python
// _dashscope_native_http_api_url (embedding_model.py:80-128): an already-native
// base is kept, known DashScope hosts map to their /api/v1 root, and anything
// else (private gateways) leaves the compatible endpoint in place.
func TestAliyunNativeEmbeddingRootMapping(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", "https://dashscope.aliyuncs.com/api/v1"},
		{"https://dashscope-intl.aliyuncs.com/compatible-mode/v1/", "https://dashscope-intl.aliyuncs.com/api/v1"},
		{"https://dashscope.aliyuncs.com/api/v1", "https://dashscope.aliyuncs.com/api/v1"},
		{"https://private-gateway.example.com/v1", ""},
		// Hostname matching, not substring: a crafted query string must not
		// select the native API.
		{"https://attacker.example/?u=dashscope.aliyuncs.com", ""},
	}
	for _, c := range cases {
		if got := aliyunNativeEmbeddingRoot(c.in); got != c.want {
			t.Errorf("aliyunNativeEmbeddingRoot(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAliyunEmbedIgnoresNonDashScopeHost pins the Python QWenEmbed contract for a
// base URL that is not a DashScope host: the configured host is IGNORED and the
// call goes to the dashscope SDK's default native root — still carrying
// text_type, so the query/document distinction survives — with a single warning
// naming the ignored host (embedding_model.py:121-127, :134-138, :448-450).
func TestAliyunEmbedIgnoresNonDashScopeHost(t *testing.T) {
	withSSRFBypass(t)

	var paths []string
	var bodies []map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"embeddings":[{"text_index":0,"embedding":[0.5]}]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()
	// DASHSCOPE_HTTP_BASE_URL is the SDK endpoint Python leaves in place when the
	// configured base URL is unrecognized (dashscope/common/env.py:20-23).
	t.Setenv("DASHSCOPE_HTTP_BASE_URL", srv.URL)

	var warnings []string
	prev := aliyunWarnSink
	t.Cleanup(func() { aliyunWarnSink = prev })
	aliyunWarnSink = func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) }

	apiKey := "test-key"
	model := "text-embedding-v4"
	// A private gateway that is not a DashScope host: it must be ignored, not used.
	m := NewAliyunModel(map[string]string{"default": "https://ignored-gateway.internal/v1"},
		URLSuffix{Embedding: "embeddings", Chat: "chat/completions"})

	if _, err := m.Embed(t.Context(), &model, EmbedRequest{Texts: []string{"d"}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err != nil {
		t.Fatalf("Embed(document): %v", err)
	}
	if _, err := m.Embed(t.Context(), &model, EmbedRequest{Texts: []string{"q"}, Query: true}, &APIConfig{ApiKey: &apiKey}, nil, nil); err != nil {
		t.Fatalf("Embed(query): %v", err)
	}
	if _, err := m.Embed(t.Context(), &model, EmbedRequest{Texts: []string{"q2"}, Query: true}, &APIConfig{ApiKey: &apiKey}, nil, nil); err != nil {
		t.Fatalf("Embed(query 2): %v", err)
	}

	// Every call lands on the native text-embedding path of the SDK endpoint.
	if len(paths) != 3 {
		t.Fatalf("requests = %d, want 3 (the ignored host must not be contacted)", len(paths))
	}
	for i, p := range paths {
		if want := "/" + aliyunNativeEmbeddingPath; p != want {
			t.Errorf("path[%d] = %q, want %q", i, p, want)
		}
	}
	// text_type survives the fallback: document for the plain call, query after.
	for i, want := range []string{"document", "query", "query"} {
		params, _ := bodies[i]["parameters"].(map[string]interface{})
		if params["text_type"] != want {
			t.Errorf("body[%d] text_type = %v, want %q", i, bodies[i]["parameters"], want)
		}
	}
	// One warning, naming the host that was ignored.
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one (once per host, not per call)", warnings)
	}
	if !strings.Contains(warnings[0], "ignored-gateway.internal") {
		t.Errorf("warning %q does not name the ignored host", warnings[0])
	}
}

// TestAliyunBaseURLHostIsHostOnly pins that the warning can only ever log a host:
// credentials, path and query string from a configured base URL stay out of the
// log line.
func TestAliyunBaseURLHostIsHostOnly(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"https://user:secret@gateway.internal/v1?key=abc", "gateway.internal"},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", "dashscope.aliyuncs.com"},
		{"gateway.internal/v1", "gateway.internal"},
		{"", ""},
	} {
		if got := aliyunBaseURLHost(c.in); got != c.want {
			t.Errorf("aliyunBaseURLHost(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// embedding call used for Tongyi-Qianwen: text_type follows EmbedRequest.Query,
// inputs are sent in batches of 4 (Python QWenEmbed.encode), and each response's
// batch-relative text_index is offset back into the caller's slice.
func TestAliyunEmbedNativeSendsTextTypeAndBatches(t *testing.T) {
	withSSRFBypass(t)
	var paths []string
	var bodies []map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		input, _ := body["input"].(map[string]interface{})
		texts, _ := input["texts"].([]interface{})
		parts := make([]string, 0, len(texts))
		for i := range texts {
			parts = append(parts, fmt.Sprintf(`{"text_index":%d,"embedding":[%d]}`, i, i))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"embeddings":[` + strings.Join(parts, ",") + `]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewAliyunModel(map[string]string{"default": srv.URL}, URLSuffix{Embedding: "embeddings"})
	apiKey := "test-key"
	got, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"a", "b", "c", "d", "e"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("embedNative: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("requests = %d, want 2 (batch of 4 + 1)", len(paths))
	}
	if want := "/" + aliyunNativeEmbeddingPath; paths[0] != want {
		t.Errorf("path = %q, want %q", paths[0], want)
	}
	if params, ok := bodies[0]["parameters"].(map[string]interface{}); !ok || params["text_type"] != "document" {
		t.Errorf("document text_type = %v, want document", bodies[0]["parameters"])
	}
	byIndex := map[int]EmbeddingData{}
	for _, e := range got {
		byIndex[e.Index] = e
	}
	if len(byIndex) != 5 {
		t.Fatalf("embeddings cover indexes %v, want 0..4", byIndex)
	}
	if _, ok := byIndex[4]; !ok {
		t.Errorf("second batch's text_index was not offset to the global index 4: %v", byIndex)
	}

	// The query path flips text_type.
	if _, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"q"}, Query: true}, &APIConfig{ApiKey: &apiKey}, nil); err != nil {
		t.Fatalf("embedNative(query): %v", err)
	}
	if params, ok := bodies[len(bodies)-1]["parameters"].(map[string]interface{}); !ok || params["text_type"] != "query" {
		t.Errorf("query text_type = %v, want query", bodies[len(bodies)-1]["parameters"])
	}
}
