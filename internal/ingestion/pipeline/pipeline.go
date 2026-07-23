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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/agent/canvas"
	_ "ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	redis2 "ragflow/internal/engine/redis"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/utility"

	"github.com/cloudwego/eino/compose"
)

// Pipeline is a compiled ingestion canvas plus task-scoped metadata.
type Pipeline struct {
	taskID     string
	documentID string // owning document; progress is mirrored back to the
	// document table so the existing GET /api/v1/datasets/{dataset_id}/documents
	// endpoint (which reads document.progress/run/progress_msg) reflects the
	// live Go pipeline progress without a bespoke endpoint (plan §8).
	canvas  *canvas.Canvas
	store   canvas.CheckPointStore // optional injected; nil -> resolve at Run
	tracker *canvas.RunTracker     // optional injected; nil -> resolve at Run
	// requireResume, when true, makes Run refuse to start if no checkpoint
	// store can be resolved (no injected store AND no global Redis client).
	// Plan §6.a M4 方案 A: a deployment that cannot persist checkpoints must
	// not silently degrade to a non-resumable run — it must surface a clear,
	// distinguishable error so the caller knows resume is unavailable.
	requireResume bool
	factory       runtime.ComponentFactory // optional instance-scoped component factory
	sink          ProgressSink             // optional progress sink; nil -> drop events (DB-independent)
}

// ErrResumeUnavailable is returned by Run when WithRequireResume is set but no
// checkpoint store can be resolved (plan §6.a M4). Callers can test for it with
// errors.Is to surface a "resume unavailable" condition instead of a generic
// failure (e.g. refuse to enqueue the task rather than start a non-resumable
// run).
var ErrResumeUnavailable = errors.New("resume unavailable: no checkpoint store (Redis down or not configured)")

// PipelineOption mutates a Pipeline before Run. Used to inject test doubles
// (in-memory store / miniredis tracker) or dedicated Redis pools.
type PipelineOption func(*Pipeline)

// WithCheckPointStore injects a checkpoint store. When unset, Run resolves
// one from the global Redis client (and degrades to a non-resumable run when
// Redis is unavailable — plan §6.a).
func WithCheckPointStore(s canvas.CheckPointStore) PipelineOption {
	return func(p *Pipeline) { p.store = s }
}

// WithRunTracker injects a RunTracker for interrupt-id persistence / crash
// recovery. When unset, Run resolves one from the global Redis client.
func WithRunTracker(t *canvas.RunTracker) PipelineOption {
	return func(p *Pipeline) { p.tracker = t }
}

// WithRequireResume makes Run refuse to start when no checkpoint store can be
// resolved (no injected store AND no global Redis client). This is plan A: a
// deployment that cannot persist checkpoints must not silently
// degrade to a non-resumable run — it must surface a clear, distinguishable
// error (ErrResumeUnavailable) so the caller knows resume is unavailable.
// Production ingestion wiring sets this; unit tests leave it off to exercise
// the non-resumable runPlain fallback.
func WithRequireResume() PipelineOption {
	return func(p *Pipeline) { p.requireResume = true }
}

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
	OnComponentTotal(taskID string, total int)
	OnComponentProgress(ev ProgressEvent)
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
	var raw map[string]any
	if err := json.Unmarshal(dsl, &raw); err != nil {
		return nil, fmt.Errorf("pipeline: decode DSL: %w", err)
	}
	canvasDSL, err := unwrapCanvasDSL(raw)
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

func unwrapCanvasDSL(raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, errNilDSL
	}
	if rawDSL, ok := raw["dsl"]; ok {
		canvasDSL, ok := rawDSL.(map[string]any)
		if !ok || len(canvasDSL) == 0 {
			return nil, errNilDSL
		}
		return canvasDSL, nil
	}
	return raw, nil
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

// defaultCheckpointTTL is the expiry applied to the eino checkpoint payload
// and the RunTracker hash. A finished run's checkpoint is deleted on success;
// the TTL only guards against leaks from crashed runs that never clean up.
var defaultCheckpointTTL = 24 * time.Hour

