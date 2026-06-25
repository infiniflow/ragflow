// Package pregel provides OpenTelemetry tracing for Pregel graph execution.
//
// This adds spans at the Pregel engine level: graph run, each superstep,
// node execution, checkpoint operations, and interrupts.
package pregel

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "ragflow/internal/harness/graph/pregel"

// Tracer holds the OpenTelemetry tracer for the Pregel engine.
// It is lazily initialized from the global TracerProvider.
var tracer trace.Tracer

func init() {
	tracer = otel.Tracer(tracerName)
}

// SpanAttr keys for Pregel engine events.
const (
	AttrStepNum        = "pregel.step"
	AttrGraphName      = "pregel.graph.name"
	AttrGraphNodes     = "pregel.graph.nodes"
	AttrGraphEdges     = "pregel.graph.edges"
	AttrNodeName       = "pregel.node.name"
	AttrNodeTrigger    = "pregel.node.trigger"
	AttrTaskCount      = "pregel.task.count"
	AttrChannelCount   = "pregel.channel.count"
	AttrThreadID       = "pregel.thread_id"
	AttrCheckpointID   = "pregel.checkpoint_id"
	AttrRecursionLimit = "pregel.recursion_limit"
	AttrInterruptNode  = "pregel.interrupt.node"
	AttrDurability     = "pregel.durability"
	AttrStreamMode     = "pregel.stream_mode"
	AttrStateKeys      = "pregel.state.keys"
	AttrInputSize      = "pregel.input.size"
	AttrOutputSize     = "pregel.output.size"
	AttrErrorCode      = "pregel.error.code"
	AttrCacheHit       = "pregel.cache.hit"
	AttrTaskDuration   = "pregel.task.duration_ms"
)

// Span names for tracing.
const (
	SpanGraphRun      = "pregel.Run"
	SpanGraphStep     = "pregel.Superstep"
	SpanNodeExecute   = "pregel.Node.Exec"
	SpanPrepareTasks  = "pregel.PrepareTasks"
	SpanApplyWrites   = "pregel.ApplyWrites"
	SpanCheckpoint    = "pregel.Checkpoint"
	SpanInterrupt     = "pregel.Interrupt"
	SpanResume        = "pregel.Resume"
	SpanBuildOutput   = "pregel.BuildOutput"
	SpanSearchChannel = "pregel.SearchChannel"
)

// TraceOption is a functional option for tracing configuration.
type TraceOption func(*traceConfig)

type traceConfig struct {
	enabled         bool
	attrFilter      func(key, value string) bool // return true to include
	recordArguments bool
	recordResults   bool
}

func defaultTraceConfig() *traceConfig {
	return &traceConfig{
		enabled:         true,
		recordArguments: true,
		recordResults:   true,
		attrFilter:      nil,
	}
}

// WithTraceDisabled disables tracing for this engine.
func WithTraceDisabled() TraceOption {
	return func(c *traceConfig) { c.enabled = false }
}

// WithTraceNoArgs disables recording of argument sizes.
func WithTraceNoArgs() TraceOption {
	return func(c *traceConfig) { c.recordArguments = false }
}

// WithTraceNoResults disables recording of result sizes.
func WithTraceNoResults() TraceOption {
	return func(c *traceConfig) { c.recordResults = false }
}

// WithTraceAttrFilter sets a filter function for attribute recording.
func WithTraceAttrFilter(fn func(key, value string) bool) TraceOption {
	return func(c *traceConfig) { c.attrFilter = fn }
}

