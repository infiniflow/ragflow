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

	"ragflow/internal/entity"
)

// NewDebugTaskContext builds an in-memory TaskContext for a canvas-debug
// (dataflow dry-run) request. The returned context is intentionally
// side-effect free:
//
//   - Doc.ID is set to CANVAS_DEBUG_DOC_ID, which disables the executor's
//     persist gate (no MinIO upload, no index insert, no pipeline_log).
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
func NewDebugTaskContext(tenantID, kbID, canvasID, fileName string, fileData []byte) *TaskContext {
	doc := entity.Document{
		ID:   CANVAS_DEBUG_DOC_ID,
		KbID: kbID,
		Name: &fileName,
	}
	if suffix, docType := deriveDocSuffixAndType(fileName); suffix != "" {
		doc.Suffix = suffix
		doc.Type = docType
	}

	return &TaskContext{
		Doc:        doc,
		KB:         entity.Knowledgebase{ID: kbID},
		Tenant:     entity.Tenant{ID: tenantID},
		PipelineID: canvasID,
		File:       fileData,
	}
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
