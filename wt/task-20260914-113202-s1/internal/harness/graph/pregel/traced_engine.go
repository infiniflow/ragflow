// Package pregel provides tracing/callback wrappers around the Pregel Engine.
//
// TracedEngine wraps Engine.Run with OpenTelemetry spans and lifecycle callbacks
// without modifying the Engine struct itself.
package pregel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

// tracedEngineTracerName is the OTel tracer name for the traced engine wrapper.
const tracedEngineTracerName = "ragflow/internal/harness/graph/pregel/traced"

// TracedEngineOption configures tracing behavior.
type TracedEngineOption func(*tracedEngineConfig)

type tracedEngineConfig struct {
	enabled         bool
	recordArguments bool
	recordResults   bool
	eventFilter     func(string) bool
	callbacks       *CallbackManager
}

func defaultTracingConfig() *tracedEngineConfig {
	return &tracedEngineConfig{
		enabled:         true,
		recordArguments: true,
		recordResults:   true,
		eventFilter:     nil,
	}
}

// WithTracedEngineDisabled disables tracing for a particular engine instance.
func WithTracedEngineDisabled() TracedEngineOption {
	return func(c *tracedEngineConfig) { c.enabled = false }
}

// WithTracedEngineRecordArgs enables/disables argument size recording.
func WithTracedEngineRecordArgs(enabled bool) TracedEngineOption {
	return func(c *tracedEngineConfig) { c.recordArguments = enabled }
}

// WithTracedEngineRecordResults enables/disables result size recording.
func WithTracedEngineRecordResults(enabled bool) TracedEngineOption {
	return func(c *tracedEngineConfig) { c.recordResults = enabled }
}

// TracedEngine wraps an Engine with OpenTelemetry tracing and callbacks.
// Callbacks are managed separately (not on the Engine struct).
type TracedEngine struct {
	inner     *Engine
	cfg       *tracedEngineConfig
	tracer    trace.Tracer
	callbacks *CallbackManager
}

// NewTracedEngine creates a new traced engine wrapper.
// When tracing is disabled, Run/RunSync still dispatch callbacks
// (if configured via WithEngineCallbacks) but do not create OTel spans.
func NewTracedEngine(inner *Engine, opts ...TracedEngineOption) *TracedEngine {
	cfg := defaultTracingConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	te := &TracedEngine{
		inner: inner,
		cfg:   cfg,
	}
	if cfg.enabled {
		te.tracer = otel.Tracer(tracedEngineTracerName)
	}
	if cfg.callbacks != nil {
		te.callbacks = cfg.callbacks
	}
	return te
}

// WithEngineCallbacks sets the callback manager for the traced engine.
func WithEngineCallbacks(cb *CallbackManager) TracedEngineOption {
	return func(c *tracedEngineConfig) {
		c.callbacks = cb
	}
}

// SetCallbacks sets the callback manager on an already-created TracedEngine.
func (te *TracedEngine) SetCallbacks(cb *CallbackManager) {
	te.callbacks = cb
}

// Run executes the graph with tracing and callbacks.
func (te *TracedEngine) Run(ctx context.Context, input any, mode types.StreamMode) (<-chan any, <-chan error) {
	if !te.cfg.enabled && te.callbacks == nil {
		return te.inner.Run(ctx, input, mode)
	}

	// Extract thread ID and graph name.
	threadID := extractThreadID(te.inner.config)
	graphName := "state_graph"
	if te.inner.graph != nil {
		nodes := te.inner.graph.GetNodes()
		if len(nodes) > 0 {
			for name := range nodes {
				graphName = "graph:" + name
				break
			}
		}
	}

	// Start root tracing span.
	var graphSpan trace.Span
	if te.tracer != nil {
		nodeCount := 0
		if te.inner.graph != nil {
			nodeCount = len(te.inner.graph.GetNodes())
		}
		attrs := []attribute.KeyValue{
			attribute.Int(AttrGraphNodes, nodeCount),
			attribute.Int(AttrRecursionLimit, te.inner.recursionLimit),
			attribute.String(AttrStreamMode, string(mode)),
		}
		if threadID != "" {
			attrs = append(attrs, attribute.String(AttrThreadID, threadID))
		}
		ctx, graphSpan = te.tracer.Start(ctx, SpanGraphRun,
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(attrs...),
		)
	}

	// Dispatch OnRunStart.
	if te.callbacks != nil {
		te.callbacks.RunStart(ctx, graphName, threadID)
	}

	// Execute the inner engine.
	outputCh, errCh := te.inner.Run(ctx, input, mode)

	// Wrap outputCh with tracing.
	if te.tracer == nil {
		return outputCh, wrapErrChWithCallback(errCh, te, graphName, threadID, graphSpan)
	}

	tracedOutputCh := make(chan any, 100)
	go func() {
		defer close(tracedOutputCh)
		for event := range outputCh {
			te.traceEvent(ctx, event, graphSpan)
			tracedOutputCh <- event
		}
	}()

	return tracedOutputCh, wrapErrChWithCallback(errCh, te, graphName, threadID, graphSpan)
}

