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

package pipeline

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/agent/canvas"
	_ "ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/globals"

	"go.uber.org/zap"
)

// Pipeline is a compiled ingestion canvas plus task-scoped metadata.
type Pipeline struct {
	taskID string
	// documentID is the owning document; progress is mirrored back to the
	// document table so the existing GET /api/v1/datasets/{dataset_id}/documents
	// endpoint (which reads document.progress/run/progress_msg) reflects the
	// live Go pipeline progress without a bespoke endpoint (plan §8).
	documentID string
	canvas     *canvas.Canvas
	factory    runtime.ComponentFactory // optional instance-scoped component factory
	sink       ProgressSink             // optional progress sink; nil -> drop events (DB-independent)
}

// PipelineOption mutates a Pipeline before Run. Used to inject test doubles
// or instance-scoped collaborators.
type PipelineOption func(*Pipeline)

// WithDocumentID binds the pipeline's owning document so progress can be
// mirrored back into the document table (document.progress / run /
// progress_msg) — the canonical store the document-list endpoint serves.
// Pass the empty string to disable the mirror (e.g. headless/test runs where
// the document row is not materialized).
func WithDocumentID(docID string) PipelineOption {
	return func(p *Pipeline) { p.documentID = docID }
}

// ProgressEvent is a structured component lifecycle event emitted by the
// pipeline to a ProgressSink. The pipeline fills the task/document/component
// identity and phase/status message; the sink caches the denominator (total)
// from OnComponentTotal and needs no canvas knowledge.
type ProgressEvent struct {
	TaskID     string
	DocumentID string
	Component  string
	Message    string
	Phase      int
}

// ProgressSink receives pipeline progress for durable persistence. It is the
// single channel through which the pipeline reports component lifecycle
// events and the component-total denominator; the pipeline itself never
// touches the DAO layer. Implementations live in the orchestration layer
// (internal/ingestion/service). A nil sink is valid: events are dropped and
// the pipeline stays DB-independent (unit tests, headless runs).
type ProgressSink interface {
	OnComponentTotal(ctx context.Context, taskID string, total int)
	OnComponentProgress(ctx context.Context, ev ProgressEvent)
}

// WithProgressSink injects a sink that receives component progress events
// and the component-total denominator. When unset, the pipeline drops
// progress events and stays DB-independent.
func WithProgressSink(s ProgressSink) PipelineOption {
	return func(p *Pipeline) { p.sink = s }
}

// NewPipelineFromDSL compiles the canonical ingestion canvas DSL.
// It accepts either the inner canvas DSL or the template wrapper whose
// top-level `dsl` field carries that canvas.
func NewPipelineFromDSL(dsl []byte, taskID string, opts ...PipelineOption) (*Pipeline, error) {
	// UnwrapCanvasDSL is the single source of truth for stripping the
	// optional {"dsl": {...}} canvas envelope; it also reports a nil/unparseable
	// DSL.
	canvasDSL, err := UnwrapCanvasDSL(dsl)
	if err != nil {
		return nil, err
	}
	cnv, err := canvas.DecodeFromDSL(canvasDSL)
	if err != nil {
		return nil, fmt.Errorf("pipeline: decode canvas DSL: %w", err)
	}
	p := &Pipeline{
		taskID: taskID,
		canvas: cnv,
	}
	for _, o := range opts {
		o(p)
	}
	return p, nil
}

// WithComponentFactory installs an instance-scoped factory override for this
// pipeline. It is used during canvas compilation so one pipeline run can
// construct task-specific component instances without mutating the process-wide
// runtime default factory.
func (p *Pipeline) WithComponentFactory(factory runtime.ComponentFactory) *Pipeline {
	if p != nil {
		p.factory = factory
	}
	return p
}

