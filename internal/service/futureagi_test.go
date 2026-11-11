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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
)

// fakeCollector records every OTLP request posted to it.
type fakeCollector struct {
	mu       sync.Mutex
	bodies   []string
	headers  []http.Header
	paths    []string
	status   int
	received chan struct{}
}

func newFakeCollector(t *testing.T, status int) (*fakeCollector, *httptest.Server) {
	t.Helper()
	t.Setenv(common.EnvAllowAnyHost, "true")
	c := &fakeCollector{status: status, received: make(chan struct{}, 64)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.bodies = append(c.bodies, string(body))
		c.headers = append(c.headers, r.Header.Clone())
		c.paths = append(c.paths, r.URL.Path)
		c.mu.Unlock()
		w.WriteHeader(c.status)
		select {
		case c.received <- struct{}{}:
		default:
		}
	}))
	t.Cleanup(srv.Close)
	return c, srv
}

func (c *fakeCollector) wait(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		c.mu.Lock()
		got := len(c.bodies)
		c.mu.Unlock()
		if got >= n {
			c.mu.Lock()
			defer c.mu.Unlock()
			return append([]string(nil), c.bodies...)
		}
		select {
		case <-c.received:
		case <-deadline:
			t.Fatalf("collector got %d of %d requests", got, n)
		}
	}
}

type otlpSpan struct {
	TraceID      string           `json:"traceId"`
	SpanID       string           `json:"spanId"`
	ParentSpanID string           `json:"parentSpanId"`
	Name         string           `json:"name"`
	Start        string           `json:"startTimeUnixNano"`
	End          string           `json:"endTimeUnixNano"`
	Attributes   []map[string]any `json:"attributes"`
	Status       struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
}

