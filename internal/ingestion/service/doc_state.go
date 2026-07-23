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

package service

import (
	"fmt"

	"ragflow/internal/common"
	taskpkg "ragflow/internal/ingestion/task"
	documentpkg "ragflow/internal/service/document"
)

// docStateSvc is the subset of *service.DocumentService needed to finalize a
// pipeline run's effect on document state. Extracted as an interface so tests
// can inject a stub without constructing a real DocumentService (which depends
// on initialized server config).
type docStateSvc interface {
	GetDocumentMetadataByID(docID string) (map[string]any, error)
	SetDocumentMetadata(docID string, meta map[string]any) error
	IncrementChunkNum(docID, kbID string, chunkNum, tokenNum int, duration float64) error
}

// docStateUpdater applies a pipeline run's results to document state: it
// merges the pipeline-produced metadata (filling only keys not already present)
// and bumps the document/dataset chunk and token counters. Both steps are
// best-effort; failures are logged and do not fail the task.
type docStateUpdater struct {
	docSvc docStateSvc
}

// newDocStateUpdater creates a docStateUpdater with the real DocumentService
// injected at construction time. Tests inject stubs via the docSvc field.
func newDocStateUpdater() *docStateUpdater {
	return &docStateUpdater{
		docSvc: documentpkg.NewDocumentService(),
	}
}

func (u *docStateUpdater) apply(r *taskpkg.PipelineResult) {
	if r == nil {
		return
	}
	if len(r.Metadata) > 0 {
		if err := mergeDocMetadata(u.docSvc, r.DocID, r.Metadata); err != nil {
			common.Warn(fmt.Sprintf("failed to update document metadata: %v", err))
		}
	}
	if err := u.docSvc.IncrementChunkNum(r.DocID, r.KbID, r.ChunkCount, r.TokenConsumption, r.Duration); err != nil {
		common.Warn(fmt.Sprintf("failed to increment chunk num: %v", err))
	}
}

// mergeDocMetadata reads existing metadata, fills in keys not already present
// (existing keys are preserved, not overwritten), and writes the merged map back.
// A read failure aborts the merge: SetDocumentMetadata is a full overwrite, so
// writing with an empty baseline would destroy existing keys.
func mergeDocMetadata(svc docStateSvc, docID string, metadata map[string]any) error {
	existing, err := svc.GetDocumentMetadataByID(docID)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = map[string]any{}
	}
	for k, v := range metadata {
		if _, exists := existing[k]; !exists {
			existing[k] = v
		}
	}
	return svc.SetDocumentMetadata(docID, existing)
}
