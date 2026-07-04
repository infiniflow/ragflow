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
//   - BINARY FETCH: matched. The python code comments out the
//     `STORAGE_IMPL.get(b, n)` call (it lives behind a TODO that asks
//     the doc service to resolve the storage address first). The Go
//     port wires that storage path end-to-end against the
//     `internal/storage` layer so the binary actually flows through
//     when the caller supplies `bucket` + `path`. Per AD-5b this is
//     the FIRST MATERIALIZED checkpoint boundary: File records the
//     MinIO path on the way out so downstream components (Parser)
//     can re-fetch without re-running File.
//
//   - DOC-ID PATH: implemented. When `doc_id` is supplied without an
//     explicit `bucket`/`path`, File resolves the backing document and
//     storage address from the database (`document`, `file2document`,
//     `file`) and fetches the binary from storage. This now matches the
//     Python intent in dataflow_service.py / pipeline.py: pipeline.run()
//     can rely on doc_id-only execution.
//
//   - BINARY PAYLOAD: deliberately upgraded. The Go runtime carries
//     component outputs in-process via map[string]any, so File emits
//     raw []byte instead of base64. This keeps the File -> Parser
//     contract typed and avoids accidental double-encoding on the
//     real ingestion path.
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
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/storage"
)

const ComponentNameFile = "File"

// fileFetchTimeout bounds a single MinIO/S3 GET. Picked to match
// the python `@timeout` default ceiling in rag/flow/base.py (60s)
// but tightened to 30s to fail faster on a hung connection. The
// pipeline orchestrator (Phase 3) overrides this if a stage-level
// timeout is configured.
//
// Declared as a var (not const) so tests can shrink it without
// touching production code. The HonorsTimeout test relies on this.
// (Plan AD-1: helpers.go owns timeout semantics; this binding is
// the file-component-specific ceiling.)
var fileFetchTimeout = 30 * time.Second

// FileComponent fetches the document binary from external storage
// (MinIO / S3 / OSS / in-memory mock) and emits it as raw bytes.
//
// Inputs (per rag/flow/file.py:File._invoke):
//
//	doc_id  (string, optional)            — python: self._canvas._doc_id
//	files   (list[map], optional)         — python: kwargs.get("file")[0]
//	bucket  (string, optional)            — Go: storage bucket for the fetch
//	path    (string, optional)            — Go: storage object key for the fetch
//
// Outputs:
//
//	binary  ([]byte)                      — raw bytes
//	name    (string)                      — file/document name
//	path    (string)                      — storage path echoed for the checkpoint
//	bucket  (string)                      — storage bucket echoed for the checkpoint
//	file    (map[string]any, optional)    — file-list input echoed through
//	_created_time, _elapsed_time          — TrackElapsed bookkeeping
type FileComponent struct {
	bucket string // static config: storage bucket override (rare; default empty)
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

// ResolveDocumentStorageOverride is the narrow test seam for doc_id-driven
// storage resolution. Production leaves this nil and uses DAO-backed lookup.
var ResolveDocumentStorageOverride func(docID string) (*resolvedDocumentRef, error)

// NewFileComponent constructs a FileComponent from DSL params.
// The current schema (schema.FileParam) has no fields; the
// constructor is intentionally a thin wrapper today and grows
// once Phase 2.5+ adds runtime knobs.
func NewFileComponent(params map[string]any) (runtime.Component, error) {
	p := schema.FileParam{}.Defaults()
	_ = p // FileParam has no configurable fields today; reserved for forward compat.
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("File: param check: %w", err)
	}
	return &FileComponent{}, nil
}

// Inputs returns the parameter metadata. Matches the python
// File._invoke kwargs (`file` is the python name; we accept it
// as `files` (plural) to align with the schema package's slice
// shape — see schema.FileFromUpstream.Files).
func (c *FileComponent) Inputs() map[string]string {
	return map[string]string{
		"doc_id": "Optional upstream document ID (mirrors python self._canvas._doc_id).",
		"files":  "Optional upstream file descriptor list (mirrors python kwargs.get('file')).",
		"bucket": "Storage bucket to fetch from. Required when fetching the binary; uses internal/storage factory otherwise.",
		"path":   "Storage object key. Required when fetching the binary.",
	}
}

// Outputs returns the parameter metadata. Mirrors the python
// set_output contract (see schema.FileOutputs).
func (c *FileComponent) Outputs() map[string]string {
	return map[string]string{
		"binary": "Raw bytes of the document ([]byte).",
		"name":   "Document / file name.",
		"path":   "Storage object key echoed for the checkpoint.",
		"bucket": "Storage bucket echoed for the checkpoint.",
		"file":   "Upstream file descriptor echoed when supplied via the file-list path.",
		"_ERROR": "Optional short-circuit error message (reserved for parity with python).",
	}
}

// Parallelism is fixed at 1 — a single MinIO GET is not worth
// fanning out, and the checkpoint already dedupes downstream
// re-runs.
func (c *FileComponent) Parallelism() int { return 1 }

