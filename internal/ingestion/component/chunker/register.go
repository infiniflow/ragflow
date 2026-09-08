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

// Package chunker holds the ingestion chunker components. The variants share
// the same upstream payload (schema.ChunkerFromUpstream) and the same output
// shape (schema.ChunkerOutputs); each registers via MustRegisterChunker from
// its own file.
//
// The package is intentionally separate from internal/agent/component/
// (the agent canvas) and from internal/ingestion/component/schema/
// (the wire types). Wiring it as a separate package keeps the
// registry tidy.
package chunker

import (
	"context"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/globals"

	"gorm.io/gorm"
)

// MustRegisterChunker registers a single chunker component under
// CategoryIngestion. Each chunker file carries exactly one
// init() that calls this with the registered component's name; the
// factory body resolves the typed constructor via newChunkerByName
// (in common.go).
// One helper call per file keeps the registration surface flat.
func MustRegisterChunker(name string) {
	factory := func(_ string, params map[string]any) (runtime.Component, error) {
		comp, err := newChunkerByName(name, params)
		if err != nil {
			return nil, err
		}
		return &imageUploadDecorator{inner: comp}, nil
	}
	runtime.MustRegister(name, runtime.CategoryIngestion, factory, runtime.Metadata{
		Version: "1.0.0",
		Inputs:  ChunkerInputs,
		Outputs: ChunkerOutputs,
	})
}

// imageUploadDecorator wraps a chunker component. Before upload it writes
// ck["id"] (the single source of chunk identity) for every chunk; then it runs
// uploadChunkImages which reads ck["id"] and uploads any raw image bytes.
type imageUploadDecorator struct {
	inner runtime.Component
}

func (d *imageUploadDecorator) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	out, err := d.inner.Invoke(ctx, db, inputs)
	if err != nil {
		return nil, err
	}
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok || len(chunks) == 0 {
		return out, nil
	}

	// Canvas-debug (dry-run) chunk cap: when set (>=1), keep only the leading
	// N chunks for preview. This is the single choke point every chunker
	// variant flows through, so the cap applies regardless of variant, and it
	// also limits downstream compiler/tokenizer work in the debug run — the
	// intended cost saving. Truncation happens FIRST, before any per-chunk
	// work below: dropped chunks get no id computation and no image upload /
	// byte dropping, so a chunk can never be persisted to storage and then
	// discarded by this cap. No chunker node (or cap == 0) → untouched.
	if chunkCap := globals.DebugChunkCap(ctx); chunkCap > 0 && len(chunks) > chunkCap {
		chunks = chunks[:chunkCap]
		out["chunks"] = chunks
	}

	kbID, docID := resolveImageUploadContext(ctx, inputs)

	// Compute and write the deterministic chunk id (component.ChunkID) for
	// every chunk. This happens here — before any upload — so uploadChunkImage
	// can read ck["id"] without deriving it itself. Downstream, the persist
	// stage reuses the same formula as a fallback when ck["id"] is absent.
	for _, ck := range chunks {
		text, _ := ck["text"].(string)
		ck["id"] = common.ChunkID(docID, text)
	}

	// kb_id is empty only in canvas debug (dry-run) mode; production ingestion
	// always supplies a KB, so kb_id == "" never occurs in normal operation.
	// Image bytes are uploaded to MinIO only when a KB is present (i.e. a
	// persist run); in debug we drop the raw bytes so they are not held in
	// memory until a persist stage that will never run.
	if kbID != "" {
		if err := uploadChunkImages(ctx, chunks, ChunkImageUploader, kbID); err != nil {
			return nil, err
		}
	} else {
		for _, ck := range chunks {
			delete(ck, "image")
		}
	}
	return out, nil
}

// ChunkerInputs is the static, registered input descriptor shared
// by all chunker variants.
var ChunkerInputs = map[string]string{
	"text":          "Plain-text input. The chunker slices this into downstream chunks.",
	"content":       "Alias for \"text\".",
	"chunks":        "Optional upstream chunk list (structured JSON form).",
	"name":          "Source document name. Not required on the payload: when absent it is read from the workflow-wide globals bag (CanvasState.Globals) via globals.GlobalOrInput.",
	"_created_time": "Optional upstream timestamp (RFC3339Nano, s).",
	"_elapsed_time": "Optional upstream elapsed time (s).",
}

// ChunkerOutputs is the static, registered output descriptor shared
// by all chunker variants.
//
// Note: this map is the component's emitted output only. Run-level metadata
// that downstream components still need — name, tenant_id, kb_id — is NOT
// re-emitted here; it lives in the workflow-wide CanvasState.Globals bag and
// is read directly from ctx (see runtime.CanvasState.Globals and
// globals.GlobalOrInput). Do not add those keys here.
var ChunkerOutputs = map[string]string{
	"output_format": "Always \"chunks\" on success.",
	"chunks":        "list[object]: per-chunk map (text + optional meta keys).",
	"_ERROR":        "Set only on validation failure.",
}