// Run executes the full ingestion graph described by the canonical DSL.
// There is no pipeline-layer partial resume entry point: execution always
// starts from the graph entry and component-level replay decisions belong to
// the components themselves.
func (p *Pipeline) Run(ctx context.Context, inputs map[string]any, override_params map[string]any) (map[string]any, error) {
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

	// Resolve the checkpoint store + run tracker. Resume (interrupt-after
	// non-terminal node) requires a checkpoint store to persist the resume
	// point; without one we fall back to a single non-resumable Invoke
	// (plan §6.a degrade — progress stays observable, the run just cannot
	// pause/resume across nodes).
	store := p.resolveStore()
	tracker := p.resolveTracker()

	// M4 (plan §6.a 方案 A): refuse to start when resume is required but no
	// checkpoint store is resolvable. A Redis-less deployment must not pretend
	// the task is resumable; it must report the gap clearly so the caller can
	// refuse to enqueue the task instead of silently running a non-resumable
	// run (which would violate "re-run same task, completed components not
	// redone").
	if p.requireResume && store == nil {
		return nil, fmt.Errorf("pipeline: Run: %w", ErrResumeUnavailable)
	}
	resumable := store != nil

	var compileOpts []canvas.CompileOption
	if resumable {
		compileOpts = append(compileOpts,
			canvas.WithCheckPointStore(store),
			canvas.WithCheckPointID(p.taskID),
			canvas.WithInterruptAfterNonTerminalCpn(),
		)
	}
	// Run-level setups (keyed by cpnID) override the DSL-baked component
	// setups at compile time (higher priority; see canvas.WithOverrideParams).
	if override_params != nil {
		compileOpts = append(compileOpts, canvas.WithOverrideParams(override_params))
	}
	compiled, err := canvas.Compile(compileCtx, p.canvas, compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("pipeline: Run: compile canvas: %w", err)
	}

	// Record the component count as the authoritative denominator for
	// progress percentage. Best-effort: a DB failure (or headless run
	// with no DB) must not abort the pipeline — progress is observability.
	if p.sink != nil {
		p.sink.OnComponentTotal(p.taskID, len(p.canvas.Components))
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
	runCtx = runtime.WithProgressCallback(runCtx, p.componentProgressCallback())

	current := cloneMapOrEmpty(inputs)

	// Seed the workflow-wide Globals bag with the run-level metadata
	// (name, tenant_id, kb_id, model_id, doc_id, ...) once, from the
	// pipeline run inputs. Downstream components read these from ctx
	// instead of relying on every node re-emitting them. The File
	// component re-publishes `name` (and storage refs) as it derives
	// them mid-run.
	globals.SeedIngestionGlobals(runCtx, current)

	if !resumable {
		return p.runPlain(runCtx, current, compiled, tracker, runState)
	}

	// Resumable path: record the run, then loop Invoke until the graph
	// completes or a non-resumable error surfaces.
	if tracker != nil {
		if err := tracker.Start(ctx, p.taskID, "", "", ""); err != nil {
			common.Error(fmt.Sprintf("pipeline: RunTracker.Start for task %s failed: %v", p.taskID, err), err)
		}
	}
	return p.runResumable(ctx, runCtx, current, compiled, store, tracker, runState)
}

// resolveStore returns the injected store, or a Redis-backed one when the
// global Redis client is available. Returns nil (degraded, non-resumable)
// when neither is present.
func (p *Pipeline) resolveStore() canvas.CheckPointStore {
	if p.store != nil {
		return p.store
	}
	if redis2.Get() != nil {
		return canvas.NewRedisCheckPointStore(defaultCheckpointTTL)
	}
	return nil
}

// resolveTracker mirrors resolveStore for the RunTracker.
func (p *Pipeline) resolveTracker() *canvas.RunTracker {
	if p.tracker != nil {
		return p.tracker
	}
	if redis2.Get() != nil {
		return canvas.NewRunTracker(defaultCheckpointTTL)
	}
	return nil
}

// runPlain executes a single Invoke with no checkpoint/resume. Used when no
// checkpoint store is available; progress is still recorded via the sink.
func (p *Pipeline) runPlain(runCtx context.Context, current map[string]any, compiled *canvas.CompiledCanvas, tracker *canvas.RunTracker, runState *canvas.CanvasState) (map[string]any, error) {
	out, err := compiled.Workflow.Invoke(runCtx, current)
	if err != nil {
		if errors.Is(runCtx.Err(), context.Canceled) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			if tracker != nil {
				utility.BestEffort(fmt.Sprintf("MarkCancelled for %s", p.taskID), func() error { return tracker.MarkCancelled(runCtx, p.taskID) })
			}
			return current, fmt.Errorf("pipeline: run cancelled: %w", runCtx.Err())
		}
		if tracker != nil {
			utility.BestEffort(fmt.Sprintf("MarkFailed for %s", p.taskID), func() error { return tracker.MarkFailed(runCtx, p.taskID, err.Error()) })
		}
		return current, fmt.Errorf("pipeline: run canvas workflow: %w", err)
	}
	if tracker != nil {
		utility.BestEffort(fmt.Sprintf("MarkSucceeded for %s", p.taskID), func() error { return tracker.MarkSucceeded(runCtx, p.taskID) })
	}
	return finalizeResult(current, out, runState), nil
}