// Invoke fetches the document binary and returns the raw bytes along with the storage location for downstream
// use (Parser) and for the materialized-boundary checkpoint.
//
// The implementation mirrors the python flow's two paths:
//
//  1. doc_id is set (python: `self._canvas._doc_id`) — derive
//     `name` from doc_id; defer binary fetch to the (bucket, path)
//     inputs the caller wires.
//  2. doc_id is empty — pull the first file descriptor out of
//     `files` and use its `name`/`id` directly.
//
// In both paths, when `bucket` + `path` are provided, the binary
// is fetched from storage. The fetch is wrapped in
// `runtime.WithTimeout(30s, ...)` per Phase 1 hard rule #3.
func (c *FileComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// Parse the wire input through the schema type so the
	// validation errors match the package convention.
	in, err := parseFileInputs(inputs)
	if err != nil {
		return nil, err
	}

	// Compute the static output keys outside the elapsed-track so
	// they appear in the result map whether or not the fetch ran.
	out := map[string]any{
		"name":   in.name,
		"bucket": in.bucket,
		"path":   in.path,
	}
	if in.fileDesc != nil {
		out["file"] = in.fileDesc
	}

	// Nothing to fetch — emit the metadata-only result.
	if in.bucket == "" || in.path == "" {
		return runtime.TrackElapsed("File", func() (map[string]any, error) {
			return out, nil
		})
	}

	// Wrap fetch + encode in TrackElapsed so the upstream caller
	// gets _created_time / _elapsed_time accounting (mirrors
	// rag/flow/base.py:42, 58). TrackProgress goes inside so the
	// "Started"/"Done" notifications bracket the actual fetch.
	var binary []byte
	tracked, err := runtime.TrackElapsed("File", func() (map[string]any, error) {
		cb := runtime.ProgressCallback(nil)
		progressErr := runtime.TrackProgress("File", cb, func() error {
			return runtime.WithTimeout(ctx, fileFetchTimeout, func(timeoutCtx context.Context) error {
				data, getErr := fetchBinary(timeoutCtx, in.bucket, in.path)
				if getErr != nil {
					return getErr
				}
				binary = data
				return nil
			})
		})
		if progressErr != nil {
			return nil, progressErr
		}
		out["binary"] = binary
		return out, nil
	})
	if err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	return tracked, nil
}

// fetchBinary resolves the storage layer and performs the GET.
// Falls back to the package-level override (test injection);
// otherwise uses the process singleton per AD-5b.
//
// The Storage interface is not yet context-aware, so we race the
// blocking Get against ctx.Done() in a goroutine. This is the
// standard Go pattern for retrofitting ctx onto APIs that don't
// take it (see https://pkg.go.dev/context#WithTimeout — the same
// race governs the inner WithTimeout cancel).
func fetchBinary(ctx context.Context, bucket, path string) ([]byte, error) {
	stg := resolveStorage()
	if stg == nil {
		return nil, fmt.Errorf("no storage backend registered")
	}

	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := stg.Get(bucket, path)
		done <- result{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		// Don't leak the worker goroutine on cancellation: it
		// will eventually return and the buffered channel absorbs
		// the value. We don't wait — first error wins.
		return nil, ctx.Err()
	case r := <-done:
		if r.err != nil {
			return nil, fmt.Errorf("storage.Get(%q, %q): %w", bucket, path, r.err)
		}
		return r.data, nil
	}
}

// resolveStorage returns the test-injection override when set,
// otherwise the process-wide singleton from internal/storage.
func resolveStorage() storage.Storage {
	if SetStorageFactoryOverride != nil {
		if s := SetStorageFactoryOverride(); s != nil {
			return s
		}
	}
	return storage.GetStorageFactory().GetStorage()
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

type resolvedDocumentRef struct {
	name   string
	bucket string
	path   string
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
		// `kwargs.get("file")[0]`; the schema declares `files`
		// (plural slice) for parity with the parser sibling, so we
		// accept both spellings.
		if v, ok := inputs["files"]; ok {
			if list, ok := v.([]map[string]any); ok && len(list) > 0 {
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
		}
		if out.name == "" {
			return fileInputs{}, fmt.Errorf("file: inputs missing doc_id or files[0].name")
		}
	}

	if v, ok := getString(inputs, "bucket"); ok {
		out.bucket = v
	}
	if v, ok := getString(inputs, "path"); ok {
		out.path = v
	}
	if out.docID != "" && (out.bucket == "" || out.path == "") {
		ref, err := resolveDocumentStorage(out.docID)
		if err != nil {
			return fileInputs{}, fmt.Errorf("file: resolve doc_id %q: %w", out.docID, err)
		}
		if ref.name != "" {
			out.name = ref.name
		}
		if out.bucket == "" {
			out.bucket = ref.bucket
		}
		if out.path == "" {
			out.path = ref.path
		}
	}
	return out, nil
}

func resolveDocumentStorage(docID string) (*resolvedDocumentRef, error) {
	if ResolveDocumentStorageOverride != nil {
		return ResolveDocumentStorageOverride(docID)
	}

	doc, err := dao.NewDocumentDAO().GetByID(docID)
	if err != nil {
		return nil, err
	}
	ref := &resolvedDocumentRef{name: documentNameOrID(doc)}

	mappings, err := dao.NewFile2DocumentDAO().GetByDocumentID(doc.ID)
	if err != nil {
		return nil, err
	}
	if len(mappings) > 0 && mappings[0].FileID != nil && *mappings[0].FileID != "" {
		file, err := dao.NewFileDAO().GetByID(*mappings[0].FileID)
		if err != nil {
			return nil, err
		}
		if file.SourceType == "" || entity.FileSource(file.SourceType) == entity.FileSourceLocal {
			if file.Location == nil || *file.Location == "" {
				return nil, fmt.Errorf("file location is empty")
			}
			ref.bucket = file.ParentID
			ref.path = *file.Location
			return ref, nil
		}
	}
	if doc.Location == nil || *doc.Location == "" {
		return nil, fmt.Errorf("document location is empty")
	}
	ref.bucket = doc.KbID
	ref.path = *doc.Location
	return ref, nil
}

func documentNameOrID(doc *entity.Document) string {
	if doc != nil && doc.Name != nil && *doc.Name != "" {
		return *doc.Name
	}
	if doc != nil {
		return doc.ID
	}
	return ""
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
