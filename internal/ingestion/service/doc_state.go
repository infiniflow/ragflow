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
	"time"

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
	ApplyDocCounts(ctx context.Context, docID, kbID string, chunkNum, tokenNum int, duration float64) error
	SaveDocumentTableColumns(ctx context.Context, docID string, names []string) error
	SaveKBTableState(ctx context.Context, kbID string, names []string, fieldMap map[string]interface{}) error
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
	if len(r.Metadata) > 0 || len(r.StripKeys) > 0 {
		if err := mergeDocMetadata(ctx, u.docSvc, r.DocID, r.Metadata, r.StripKeys); err != nil {
			common.Warn(fmt.Sprintf("failed to update document metadata: %v", err))
		}
	}
	if len(r.DiscoveredColumns) > 0 && r.DocID != "" {
		if err := u.docSvc.SaveDocumentTableColumns(ctx, r.DocID, r.DiscoveredColumns); err != nil {
			common.Warn(fmt.Sprintf("failed to save table columns for document %s: %v", r.DocID, err))
		}
	}
	// The discovered schema belongs to the dataset as well: the dataset-level
	// role selector reads it, and an older document picks it up at task time.
	// Mirrors Python's table chunker, which updates the knowledgebase with
	// table_column_names + field_map on every parse (rag/app/table.py:675-678).
	if len(r.DiscoveredColumns) > 0 && r.KbID != "" {
		if err := u.docSvc.SaveKBTableState(ctx, r.KbID, r.DiscoveredColumns, r.FieldMapUpdates); err != nil {
			common.Warn(fmt.Sprintf("failed to sync table schema to KB %s: %v", r.KbID, err))
		}
	}
	// Built-in metadata (update_time / file_name) is applied on top of the
	// LLM-extracted metadata, mirroring Python apply_built_in_metadata
	// (task_executor_refactor/chunk_post_processor.py): it runs when
	// auto-metadata is enabled and built-in fields are configured, and its
	// values overwrite whatever is already stored.
	if r.AutoMetadataEnabled && len(r.BuiltInMetadataConfig) > 0 {
		if err := applyBuiltInMetadata(ctx, u.docSvc, r.DocID, r.DocName, r.BuiltInMetadataConfig); err != nil {
			common.Warn(fmt.Sprintf("failed to apply built-in metadata: %v", err))
		}
	}
	if err := u.docSvc.ApplyDocCounts(ctx, r.DocID, r.KbID, r.ChunkCount, r.TokenConsumption, r.Duration); err != nil {
		common.Warn(fmt.Sprintf("failed to apply doc counts: %v", err))
	}
}

// mergeDocMetadata reads existing metadata, unions it with the freshly
// aggregated doc metadata (list values merged + de-duplicated, scalars from the
// stored map winning — matching Python task_executor.py:572
// update_metadata_to(metadata, existing_meta)), then splits combined values
// before writing the merged map back (Python doc_metadata_service.py:468
// _split_combined_values). A read failure aborts the merge: SetDocumentMetadata
// is a full overwrite, so writing with an empty baseline would destroy existing
// keys. StripKeys (e.g. from table column mode on reparse) are removed from
// existing prior to merge.
func mergeDocMetadata(ctx context.Context, svc docStateSvc, docID string, metadata map[string]any, stripKeys []string) error {
	existing, err := svc.GetDocumentMetadataByID(ctx, docID)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = map[string]any{}
	}
	for _, k := range stripKeys {
		delete(existing, k)
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	merged := utility.UpdateMetadataTo(metadata, existing)
	merged = common.SplitCombinedMetadataValues(merged)
	return svc.SetDocumentMetadata(ctx, docID, merged)
}

// applyBuiltInMetadata writes the configured built-in metadata fields into the
// document's metadata, overwriting existing values. Mirrors Python
// apply_built_in_metadata (task_executor_refactor/chunk_post_processor.py):
//   - update_time -> current timestamp "2006-01-02 15:04:05"
//   - file_name   -> the document name
func applyBuiltInMetadata(ctx context.Context, svc docStateSvc, docID, docName string, config []any) error {
	builtIn := make(map[string]any, 2)
	for _, raw := range config {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key, _ := item["key"].(string)
		switch key {
		case "update_time":
			builtIn["update_time"] = time.Now().Format("2006-01-02 15:04:05")
		case "file_name":
			if docName != "" {
				builtIn["file_name"] = docName
			}
		}
	}
	if len(builtIn) == 0 {
		return nil
	}
	existing, err := svc.GetDocumentMetadataByID(ctx, docID)
	if err != nil {
		return err
	}
	if existing == nil {
		existing = map[string]any{}
	}
	merged := utility.UpdateMetadataTo(existing, builtIn)
	merged = common.SplitCombinedMetadataValues(merged)
	return svc.SetDocumentMetadata(ctx, docID, merged)
}