// runResumable drives the graph with eino's interrupt-after-node + resume
// loop (plan §8 step 3). Every non-terminal-node pause is auto-resumed with
// nil data (ingestion resume needs no user input). The loop's TOP reads any
// persisted interrupt id — from the RunTracker (cross-process crash
// recovery) or an in-process fallback — and resumes; the BOTTOM only persists
// the id, never inline-resumes (avoids double-resuming one ctx, plan §4.2
// 建议2).
func (p *Pipeline) runResumable(ctx context.Context, runCtx context.Context, current map[string]any, compiled *canvas.CompiledCanvas, store canvas.CheckPointStore, tracker *canvas.RunTracker, runState *canvas.CanvasState) (map[string]any, error) {
	cpID := compiled.CheckPointID
	var localInterruptID string // in-process resume fallback when tracker is nil
	invokeInput := current

	const maxResumeRounds = 1000
	for round := 0; round < maxResumeRounds; round++ {
		// TOP: recover the pending interrupt (crash recovery or in-process).
		resumeID := ""
		if tracker != nil {
			if id, ok, _ := tracker.GetInterruptID(ctx, cpID); ok && id != "" {
				resumeID = id
			}
		}
		if resumeID == "" {
			resumeID = localInterruptID
		}
		if resumeID != "" {
			runCtx = compose.ResumeWithData(runCtx, resumeID, nil)
			invokeInput = nil // resume restores the graph input from checkpoint
		}

		out, invokeErr := compiled.Workflow.Invoke(runCtx, invokeInput, compose.WithCheckPointID(cpID))
		if invokeErr == nil {
			if tracker != nil {
				utility.BestEffort(fmt.Sprintf("ClearInterruptID for %s", p.taskID), func() error { return tracker.ClearInterruptID(ctx, cpID) })
				utility.BestEffort(fmt.Sprintf("MarkSucceeded for %s", p.taskID), func() error { return tracker.MarkSucceeded(ctx, cpID) })
			}
			if store != nil {
				utility.BestEffort(fmt.Sprintf("delete checkpoint for %s", p.taskID), func() error { return store.Delete(ctx, cpID) })
			}
			return finalizeResult(current, out, runState), nil
		}

		// Cancellation (plan §4.3.b): wipe the checkpoint and mark cancelled.
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			p.cleanupCheckpoint(ctx, store, tracker, cpID)
			if tracker != nil {
				utility.BestEffort(fmt.Sprintf("MarkCancelled for %s", p.taskID), func() error { return tracker.MarkCancelled(ctx, cpID) })
			}
			return current, fmt.Errorf("pipeline: run cancelled: %w", ctx.Err())
		}

		if !canvas.IsInterruptError(invokeErr) {
			if tracker != nil {
				utility.BestEffort(fmt.Sprintf("MarkFailed for %s", p.taskID), func() error { return tracker.MarkFailed(ctx, cpID, invokeErr.Error()) })
			}
			return current, fmt.Errorf("pipeline: run canvas workflow: %w", invokeErr)
		}

		// Paused at a non-terminal node: persist for crash recovery, then
		// resume on the next loop iteration's TOP.
		ctxs := canvas.ExtractInterruptContexts(invokeErr)
		id := canvas.FirstInterruptID(ctxs)
		localInterruptID = id
		if tracker != nil {
			if err := tracker.AttachInterrupt(ctx, cpID, id); err != nil {
				common.Error(fmt.Sprintf("pipeline: AttachInterrupt for task %s failed: %v", p.taskID, err), err)
			}
		}
	}
	return current, fmt.Errorf("pipeline: run exceeded max resume rounds (%d) for task %s", maxResumeRounds, p.taskID)
}

// cleanupCheckpoint wipes the eino checkpoint payload and the persisted
// interrupt id (plan §4.3.b cancelled path).
func (p *Pipeline) cleanupCheckpoint(ctx context.Context, store canvas.CheckPointStore, tracker *canvas.RunTracker, cpID string) {
	if store != nil {
		if err := store.Delete(ctx, cpID); err != nil {
			common.Error(fmt.Sprintf("pipeline: delete checkpoint %s failed: %v", cpID, err), err)
		}
	}
	if tracker != nil {
		_ = tracker.ClearInterruptID(ctx, cpID)
	}
}

// finalizeResult merges the graph output into the input map and attaches the
// canvas state snapshot — the shared success payload for both run paths.
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
func (p *Pipeline) componentProgressCallback() runtime.ProgressCallback {
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
		p.sink.OnComponentProgress(ProgressEvent{
			TaskID:     p.taskID,
			DocumentID: p.documentID,
			Component:  ev.Component,
			Message:    msg,
			Phase:      int(ev.Phase),
		})
	}
}
