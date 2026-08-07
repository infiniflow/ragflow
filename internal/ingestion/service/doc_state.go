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
	"context"
	"fmt"

	"ragflow/internal/common"
	taskpkg "ragflow/internal/ingestion/task"
	documentpkg "ragflow/internal/service/document"
	"ragflow/internal/utility"
)

// docStateSvc is the subset of *service.DocumentService needed to finalize a
// pipeline run's effect on document state. Extracted as an interface so tests
// can inject a stub without constructing a real DocumentService (which depends
// on initialized server config).
type docStateSvc interface {
	GetDocumentMetadataByID(ctx context.Context, docID string) (map[string]any, error)
	SetDocumentMetadata(ctx context.Context, docID string, meta map[string]any) error
	IncrementChunkNum(ctx context.Context, docID, kbID string, chunkNum, tokenNum int, duration float64) error
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

func (u *docStateUpdater) apply(ctx context.Context, r *taskpkg.PipelineResult) {
	if r == nil {
		return
	}
	if len(r.Metadata) > 0 {
		if err := mergeDocMetadata(ctx, u.docSvc, r.DocID, r.Metadata); err != nil {
			common.Warn(fmt.Sprintf("failed to update document metadata: %v", err))
		}
	}
	if err := u.docSvc.IncrementChunkNum(ctx, r.DocID, r.KbID, r.ChunkCount, r.TokenConsumption, r.Duration); err != nil {
		common.Warn(fmt.Sprintf("failed to increment chunk num: %v", err))
	}
}

// mergeDocMetadata reads existing metadata, unions it with the freshly
// aggregated doc metadata (list values merged + de-duplicated, scalars from the
// stored map winning — matching Python task_executor.py:572
// update_metadata_to(metadata, existing_meta)), then splits combined values
// before writing the merged map back (Python doc_metadata_service.py:468
// _split_combined_values). A read failure aborts the merge: SetDocumentMetadata
// is a full overwrite, so writing with an empty baseline would destroy existing
// keys.
func mergeDocMetadata(ctx context.Context, svc docStateSvc, docID string, metadata map[string]any) error {
	existing, err := svc.GetDocumentMetadataByID(ctx, docID)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = map[string]any{}
	}
	merged := utility.UpdateMetadataTo(metadata, existing)
	merged = common.SplitCombinedMetadataValues(merged)
	return svc.SetDocumentMetadata(ctx, docID, merged)
}
