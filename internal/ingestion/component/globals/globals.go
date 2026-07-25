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
// The embedding-model id is intentionally NOT a global: it is a
// Tokenizer-scoped setup (params["setups"]["embedding_model"]). Keeping it out
// of the shared bag prevents another component (e.g. one expecting a chat
// model) from misreading a generic "model_id" global as its own.
var GlobalMetadataKeys = []string{
	"name",
	"doc_id",
	"bucket",
	"path",
	"file",
	"tenant_id",
	"kb_id",
	"lang",
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
