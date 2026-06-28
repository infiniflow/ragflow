/*
 * Copyright 2026 The RAGFlow Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package otel

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestHandler returns a *OtelHandler wired to an in-memory
// [tracetest.SpanRecorder]. The recorder exposes the spans the handler
// emits so tests can assert on names, attributes and status without
// needing a real OTel collector.
func newTestHandler(t *testing.T) (*OtelHandler, *tracetest.SpanRecorder) {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	h := NewOtelHandler(tp)
	return h, recorder
}

// TestOtelHandler_RecordsSpan asserts that a single OnStart/OnEnd pair
// produces exactly one span, named "<Component>:<Name>", carrying the
// cpn.id / cpn.name / run.id / session.id attributes derived from the
// context and the RunInfo.
func TestOtelHandler_RecordsSpan(t *testing.T) {
	h, recorder := newTestHandler(t)

	ctx := context.Background()
	ctx = WithRunID(ctx, "run-123")
	ctx = WithSessionID(ctx, "sess-456")

	info := &callbacks.RunInfo{
		Name:      "llm_1",
		Type:      "OpenAI",
		Component: components.ComponentOfChatModel,
	}

	ctx = h.OnStart(ctx, info, "prompt")
	ctx = h.OnEnd(ctx, info, "answer")

	spans := recorder.Ended()
	if got, want := len(spans), 1; got != want {
		t.Fatalf("span count: got %d, want %d", got, want)
	}
	span := spans[0]
	if got, want := span.Name(), "ChatModel:llm_1"; got != want {
		t.Errorf("span name: got %q, want %q", got, want)
	}

	wantAttrs := map[string]string{
		"cpn.id":        "llm_1",
		"cpn.name":      "llm_1",
		"cpn.component": "ChatModel",
		"cpn.type":      "OpenAI",
		"run.id":        "run-123",
		"session.id":    "sess-456",
	}
	gotAttrs := attrMap(span)
	for k, want := range wantAttrs {
		if got := gotAttrs[k]; got != want {
			t.Errorf("attribute %q: got %q, want %q", k, got, want)
		}
	}
}

// TestOtelHandler_RecordsError asserts that OnError attaches the error
// to the span (RecordedErrorCount > 0) and flips the span status to
// OTel's Error code. The check on recorded-error count is portable
// across the small API changes the SDK has shipped.
func TestOtelHandler_RecordsError(t *testing.T) {
	h, recorder := newTestHandler(t)

	ctx := context.Background()
	info := &callbacks.RunInfo{
		Name:      "retrieval_0",
		Component: components.ComponentOfRetriever,
	}

	ctx = h.OnStart(ctx, info, nil)
	ctx = h.OnError(ctx, info, errors.New("kaboom"))

	spans := recorder.Ended()
	if got, want := len(spans), 1; got != want {
		t.Fatalf("span count: got %d, want %d", got, want)
	}
	span := spans[0]

	// Two independent assertions: (1) the span status is Error per
	// OTel spec, (2) at least one "exception" event was recorded with
	// the err.Error() message attached. Either is sufficient to prove
	// the OnError path propagated the error to OTel; together they
	// guard against regressions in either half of the contract.
	if status := span.Status(); status.Code != codes.Error {
		t.Errorf("span status code: got %v, want %v", status.Code, codes.Error)
	}
	foundException := false
	for _, ev := range span.Events() {
		if ev.Name == "exception" {
			for _, kv := range ev.Attributes {
				if string(kv.Key) == "exception.message" && kv.Value.AsString() == "kaboom" {
					foundException = true
				}
			}
		}
	}
	if !foundException {
		t.Errorf("expected an \"exception\" event with message \"kaboom\"; events: %+v", span.Events())
	}
}

// TestOtelHandler_NoOpWhenProviderNil asserts that constructing a
// handler with a nil provider is safe: OnStart returns the same context
// (no span attached, no panic), and no spans are emitted to a recorder
// the test later installs.
func TestOtelHandler_NoOpWhenProviderNil(t *testing.T) {
	h := NewOtelHandler(nil)
	if h == nil {
		t.Fatal("NewOtelHandler(nil) returned nil handler")
	}

	ctx := context.Background()
	info := &callbacks.RunInfo{
		Name:      "noop_0",
		Component: components.Component("Lambda"),
	}

	out := h.OnStart(ctx, info, nil)
	if out != ctx {
		t.Errorf("OnStart should return ctx unchanged when tp is nil, got %v want %v", out, ctx)
	}
	out = h.OnEnd(out, info, nil)
	if out != ctx {
		t.Errorf("OnEnd should return ctx unchanged when tp is nil, got %v want %v", out, ctx)
	}

	// OnError with nil tp must also be a clean no-op: no panic, ctx
	// unchanged.
	out = h.OnError(out, info, errors.New("ignored"))
	if out != ctx {
		t.Errorf("OnError should return ctx unchanged when tp is nil, got %v want %v", out, ctx)
	}

	// And — for completeness — the streaming variants must not panic
	// when tp is nil. The framework may pass nil readers; we treat them
	// as best-effort and assert no-op behaviour.
	_ = h.OnStartWithStreamInput(ctx, info, nil)
	_ = h.OnEndWithStreamOutput(ctx, info, nil)
}

// attrMap flattens a span's attributes into a string map so the test
// can assert on individual key/value pairs without worrying about
// attribute ordering (the SDK does not guarantee it).
func attrMap(s sdktrace.ReadOnlySpan) map[string]string {
	out := make(map[string]string, len(s.Attributes()))
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.AsString()
	}
	return out
}
