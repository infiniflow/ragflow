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
	"sync"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
	"go.opentelemetry.io/otel/trace/noop"
)

// TracerName is the instrumentation scope used for every span created by
// [OtelHandler]. The constant is exported so other components can refer to
// the same scope if they need to look up the tracer.
const TracerName = "github.com/infiniflow/ragflow/internal/observability/otel"

// Context-key types. These are unexported (the value is exported via the
// constructor helpers below) so that external packages cannot collide
// with the keys we attach to the callback context.
type (
	runIDKeyType       struct{}
	sessionIDKeyType   struct{}
	spanContextKeyType struct{}
)

// WithRunID returns a copy of ctx that carries the supplied canvas run
// id. The handler will read this value and attach it as a "run.id"
// attribute on every emitted span. Pass an empty string to clear.
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
// session id. The handler will read this value and attach it as a
// "session.id" attribute on every emitted span.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKeyType{}, sessionID)
}

// SessionIDFromContext returns the session id stored on ctx, or "" if
// none.
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDKeyType{}).(string); ok {
		return v
	}
	return ""
}

var (
	spanContextKey = spanContextKeyType{}
	_              = sdktrace.TracerProvider{} // keep sdktrace import meaningful across refactors
)

// spanContextValue bundles the live span with its start time and any
// stream reader that the streaming callbacks need to clean up.
type spanContextValue struct {
	span      trace.Span
	startTime time.Time
	// streamIn is the OnStartWithStreamInput copy the framework handed to
	// us; we must close it once we have read (or decided to skip) the
	// stream so the framework can recycle the original.
	streamIn *schema.StreamReader[callbacks.CallbackInput]
	// streamOut is the OnEndWithStreamOutput copy; same cleanup contract.
	streamOut *schema.StreamReader[callbacks.CallbackOutput]
}

// OtelHandler implements [callbacks.Handler] and bridges every eino
// component invocation to an OTel span.
//
// The handler is safe for concurrent use: it derives the per-call span
// from the provider's [trace.Tracer] (which is itself goroutine-safe) and
// stores the in-flight span on the callback context using an unexported
// key. OnEnd / OnError / streaming variants look up that key to finalise
// the span.
//
// A nil *OtelHandler.tp is treated as a no-op: every method returns the
// received context unchanged and creates no span. This makes it cheap to
// install the handler globally in environments that have not configured
// an OTel collector yet.
type OtelHandler struct {
	// tp is the provider that owns the tracer used to mint spans. nil
	// means "skip everything" — the handler becomes a transparent pass-
	// through.
	tp *sdktrace.TracerProvider

	// tracer is cached to avoid re-resolving it on every span start.
	// It is built lazily from tp so a nil tp does not panic.
	tracer   trace.Tracer
	initOnce sync.Once
}

// NewOtelHandler wraps tp in a callbacks.Handler. A nil tp is accepted;
// the returned handler then behaves as a pass-through that never emits
// spans, so callers can wire it up unconditionally.
func NewOtelHandler(tp *sdktrace.TracerProvider) *OtelHandler {
	return &OtelHandler{tp: tp}
}

// resolveTracer returns the cached tracer, falling back to the global
// noop tracer when tp is nil. It is the only place that touches tp, so
// the rest of the methods can assume a non-nil tracer.
func (h *OtelHandler) resolveTracer() trace.Tracer {
	h.initOnce.Do(func() {
		if h.tp == nil {
			h.tracer = noop.NewTracerProvider().Tracer(TracerName)
			return
		}
		h.tracer = h.tp.Tracer(TracerName)
	})
	return h.tracer
}

// spanName builds the OTel span name for a given RunInfo. The convention
// is "<Component>:<Name>" — component category first, then the business
// name (which is the canvas node id for nodes created via
// compose.WithNodeName). When info is nil or both fields are empty the
// span is named just "component" so it is still visible in the trace UI.
func spanName(info *callbacks.RunInfo) string {
	if info == nil {
		return "component"
	}
	component := string(info.Component)
	name := info.Name
	switch {
	case component == "" && name == "":
		return "component"
	case component == "":
		return name
	case name == "":
		return component
	default:
		return component + ":" + name
	}
}