// RunSync executes the graph synchronously with tracing.
func (te *TracedEngine) RunSync(ctx context.Context, input any) (any, error) {
	outputCh, errCh := te.Run(ctx, input, types.StreamModeValues)
	// Drain outputCh.
	var finalState any
	for result := range outputCh {
		if se, ok := result.(*StreamEvent); ok && se.Type == EventTypeFinal {
			if data, ok := se.Data.(map[string]any); ok {
				if state, ok := data["state"]; ok {
					finalState = state
				}
			}
		}
	}
	err := <-errCh
	return finalState, err
}

// ---- helpers ----

// extractThreadID gets the thread ID from the engine config.
func extractThreadID(cfg *types.RunnableConfig) string {
	if cfg == nil || cfg.Configurable == nil {
		return ""
	}
	if tid, _ := cfg.Configurable[constants.ConfigKeyThreadID].(string); tid != "" {
		return tid
	}
	return ""
}

// traceEvent decorates a stream event with sub-spans.
func (te *TracedEngine) traceEvent(ctx context.Context, event any, rootSpan trace.Span) {
	if te.tracer == nil || rootSpan == nil {
		return
	}
	se, ok := event.(*StreamEvent)
	if !ok {
		return
	}
	switch se.Type {
	case EventTypeCheckpoint:
		// Checkpoint events under root span.
		_, cpSpan := te.tracer.Start(ctx, SpanCheckpoint,
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(
				attribute.Int(AttrStepNum, se.Step),
				attribute.String(AttrNodeName, se.Node),
			),
		)
		cpSpan.SetStatus(codes.Ok, "")
		cpSpan.End()
	case EventTypeInterrupt:
		_, intSpan := te.tracer.Start(ctx, SpanInterrupt,
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(
				attribute.Int(AttrStepNum, se.Step),
				attribute.String(AttrInterruptNode, se.Node),
			),
		)
		intSpan.SetStatus(codes.Ok, "")
		intSpan.End()
	case EventTypeError:
		if rootSpan != nil {
			rootSpan.SetStatus(codes.Error, fmt.Sprintf("%v", se.Error))
			rootSpan.RecordError(se.Error)
		}
	case EventTypeTaskStart:
		_, taskSpan := te.tracer.Start(ctx, SpanNodeExecute,
			trace.WithSpanKind(trace.SpanKindInternal),
			trace.WithAttributes(
				attribute.Int(AttrStepNum, se.Step),
				attribute.String(AttrNodeName, se.Node),
			),
		)
		taskSpan.SetStatus(codes.Ok, "")
		taskSpan.End()
	}
}

// wrapErrChWithCallback wraps the error channel with callback dispatch.
func wrapErrChWithCallback(errCh <-chan error, te *TracedEngine, graphName, threadID string, graphSpan trace.Span) <-chan error {
	if te.callbacks == nil && (te.tracer == nil || graphSpan == nil) {
		return errCh
	}
	wrapped := make(chan error, 1)
	go func() {
		defer close(wrapped)
		err, ok := <-errCh
		// Dispatch callbacks.
		if te.callbacks != nil {
			te.callbacks.RunEnd(context.Background(), graphName, threadID, err)
		}
		// End root span.
		if graphSpan != nil {
			if err != nil {
				graphSpan.SetStatus(codes.Error, err.Error())
				graphSpan.RecordError(err)
			} else {
				graphSpan.SetStatus(codes.Ok, "")
			}
			graphSpan.End()
		}
		if ok {
			wrapped <- err
		}
	}()
	return wrapped
}
