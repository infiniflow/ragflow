// Package telemetry provides an OpenTelemetry ReAct middleware for harness-go.
//
// Usage:
//
//	import telemetrymw "ragflow/internal/harness/core/middlewares/telemetry"
//
//	cfg := core.DefaultReActConfig[*schema.Message]()
//	cfg.Middlewares = append(cfg.Middlewares, telemetrymw.New())
//
// To customize:
//
//	mw := telemetrymw.New(telemetrymw.WithTracing(false))
//
// The middleware uses RAGFlow's global TracerProvider (configured in
// internal/observability/otel). Tracing is only active when the provider
// has been initialized with an OTLP collector endpoint.
package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// Config holds configuration for the telemetry middleware.
type Config struct {
	EnableTracing bool
}

// Option configures the telemetry middleware.
type Option func(*Config)

// WithTracing enables or disables distributed tracing.
func WithTracing(enabled bool) Option {
	return func(c *Config) { c.EnableTracing = enabled }
}

func defaultConfig() *Config {
	return &Config{EnableTracing: true}
}

const tracerName = "ragflow/internal/harness/core/middlewares/telemetry"

// Middleware is a ReAct middleware that instruments agent execution with
// OpenTelemetry tracing spans. It wraps model calls and tool invocations.
//
// NOTE: Metrics are not yet supported — RAGFlow currently only configures
// a TracerProvider (see internal/observability/otel). Once a MeterProvider
// is added, metrics recording can be restored here.
//
// TODO: Make this generic (Middleware[M]) to support AgenticMessage alongside
// *schema.Message. Currently hardcoded to *schema.Message, unlike other
// middlewares that use BaseMiddleware[M].
type Middleware struct {
	core.BaseMiddleware[*schema.Message]
	cfg    *Config
	tracer trace.Tracer
}

// New creates a new telemetry middleware with default settings.
func New(opts ...Option) *Middleware {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	m := &Middleware{cfg: cfg}
	if cfg.EnableTracing {
		m.tracer = otel.Tracer(tracerName)
	}
	return m
}

// recordSpanError sets span status and records the error.
func recordSpanError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.SetStatus(codes.Error, err.Error())
	span.RecordError(err)
}

// WrapModel wraps the model call with a tracing span.
func (m *Middleware) WrapModel(ctx context.Context, model core.Model[*schema.Message], mc *core.ModelContext) (core.Model[*schema.Message], error) {
	if m.tracer == nil {
		return model, nil
	}
	return &tracedModel{
		inner:   model,
		mw:      m,
		toolCnt: len(mc.Tools),
	}, nil
}

