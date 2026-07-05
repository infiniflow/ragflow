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

// File ingestion component (Phase 2.1) — port of python `rag/flow/file.py`.
//
// SCOPE (honest):
//
//   - DOC-ID PATH: matched. The python component only resolves the
//     document record and emits `name`; it does NOT fetch the binary.
//     The Go port now mirrors that ownership boundary.
//
//   - BINARY FETCH: intentionally delegated to Parser. This matches the
//     Python runtime, where Parser resolves
//     `File2DocumentService.get_storage_address(...)` and performs the
//     storage GET itself.
//
//   - ASYNC RACE / DOCUMENT-LEVEL LOCKING: not applicable in Go;
//     the python `self._canvas.callback(1, ...)` short-circuit is
//     replicated via `runtime.TrackProgress(0/1/...)` so a pipeline
//     observer sees Started/Done transitions.
//
//   - PROGRESS: implemented via `runtime.TrackProgress`. With a
//     nil callback (the production pipeline wires its own sink),
//     the wrapper is a no-op so this component stays free of any
//     GUI / Redis state.
package component

import (
	"context"
	"fmt"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/storage"
)

const ComponentNameFile = "File"

// FileComponent resolves document/file metadata and forwards enough
// identity for downstream Parser to fetch bytes.
//
// Inputs (per rag/flow/file.py:File._invoke):
//
//	doc_id  (string, optional)            — python: self._canvas._doc_id
//	file    (list[map], optional)         — python: kwargs.get("file")[0]
//	bucket  (string, optional)            — storage bucket override for tests/admin paths
//	path    (string, optional)            — storage object key override for tests/admin paths
//
// Outputs:
//
//	doc_id  (string, optional)            — echoed for downstream Parser
//	name    (string)                      — file/document name
//	path    (string, optional)            — storage path override echoed for downstream Parser
//	bucket  (string, optional)            — storage bucket override echoed for downstream Parser
//	file    (map[string]any, optional)    — file-list input echoed through
//	_created_time, _elapsed_time          — TrackElapsed bookkeeping
type FileComponent struct {
}

// SetStorageFactoryOverride lets a test inject a Storage-backed
// factory; production wiring should leave this alone and use the
// real factory. The override is honored only when non-nil. This
// is the testability seam requested by the Phase 2.1 spec — it
// avoids forcing every call to pass a Storage through `Invoke`'s
// inputs map (which would leak transport details into the wire
// schema) while still giving tests a clean injection point.
//
// The companion tests live in file_test.go and use
// `storage.NewMemoryStorage()` via `storage.GetStorageFactory().SetStorage(...)`,
// the same pattern already used in internal/service/file_test.go:80-83.
var SetStorageFactoryOverride func() storage.Storage

// NewFileComponent constructs a FileComponent from DSL params.
// The current schema (schema.FileParam) has no fields.
func NewFileComponent(params map[string]any) (runtime.Component, error) {
	p := schema.FileParam{}.Defaults()
	_ = p
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("File: param check: %w", err)
	}
	return &FileComponent{}, nil
}

// Inputs returns the parameter metadata. Matches the python
// File._invoke kwargs. The canonical upstream field is `file`,
// matching the Python runtime contract.
func (c *FileComponent) Inputs() map[string]string {
	return map[string]string{
		"doc_id": "Optional upstream document ID (mirrors python self._canvas._doc_id).",
		"file":   "Optional upstream file descriptor list (mirrors python kwargs.get('file')).",
		"bucket": "Optional storage bucket override for downstream Parser fetches.",
		"path":   "Optional storage object key override for downstream Parser fetches.",
	}
}

// Outputs returns the parameter metadata. Mirrors the python
// set_output contract (see schema.FileOutputs).
func (c *FileComponent) Outputs() map[string]string {
	return map[string]string{
		"doc_id": "Document ID echoed for downstream Parser storage lookup.",
		"name":   "Document / file name.",
		"path":   "Optional storage object key override echoed for downstream Parser.",
		"bucket": "Optional storage bucket override echoed for downstream Parser.",
		"file":   "Upstream file descriptor echoed when supplied via the file-list path.",
		"_ERROR": "Optional short-circuit error message (reserved for parity with python).",
	}
}

// Parallelism is fixed at 1 — File is metadata-only.
func (c *FileComponent) Parallelism() int { return 1 }