// startGraphSpan starts a root span for a full graph run.
// It returns the span and context with the span attached.
func startGraphSpan(ctx context.Context, graphName string, nodeCount, edgeCount, recLimit int, threadID string, durability, streamMode string) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	opts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(AttrGraphName, graphName),
			attribute.Int(AttrGraphNodes, nodeCount),
			attribute.Int(AttrGraphEdges, edgeCount),
			attribute.Int(AttrRecursionLimit, recLimit),
			attribute.String(AttrDurability, durability),
			attribute.String(AttrStreamMode, streamMode),
		),
	}
	if threadID != "" {
		opts = append(opts, trace.WithAttributes(attribute.String(AttrThreadID, threadID)))
	}
	ctx, span := tracer.Start(ctx, SpanGraphRun, opts...)
	return ctx, span
}

// endGraphSpan ends the root graph span with status.
func endGraphSpan(span trace.Span, err error) {
	if span == nil || !span.IsRecording() {
		return
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// startStepSpan starts a span for a single Pregel superstep.
func startStepSpan(ctx context.Context, step int, taskCount int) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	ctx, span := tracer.Start(ctx, SpanGraphStep,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.Int(AttrStepNum, step),
			attribute.Int(AttrTaskCount, taskCount),
		),
	)
	return ctx, span
}

// endStepSpan ends the step span.
func endStepSpan(span trace.Span, err error) {
	if span == nil || !span.IsRecording() {
		return
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// startNodeSpan starts a span for a single node execution.
func startNodeSpan(ctx context.Context, nodeName string, triggerCount int, inputSize int) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	ctx, span := tracer.Start(ctx, SpanNodeExecute,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(AttrNodeName, nodeName),
			attribute.Int("pregel.node.trigger_count", triggerCount),
			attribute.Int(AttrInputSize, inputSize),
		),
	)
	return ctx, span
}

// endNodeSpan ends the node span with output stats.
func endNodeSpan(span trace.Span, outputSize int, err error) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(attribute.Int(AttrOutputSize, outputSize))
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// startCheckpointSpan starts a span for a checkpoint save/load.
func startCheckpointSpan(ctx context.Context, operation string, threadID, checkpointID string, stateSize int) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	ctx, span := tracer.Start(ctx, SpanCheckpoint,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("pregel.checkpoint.operation", operation),
			attribute.Int(AttrStateKeys, stateSize),
		),
	)
	if threadID != "" {
		span.SetAttributes(attribute.String(AttrThreadID, threadID))
	}
	if checkpointID != "" {
		span.SetAttributes(attribute.String(AttrCheckpointID, checkpointID))
	}
	return ctx, span
}

// endCheckpointSpan ends the checkpoint span.
func endCheckpointSpan(span trace.Span, err error) {
	endSpan(span, err)
}

// endSpan ends any span with status.
func endSpan(span trace.Span, err error) {
	if span == nil || !span.IsRecording() {
		return
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// startInterruptSpan starts a span for an interrupt.
func startInterruptSpan(ctx context.Context, nodeNames []string) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	var names []attribute.KeyValue
	for _, n := range nodeNames {
		names = append(names, attribute.String(AttrInterruptNode, n))
	}
	ctx, span := tracer.Start(ctx, SpanInterrupt,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(names...),
	)
	return ctx, span
}

// startPrepareTasksSpan starts a span for prepareNextTasks.
func startPrepareTasksSpan(ctx context.Context, completedCount int) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	ctx, span := tracer.Start(ctx, SpanPrepareTasks,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.Int("pregel.completed_tasks", completedCount)),
	)
	return ctx, span
}

// endPrepareTasksSpan ends the prepare-tasks span with task count.
func endPrepareTasksSpan(span trace.Span, taskCount int) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(attribute.Int(AttrTaskCount, taskCount))
	span.SetStatus(codes.Ok, "")
	span.End()
}

// startApplyWritesSpan starts a span for applyWrites.
func startApplyWritesSpan(ctx context.Context, resultCount int) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	ctx, span := tracer.Start(ctx, SpanApplyWrites,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.Int("pregel.results", resultCount)),
	)
	return ctx, span
}

// endApplyWritesSpan ends the apply-writes span.
func endApplyWritesSpan(span trace.Span, err error) {
	endSpan(span, err)
}
