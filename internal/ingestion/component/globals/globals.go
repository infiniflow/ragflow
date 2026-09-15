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

// Package globals owns the ingestion-specific run-level metadata contract.
//
// The generic cross-component scratch space is CanvasState.Globals (in the
// agent runtime). Which keys an ingestion pipeline elects to store there, and
// how they are seeded / read, is ingestion-specific — so it lives here rather
// than in the generic canvas runtime, and in a leaf package that neither
// imports the component package nor the pipeline package (so it cannot
// participate in an import cycle).
package globals

import (
	"context"

	"ragflow/internal/agent/runtime"
)

// canvasStateFromContext resolves the per-run CanvasState attached by the
// pipeline (canvas.WithState). Returns nil when no state is present (e.g.
// headless unit tests that don't attach a CanvasState).
func canvasStateFromContext(ctx context.Context) *runtime.CanvasState {
	st, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		return nil
	}
	return st
}

// GlobalMetadataKeys enumerates the run-level metadata fields that every
// ingestion component may rely on and that the workflow carries for the whole
// run, instead of threading through each component's output map.
//
// The Go ingestion runtime wires components through eino: a component's output
// map is the sole input to the next node, and — unlike the Python
// ProcessBase.invoke (rag/flow/base.py:42-44) — it does NOT auto-merge every
// input kwarg into the output. A narrowing component (File, Parser, Chunker,
// Tokenizer, Extractor, ...) would otherwise drop fields the next node still
// depends on (e.g. TokenChunker drops `name`, which Tokenizer consumes for
// title embedding). Storing the shared fields in CanvasState.Globals restores
// the Python behaviour without mutating every component output.
// The embedding-model id is intentionally NOT a global: the Tokenizer resolves
// it from the dataset's own embd_id (kb_id) via its injected resolver, never
// from a run input. Keeping it out of the shared bag prevents another component
// (e.g. one expecting a chat model) from misreading a generic "model_id" global
// as its own.
var GlobalMetadataKeys = []string{
	"name",
	"doc_id",
	"bucket",
	"path",
	"file",
	"tenant_id",
	"kb_id",
	"lang",
	// debug_chunk_cap is debug-only (canvas dry-run). Production pipeline
	// inputs never set it, and no component outputs it, so SeedIngestionGlobals
	// / PublishGlobals only ever propagate it in a debug run — where the
	// chunker decorator reads it to limit preview chunks.
	DebugChunkCapKey,
}

// taskIDKey is the CanvasState.Globals slot carrying the ingestion task id of
// the current run. It is deliberately unexported and absent from
// GlobalMetadataKeys: the pipeline seeds it once via SetTaskID, so no run
// input and no component output can overwrite the run's task scope.
const taskIDKey = "task_id"

// SetTaskID records the ingestion task id of the current run so components can
// scope per-task bookkeeping (e.g. the per-chunk cache manifest). Called once
// by the pipeline at run start. No-op when no CanvasState is attached.
func SetTaskID(ctx context.Context, taskID string) {
	if taskID == "" {
		return
	}
	if st := canvasStateFromContext(ctx); st != nil {
		st.SetGlobal(taskIDKey, taskID)
	}
}

// TaskID returns the ingestion task id of the current run, or "" when the run
// has no task scope (headless component tests, or a canvas invoked outside the
// ingestion pipeline). Callers must treat "" as "skip per-task bookkeeping".
func TaskID(ctx context.Context) string {
	if st := canvasStateFromContext(ctx); st != nil {
		if v, ok := st.GetGlobal(taskIDKey); ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// DebugChunkCapKey is the run-input key (seeded into CanvasState.Globals)
// carrying the canvas-debug (dry-run) chunk cap: when >= 1, a chunker node in
// a debug run emits at most this many leading chunks for preview. 0 means "no
// cap" (no chunker, or cap disabled). The chunker decorator reads it via
// DebugChunkCap; the executor seeds the default into run inputs only in the
// debug branch of runPipelineWithDSL.
const DebugChunkCapKey = "debug_chunk_cap"

// DebugChunkCap reads the canvas-debug chunk cap from CanvasState.Globals.
// Returns 0 when no cap is set (no debug run, no chunker node, or cap
// disabled). The stored value is normally an int (Go-side default); a
// JSON-decoded override may arrive as float64, both are accepted.
func DebugChunkCap(ctx context.Context) int {
	if st := canvasStateFromContext(ctx); st != nil {
		if v, ok := st.GetGlobal(DebugChunkCapKey); ok {
			switch n := v.(type) {
			case int:
				return n
			case int64:
				return int(n)
			case float64:
				return int(n)
			}
		}
	}
	return 0
}

// SeedIngestionGlobals copies the whitelisted run-level metadata from `in`
// into the CanvasState.Globals bag (last writer wins per key). It is the
// single entry point for populating the shared workflow metadata: call it
// once at run start (from the pipeline run inputs) and again from components
// that derive a field mid-run (e.g. the File component publishing `name`).
func SeedIngestionGlobals(ctx context.Context, in map[string]any) {
	if in == nil {
		return
	}
	if st := canvasStateFromContext(ctx); st != nil {
		for _, k := range GlobalMetadataKeys {
			if v, ok := in[k]; ok {
				st.SetGlobal(k, v)
			}
		}
	}
}

// PublishGlobals copies the resolved run-level metadata from a component's
// output into the workflow-wide CanvasState.Globals bag so downstream
// components can read it from ctx. No-op when state is absent.
func PublishGlobals(ctx context.Context, out map[string]any) {
	if out == nil {
		return
	}
	SeedIngestionGlobals(ctx, out)
}

// GetGlobal returns a run-level field from CanvasState.Globals if present.
func GetGlobal(ctx context.Context, key string) (any, bool) {
	if st := canvasStateFromContext(ctx); st != nil {
		return st.GetGlobal(key)
	}
	return nil, false
}

// GlobalOrInput resolves a run-level field from CanvasState.Globals first,
// then from the component's own input map, then def. Globals is the canonical
// home for shared run metadata; the input fallback keeps headless tests
// (which attach no CanvasState) working.
func GlobalOrInput(ctx context.Context, inputs map[string]any, key, def string) string {
	if st := canvasStateFromContext(ctx); st != nil {
		if v, ok := st.GetGlobal(key); ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	if v, ok := inputs[key].(string); ok && v != "" {
		return v
	}
	return def
}