// WrapToolInvoke wraps a synchronous tool call with a span.
func (m *Middleware) WrapToolInvoke(ctx context.Context, ep core.InvokableToolEndpoint, tc *core.ToolContext) (core.InvokableToolEndpoint, error) {
	if m.tracer == nil {
		return ep, nil
	}
	return func(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
		var span trace.Span
		if m.cfg.EnableTracing {
			ctx, span = m.tracer.Start(ctx, "tool."+tc.Name,
				trace.WithAttributes(
					attribute.String("tool.name", tc.Name),
					attribute.Int("args.size", len(args)),
				),
				trace.WithSpanKind(trace.SpanKindInternal),
			)
		}
		result, err := ep(ctx, args, opts...)
		if span != nil && span.IsRecording() {
			if err != nil {
				recordSpanError(span, err)
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.End()
		}
		return result, err
	}, nil
}

// WrapToolStream wraps a streaming tool call with a span.
func (m *Middleware) WrapToolStream(ctx context.Context, ep core.StreamableToolEndpoint, tc *core.ToolContext) (core.StreamableToolEndpoint, error) {
	if m.tracer == nil {
		return ep, nil
	}
	return func(ctx context.Context, args string, opts ...core.ToolOption) (*schema.StreamReader[string], error) {
		var span trace.Span
		if m.cfg.EnableTracing {
			ctx, span = m.tracer.Start(ctx, "tool.stream."+tc.Name,
				trace.WithAttributes(attribute.String("tool.name", tc.Name)),
				trace.WithSpanKind(trace.SpanKindInternal),
			)
		}
		result, err := ep(ctx, args, opts...)
		if err != nil {
			if span != nil {
				recordSpanError(span, err)
				span.End()
			}
			return nil, err
		}
		if span != nil {
			span.SetStatus(codes.Ok, "")
			span.End()
		}
		return result, nil
	}, nil
}

// WrapEnhancedInvokableToolCall wraps an enhanced tool call with a span.
func (m *Middleware) WrapEnhancedInvokableToolCall(ctx context.Context, ep core.EnhancedInvokableToolEndpoint, tc *core.ToolContext) (core.EnhancedInvokableToolEndpoint, error) {
	if m.tracer == nil {
		return ep, nil
	}
	return func(ctx context.Context, args *schema.ToolArgument, opts ...core.ToolOption) (*schema.ToolResult, error) {
		var span trace.Span
		if m.cfg.EnableTracing {
			ctx, span = m.tracer.Start(ctx, "enhanced_tool."+tc.Name,
				trace.WithAttributes(attribute.String("tool.name", tc.Name)),
				trace.WithSpanKind(trace.SpanKindInternal),
			)
		}
		result, err := ep(ctx, args, opts...)
		if span != nil && span.IsRecording() {
			if err != nil {
				recordSpanError(span, err)
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.End()
		}
		return result, err
	}, nil
}

// WrapEnhancedStreamableToolCall wraps an enhanced streaming tool call.
func (m *Middleware) WrapEnhancedStreamableToolCall(ctx context.Context, ep core.EnhancedStreamableToolEndpoint, tc *core.ToolContext) (core.EnhancedStreamableToolEndpoint, error) {
	if m.tracer == nil {
		return ep, nil
	}
	return func(ctx context.Context, args *schema.ToolArgument, opts ...core.ToolOption) (*schema.StreamReader[*schema.ToolResult], error) {
		var span trace.Span
		if m.cfg.EnableTracing {
			ctx, span = m.tracer.Start(ctx, "enhanced_tool.stream."+tc.Name,
				trace.WithAttributes(attribute.String("tool.name", tc.Name)),
				trace.WithSpanKind(trace.SpanKindInternal),
			)
		}
		result, err := ep(ctx, args, opts...)
		if err != nil {
			if span != nil {
				recordSpanError(span, err)
				span.End()
			}
			return nil, err
		}
		if span != nil {
			span.SetStatus(codes.Ok, "")
			span.End()
		}
		return result, nil
	}, nil
}

// tracedModel wraps a Model with OpenTelemetry tracing.
type tracedModel struct {
	inner   core.Model[*schema.Message]
	mw      *Middleware
	toolCnt int
}

func (m *tracedModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	var span trace.Span
	if m.mw.cfg.EnableTracing && m.mw.tracer != nil {
		ctx, span = m.mw.tracer.Start(ctx, "model.generate",
			trace.WithAttributes(
				attribute.Int("messages.count", len(msgs)),
				attribute.Int("tools.count", m.toolCnt),
			),
			trace.WithSpanKind(trace.SpanKindClient),
		)
	}
	resp, err := m.inner.Generate(ctx, msgs, opts...)
	if span != nil && span.IsRecording() {
		if err != nil {
			recordSpanError(span, err)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
	return resp, err
}

func (m *tracedModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	var span trace.Span
	if m.mw.cfg.EnableTracing && m.mw.tracer != nil {
		ctx, span = m.mw.tracer.Start(ctx, "model.stream",
			trace.WithAttributes(
				attribute.Int("messages.count", len(msgs)),
				attribute.Int("tools.count", m.toolCnt),
			),
			trace.WithSpanKind(trace.SpanKindClient),
		)
	}
	result, err := m.inner.Stream(ctx, msgs, opts...)
	if err != nil {
		if span != nil {
			recordSpanError(span, err)
			span.End()
		}
		return nil, err
	}
	if span != nil {
		span.SetStatus(codes.Ok, "")
		span.End()
	}
	return result, nil
}

func (m *tracedModel) BindTools(tools []*schema.ToolInfo) error {
	return m.inner.BindTools(tools)
}

// Ensure Middleware implements the core middleware interface.
var _ core.ReActMiddleware = (*Middleware)(nil)
