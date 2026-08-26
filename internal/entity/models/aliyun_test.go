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
