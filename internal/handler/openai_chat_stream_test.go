// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

type openAIFlushRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (r *openAIFlushRecorder) Flush() { r.flushed++ }

func newOpenAIStreamTestContext() (*gin.Context, *openAIFlushRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := &openAIFlushRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/openai/c1/chat/completions", nil)
	return c, recorder
}

func TestWriteOpenAIChatSSEPreservesSuccessWireFormat(t *testing.T) {
	events := make(chan service.OpenAIStreamEvent, 4)
	events <- service.OpenAIStreamEvent{Kind: service.OpenAIEventContent, Delta: "Hello"}
	events <- service.OpenAIStreamEvent{Kind: service.OpenAIEventContent, Delta: " world"}
	events <- service.OpenAIStreamEvent{Kind: service.OpenAIEventReasoning, Delta: "thinking..."}
	events <- service.OpenAIStreamEvent{
		Kind:             service.OpenAIEventFinal,
		FinalAnswer:      "Hello world",
		FinalReference:   []service.FormattedChunk{{ID: "chunk-1"}},
		PromptTokens:     5,
		CompletionTokens: 2,
		TotalTokens:      7,
	}

	c, recorder := newOpenAIStreamTestContext()
	err := writeOpenAIChatSSE(t.Context(), c, &service.OpenAIChatStream{
		Events:        events,
		Model:         "test-model",
		NeedReference: true,
	}, "chatcmpl-test")
	if err != nil {
		t.Fatalf("writeOpenAIChatSSE() error = %v", err)
	}

	body := recorder.Body.String()
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := strings.Count(body, `"object":"chat.completion.chunk"`); got != 4 {
		t.Fatalf("chunk count = %d, body=%s", got, body)
	}
	if got := strings.Count(body, "data: [DONE]\n\n"); got != 1 {
		t.Fatalf("[DONE] count = %d, body=%s", got, body)
	}
	for _, want := range []string{
		`"content":"Hello"`,
		`"content":" world"`,
		`"reasoning_content":"thinking..."`,
		`"final_content":"Hello world"`,
		`"reference"`,
		`"prompt_tokens":5`,
		`"completion_tokens":2`,
		`"total_tokens":7`,
		`"finish_reason":"stop"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"content":"Hello world"`) {
		t.Fatalf("final answer leaked into delta.content: %s", body)
	}
	if got := strings.Count(body, `"model":"test-model"`); got != 4 {
		t.Fatalf("model count = %d, body=%s", got, body)
	}
	if recorder.flushed != 5 {
		t.Fatalf("flush count = %d, want 5", recorder.flushed)
	}
}

func TestWriteOpenAIChatSSEOmitsReferenceWhenNotRequested(t *testing.T) {
	events := make(chan service.OpenAIStreamEvent, 1)
	events <- service.OpenAIStreamEvent{
		Kind:           service.OpenAIEventFinal,
		FinalAnswer:    "answer",
		FinalReference: []service.FormattedChunk{{ID: "chunk-1"}},
	}

	c, recorder := newOpenAIStreamTestContext()
	err := writeOpenAIChatSSE(t.Context(), c, &service.OpenAIChatStream{
		Events: events,
		Model:  "test-model",
	}, "chatcmpl-test")
	if err != nil {
		t.Fatalf("writeOpenAIChatSSE() error = %v", err)
	}
	if body := recorder.Body.String(); strings.Contains(body, `"reference"`) || strings.Contains(body, `"final_content"`) {
		t.Fatalf("reference fields must be omitted: %s", body)
	}
}