// runAttributes returns the standard set of attributes that every span
// emitted by the handler carries. The cpn.* / run.id / session.id tuple
// makes the span easy to slice by tenant, canvas or individual run in
// any OTel backend.
func runAttributes(info *callbacks.RunInfo, runID, sessionID string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("run.id", runID),
		attribute.String("session.id", sessionID),
	}
	if info == nil {
		return attrs
	}
	// Canvas DSL loads each node with cpn_id as the node name, so
	// info.Name is a reliable cpn.id surrogate. We expose it under a
	// dedicated attribute so dashboards can filter by it directly.
	if info.Name != "" {
		attrs = append(attrs,
			attribute.String("cpn.id", info.Name),
			attribute.String("cpn.name", info.Name),
		)
	}
	if component := string(info.Component); component != "" {
		attrs = append(attrs, attribute.String("cpn.component", component))
	}
	if info.Type != "" {
		attrs = append(attrs, attribute.String("cpn.type", info.Type))
	}
	return attrs
}

// OnStart is the entry point for a non-streaming component invocation.
// It starts a span, attaches the standard run attributes, and stores the
// live span on the returned context so the matching OnEnd/OnError call
// can finalise it.
func (h *OtelHandler) OnStart(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
	if h.tp == nil {
		return ctx
	}
	runID := RunIDFromContext(ctx)
	sessionID := SessionIDFromContext(ctx)
	startedCtx, span := h.resolveTracer().Start(ctx, spanName(info),
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(runAttributes(info, runID, sessionID)...),
	)
	return context.WithValue(startedCtx, spanContextKey, &spanContextValue{
		span:      span,
		startTime: time.Now(),
	})
}

// OnEnd finalises the span started by the matching OnStart. It is a no-op
// if there is no in-flight span on the context (which can happen when
// the handler is installed in a chain alongside another handler that
// consumed the span first — defensive programming for the
// order-independent multi-handler contract).
func (h *OtelHandler) OnEnd(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackOutput) context.Context {
	v, ok := ctx.Value(spanContextKey).(*spanContextValue)
	if !ok || v == nil {
		return ctx
	}
	// Drop the key so the same ctx is not reused for a different span
	// should eino (or a future refactor) ever share callbacks across
	// goroutines.
	v.span.End(trace.WithTimestamp(time.Now()))
	return context.WithValue(ctx, spanContextKey, (*spanContextValue)(nil))
}

// OnError records the error on the in-flight span and marks it with
// OTel's "Error" status code. If no span is on the context (e.g. OnStart
// was never called, or the handler is in no-op mode), OnError is a no-op.
func (h *OtelHandler) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	if err == nil {
		// Treat nil error as a non-event so we never mark a span Error
		// when the framework calls us defensively.
		return ctx
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

// OnStartWithStreamInput mirrors [OtelHandler.OnStart] for streaming
// inputs. eino hands us a *schema.StreamReader that we own a copy of;
// we must close it after we are done so the framework can release the
// original. The handler does not consume the stream — that is the
// downstream component's job — so the close is all the bookkeeping we
// need.
func (h *OtelHandler) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo,
	input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	if h.tp == nil {
		if input != nil {
			input.Close()
		}
		return ctx
	}
	runID := RunIDFromContext(ctx)
	sessionID := SessionIDFromContext(ctx)
	startedCtx, span := h.resolveTracer().Start(ctx, spanName(info),
		trace.WithTimestamp(time.Now()),
		trace.WithAttributes(runAttributes(info, runID, sessionID)...),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	if input != nil {
		input.Close()
	}
	return context.WithValue(startedCtx, spanContextKey, &spanContextValue{
		span:      span,
		startTime: time.Now(),
		streamIn:  input,
	})
}

// OnEndWithStreamOutput mirrors [OtelHandler.OnEnd] for streaming
// outputs. The framework hands us a copy of the output stream; we
// close it without consuming it (the SSE handler in the canvas package
// is the actual consumer) and finalise the span.
func (h *OtelHandler) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo,
	output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	v, ok := ctx.Value(spanContextKey).(*spanContextValue)
	if !ok || v == nil {
		if output != nil {
			output.Close()
		}
		return ctx
	}
	if output != nil {
		output.Close()
		v.streamOut = output
	}
	v.span.End(trace.WithTimestamp(time.Now()))
	return context.WithValue(ctx, spanContextKey, (*spanContextValue)(nil))
}

// Compile-time assertion that *OtelHandler satisfies the eino Handler
// interface. Catches signature drift the moment the file is compiled.
var _ callbacks.Handler = (*OtelHandler)(nil)

// Keep the embedded interface referenced so an upgrade that drops
// trace/embedded from go.opentelemetry.io/otel still surfaces a
// compile error here rather than a silent Tracer regression.
var _ embedded.Tracer = (embedded.Tracer)(nil)
