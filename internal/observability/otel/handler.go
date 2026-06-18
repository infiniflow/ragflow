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

// Package otel provides OpenTelemetry tracing infrastructure for
// agent runs. After the eino-to-harness migration, this package
// exposes context helpers (WithRunID / WithSessionID) and a span
// lifecycle API (StartSpan / EndSpan / RecordError) that harness
// middlewares and other agent runtime components can use directly
// — independent of any framework's callback system.
package otel

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
	"go.opentelemetry.io/otel/trace/noop"
)

// TracerName is the instrumentation scope used for every span created
// through this package. Exported so other components can refer to the
// same scope if they need to look up the tracer.
const TracerName = "github.com/infiniflow/ragflow/internal/observability/otel"

// Context-key types for run/session metadata.
type (
	runIDKeyType       struct{}
	sessionIDKeyType   struct{}
	spanContextKeyType struct{}
)

// WithRunID returns a copy of ctx that carries the supplied agent run
// id. The value is attached as a "run.id" attribute on every span.
func WithRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, runIDKeyType{}, runID)
}

// RunIDFromContext returns the run id stored on ctx, or "" if none.
func RunIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(runIDKeyType{}).(string); ok {
		return v
	}
	return ""
}

// WithSessionID returns a copy of ctx that carries the supplied chat
// session id. Attached as "session.id" on every span.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKeyType{}, sessionID)
}

// SessionIDFromContext returns the session id stored on ctx, or "".
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDKeyType{}).(string); ok {
		return v
	}
	return ""
}

// tracerProvider is the package-level TracerProvider that drives all
// span creation. Set via SetTracerProvider; defaults to noop.
var (
	tpMu       sync.Mutex
	tp         *sdktrace.TracerProvider
	tracer     trace.Tracer
	initTracer sync.Once
)

// SetTracerProvider replaces the package-level tracer. Safe to call
// once during server startup. A nil provider results in no-op spans.
func SetTracerProvider(provider *sdktrace.TracerProvider) {
	tpMu.Lock()
	defer tpMu.Unlock()
	tp = provider
	initTracer = sync.Once{} // reset so resolveTracer re-initialises
}

// resolveTracer returns a cached tracer derived from tp. When tp is
// nil a noop tracer is returned so every caller can call Start/End
// unconditionally.
func resolveTracer() trace.Tracer {
	tpMu.Lock()
	tpVal := tp
	tpMu.Unlock()
	initTracer.Do(func() {
		if tpVal == nil {
			tracer = noop.NewTracerProvider().Tracer(TracerName)
			return
		}
		tracer = tpVal.Tracer(TracerName)
	})
	return tracer
}

// spanContextKey is the typed context key for in-flight span metadata.
var spanContextKey = spanContextKeyType{}

// spanContextValue carries the in-flight span metadata on the context.
type spanContextValue struct {
	span      trace.Span
	startTime time.Time
}

// spanAttrs builds the standard set of attributes that flow onto every
// span emitted through this package.
func spanAttrs(runID, sessionID string, extra ...attribute.KeyValue) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2+len(extra))
	attrs = append(attrs,
		attribute.String("run.id", runID),
		attribute.String("session.id", sessionID),
	)
	attrs = append(attrs, extra...)
	return attrs
}

// StartSpan begins a new span named name and attaches it to the returned
// context. The caller MUST call EndSpan (or EndSpanWithError) with the
// returned context to finalise the span.
//
// extraKeyValues are appended as span attributes.
func StartSpan(ctx context.Context, name string, extraKeyValues ...attribute.KeyValue) context.Context {
	runID := RunIDFromContext(ctx)
	sessionID := SessionIDFromContext(ctx)
	startedCtx, span := resolveTracer().Start(ctx, name,
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(spanAttrs(runID, sessionID, extraKeyValues...)...),
	)
	return context.WithValue(startedCtx, spanContextKey, &spanContextValue{
		span:      span,
		startTime: time.Now(),
	})
}

// EndSpan finalises the span previously started by StartSpan on this
// context. It is a no-op when no in-flight span is found.
func EndSpan(ctx context.Context) context.Context {
	v, ok := ctx.Value(spanContextKey).(*spanContextValue)
	if !ok || v == nil {
		return ctx
	}
	v.span.End(trace.WithTimestamp(time.Now()))
	return context.WithValue(ctx, spanContextKey, (*spanContextValue)(nil))
}

// EndSpanWithError records err on the in-flight span, marks it with
// OTel's "Error" status, and ends it. No-op when err is nil or no
// span is on the context.
func EndSpanWithError(ctx context.Context, err error) context.Context {
	if err == nil {
		return EndSpan(ctx)
	}
	v, ok := ctx.Value(spanContextKey).(*spanContextValue)
	if !ok || v == nil {
		return ctx
	}
	v.span.RecordError(err, trace.WithTimestamp(time.Now()))
	v.span.SetStatus(codes.Error, err.Error())
	v.span.End(trace.WithTimestamp(time.Now()))
	return context.WithValue(ctx, spanContextKey, (*spanContextValue)(nil))
}

// Compile-time assertions that keep the OTel SDK imports meaningful.
var (
	_ = sdktrace.TracerProvider{}
	_ = embedded.Tracer(nil)
)