func TestOpenAIChatNonStreamPreservesSuccessWireFormat(t *testing.T) {
	created := int64(123)
	stub := &stubOpenAIChatService{
		complete: func(context.Context, string, string, service.OpenAIChatRequest) (*service.OpenAICompletionResponse, error) {
			return &service.OpenAICompletionResponse{
				Model:            "resolved-model",
				Content:          "answer",
				Reference:        []service.FormattedChunk{{ID: "chunk-1"}},
				PromptTokens:     5,
				CompletionTokens: 2,
				TotalTokens:      7,
				ContextTokens:    3,
				Created:          &created,
			}, nil
		},
	}
	c, recorder := newOpenAITestContext(t, "c1", `{
		"model":"model",
		"messages":[{"role":"user","content":"hi"}]
	}`)

	NewOpenAIChatHandler(stub).OpenAIChatCompletions(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	if payload["id"] != "chatcmpl-c1" || payload["object"] != "chat.completion" || payload["model"] != "resolved-model" || payload["created"] != float64(123) {
		t.Fatalf("top-level response = %+v", payload)
	}
	usage, ok := payload["usage"].(map[string]interface{})
	if !ok || usage["prompt_tokens"] != float64(5) || usage["completion_tokens"] != float64(2) || usage["total_tokens"] != float64(7) {
		t.Fatalf("usage = %+v", payload["usage"])
	}
	details, ok := usage["completion_tokens_details"].(map[string]interface{})
	if !ok || details["reasoning_tokens"] != float64(3) || details["accepted_prediction_tokens"] != float64(2) || details["rejected_prediction_tokens"] != float64(0) {
		t.Fatalf("completion token details = %+v", usage["completion_tokens_details"])
	}
	choices, ok := payload["choices"].([]interface{})
	if !ok || len(choices) != 1 {
		t.Fatalf("choices = %+v", payload["choices"])
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok || choice["finish_reason"] != "stop" || choice["logprobs"] != nil {
		t.Fatalf("choice = %+v", choices[0])
	}
	message, ok := choice["message"].(map[string]interface{})
	if !ok || message["role"] != "assistant" || message["content"] != "answer" {
		t.Fatalf("message = %+v", choice["message"])
	}
	reference, ok := message["reference"].([]interface{})
	if !ok || len(reference) != 1 {
		t.Fatalf("reference = %+v", message["reference"])
	}
}

type stubOpenAIChatService struct {
	complete func(context.Context, string, string, service.OpenAIChatRequest) (*service.OpenAICompletionResponse, error)
	stream   func(context.Context, string, string, service.OpenAIChatRequest) (*service.OpenAIChatStream, error)
}

func (s *stubOpenAIChatService) Complete(
	ctx context.Context,
	userID, chatID string,
	req service.OpenAIChatRequest,
) (*service.OpenAICompletionResponse, error) {
	return s.complete(ctx, userID, chatID, req)
}

func (s *stubOpenAIChatService) Stream(
	ctx context.Context,
	userID, chatID string,
	req service.OpenAIChatRequest,
) (*service.OpenAIChatStream, error) {
	return s.stream(ctx, userID, chatID, req)
}

func TestOpenAIChatPrepareErrorRemainsJSONBeforeSSE(t *testing.T) {
	stub := &stubOpenAIChatService{
		stream: func(context.Context, string, string, service.OpenAIChatRequest) (*service.OpenAIChatStream, error) {
			return nil, common.NewCodedError(common.CodeDataError, "prepare failed")
		},
	}
	h := NewOpenAIChatHandler(stub)
	c, recorder := newOpenAITestContext(t, "c1", `{
		"model":"model",
		"stream":true,
		"messages":[{"role":"user","content":"hi"}]
	}`)

	h.OpenAIChatCompletions(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	if got := recorder.Header().Get("Content-Type"); strings.Contains(got, "text/event-stream") {
		t.Fatalf("prepare error committed SSE headers: %q", got)
	}
	for _, want := range []string{`"code":102`, `"data":null`, `"message":"prepare failed"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, recorder.Body.String())
		}
	}
}

func TestOpenAIChatNonStreamGenerationFailureUsesSafeEnvelope(t *testing.T) {
	stub := &stubOpenAIChatService{
		complete: func(context.Context, string, string, service.OpenAIChatRequest) (*service.OpenAICompletionResponse, error) {
			return nil, common.NewCodedError(common.CodeDataError, "an internal error occurred")
		},
	}
	h := NewOpenAIChatHandler(stub)
	c, recorder := newOpenAITestContext(t, "c1", `{
		"model":"model",
		"messages":[{"role":"user","content":"hi"}]
	}`)

	h.OpenAIChatCompletions(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	for _, want := range []string{`"code":102`, `"data":null`, `"message":"an internal error occurred"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("body missing %s: %s", want, recorder.Body.String())
		}
	}
}

type nonFlushingOpenAIResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *nonFlushingOpenAIResponseWriter) Header() http.Header { return w.header }
func (w *nonFlushingOpenAIResponseWriter) WriteHeader(status int) {
	w.status = status
}
func (w *nonFlushingOpenAIResponseWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func TestOpenAIChatUnsupportedStreamingReturnsJSONEnvelope(t *testing.T) {
	events := make(chan service.OpenAIStreamEvent)
	close(events)
	stub := &stubOpenAIChatService{
		stream: func(context.Context, string, string, service.OpenAIChatRequest) (*service.OpenAIChatStream, error) {
			return &service.OpenAIChatStream{Events: events, Model: "model"}, nil
		},
	}

	gin.SetMode(gin.TestMode)
	writer := &nonFlushingOpenAIResponseWriter{header: make(http.Header)}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/openai/c1/chat/completions", strings.NewReader(`{
		"model":"model",
		"stream":true,
		"messages":[{"role":"user","content":"hi"}]
	}`))
	c.Params = gin.Params{{Key: "chat_id", Value: "c1"}}
	fakeOpenAIUser(c)

	NewOpenAIChatHandler(stub).OpenAIChatCompletions(c)

	if got := writer.header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	for _, want := range []string{`"code":102`, `"data":null`, `"message":"streaming unsupported"`} {
		if !strings.Contains(writer.body.String(), want) {
			t.Fatalf("body missing %s: %s", want, writer.body.String())
		}
	}
	if strings.Contains(writer.body.String(), "data:") {
		t.Fatalf("unsupported streaming committed an SSE frame: %s", writer.body.String())
	}
}

type failingOpenAIResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	writes int
}

func (w *failingOpenAIResponseWriter) Header() http.Header { return w.header }
func (w *failingOpenAIResponseWriter) WriteHeader(int)     {}
func (w *failingOpenAIResponseWriter) Flush()              {}
func (w *failingOpenAIResponseWriter) Write(p []byte) (int, error) {
	if w.writes > 0 {
		return 0, errors.New("client disconnected")
	}
	w.writes++
	return w.body.Write(p)
}

func TestOpenAIChatWriterFailureCancelsServiceContext(t *testing.T) {
	producerDone := make(chan struct{})
	stub := &stubOpenAIChatService{
		stream: func(ctx context.Context, _, _ string, _ service.OpenAIChatRequest) (*service.OpenAIChatStream, error) {
			events := make(chan service.OpenAIStreamEvent)
			go func() {
				defer close(producerDone)
				defer close(events)
				events <- service.OpenAIStreamEvent{Kind: service.OpenAIEventContent, Delta: "first"}
				events <- service.OpenAIStreamEvent{Kind: service.OpenAIEventContent, Delta: "second"}
				<-ctx.Done()
			}()
			return &service.OpenAIChatStream{Events: events, Model: "model"}, nil
		},
	}

	gin.SetMode(gin.TestMode)
	writer := &failingOpenAIResponseWriter{header: make(http.Header)}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/openai/c1/chat/completions", strings.NewReader(`{
		"model":"model",
		"stream":true,
		"messages":[{"role":"user","content":"hi"}]
	}`))
	c.Params = gin.Params{{Key: "chat_id", Value: "c1"}}
	fakeOpenAIUser(c)

	NewOpenAIChatHandler(stub).OpenAIChatCompletions(c)

	select {
	case <-producerDone:
	case <-time.After(time.Second):
		t.Fatal("service producer did not observe handler cancellation")
	}
	if strings.Contains(writer.body.String(), "[DONE]") {
		t.Fatalf("writer failure must not be followed by [DONE]: %s", writer.body.String())
	}
}

type failingFlushOpenAIResponseWriter struct {
	header http.Header
	body   bytes.Buffer
}

func (w *failingFlushOpenAIResponseWriter) Header() http.Header { return w.header }
func (w *failingFlushOpenAIResponseWriter) WriteHeader(int)     {}
func (w *failingFlushOpenAIResponseWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}
func (w *failingFlushOpenAIResponseWriter) FlushError() error {
	return errors.New("flush failed")
}

func TestOpenAIChatFlushFailureCancelsServiceContext(t *testing.T) {
	producerDone := make(chan struct{})
	stub := &stubOpenAIChatService{
		stream: func(ctx context.Context, _, _ string, _ service.OpenAIChatRequest) (*service.OpenAIChatStream, error) {
			events := make(chan service.OpenAIStreamEvent)
			go func() {
				defer close(producerDone)
				defer close(events)
				events <- service.OpenAIStreamEvent{Kind: service.OpenAIEventContent, Delta: "first"}
				<-ctx.Done()
			}()
			return &service.OpenAIChatStream{Events: events, Model: "model"}, nil
		},
	}

	gin.SetMode(gin.TestMode)
	writer := &failingFlushOpenAIResponseWriter{header: make(http.Header)}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/openai/c1/chat/completions", strings.NewReader(`{
		"model":"model",
		"stream":true,
		"messages":[{"role":"user","content":"hi"}]
	}`))
	c.Params = gin.Params{{Key: "chat_id", Value: "c1"}}
	fakeOpenAIUser(c)

	NewOpenAIChatHandler(stub).OpenAIChatCompletions(c)

	select {
	case <-producerDone:
	case <-time.After(time.Second):
		t.Fatal("service producer did not observe flush-failure cancellation")
	}
	if strings.Contains(writer.body.String(), "[DONE]") {
		t.Fatalf("flush failure must not be followed by [DONE]: %s", writer.body.String())
	}
}

func TestOpenAIChatClientCancellationStopsWithoutTerminalFrame(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	serviceStarted := make(chan struct{})
	producerDone := make(chan struct{})
	stub := &stubOpenAIChatService{
		stream: func(ctx context.Context, _, _ string, _ service.OpenAIChatRequest) (*service.OpenAIChatStream, error) {
			events := make(chan service.OpenAIStreamEvent)
			go func() {
				defer close(producerDone)
				defer close(events)
				close(serviceStarted)
				<-ctx.Done()
			}()
			return &service.OpenAIChatStream{Events: events, Model: "model"}, nil
		},
	}

	gin.SetMode(gin.TestMode)
	recorder := &openAIFlushRecorder{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/openai/c1/chat/completions", strings.NewReader(`{
		"model":"model",
		"stream":true,
		"messages":[{"role":"user","content":"hi"}]
	}`))
	c.Request = request.WithContext(requestCtx)
	c.Params = gin.Params{{Key: "chat_id", Value: "c1"}}
	fakeOpenAIUser(c)

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		NewOpenAIChatHandler(stub).OpenAIChatCompletions(c)
	}()
	<-serviceStarted
	cancelRequest()

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not stop after client cancellation")
	}
	select {
	case <-producerDone:
	case <-time.After(time.Second):
		t.Fatal("service producer did not observe client cancellation")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("client cancellation wrote a terminal frame: %s", recorder.Body.String())
	}
}