func decodeExport(t *testing.T, body string) (map[string]string, []otlpSpan) {
	t.Helper()
	var payload struct {
		ResourceSpans []struct {
			Resource struct {
				Attributes []map[string]any `json:"attributes"`
			} `json:"resource"`
			ScopeSpans []struct {
				Spans []otlpSpan `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if len(payload.ResourceSpans) != 1 || len(payload.ResourceSpans[0].ScopeSpans) != 1 {
		t.Fatalf("unexpected export shape: %s", body)
	}
	return attrMap(payload.ResourceSpans[0].Resource.Attributes), payload.ResourceSpans[0].ScopeSpans[0].Spans
}

// attrMap flattens OTLP attributes to strings for assertions.
func attrMap(attrs []map[string]any) map[string]string {
	out := map[string]string{}
	for _, a := range attrs {
		key, _ := a["key"].(string)
		v, _ := a["value"].(map[string]any)
		switch {
		case v["stringValue"] != nil:
			out[key] = v["stringValue"].(string)
		case v["intValue"] != nil:
			out[key] = v["intValue"].(string)
		case v["boolValue"] != nil:
			out[key] = strconv.FormatBool(v["boolValue"].(bool))
		case v["doubleValue"] != nil:
			out[key] = strconv.FormatFloat(v["doubleValue"].(float64), 'f', -1, 64)
		}
	}
	return out
}

func spanByName(t *testing.T, spans []otlpSpan, name string) otlpSpan {
	t.Helper()
	for _, s := range spans {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no span %q in %d spans", name, len(spans))
	return otlpSpan{}
}

func newTestTurn(client *FutureAGIClient) *futureAGITurn {
	return &futureAGITurn{
		client:    client,
		traceID:   newFutureAGITraceID(),
		rootID:    newFutureAGISpanID(),
		start:     time.Now(),
		chatID:    "chat-1",
		sessionID: "session-1",
		tenantID:  "tenant-1",
		question:  "what is in the handbook?",
		stream:    true,
	}
}

// sampleKBInfos has the chunk shape nlp.Retrieval and the web search return.
func sampleKBInfos() map[string]interface{} {
	return map[string]interface{}{
		"total": 2,
		"chunks": []map[string]interface{}{
			{"chunk_id": "c1", "doc_id": "d1", "kb_id": "kb1", "docnm_kwd": "handbook.pdf", "content_with_weight": "chunk one text", "similarity": 0.82, "vector_similarity": 0.9, "term_similarity": 0.6},
			{"chunk_id": "w1", "doc_id": "w1", "docnm_kwd": "Example page", "url": "https://example.com/page", "content_with_weight": "web text", "similarity": 0.5},
		},
	}
}

func runSampleTurn(client *FutureAGIClient, callErr error) {
	turn := newTestTurn(client)
	chat := &entity.Chat{ID: "chat-1", TenantID: "tenant-1", TopN: 6, TopK: 1024, SimilarityThreshold: 0.2, VectorSimilarityWeight: 0.3, KBIDs: entity.JSONSlice{"kb1"}}
	turn.retrieved("what is in the handbook?", sampleKBInfos(), true, 40*time.Millisecond, chat)
	turn.llmStart("gpt-x", "OpenAI-API-Compatible", []modelModule.Message{
		{Role: "system", Content: "You answer from the knowledge base."},
		{Role: "user", Content: "what is in the handbook?"},
	})
	if callErr != nil {
		turn.llmEnd("", 0, 0, callErr)
		turn.finish("**ERROR**: "+callErr.Error(), "**ERROR**: "+callErr.Error(), true)
		return
	}
	turn.llmEnd("The handbook covers leave.", 120, 7, nil)
	turn.finish("The handbook covers leave.", "", true)
}

func TestFutureAGITurnExportsChainRetrieverAndLLM(t *testing.T) {
	sink, srv := newFakeCollector(t, http.StatusOK)
	client := NewFutureAGIClient(srv.URL, "key-1", "secret-1", "proj", false)
	defer client.Shutdown(t.Context())

	runSampleTurn(client, nil)

	bodies := sink.wait(t, 1)
	if sink.paths[0] != futureAGICollectorPath {
		t.Fatalf("path %q", sink.paths[0])
	}
	h := sink.headers[0]
	if h.Get("X-Api-Key") != "key-1" || h.Get("X-Secret-Key") != "secret-1" {
		t.Fatalf("auth headers missing")
	}
	if h.Get("Content-Encoding") != "" || h.Get("Content-Type") != "application/json" {
		t.Fatalf("want uncompressed JSON, got encoding=%q type=%q", h.Get("Content-Encoding"), h.Get("Content-Type"))
	}
	resource, spans := decodeExport(t, bodies[0])
	if resource["project_name"] != "proj" || resource["project_type"] != "observe" || resource["service.name"] != "ragflow" {
		t.Fatalf("resource %v", resource)
	}
	if len(spans) != 3 {
		t.Fatalf("want 3 spans, got %d", len(spans))
	}
	root := spanByName(t, spans, "ragflow.chat")
	retrieval := spanByName(t, spans, "ragflow.retrieval")
	llm := spanByName(t, spans, "ragflow.llm")
	if root.ParentSpanID != "" || retrieval.ParentSpanID != root.SpanID || llm.ParentSpanID != root.SpanID {
		t.Fatalf("hierarchy wrong: root=%s retrieval.parent=%s llm.parent=%s", root.SpanID, retrieval.ParentSpanID, llm.ParentSpanID)
	}
	for _, s := range spans {
		if s.TraceID != root.TraceID || len(s.SpanID) != 16 || s.Status.Code != 1 {
			t.Fatalf("span %s: trace=%s id=%s status=%d", s.Name, s.TraceID, s.SpanID, s.Status.Code)
		}
	}

	ra := attrMap(root.Attributes)
	for k, want := range map[string]string{
		"fi.span.kind": "CHAIN", "session.id": "session-1", "user.id": "tenant-1", "ragflow.tenant_id": "tenant-1",
		"ragflow.chat_id": "chat-1", "input.value": "what is in the handbook?", "output.value": "The handbook covers leave.",
		"ragflow.pipeline": "rag", "ragflow.stream": "true",
	} {
		if ra[k] != want {
			t.Errorf("root %s = %q, want %q", k, ra[k], want)
		}
	}

	rt := attrMap(retrieval.Attributes)
	for k, want := range map[string]string{
		"fi.span.kind": "RETRIEVER", "input.value": "what is in the handbook?", "ragflow.retrieval.chunk_count": "2",
		"retrieval.documents.0.document.id": "c1", "retrieval.documents.0.document.score": "0.82",
		"retrieval.documents.0.document.content": "chunk one text", "retrieval.documents.1.document.id": "w1",
		"ragflow.retrieval.top_n": "6", "ragflow.kb_count": "1",
	} {
		if rt[k] != want {
			t.Errorf("retrieval %s = %q, want %q", k, rt[k], want)
		}
	}
	if !strings.Contains(rt["retrieval.documents.0.document.metadata"], `"document_name":"handbook.pdf"`) ||
		!strings.Contains(rt["retrieval.documents.1.document.metadata"], `"url":"https://example.com/page"`) {
		t.Errorf("document metadata: %q / %q", rt["retrieval.documents.0.document.metadata"], rt["retrieval.documents.1.document.metadata"])
	}

	la := attrMap(llm.Attributes)
	for k, want := range map[string]string{
		"fi.span.kind": "LLM", "llm.model_name": "gpt-x", "gen_ai.request.model": "gpt-x", "gen_ai.provider.name": "OpenAI-API-Compatible",
		"llm.token_count.prompt": "120", "llm.token_count.completion": "7", "llm.token_count.total": "127",
		"gen_ai.usage.input_tokens": "120", "gen_ai.usage.output_tokens": "7",
		"llm.input_messages.0.message.role": "system", "llm.input_messages.1.message.content": "what is in the handbook?",
		"output.value": "The handbook covers leave.",
	} {
		if la[k] != want {
			t.Errorf("llm %s = %q, want %q", k, la[k], want)
		}
	}
}

func TestFutureAGIRedactKeepsShapeAndDropsContent(t *testing.T) {
	sink, srv := newFakeCollector(t, http.StatusOK)
	client := NewFutureAGIClient(srv.URL, "key", "secret", "proj", true)
	defer client.Shutdown(t.Context())

	runSampleTurn(client, nil)
	runSampleTurn(client, errors.New("upstream said: what is in the handbook?"))

	bodies := sink.wait(t, 2)
	for _, body := range bodies {
		for _, leak := range []string{"handbook", "chunk one text", "web text", "Example page", "example.com", "The handbook covers", "knowledge base"} {
			if strings.Contains(body, leak) {
				t.Fatalf("redact left %q in the export: %s", leak, body)
			}
		}
		_, spans := decodeExport(t, body)
		if len(spans) != 3 {
			t.Fatalf("redact must keep the hierarchy, got %d spans", len(spans))
		}
		for _, s := range spans {
			a := attrMap(s.Attributes)
			for k := range a {
				if k == "input.value" || k == "output.value" || strings.HasSuffix(k, ".content") {
					t.Fatalf("span %s kept %s with redact on", s.Name, k)
				}
			}
		}
	}
	_, ok := decodeExport(t, bodies[0])
	llm := attrMap(spanByName(t, ok, "ragflow.llm").Attributes)
	rt := attrMap(spanByName(t, ok, "ragflow.retrieval").Attributes)
	if id := rt["retrieval.documents.1.document.id"]; !strings.HasPrefix(id, "web-") || strings.Contains(id, "w1") {
		t.Fatalf("web chunk id with redact on = %q", id)
	}
	if llm["llm.model_name"] != "gpt-x" || llm["llm.token_count.prompt"] != "120" || rt["retrieval.documents.0.document.id"] != "c1" || rt["retrieval.documents.0.document.score"] != "0.82" {
		t.Fatalf("redact dropped ids, scores, model or tokens: llm=%v retrieval=%v", llm, rt)
	}
	_, failed := decodeExport(t, bodies[1])
	for _, s := range failed {
		if s.Name == "ragflow.retrieval" {
			continue
		}
		if s.Status.Code != 2 || s.Status.Message != futureAGIRedactedError {
			t.Fatalf("%s status = %d %q", s.Name, s.Status.Code, s.Status.Message)
		}
	}
}

// Exercise both the explicit llmEnd hook and the streaming/solo fallback.
// Scan the entire export, not just status.message: failed root output must not
// carry a provider's echoed credential either. Content capture remains enabled.
func TestFutureAGIProviderErrorBodyNeverExported(t *testing.T) {
	const secret = "synthetic-provider-secret"
	for _, fallback := range []bool{false, true} {
		t.Run(strconv.FormatBool(fallback), func(t *testing.T) {
			sink, srv := newFakeCollector(t, http.StatusOK)
			client := NewFutureAGIClient(srv.URL, "key", "secret", "proj", false)
			defer client.Shutdown(t.Context())
			callErr := &modelModule.APIStatusError{Status: http.StatusUnauthorized, Body: "invalid api_key=" + secret}
			turn := newTestTurn(client)
			turn.llmStart("gpt-x", "OpenAI-API-Compatible", []modelModule.Message{{Role: "user", Content: "safe prompt"}})
			if !fallback {
				turn.llmEnd(callErr.Error(), 0, 0, callErr)
			}
			in := make(chan AsyncChatResult, 1)
			in <- AsyncChatResult{Answer: "**ERROR**: " + callErr.Error(), Final: true}
			close(in)
			out, _ := turn.observe(t.Context(), in, nil)
			for result := range out {
				if !strings.Contains(result.Answer, secret) {
					t.Fatal("tracing changed the user-visible result")
				}
			}
			body := sink.wait(t, 1)[0]
			if strings.Contains(body, secret) || strings.Contains(body, "invalid api_key") {
				t.Fatal("provider error body reached the tracing destination")
			}
			_, spans := decodeExport(t, body)
			root := spanByName(t, spans, "ragflow.chat")
			llm := spanByName(t, spans, "ragflow.llm")
			for _, span := range []otlpSpan{root, llm} {
				if span.Status.Code != 2 || span.Status.Message != futureAGIFailedError {
					t.Fatalf("missing safe failure status: %+v", span.Status)
				}
			}
			if attrMap(llm.Attributes)["llm.input_messages.0.message.content"] != "safe prompt" {
				t.Fatal("regression did not exercise content capture")
			}
			if attrMap(root.Attributes)["output.value"] != "**ERROR**: "+futureAGIFailedError || attrMap(llm.Attributes)["output.value"] != "" {
				t.Fatal("failed-turn output was not replaced")
			}
		})
	}
}

func TestFutureAGIFailedTurnIsError(t *testing.T) {
	sink, srv := newFakeCollector(t, http.StatusOK)
	client := NewFutureAGIClient(srv.URL, "key", "secret", "proj", false)
	defer client.Shutdown(t.Context())

	runSampleTurn(client, errors.New("openai: Post \"http://127.0.0.1:9/v1/chat/completions\": 401 Unauthorized"))

	_, spans := decodeExport(t, sink.wait(t, 1)[0])
	root := spanByName(t, spans, "ragflow.chat")
	llm := spanByName(t, spans, "ragflow.llm")
	if root.Status.Code != 2 || llm.Status.Code != 2 {
		t.Fatalf("status root=%d llm=%d", root.Status.Code, llm.Status.Code)
	}
	if llm.Status.Message != futureAGIFailedError {
		t.Fatalf("llm status message %q", llm.Status.Message)
	}
	if out := attrMap(llm.Attributes)["output.value"]; out != "" {
		t.Fatalf("failed call has output %q", out)
	}
	if out := attrMap(root.Attributes)["output.value"]; strings.Contains(out, "127.0.0.1") || !strings.HasPrefix(out, "**ERROR**") {
		t.Fatalf("root output %q", out)
	}
}

func TestFutureAGIQueueFullDropsWithoutBlocking(t *testing.T) {
	client := &FutureAGIClient{ProjectName: "proj", events: make(chan []byte, 1)}
	client.events <- []byte("full")
	done := make(chan struct{})
	go func() {
		turn := newTestTurn(client)
		turn.finish("answer", "", true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("a full queue blocked the chat path")
	}
	if client.dropped.Load() != 1 {
		t.Fatalf("dropped = %d", client.dropped.Load())
	}
}

func TestFutureAGICollectorErrorsDoNotLeakKeys(t *testing.T) {
	_, srv := newFakeCollector(t, http.StatusUnauthorized)
	client := &FutureAGIClient{Endpoint: srv.URL, APIKey: "key-should-not-appear", SecretKey: "secret-should-not-appear"}
	err := client.post(t.Context(), []byte("{}"))
	if err == nil || err.Error() != "Future AGI collector answered HTTP 401" {
		t.Fatalf("err = %v", err)
	}
	down := &FutureAGIClient{Endpoint: "http://127.0.0.1:1", APIKey: "key-should-not-appear", SecretKey: "secret-should-not-appear"}
	err = down.post(t.Context(), []byte("{}"))
	if err == nil || strings.Contains(err.Error(), "should-not-appear") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("collector-down error = %v", err)
	}
}

func TestFutureAGIShutdownPostsQueuedTraces(t *testing.T) {
	sink, srv := newFakeCollector(t, http.StatusOK)
	for i := 0; i < 20; i++ {
		client := &FutureAGIClient{
			Endpoint:    srv.URL,
			ProjectName: "proj",
			events:      make(chan []byte, 1),
			stop:        make(chan struct{}),
			stopped:     make(chan struct{}),
		}
		client.enqueue([]byte(`{"resourceSpans":[]}`))
		client.once.Do(func() { close(client.stop) })
		go client.worker()
		<-client.stopped
	}
	if got := len(sink.wait(t, 20)); got != 20 {
		t.Fatalf("posted %d of 20 queued traces", got)
	}
}

func TestCollectorEndpoint(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.futureagi.com":                  "https://api.futureagi.com/tracer/v1/traces",
		"https://api.futureagi.com/":                 "https://api.futureagi.com/tracer/v1/traces",
		"https://api.futureagi.com/tracer/v1/traces": "https://api.futureagi.com/tracer/v1/traces",
		"": "https://api.futureagi.com/tracer/v1/traces",
		"http://collector:4318/tracer/v1/traces/": "http://collector:4318/tracer/v1/traces",
	} {
		if got := collectorEndpoint(in); got != want {
			t.Errorf("collectorEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFutureAGIIDsAreRandom(t *testing.T) {
	a, b := newFutureAGITraceID(), newFutureAGITraceID()
	if len(a) != 32 || a == b {
		t.Fatalf("trace ids %q and %q", a, b)
	}
	if s := newFutureAGISpanID(); len(s) != 16 {
		t.Fatalf("span id %q", s)
	}
}

// --- turn lifecycle through the pipeline wrapper -------------------------

func useFutureAGITestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.TenantFutureAGI{}, &entity.User{}, &entity.UserTenant{}); err != nil {
		t.Fatal(err)
	}
	active := string(entity.StatusValid)
	if err := db.Create(&entity.User{ID: "user-1", Email: "member@example.com", Nickname: "member"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.UserTenant{ID: "membership-1", UserID: "user-1", TenantID: "tenant-1", Role: "owner", Status: &active}).Error; err != nil {
		t.Fatal(err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = orig
		forgetFutureAGIClient("tenant-1")
	})
}

func TestStartFutureAGITurnNeedsACompleteRow(t *testing.T) {
	useFutureAGITestDB(t)
	chat := &entity.Chat{ID: "chat-1", TenantID: "tenant-1"}
	msgs := []map[string]interface{}{{"role": "user", "content": "hi"}}

	if _, turn := startFutureAGITurn(t.Context(), chat, msgs, false); turn != nil {
		t.Fatal("no row must mean no export")
	}
	dao.DB.Create(&entity.TenantFutureAGI{TenantID: "tenant-1", APIKey: "k", SecretKey: "s", Endpoint: "http://collector", ProjectName: ""})
	if _, turn := startFutureAGITurn(t.Context(), chat, msgs, false); turn != nil {
		t.Fatal("a row without project_name must not export")
	}
	dao.DB.Model(&entity.TenantFutureAGI{}).Where("tenant_id = ?", "tenant-1").Update("project_name", "proj")

	ctx, turn := startFutureAGITurn(t.Context(), chat, msgs, false)
	if turn == nil || futureAGITurnFrom(ctx) != turn {
		t.Fatal("complete row must start a turn")
	}
	defer turn.abandon()
	if turn.sessionID != "chat-1" || turn.question != "hi" {
		t.Fatalf("session=%q question=%q", turn.sessionID, turn.question)
	}
	_, again := startFutureAGITurn(common.WithSessionID(t.Context(), "conv-9"), chat, msgs, false)
	defer again.abandon()
	if again.sessionID != "conv-9" {
		t.Fatalf("conversation id not used as session: %q", again.sessionID)
	}
	if again.client != turn.client {
		t.Fatal("same settings must reuse the cached client")
	}
	dao.DB.Model(&entity.TenantFutureAGI{}).Where("tenant_id = ?", "tenant-1").Update("project_name", "proj-2")
	_, rotated := startFutureAGITurn(t.Context(), chat, msgs, false)
	defer rotated.abandon()
	if rotated.client == turn.client || rotated.client.ProjectName != "proj-2" {
		t.Fatal("changed settings must build a new client")
	}
}

func TestObserveForwardsResultsAndExportsOnClose(t *testing.T) {
	sink, srv := newFakeCollector(t, http.StatusOK)
	client := NewFutureAGIClient(srv.URL, "key", "secret", "proj", false)
	defer client.Shutdown(t.Context())

	in := make(chan AsyncChatResult, 4)
	in <- AsyncChatResult{Answer: "Hel"}
	in <- AsyncChatResult{Answer: "lo"}
	in <- AsyncChatResult{Answer: "Hello", Final: true}
	close(in)

	turn := newTestTurn(client)
	turn.llmStart("gpt-x", "OpenAI-API-Compatible", []modelModule.Message{{Role: "user", Content: "say hello"}})
	out, err := turn.observe(t.Context(), in, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for r := range out {
		got = append(got, r.Answer)
	}
	if strings.Join(got, "|") != "Hel|lo|Hello" {
		t.Fatalf("forwarded %v", got)
	}
	_, spans := decodeExport(t, sink.wait(t, 1)[0])
	root := attrMap(spanByName(t, spans, "ragflow.chat").Attributes)
	llm := attrMap(spanByName(t, spans, "ragflow.llm").Attributes)
	if root["output.value"] != "Hello" || root["ragflow.pipeline"] != "solo" {
		t.Fatalf("root %v", root)
	}
	if llm["output.value"] != "Hello" || llm["llm.model_name"] != "gpt-x" {
		t.Fatalf("llm %v", llm)
	}
}

func TestObserveMarksErrorAnswers(t *testing.T) {
	sink, srv := newFakeCollector(t, http.StatusOK)
	client := NewFutureAGIClient(srv.URL, "key", "secret", "proj", false)
	defer client.Shutdown(t.Context())

	in := make(chan AsyncChatResult, 1)
	in <- AsyncChatResult{Answer: "**ERROR**: model gpt-x not found", Final: true}
	close(in)
	out, _ := newTestTurn(client).observe(t.Context(), in, nil)
	for range out {
	}
	_, spans := decodeExport(t, sink.wait(t, 1)[0])
	root := spanByName(t, spans, "ragflow.chat")
	if root.Status.Code != 2 || root.Status.Message != futureAGIFailedError {
		t.Fatalf("root status %d %q", root.Status.Code, root.Status.Message)
	}
}

func TestObserveKeepsDrainingAfterTheClientLeaves(t *testing.T) {
	_, srv := newFakeCollector(t, http.StatusOK)
	client := NewFutureAGIClient(srv.URL, "key", "secret", "proj", false)
	defer client.Shutdown(t.Context())

	ctx, cancel := context.WithCancel(t.Context())
	in := make(chan AsyncChatResult)
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		defer close(in)
		for i := 0; i < 50; i++ {
			in <- AsyncChatResult{Answer: "x"}
		}
	}()
	out, _ := newTestTurn(client).observe(ctx, in, nil)
	<-out
	cancel()
	select {
	case <-producerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline goroutine blocked after the consumer left")
	}
}

func TestObserveSkipsRejectedRequests(t *testing.T) {
	client := &FutureAGIClient{ProjectName: "proj", events: make(chan []byte, 1)}
	want := errors.New("the last content of this conversation is not from user")
	out, err := newTestTurn(client).observe(t.Context(), nil, want)
	if out != nil || err != want || len(client.events) != 0 {
		t.Fatalf("rejected request: out=%v err=%v queued=%d", out, err, len(client.events))
	}
	if out, err := (*futureAGITurn)(nil).observe(t.Context(), nil, nil); out != nil || err != nil {
		t.Fatal("nil turn must pass through")
	}
}

// TestDecorateAnswerClosesTheLLMSpan pins the hook in decorateAnswer: the
// answer text and RAGFlow's own token counts end the LLM span.
func TestDecorateAnswerClosesTheLLMSpan(t *testing.T) {
	sink, srv := newFakeCollector(t, http.StatusOK)
	client := NewFutureAGIClient(srv.URL, "key", "secret", "proj", false)
	defer client.Shutdown(t.Context())

	turn := newTestTurn(client)
	ctx := context.WithValue(t.Context(), futureAGITurnKey, turn)
	timer := common.NewTimer()
	timer.Start()
	turn.llmStart("model-x", "", []modelModule.Message{{Role: "user", Content: "question"}})
	time.Sleep(20 * time.Millisecond)

	s := &ChatPipelineService{}
	s.decorateAnswer(ctx, "hello", map[string]interface{}{"chunks": []interface{}{}, "doc_aggs": []interface{}{}},
		"system prompt", []string{"question"}, 7, timer, nil, 0, false, nil, "",
		map[string]interface{}{"llm_name": "model-x"}, "tenant-1", nil, false)
	turn.finish("hello", "", true)

	_, spans := decodeExport(t, sink.wait(t, 1)[0])
	llm := spanByName(t, spans, "ragflow.llm")
	a := attrMap(llm.Attributes)
	if a["output.value"] != "hello" || a["llm.token_count.prompt"] != "7" || a["llm.token_count.completion"] == "" {
		t.Fatalf("llm attrs %v", a)
	}
	start, _ := strconv.ParseInt(llm.Start, 10, 64)
	end, _ := strconv.ParseInt(llm.End, 10, 64)
	if end-start < int64(20*time.Millisecond) {
		t.Fatalf("llm span lasts %v", time.Duration(end-start))
	}
}

// --- settings service ------------------------------------------------------

type stubFutureAGIVerifier struct {
	err   error
	calls int
}

func (s *stubFutureAGIVerifier) Verify(ctx context.Context, endpoint, apiKey, secretKey string) error {
	s.calls++
	return s.err
}

func TestFutureAGIServiceSetGetDelete(t *testing.T) {
	useFutureAGITestDB(t)
	v := &stubFutureAGIVerifier{}
	svc := &FutureAGIService{futureAGIDAO: dao.NewFutureAGI(), verifier: v}
	ctx := permission.WithIdentity(t.Context(), "user-1", "tenant-1")

	if _, code, err := svc.SetAPIKey(ctx, FutureAGISettings{APIKey: "k"}); err == nil || code != common.CodeDataError {
		t.Fatalf("missing fields: code=%v err=%v", code, err)
	}
	if _, code, err := svc.SetAPIKey(ctx, FutureAGISettings{APIKey: "k", SecretKey: "s", ProjectName: "p", Endpoint: "ftp://x"}); err == nil || code != common.CodeDataError {
		t.Fatalf("bad endpoint: code=%v err=%v", code, err)
	}
	if v.calls != 0 {
		t.Fatal("invalid input must not reach the collector")
	}

	info, code, err := svc.SetAPIKey(ctx, FutureAGISettings{APIKey: "api-1234", SecretKey: "sec-5678", ProjectName: "proj"})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("save: code=%v err=%v", code, err)
	}
	if info.Endpoint != FutureAGIDefaultEndpoint || info.APIKey != "****1234" || info.SecretKey != "****5678" {
		t.Fatalf("info %+v", info)
	}
	row, _ := dao.NewFutureAGI().GetByTenantID(ctx, dao.DB, "tenant-1")
	if row == nil || row.SecretKey != "sec-5678" || row.Redact {
		t.Fatalf("stored row %+v", row)
	}

	if _, _, err := svc.SetAPIKey(ctx, FutureAGISettings{APIKey: "api-1234", SecretKey: "sec-5678", ProjectName: "proj-2", Endpoint: "http://collector:4318/", Redact: true}); err != nil {
		t.Fatal(err)
	}
	got, _, _, err := svc.GetAPIKey(ctx)
	if err != nil || got.ProjectName != "proj-2" || got.Endpoint != "http://collector:4318" || !got.Redact || strings.Contains(got.SecretKey, "sec") {
		t.Fatalf("get %+v err=%v", got, err)
	}

	v.err = ErrFutureAGIUnauthorized
	if _, code, err := svc.SetAPIKey(ctx, FutureAGISettings{APIKey: "bad", SecretKey: "bad", ProjectName: "proj"}); !errors.Is(err, ErrFutureAGIUnauthorized) || code != common.CodeDataError {
		t.Fatalf("bad keys: code=%v err=%v", code, err)
	}
	row, _ = dao.NewFutureAGI().GetByTenantID(ctx, dao.DB, "tenant-1")
	if row.APIKey != "api-1234" {
		t.Fatal("a rejected save must not replace the stored keys")
	}

	ok, _, msg, err := svc.DeleteAPIKey(ctx)
	if !ok || msg != "" || err != nil {
		t.Fatalf("delete ok=%v msg=%q err=%v", ok, msg, err)
	}
	if _, _, msg, _ := svc.DeleteAPIKey(ctx); msg == "" {
		t.Fatal("second delete must report no record")
	}
	if got, _, msg, _ := svc.GetAPIKey(ctx); got != nil || msg == "" {
		t.Fatal("get after delete must report no record")
	}
}

func TestDefaultFutureAGIVerifier(t *testing.T) {
	sink, ok := newFakeCollector(t, http.StatusOK)
	if err := (defaultFutureAGIVerifier{}).Verify(t.Context(), ok.URL, "k", "s"); err != nil {
		t.Fatalf("200: %v", err)
	}
	if body := sink.wait(t, 1)[0]; body != `{"resourceSpans":[]}` {
		t.Fatalf("verify body %q", body)
	}
	_, denied := newFakeCollector(t, http.StatusUnauthorized)
	if err := (defaultFutureAGIVerifier{}).Verify(t.Context(), denied.URL, "k", "s"); !errors.Is(err, ErrFutureAGIUnauthorized) {
		t.Fatalf("401: %v", err)
	}
	if err := (defaultFutureAGIVerifier{}).Verify(t.Context(), "http://127.0.0.1:1", "k", "s"); err == nil || errors.Is(err, ErrFutureAGIUnauthorized) {
		t.Fatalf("down: %v", err)
	}
}

// TestShutdownWaitsForInFlightTurns pins the server-stop order: a handler can
// return on the final result while the pipeline is still closing its channel,
// so the flush must wait for that turn instead of dropping it.
func TestShutdownWaitsForInFlightTurns(t *testing.T) {
	useFutureAGITestDB(t)
	sink, srv := newFakeCollector(t, http.StatusOK)
	dao.DB.Create(&entity.TenantFutureAGI{TenantID: "tenant-1", APIKey: "k", SecretKey: "s", Endpoint: srv.URL, ProjectName: "proj"})
	chat := &entity.Chat{ID: "chat-1", TenantID: "tenant-1"}
	msgs := []map[string]interface{}{{"role": "user", "content": "hi"}}

	_, rejected := startFutureAGITurn(t.Context(), chat, msgs, false)
	if out, err := rejected.observe(t.Context(), nil, errors.New("rejected")); out != nil || err == nil {
		t.Fatal("rejected request must pass the error through")
	}

	_, turn := startFutureAGITurn(t.Context(), chat, msgs, false)
	go func() {
		time.Sleep(100 * time.Millisecond)
		turn.finish("late answer", "", true)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	ShutdownFutureAGIClients(ctx)
	if ctx.Err() != nil {
		t.Fatal("shutdown timed out: a turn was never released")
	}
	body := sink.wait(t, 1)[0]
	if !strings.Contains(body, "late answer") {
		t.Fatalf("late turn not exported: %s", body)
	}
}
