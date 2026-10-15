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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// TestAliyunEmbedNativePlacesVectorsByTextIndex pins that an out-of-order
// response is reordered onto the caller's inputs: nav_embedder/dataset-utils
// read the result positionally and discard Index, so placing by text_index is
// what keeps each vector bound to the right text.
func TestAliyunEmbedNativePlacesVectorsByTextIndex(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Deliberately reversed: text_index 3,2,1,0 with distinct vectors.
		_, _ = w.Write([]byte(`{"output":{"embeddings":[` +
			`{"text_index":3,"embedding":[3]},` +
			`{"text_index":2,"embedding":[2]},` +
			`{"text_index":1,"embedding":[1]},` +
			`{"text_index":0,"embedding":[0]}` +
			`]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewAliyunModel(map[string]string{"default": srv.URL}, URLSuffix{Embedding: "embeddings"})
	apiKey := "test-key"
	got, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"a", "b", "c", "d"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("embedNative: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	for i, e := range got {
		if e.Index != i || len(e.Embedding) != 1 || e.Embedding[0] != float64(i) {
			t.Errorf("got[%d] = {index:%d vec:%v}, want {index:%d vec:[%d]}", i, e.Index, e.Embedding, i, i)
		}
	}
}

// TestAliyunEmbedNativeDuplicateIndexLastWins pins Python QWenEmbed.encode's
// "embds[e['text_index']] = e['embedding']": a repeated text_index overwrites
// the earlier vector and the slot left unfilled stays empty — no error.
func TestAliyunEmbedNativeDuplicateIndexLastWins(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"embeddings":[` +
			`{"text_index":0,"embedding":[0]},` +
			`{"text_index":1,"embedding":[1]},` +
			`{"text_index":1,"embedding":[9]}` +
			`]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewAliyunModel(map[string]string{"default": srv.URL}, URLSuffix{Embedding: "embeddings"})
	apiKey := "test-key"
	got, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"a", "b", "c"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("embedNative: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (one chunk entry per returned embedding)", len(got))
	}
	if len(got[0].Embedding) != 1 || got[0].Embedding[0] != 0 {
		t.Errorf("got[0] = %v, want [0]", got[0].Embedding)
	}
	if len(got[1].Embedding) != 1 || got[1].Embedding[0] != 9 {
		t.Errorf("got[1] = %v, want the LAST duplicate [9]", got[1].Embedding)
	}
	if len(got[2].Embedding) != 0 {
		t.Errorf("got[2] = %v, want the unfilled slot to stay empty", got[2].Embedding)
	}
}

// TestAliyunEmbedNativeShortResponseStaysShort pins that Go mirrors Python's
// len(resp.embeddings)-sized chunks: a response covering only part of the batch
// is NOT rejected, it just yields a shorter result (Python's res.extend).
func TestAliyunEmbedNativeShortResponseStaysShort(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"embeddings":[` +
			`{"text_index":0,"embedding":[0]},` +
			`{"text_index":1,"embedding":[1]}` +
			`]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewAliyunModel(map[string]string{"default": srv.URL}, URLSuffix{Embedding: "embeddings"})
	apiKey := "test-key"
	got, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"a", "b", "c", "d"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("embedNative: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (response count, Python res.extend)", len(got))
	}
	if got[0].Embedding[0] != 0 || got[1].Embedding[0] != 1 {
		t.Errorf("got = %v, want [[0],[1]]", got)
	}
}

// TestAliyunEmbedNativeRejectsOutOfRangeIndex pins Python's IndexError: a
// text_index past the returned chunk is an error.
func TestAliyunEmbedNativeRejectsOutOfRangeIndex(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"embeddings":[` +
			`{"text_index":5,"embedding":[5]}` +
			`]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewAliyunModel(map[string]string{"default": srv.URL}, URLSuffix{Embedding: "embeddings"})
	apiKey := "test-key"
	_, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"a", "b"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("err = %v, want out-of-range error", err)
	}
}

// TestAliyunEmbedNativeRejectsMissingTextIndexKey pins Python's KeyError on
// e["text_index"]: an item without the key is an error, not a silent index 0.
func TestAliyunEmbedNativeRejectsMissingTextIndexKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"embeddings":[` +
			`{"embedding":[1]}` +
			`]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewAliyunModel(map[string]string{"default": srv.URL}, URLSuffix{Embedding: "embeddings"})
	apiKey := "test-key"
	_, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"a"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "text_index") {
		t.Fatalf("err = %v, want missing-text_index error", err)
	}
}

// TestAliyunEmbedNativeNegativeIndexCountsFromEnd pins Python list-assignment
// semantics: a negative text_index indexes from the END of the chunk, so
// embds[-1] overwrites the last slot.
func TestAliyunEmbedNativeNegativeIndexCountsFromEnd(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"embeddings":[` +
			`{"text_index":-1,"embedding":[7]},` +
			`{"text_index":0,"embedding":[0]},` +
			`{"text_index":1,"embedding":[1]}` +
			`]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewAliyunModel(map[string]string{"default": srv.URL}, URLSuffix{Embedding: "embeddings"})
	apiKey := "test-key"
	got, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"a", "b", "c"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err != nil {
		t.Fatalf("embedNative: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// embds[-1] = [7] lands in the last slot: [[0],[1],[7]].
	want := []float64{0, 1, 7}
	for i, w := range want {
		if len(got[i].Embedding) != 1 || got[i].Embedding[0] != w {
			t.Errorf("got[%d] = %v, want [%v]", i, got[i].Embedding, w)
		}
	}
}

// TestAliyunEmbedNativeRejectsTooNegativeIndex pins that an index below
// -len(chunk) raises, exactly like Python's IndexError.
func TestAliyunEmbedNativeRejectsTooNegativeIndex(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"embeddings":[` +
			`{"text_index":-5,"embedding":[5]}` +
			`]},"usage":{"total_tokens":1}}`))
	}))
	defer srv.Close()

	m := NewAliyunModel(map[string]string{"default": srv.URL}, URLSuffix{Embedding: "embeddings"})
	apiKey := "test-key"
	_, err := m.embedNative(t.Context(), srv.URL, "text-embedding-v4",
		EmbedRequest{Texts: []string{"a", "b"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("err = %v, want out-of-range error", err)
	}
}

const aliyunASRTestSuffix = "api/v1/services/aigc/multimodal-generation/generation"

// newAliyunASRTestServer stubs the DashScope multimodal-generation endpoint for
// ASR: POST returns one message choice whose content carries the transcription.
func newAliyunASRTestServer(t *testing.T, transcription string, requestBody chan<- map[string]interface{}, requestPath chan<- string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if _, err := fmt.Fprintf(w, `{"output":{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":[{"text":%q}]}}]},"request_id":"req-1"}`, transcription); err != nil {
			t.Errorf("failed to write ASR response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeAliyunASRTestFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write audio file: %v", err)
	}
	return path
}

func aliyunASRMessages(t *testing.T, body map[string]interface{}) []map[string]interface{} {
	t.Helper()
	input, ok := body["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("input = %T, want JSON object", body["input"])
	}
	raw, ok := input["messages"].([]interface{})
	if !ok {
		t.Fatalf("input.messages = %T, want JSON array", input["messages"])
	}
	messages := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		message, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("message = %T, want JSON object", item)
		}
		messages = append(messages, message)
	}
	return messages
}

