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

// Package schema holds the wire-level *FromUpstream / *Param / *Outputs types
// that flow between ingestion pipeline components. Each file mirrors the
// Pydantic schema in rag/flow/<component>/schema.py (or, when none exists,
// the runtime contract implied by the component's _invoke signature).
//
// Files in this package are pure data definitions — they import only stdlib.
// Business logic (the component Implementations) lives elsewhere in
// internal/ingestion/component/.
package schema

// FileFromUpstream is the upstream payload consumed by the File component.
//
// Mirrors the runtime contract of rag/flow/file.py:File._invoke. There is no
// dedicated Pydantic schema for File in the Python codebase, so the fields
// below were derived from the keyword arguments File reads at runtime:
//
//	async def _invoke(self, **kwargs):
//	    if self._canvas._doc_id: ...            # doc_id path
//	    else:
//	        file = kwargs.get("file")[0]        # file-list path
//
// `_doc_id` is taken from the surrounding canvas, not kwargs, so it lives on
// a separate optional field here for explicitness.
type FileFromUpstream struct {
	// CreatedTime is the upstream component's wall-clock start time (s).
	CreatedTime *float64 `json:"_created_time,omitempty"`
	// ElapsedTime is the upstream component's elapsed time (s).
	ElapsedTime *float64 `json:"_elapsed_time,omitempty"`

	// DocID is the canvas-bound document ID. When non-empty the File
	// component resolves the binary via the document service; otherwise
	// the File payload below is used. Optional in wire terms — the
	// Python code branches on truthiness, so we use *string.
	DocID *string `json:"doc_id,omitempty"`

	// File is the optional list of file descriptors passed when no
	// doc_id is bound. In Python: `file = kwargs.get("file")[0]`.
	// Shape: `[]map[string]any` — same as rag/flow/parser's `file` field.
	File []map[string]any `json:"file,omitempty"`
}

// Validate enforces the File component's wire-shape requirement: at
// least one of DocID or File must be set. The Python code branches on
// `_canvas._doc_id` vs. `kwargs.get("file")[0]`, which corresponds to
// "one of the two upstream paths is wired".
func (f *FileFromUpstream) Validate() error {
	if (f.DocID == nil || *f.DocID == "") && len(f.File) == 0 {
		return errRequiredField{Field: "doc_id|file"}
	}
	return nil
}

// FileParam is the static configuration for the File component.
//
// Mirrors rag/flow/file.py:FileParam. The Python class has no fields
// beyond what ProcessParamBase provides (none stored on the instance), so
// the Go struct is intentionally empty. It is kept as a distinct type so
// Phase 2.1 can attach config knobs (e.g., a `with_blob` flag) without a
// rename.
type FileParam struct{}

// Defaults returns a FileParam populated with the Python default values.
// Today there are no fields; the constructor exists for forward
// compatibility and to satisfy the "every type has a Defaults()"
// convention adopted in this package.
func (FileParam) Defaults() FileParam { return FileParam{} }

// Validate returns nil. FileParam has no required fields.
func (FileParam) Validate() error { return nil }

// FileOutputs is the result of invoking the File component. It mirrors
// the values File sets via `self.set_output(...)` in rag/flow/file.py:
//
//	set_output("name", doc.name)             # name
//	set_output("file", file)                  # file  (full descriptor)
//	set_output("_ERROR", "...")               # error
//
// The binary blob (`blob`) is intentionally NOT part of the wire schema —
// it lives on the storage layer (MinIO/S3) and is referenced by path.
type FileOutputs struct {
	// Name is the resolved document/file name.
	Name string `json:"name"`
	// File is the upstream file descriptor (dict in Python). Optional —
	// when invoked via doc_id the Python code does not re-emit it.
	File map[string]any `json:"file,omitempty"`
	// Error is set when the component short-circuits with an error
	// message (Python: set_output("_ERROR", ...)).
	Error string `json:"_ERROR,omitempty"`
}
