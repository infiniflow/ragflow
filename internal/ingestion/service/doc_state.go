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
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	documentpkg "ragflow/internal/service/document"
	taskpkg "ragflow/internal/ingestion/task"
)

// docStateSvc is the subset of *service.DocumentService needed to finalize a
// pipeline run's effect on document state. Extracted as an interface so tests
// can inject a stub without constructing a real DocumentService (which depends
// on initialized server config).
type docStateSvc interface {
	WithDocumentMetadataLock(context.Context, string, func(context.Context) error) error
	RevokeTableProfile(ctx context.Context, docID string) error
	GetDocumentMetadataRaw(ctx context.Context, docID string) (map[string]any, error)
	SetDocumentMetadataRaw(ctx context.Context, docID string, meta map[string]any) error
	DeleteDocumentMetadataRaw(ctx context.Context, docID string, keys []string) error
	ApplyDocCounts(ctx context.Context, docID, kbID string, chunkNum, tokenNum int, duration float64) error
}

// docStateUpdater applies a pipeline run's results to document state: it
// publishes the run's metadata, then bumps the document/dataset chunk and token
// counters. Publishing carries the document's derived table profile, so a run
// that indexes spreadsheet rows fails when that write fails; counters stay
// best-effort.
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

func (u *docStateUpdater) apply(ctx context.Context, r *taskpkg.PipelineResult) error {
	if r == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := publishDocMetadata(ctx, u.docSvc, r); err != nil {
		return err
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
	return nil
}

// publishDocMetadata writes one run's metadata into the document record in a
// single write: the engines replace the whole field map, so splitting the table
// contributions from the rest would leave the record briefly holding half of it.
//
// A spreadsheet run replaces rather than accumulates: the values the previous
// run published are dropped first, using the ownership list in the profile it
// left behind, so re-parsing narrows a column to the values the document now
// holds instead of unioning in rows that no longer exist. Every other key keeps
// the long-standing behaviour where a stored scalar wins over a freshly
// extracted one and lists merge.
//
// A run with no table profile publishes none, which also clears the record an
// earlier run left: those rows are gone, so their columns are not queryable.
func publishDocMetadata(ctx context.Context, svc docStateSvc, r *taskpkg.PipelineResult) error {
	if len(r.Metadata) == 0 && r.TableProfile == nil {
		return svc.RevokeTableProfile(ctx, r.DocID)
	}
	return svc.WithDocumentMetadataLock(ctx, r.DocID, func(ctx context.Context) error {
		return publishDocMetadataLocked(ctx, svc, r)
	})
}

func publishDocMetadataLocked(ctx context.Context, svc docStateSvc, r *taskpkg.PipelineResult) error {
	// The raw record: this map is written back whole, and a reader that filters
	// out system keys would silently drop the profile on every later metadata
	// edit.
	existing, err := svc.GetDocumentMetadataRaw(ctx, r.DocID)
	if err != nil {
		return fmt.Errorf("read metadata of document %s: %w", r.DocID, err)
	}
	if existing == nil {
		existing = map[string]any{}
	}

	previous, _, _ := entity.DecodeTableProfile(existing[entity.TableProfileMetadataField])
	owned := make(map[string]struct{}, len(r.TableProfile.OwnedKeys()))
	for _, key := range r.TableProfile.OwnedKeys() {
		if _, exists := existing[key]; exists && (previous == nil || !ownsKey(previous, key)) {
			continue
		}
		owned[key] = struct{}{}
	}

	// Keys this run no longer produces are deleted, not merely left out of the
	// write: Infinity folds an update into the stored map, so an omitted key
	// keeps its old value and the next read would quote rows that no longer
	// exist.
	if previous != nil {
		stale := make([]string, 0, len(previous.OwnedMetadata)+1)
		for _, key := range previous.OwnedMetadata {
			if _, produced := owned[key]; !produced {
				stale = append(stale, key)
			}
		}
		if r.TableProfile == nil {
			stale = append(stale, entity.TableProfileMetadataField)
		}
		if len(stale) > 0 {
			if err := svc.DeleteDocumentMetadataRaw(ctx, r.DocID, stale); err != nil {
				return fmt.Errorf("revoke retired metadata of document %s: %w", r.DocID, err)
			}
		}
	}

	baseline := make(map[string]any, len(existing))
	for key, value := range existing {
		if key == entity.TableProfileMetadataField || (previous != nil && ownsKey(previous, key)) {
			continue
		}
		baseline[key] = value
	}

	incoming := make(map[string]any, len(r.Metadata))
	for key, value := range r.Metadata {
		incoming[key] = value
	}
	for _, key := range r.TableProfile.OwnedKeys() {
		if _, accepted := owned[key]; !accepted {
			delete(incoming, key)
		}
	}
	merged := utility.UpdateMetadataTo(incoming, baseline)
	// A column this run produced keeps exactly this run's values: merging would
	// fold in whatever the same-named key held before, which for a narrowed
	// re-parse means rows that no longer exist stay queryable.
	for key := range owned {
		if value, ok := r.Metadata[key]; ok {
			merged[key] = value
		}
	}
	merged = common.SplitCombinedMetadataValues(merged)
	// A rejected table contribution must preserve the user value verbatim,
	// including empty values that the ordinary metadata merge drops.
	for _, key := range r.TableProfile.OwnedKeys() {
		if _, accepted := owned[key]; !accepted {
			merged[key] = existing[key]
		}
	}
	if r.TableProfile != nil {
		profile := *r.TableProfile
		profile.OwnedMetadata = make([]string, 0, len(owned))
		for key := range owned {
			profile.OwnedMetadata = append(profile.OwnedMetadata, key)
		}
		encoded, err := profile.Encode()
		if err != nil {
			return fmt.Errorf("encode table profile of document %s: %w", r.DocID, err)
		}
		merged[entity.TableProfileMetadataField] = encoded
	}
	if err := svc.SetDocumentMetadataRaw(ctx, r.DocID, merged); err != nil {
		return fmt.Errorf("publish metadata of document %s: %w", r.DocID, err)
	}
	return nil
}

// ownsKey reports whether a metadata key still belongs to the table system. A
// key a user or the LLM took over is absent from the published list, so their
// value survives the next re-parse instead of being replaced by a column.
func ownsKey(profile *entity.TableProfile, key string) bool {
	for _, owned := range profile.OwnedMetadata {
		if owned == key {
			return true
		}
	}
	return false
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
	return svc.WithDocumentMetadataLock(ctx, docID, func(ctx context.Context) error {
		existing, err := svc.GetDocumentMetadataRaw(ctx, docID)
		if err != nil {
			return err
		}
		if existing == nil {
			existing = map[string]any{}
		}
		merged := utility.UpdateMetadataTo(existing, builtIn)
		merged = common.SplitCombinedMetadataValues(merged)
		profile, ok, err := entity.DecodeTableProfile(merged[entity.TableProfileMetadataField])
		if err != nil {
			return err
		}
		if ok {
			remaining := make([]string, 0, len(profile.OwnedMetadata))
			for _, key := range profile.OwnedMetadata {
				if _, replaced := builtIn[key]; !replaced {
					remaining = append(remaining, key)
				}
			}
			profile.OwnedMetadata = remaining
			raw, err := profile.Encode()
			if err != nil {
				return err
			}
			merged[entity.TableProfileMetadataField] = raw
		}
		return svc.SetDocumentMetadataRaw(ctx, docID, merged)
	})
}