func aliyunASRContentString(t *testing.T, message map[string]interface{}, key string) string {
	t.Helper()
	content, ok := message["content"].([]interface{})
	if !ok || len(content) == 0 {
		t.Fatalf("content = %v, want non-empty JSON array", message["content"])
	}
	part, ok := content[0].(map[string]interface{})
	if !ok {
		t.Fatalf("content[0] = %T, want JSON object", content[0])
	}
	value, _ := part[key].(string)
	return value
}

func TestAliyunTranscribeAudioSendsDataURLAndParsesText(t *testing.T) {
	withSSRFBypass(t)
	requestBody := make(chan map[string]interface{}, 1)
	requestPath := make(chan string, 1)
	server := newAliyunASRTestServer(t, "hello world", requestBody, requestPath)
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{ASR: aliyunASRTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-asr-flash"
	audio := []byte("RIFF....WAVE")
	file := writeAliyunASRTestFile(t, "clip.wav", audio)

	response, err := model.TranscribeAudio(ctx, &modelName, &file, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	if response.Text != "hello world" {
		t.Errorf("text = %q, want %q", response.Text, "hello world")
	}

	if got := <-requestPath; got != "/"+aliyunASRTestSuffix {
		t.Errorf("request path = %q, want /%s", got, aliyunASRTestSuffix)
	}
	body := <-requestBody
	if body["model"] != "qwen-asr-flash" {
		t.Errorf("model = %v, want qwen-asr-flash", body["model"])
	}
	messages := aliyunASRMessages(t, body)
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2 (system + user)", len(messages))
	}
	if messages[0]["role"] != "system" {
		t.Errorf("messages[0].role = %v, want system", messages[0]["role"])
	}
	if messages[1]["role"] != "user" {
		t.Errorf("messages[1].role = %v, want user", messages[1]["role"])
	}
	wantURL := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(audio)
	if got := aliyunASRContentString(t, messages[1], "audio"); got != wantURL {
		t.Errorf("audio = %q, want %q", got, wantURL)
	}

	params, ok := body["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters = %T, want JSON object", body["parameters"])
	}
	if params["result_format"] != "message" {
		t.Errorf("result_format = %v, want message", params["result_format"])
	}
	options, ok := params["asr_options"].(map[string]interface{})
	if !ok {
		t.Fatalf("asr_options = %T, want JSON object", params["asr_options"])
	}
	if options["enable_lid"] != true || options["enable_itn"] != false {
		t.Errorf("asr_options = %v, want defaults enable_lid=true enable_itn=false", options)
	}
}

