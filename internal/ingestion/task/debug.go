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

package task

import (
	"strings"

	"github.com/google/uuid"
	"ragflow/internal/entity"
)

// NewDebugTaskContext builds an in-memory TaskContext for a canvas-debug
// (dataflow dry-run) request. The returned context is intentionally
// side-effect free:
//
//   - It carries no knowledgebase: KB.ID and Doc.KbID are empty. A canvas
//     debug run has no KB, so kb_id == "" — this is the single debug signal
//     used throughout the ingestion pipeline (the tokenizer skips embedding,
//     the chunker skips image upload, the executor skips the persist stage).
//     kb_id == "" occurs ONLY in debug mode; production ingestion always
//     supplies a KB, so this branch is never reached in normal operation.
//   - Doc.ID is a fresh uuid (a throwaway marker, not a persisted row); it
//     only needs to be non-empty for the executor's input validation.
//   - The parser page cap (debug preview only parses the first few pages)
//     is injected at run time by the executor via Run's override_params
//     channel, keyed by the Parser component's cpnID and the file's family
//     (see injectDebugPageCap). It deliberately does NOT live in
//     ParserConfig as a flat key, because flat keys are dropped by
//     override_params merging and never reach the parser.
//
// fileData is the raw bytes of the uploaded debug document (may be nil for a
// file-less dry-run). It is stored on TaskContext.File and threaded into the
// pipeline run as the "file" / "binary" input.
//
// The caller-supplied kb_id is deliberately ignored: debug never resolves an
// embedder or writes to a KB, so a debug context has none.
func NewDebugTaskContext(tenantID, canvasID, fileName string, fileData []byte) *TaskContext {
	doc := entity.Document{
		ID:   uuid.New().String(),
		KbID: "",
		Name: &fileName,
	}
	if suffix, docType := deriveDocSuffixAndType(fileName); suffix != "" {
		doc.Suffix = suffix
		doc.Type = docType
	}

	return &TaskContext{
		Doc:        doc,
		KB:         entity.Knowledgebase{ID: ""},
		Tenant:     entity.Tenant{ID: tenantID},
		PipelineID: canvasID,
		File:       fileData,
	}
}

// IsDebug reports whether this context is a canvas-debug (dry-run) context.
// A debug context carries no knowledgebase (KB.ID == "", forced by
// NewDebugTaskContext) — the single debug signal used across the ingestion
// pipeline. Production ingestion always supplies a KB (LoadFromIngestionTask
// follows doc -> kb), so KB.ID == "" occurs ONLY in debug mode. A debug run
// must produce no persistent side effect: the executor skips the persist
// stage, the tokenizer skips embedding, and the chunker skips image upload.
func (t *TaskContext) IsDebug() bool {
	return t.KB.ID == ""
}

// deriveDocSuffixAndType extracts the file extension (e.g. ".pdf") and a
// lower-cased document type from a file name. It is best-effort: when the
// name has no usable extension both return "".
func deriveDocSuffixAndType(fileName string) (suffix, docType string) {
	dot := strings.LastIndex(fileName, ".")
	if dot < 0 || dot == len(fileName)-1 {
		return "", ""
	}
	ext := strings.ToLower(fileName[dot+1:])
	return "." + ext, ext
}