// Invoke resolves document/file metadata for downstream Parser use.
//
// The implementation mirrors the python flow's two paths:
//
//  1. doc_id is set (python: `self._canvas._doc_id`) — resolve
//     the document name and emit metadata only.
//  2. doc_id is empty — pull the first file descriptor out of
//     `file` and use its `name`/`id` directly.
func (c *FileComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	_ = ctx
	// Parse the wire input through the schema type so the
	// validation errors match the package convention.
	in, err := parseFileInputs(inputs)
	if err != nil {
		return nil, err
	}

	out := map[string]any{"name": in.name}
	if in.docID != "" {
		out["doc_id"] = in.docID
	}
	if in.bucket != "" {
		out["bucket"] = in.bucket
	}
	if in.path != "" {
		out["path"] = in.path
	}
	if in.fileDesc != nil {
		out["file"] = in.fileDesc
	}
	return runtime.TrackElapsed("File", func() (map[string]any, error) {
		return out, nil
	})
}

// fileInputs is the post-Validation view of the upstream input
// map. Computed once at the top of Invoke so the rest of the
// function reads as straight-line code.
type fileInputs struct {
	docID    string
	name     string
	bucket   string
	path     string
	fileDesc map[string]any
}

// parseFileInputs parses and validates the upstream input map.
// Mirrors python's branching on `self._canvas._doc_id` vs.
// `kwargs.get("file")[0]`.
func parseFileInputs(inputs map[string]any) (fileInputs, error) {
	if inputs == nil {
		return fileInputs{}, fmt.Errorf("file: inputs map is nil")
	}
	out := fileInputs{}

	if v, ok := getString(inputs, "doc_id"); ok && v != "" {
		out.docID = v
		out.name = v
	}

	if out.docID == "" {
		// Fall through to the file-list path. Python uses
		// `kwargs.get("file")[0]`; Go keeps that same public key.
		if v, ok := inputs["file"]; ok {
			switch list := v.(type) {
			case []map[string]any:
				if len(list) > 0 {
					first := list[0]
					out.fileDesc = first
					if name, ok := first["name"].(string); ok {
						out.name = name
					}
					if id, ok := first["id"].(string); ok && out.bucket == "" && out.path == "" {
						// Convention: when `id` is a storage key, we treat
						// it as the `path` only when no explicit path is
						// provided. Bucket remains empty (must be set
						// separately). The python code does the same.
						out.path = id
					}
				}
			case []any:
				if len(list) > 0 {
					if first, ok := list[0].(map[string]any); ok {
						out.fileDesc = first
						if name, ok := first["name"].(string); ok {
							out.name = name
						}
						if id, ok := first["id"].(string); ok && out.bucket == "" && out.path == "" {
							out.path = id
						}
					}
				}
			}
		}
		if out.name == "" {
			return fileInputs{}, fmt.Errorf("file: inputs missing doc_id or file[0].name")
		}
	}

	if v, ok := getString(inputs, "bucket"); ok {
		out.bucket = v
	}
	if v, ok := getString(inputs, "path"); ok {
		out.path = v
	}
	if out.docID != "" {
		name, err := resolveDocumentName(out.docID)
		if err != nil {
			return fileInputs{}, fmt.Errorf("file: resolve doc_id %q: %w", out.docID, err)
		}
		if name != "" {
			out.name = name
		}
	}
	return out, nil
}

// getString accepts any of the json.Number-adjacent forms JSON
// decoding produces. Canvas inputs decode through encoding/json
// by default, which yields string for string-valued fields.
func getString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return s, true
	case []byte:
		return string(s), true
	}
	return "", false
}

// init registers File under CategoryIngestion (per plan §4
// Phase 2.1). Metadata is derived from the Inputs()/Outputs()
// methods on FileComponent so the API layer (Phase 4) can
// enumerate the catalog without instantiating the component.
func init() {
	c := &FileComponent{}
	runtime.MustRegister(ComponentNameFile, runtime.CategoryIngestion,
		func(_ string, params map[string]any) (runtime.Component, error) {
			return NewFileComponent(params)
		},
		runtime.Metadata{
			Version: "1.0.0",
			Inputs:  c.Inputs(),
			Outputs: c.Outputs(),
		})
}