func TestAliyunTranscribeAudioHonorsContextAndASROptions(t *testing.T) {
	withSSRFBypass(t)
	requestBody := make(chan map[string]interface{}, 1)
	server := newAliyunASRTestServer(t, "ok", requestBody, nil)
	ctx := t.Context()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{ASR: aliyunASRTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-asr-flash"
	file := writeAliyunASRTestFile(t, "clip.mp3", []byte("ID3...."))

	_, err := model.TranscribeAudio(ctx, &modelName, &file, &APIConfig{ApiKey: &apiKey}, &ASRConfig{Params: map[string]interface{}{
		"context":    "NAVI vs Faze",
		"language":   "en",
		"enable_itn": true,
	}}, nil)
	if err != nil {
		t.Fatalf("TranscribeAudio: %v", err)
	}
	body := <-requestBody
	messages := aliyunASRMessages(t, body)
	if got := aliyunASRContentString(t, messages[0], "text"); got != "NAVI vs Faze" {
		t.Errorf("system text = %q, want NAVI vs Faze", got)
	}
	if got := aliyunASRContentString(t, messages[1], "audio"); !strings.HasPrefix(got, "data:audio/mpeg;base64,") {
		t.Errorf("audio data URL = %q, want mp3 MIME prefix", got)
	}
	params, ok := body["parameters"].(map[string]interface{})
	if !ok {
		t.Fatalf("parameters = %T, want JSON object", body["parameters"])
	}
	options, ok := params["asr_options"].(map[string]interface{})
	if !ok {
		t.Fatalf("asr_options = %T, want JSON object", params["asr_options"])
	}
	if options["language"] != "en" {
		t.Errorf("language = %v, want en", options["language"])
	}
	if options["enable_itn"] != true {
		t.Errorf("enable_itn = %v, want true (explicit override)", options["enable_itn"])
	}
	if options["enable_lid"] != true {
		t.Errorf("enable_lid = %v, want default true", options["enable_lid"])
	}
}

func TestAliyunTranscribeAudioSurfacesAPIError(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"InvalidApiKey","message":"Invalid API-key provided."}`))
	}))
	defer server.Close()

	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{ASR: aliyunASRTestSuffix},
	)
	apiKey := "bad-key"
	modelName := "qwen-asr-flash"
	file := writeAliyunASRTestFile(t, "clip.wav", []byte("data"))

	_, err := model.TranscribeAudio(t.Context(), &modelName, &file, &APIConfig{ApiKey: &apiKey}, nil, nil)
	var statusErr *APIStatusError
	if !errors.As(err, &statusErr) || statusErr.Status != http.StatusUnauthorized {
		t.Fatalf("error = %v, want APIStatusError with status 401", err)
	}
	if !strings.Contains(statusErr.Body, "InvalidApiKey") {
		t.Fatalf("error body = %q, want to include InvalidApiKey", statusErr.Body)
	}
}

func TestAliyunTranscribeAudioRejectsEmptyFile(t *testing.T) {
	withSSRFBypass(t)
	model := NewAliyunModel(
		map[string]string{"default": "https://dashscope.example"},
		URLSuffix{ASR: aliyunASRTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-asr-flash"
	file := writeAliyunASRTestFile(t, "empty.wav", nil)

	_, err := model.TranscribeAudio(t.Context(), &modelName, &file, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || err.Error() != "audio file is empty" {
		t.Fatalf("error = %v, want empty audio error", err)
	}
}

func TestAliyunTranscribeAudioRequiresASRSuffix(t *testing.T) {
	withSSRFBypass(t)
	model := NewAliyunModel(
		map[string]string{"default": "https://dashscope.example"},
		URLSuffix{Chat: "compatible-mode/v1/chat/completions"},
	)
	apiKey := "test-key"
	modelName := "qwen-asr-flash"
	file := writeAliyunASRTestFile(t, "clip.wav", []byte("data"))

	_, err := model.TranscribeAudio(t.Context(), &modelName, &file, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || err.Error() != "aliyun ASR URL suffix is required" {
		t.Fatalf("error = %v, want missing ASR suffix error", err)
	}
}

func TestAliyunTranscribeAudioWithSenderSendsSingleChunk(t *testing.T) {
	withSSRFBypass(t)
	server := newAliyunASRTestServer(t, "transcribed", nil, nil)
	model := NewAliyunModel(
		map[string]string{"default": server.URL},
		URLSuffix{ASR: aliyunASRTestSuffix},
	)
	apiKey := "test-key"
	modelName := "qwen-asr-flash"
	file := writeAliyunASRTestFile(t, "clip.wav", []byte("data"))

	var chunks []string
	err := model.TranscribeAudioWithSender(t.Context(), &modelName, &file, &APIConfig{ApiKey: &apiKey}, nil, nil, func(content, _ *string) error {
		if content != nil {
			chunks = append(chunks, *content)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("TranscribeAudioWithSender: %v", err)
	}
	if len(chunks) != 1 || chunks[0] != "transcribed" {
		t.Fatalf("chunks = %v, want [transcribed]", chunks)
	}
}
