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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// setupTestTracer configures a tracer backed by an in-memory span
// recorder so tests can assert on emitted spans without a collector.
func setupTestTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})
	return recorder
}

// TestStartEndSpan asserts a StartSpan / EndSpan pair produces one
// span with the expected name and attributes.
func TestStartEndSpan(t *testing.T) {
	recorder := setupTestTracer(t)
	ctx := context.Background()
	ctx = WithRunID(ctx, "run-1")
	ctx = WithSessionID(ctx, "sess-1")

	ctx = StartSpan(ctx, "test-agent", attribute.String("extra.key", "extra-val"))
	ctx = EndSpan(ctx)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name() != "test-agent" {
		t.Errorf("span name = %q, want %q", s.Name(), "test-agent")
	}
	attrs := attrsToMap(s.Attributes())
	if attrs["run.id"] != "run-1" {
		t.Errorf("run.id = %q, want %q", attrs["run.id"], "run-1")
	}
	if attrs["session.id"] != "sess-1" {
		t.Errorf("session.id = %q, want %q", attrs["session.id"], "sess-1")
	}
	if attrs["extra.key"] != "extra-val" {
		t.Errorf("extra.key = %q, want %q", attrs["extra.key"], "extra-val")
	}
}

// TestEndSpanWithError records an error on the span and marks it.
func TestEndSpanWithError(t *testing.T) {
	recorder := setupTestTracer(t)
	ctx := StartSpan(context.Background(), "err-agent")

	err := errors.New("something went wrong")
	ctx = EndSpanWithError(ctx, err)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Status().Code != codes.Error {
		t.Errorf("status code = %v, want Error", s.Status().Code)
	}
}

// TestStartEndSpan_NoopWithoutProvider verifies that calling span
// lifecycle functions without setting a tracer provider does not
// panic and produces no spans.
func TestStartEndSpan_NoopWithoutProvider(t *testing.T) {
	// Ensure default noop state.
	SetTracerProvider(nil)

	ctx := context.Background()
	ctx = StartSpan(ctx, "ghost")
	ctx = EndSpan(ctx)

	// If no panic occurs, the test passes.
}

// TestWithRunID_SessionID verifies context helpers round-trip.
func TestWithRunID_SessionID(t *testing.T) {
	ctx := WithRunID(context.Background(), "abc")
	if got := RunIDFromContext(ctx); got != "abc" {
		t.Errorf("RunIDFromContext = %q, want %q", got, "abc")
	}
	ctx = WithSessionID(ctx, "xyz")
	if got := SessionIDFromContext(ctx); got != "xyz" {
		t.Errorf("SessionIDFromContext = %q, want %q", got, "xyz")
	}
}

// attrsToMap converts a []attribute.KeyValue to a map for easy
// assertion.
func attrsToMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = a.Value.AsString()
	}
	return m
}