func mergeInto(dst, src map[string]any) map[string]any {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneMapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Run executes the full ingestion graph described by the canonical DSL.
// Execution always starts from the graph entry and component-level replay
// decisions belong to the components themselves.
func (p *Pipeline) Run(ctx context.Context, inputs map[string]any, overrideParams map[string]any) (map[string]any, error) {
	if p == nil {
		return nil, fmt.Errorf("pipeline: Run on nil pipeline")
	}
	if p.canvas == nil {
		return nil, fmt.Errorf("pipeline: canvas is nil")
	}
	if runtime.DefaultFactory() == nil {
		runtime.InstallDefaultRegistryFactory()
	}
	if runtime.DefaultFactory() == nil {
		return nil, fmt.Errorf("pipeline: Run: runtime default component factory is not installed")
	}

	compileCtx := ctx
	if p.factory != nil {
		compileCtx = canvas.WithComponentFactory(compileCtx, p.factory)
	}

	var compileOpts []canvas.CompileOption
	// Run-level setups (keyed by cpnID) override the DSL-baked component
	// setups at compile time (higher priority; see canvas.WithOverrideParams).
	if overrideParams != nil {
		compileOpts = append(compileOpts, canvas.WithOverrideParams(overrideParams))
	}
	compiled, err := canvas.Compile(compileCtx, p.canvas, compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("pipeline: Run: compile canvas: %w", err)
	}

	// Record the component count as the authoritative denominator for
	// progress percentage. Best-effort: a DB failure (or headless run
	// with no DB) must not abort the pipeline — progress is observability.
	if p.sink != nil {
		p.sink.OnComponentTotal(ctx, p.taskID, len(p.canvas.Components))
	}

	runState := canvas.NewCanvasState("", p.taskID)
	runCtx := canvas.WithState(ctx, runState)
	// Framework-level progress fan-out: the canvas framework
	// (realComponentBody) pulls this callback from ctx via
	// runtime.ProgressCallbackFromContext and records every component
	// start/done/fail event as an ingestion_task_log row. The callback
	// is nil when the DB is not initialized (unit tests, headless
	// runs), in which case TrackProgress is a no-op — progress is an
	// observability concern, not a data dependency.
	runCtx = runtime.WithProgressCallback(runCtx, p.componentProgressCallback(ctx))

	current := cloneMapOrEmpty(inputs)

	// Seed the workflow-wide Globals bag with the run-level metadata
	// (name, tenant_id, kb_id, model_id, doc_id, ...) once, from the
	// pipeline run inputs. Downstream components read these from ctx
	// instead of relying on every node re-emitting them. The File
	// component re-publishes `name` (and storage refs) as it derives
	// them mid-run.
	globals.SeedIngestionGlobals(runCtx, current)

	return p.run(runCtx, current, compiled, runState)
}

// run executes a single Invoke of the compiled workflow, records progress via
// the sink, and returns the merged output.
func (p *Pipeline) run(runCtx context.Context, current map[string]any, compiled *canvas.CompiledCanvas, runState *canvas.CanvasState) (map[string]any, error) {
	out, err := compiled.Workflow.Invoke(runCtx, current)
	if err != nil {
		if errors.Is(runCtx.Err(), context.Canceled) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return current, fmt.Errorf("pipeline: run cancelled: %w", runCtx.Err())
		}
		return current, fmt.Errorf("pipeline: run canvas workflow: %w", err)
	}
	return finalizeResult(current, out, runState), nil
}

// finalizeResult merges the graph output into the input map and attaches the
// canvas state snapshot — the shared success payload.
func finalizeResult(current, out map[string]any, runState *canvas.CanvasState) map[string]any {
	if out == nil {
		current["state"] = runState.Snapshot()
		return current
	}
	merged := mergeInto(current, out)
	merged["state"] = runState.Snapshot()
	return merged
}

// componentProgressCallback returns a runtime.ProgressCallback that forwards
// every component lifecycle event (start/done/fail) to the pipeline's
// ProgressSink. The sink owns all persistence; this callback only shapes the
// event - deriving the message string the frontend expects - so the pipeline
// never touches the DAO layer. Returns nil when no sink is attached, leaving
// TrackProgress a no-op and the pipeline DB-independent (unit tests, headless
// runs).
func (p *Pipeline) componentProgressCallback(ctx context.Context) runtime.ProgressCallback {
	if p.sink == nil {
		return nil
	}
	return func(ev runtime.ProgressEvent) {
		var msg string
		switch ev.Phase {
		case runtime.PhaseEnter:
			msg = ev.Component + " Started"
		case runtime.PhaseExit:
			msg = ev.Component + " Done"
		case runtime.PhaseError:
			if ev.Err != nil {
				msg = ev.Component + ": " + ev.Err.Error()
			} else {
				msg = ev.Component + " Error"
			}
		}
		// Surface every component lifecycle event as a structured log line so
		// a component failure (e.g. an LLM/client error) is captured in
		// ingestor_server.log even if the wrapped error never reaches the
		// higher-level "Task ... failed" branch.
		switch ev.Phase {
		case runtime.PhaseError:
			if ev.Err != nil {
				common.Error("component progress: error", ev.Err,
					zap.String("component", ev.Component),
					zap.String("task_id", p.taskID),
					zap.String("document_id", p.documentID))
			} else {
				common.Info("component progress: error",
					zap.String("component", ev.Component),
					zap.String("task_id", p.taskID),
					zap.String("document_id", p.documentID))
			}
		default:
			// Keep the message constant: msg may carry component names or
			// error-derived text, and a newline in it could forge a log record
			// (CWE-117). Pass msg and the phase as structured fields instead.
			common.Info("component progress",
				zap.String("message", msg),
				zap.Int("phase", int(ev.Phase)),
				zap.String("component", ev.Component),
				zap.String("task_id", p.taskID),
				zap.String("document_id", p.documentID))
		}
		p.sink.OnComponentProgress(ctx, ProgressEvent{
			TaskID:     p.taskID,
			DocumentID: p.documentID,
			Component:  ev.Component,
			Message:    msg,
			Phase:      int(ev.Phase),
		})
	}
}
