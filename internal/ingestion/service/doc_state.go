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

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/service/chunkversion"
	"ragflow/internal/service/datasetversion"
	documentpkg "ragflow/internal/service/document"
)

// docStateSvc is the subset of *service.DocumentService needed to finalize a
// pipeline run's effect on document state. Extracted as an interface so tests
// can inject a stub without constructing a real DocumentService (which depends
// on initialized server config).
type docStateSvc interface {
	GetDocumentMetadataByID(ctx context.Context, docID string) (map[string]any, error)
	SetDocumentMetadata(ctx context.Context, docID string, meta map[string]any) error
	ApplyDocCounts(ctx context.Context, docID, kbID string, chunkNum, tokenNum int, duration float64) error
}

// docStateUpdater applies a pipeline run's results to document state: it
// replaces metadata keys produced by the pipeline while preserving other keys,
// and bumps the document/dataset chunk and token counters. Both steps are
// best-effort; failures are logged and do not fail the task.
type docStateUpdater struct {
	docSvc docStateSvc
	// db is the relational handle from the injected ingestion.Deps bundle. The
	// history writers need it, and the orchestration layer must not resolve the
	// process-wide handle itself (arch_test.go guards that rule).
	db *gorm.DB
	// engine drops the rows a re-parse did not produce, so the chunk list stops
	// showing them. Only their rows go; the history keeps their content and a
	// document restore rebuilds them from it.
	engine chunkRowDeleter
}

// chunkRowDeleter is the slice of the document engine the finalizer needs: it
// removes the rows a re-parse dropped. Narrowing it keeps the tests free of the
// whole engine interface.
type chunkRowDeleter interface {
	DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName string, datasetID string) (int64, error)
}

// newDocStateUpdater creates a docStateUpdater with the real DocumentService
// injected at construction time. Tests inject stubs via the docSvc field.
func newDocStateUpdater(db *gorm.DB, docEngine chunkRowDeleter) *docStateUpdater {
	return &docStateUpdater{
		docSvc: documentpkg.NewDocumentService(),
		db:     db,
		engine: docEngine,
	}
}

func (u *docStateUpdater) apply(ctx context.Context, r *taskpkg.PipelineResult) error {
	if r == nil {
		return nil
	}
	// The metadata values this run writes are recorded under the parse version,
	// which the parser reserved for its chunk baselines and this finalizer closes
	// at the same number (see recordParseVersion): the sink holds them until the
	// number is known, so the run leaves one version instead of one per value it
	// touched.
	ctx = datasetversion.WithStateSink(ctx)
	if len(r.Metadata) > 0 || len(r.ManagedMetadataKeys) > 0 {
		if err := mergeDocMetadata(ctx, u.docSvc, r.DocID, r.Metadata, r.ManagedMetadataKeys); err != nil {
			common.Warn(fmt.Sprintf("failed to update document metadata: %v", err))
		}
	}
	// Built-in metadata (update_time / file_name) is applied on top of the
	// LLM-extracted metadata, mirroring applyBuiltInMetadata: it runs when
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
	// The diff uses every row the run wrote, not just the source chunks: the
	// parent rows and compiled products carry the same document and are
	// written into the engine by the same run, so treating them as "not kept"
	// would tombstone them the moment the parse lands.
	dropped, err := recordParseVersion(ctx, u.db, r.DocID, r.KbID, r.DocName, r.WrittenChunkIDs, r.DatasetVersion)
	if err != nil {
		return err
	}
	u.removeDroppedChunks(ctx, r, dropped)
	return nil
}

// removeDroppedChunks deletes the engine rows of the chunks a re-parse did not
// produce, so the chunk list and retrieval stop showing them. The history keeps
// their content: a document version recorded before the parse still describes
// them, and replaying it rebuilds them from the snapshots.
//
// A failure is logged rather than returned: the run already succeeded and its
// history is written; a stale row that still shows up is better than failing a
// parse the user watched complete.
func (u *docStateUpdater) removeDroppedChunks(ctx context.Context, r *taskpkg.PipelineResult, dropped []string) {
	if u.engine == nil || r == nil || r.TenantID == "" || len(dropped) == 0 {
		return
	}
	indexName := fmt.Sprintf("ragflow_%s", r.TenantID)
	if _, err := u.engine.DeleteChunks(ctx, map[string]interface{}{
		"id":     dropped,
		"kb_id":  r.KbID,
		"doc_id": r.DocID,
	}, indexName, r.KbID); err != nil {
		common.Warn("Ingestion: failed to delete the chunks a re-parse dropped",
			zap.String("document_id", r.DocID),
			zap.String("dataset_id", r.KbID),
			zap.Error(err))
	}
}

// recordParseVersion closes a parse run in the dataset history.
//
// Two things have to happen here rather than inside the executor:
//
//   - the version's counters come from the dataset row, and the row only reaches
//     its post-parse counts after ApplyDocCounts above;
//   - a re-parse replaces the document's chunk set, so the chunks the new run did
//     not produce have to be tombstoned before the version that describes the
//     result. Chunk ids are derived from the text, so anything the run left out is
//     a chunk that disappeared; without the tombstone a replay of this version
//     would rebuild it from its last content row. keptChunkIDs is every row the
//     run wrote — source chunks, parent rows and compiled products — because all
//     of them exist in the document's history as open chunks.
//
// The version also has to follow the chunk baselines the parser wrote: a restore
// replays the chunk history up to the version's own anchor, and an anchor taken
// before the parse would rebuild the document empty.
//
// pinnedVersion is the number the parser reserved for its baselines
// (PipelineResult.DatasetVersion). When it is positive the version row is
// recorded on that same number instead of taking a new one: the baselines are
// already stamped with it, so a fresh number would leave the reserved one in the
// history as a gap nothing explains, and one parse would advance the dataset
// twice. The tombstones below share that number for the same reason — a replay
// includes rows at or before the anchor, so a removal that belongs to this parse
// is visible exactly at the version this parse records.
//
// The version records the run, not the diff: a parse that succeeds leaves one
// version even when it changed nothing — an empty document re-parsed, or a file
// whose chunks all came out identical. Skipping the version there made a run the
// user saw succeed leave no trace in the history, which is what the history is
// read for (what happened to this dataset, and when). The equivalent versions
// this produces cost a row each and restore to the same content, so the noise is
// bounded by how often someone re-parses a document that produces nothing.
//
// A failure is returned to the run that produced it: the chunks are indexed, so
// the run reports the failure and is retried, but the history never silently
// loses the version that would have closed it.
func recordParseVersion(ctx context.Context, db *gorm.DB, docID, datasetID, docName string, keptChunkIDs []string, pinnedVersion int64) ([]string, error) {
	if docID == "" || datasetID == "" || db == nil {
		return nil, nil
	}
	writeCtx := context.WithoutCancel(ctx)

	openIDs, err := chunkversion.OpenChunkIDs(writeCtx, db, docID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the chunk history of document %s: %w", docID, err)
	}
	dropped := missingIDs(openIDs, keptChunkIDs)
	// anchor is the one number this run occupies. A run that wrote chunks pinned
	// it before writing them; any other run takes one here, and the version row
	// below carries it too, so every run advances the dataset by exactly one.
	anchor := pinnedVersion
	if anchor <= 0 {
		reserved, reserveErr := datasetversion.ReserveVersion(writeCtx, db, datasetID)
		if reserveErr != nil {
			return nil, fmt.Errorf("failed to reserve a dataset version for document %s: %w", docID, reserveErr)
		}
		anchor = reserved
	}
	if anchor > 0 && len(dropped) > 0 {
		// The tombstones belong to this run, so they carry its number: a replay
		// includes rows at or before the anchor, so a removal written at the
		// version this run records is visible exactly there. Inside one chunk the
		// chunk version orders the removal after the baseline it cancels, so the
		// two can share the dataset number.
		// No actor: a re-parse dropped these chunks, so the history credits the
		// parse rather than a person.
		chunkversion.AppendTombstones(writeCtx, db, datasetID, docID, anchor, dropped, "")
	}
	// A parse that produced no chunks still needs a history row, or a restore
	// cannot tell it from a lost history. Best effort, like the per-batch baselines.
	if anchor > 0 && len(keptChunkIDs) == 0 {
		if err := chunkversion.AppendEmptyParseBaseline(writeCtx, db, datasetID, anchor, docID); err != nil {
			common.Warn("Ingestion: failed to record that the parse produced no chunks",
				zap.String("dataset_id", datasetID),
				zap.String("document_id", docID),
				zap.Error(err))
		}
	}
	// Nothing is skipped here: the version is owed even when keptChunkIDs and
	// dropped are both empty. A reserved number with no row would be a hole in
	// the history, the states the run parked in the sink need a version to be
	// addressed by, and a run that changed nothing is still a run the history is
	// asked about. A deployment that keeps no history answers zero to the
	// reservation, and the pinned write below is a no-op there as well.
	changes := []entity.DatasetVersionChange{{
		Entity: entity.DatasetVersionChangeEntityDocument,
		Action: entity.DatasetVersionChangeActionParse,
		DocID:  docID,
		Name:   docName,
	}}
	var version int64
	if anchor > 0 {
		version, err = datasetversion.RecordContentVersionChangesAtVersionOwnTx(
			writeCtx,
			datasetID,
			entity.DatasetVersionKindParse,
			anchor,
			changes,
		)
	} else {
		version, err = datasetversion.RecordContentVersionChangesOwnTx(
			writeCtx,
			datasetID,
			entity.DatasetVersionKindParse,
			changes,
		)
	}
	if err != nil {
		return dropped, err
	}
	return dropped, datasetversion.FlushStates(writeCtx, db, datasetID, version)
}

// missingIDs returns the ids of `known` that `kept` does not contain.
func missingIDs(known, kept []string) []string {
	if len(known) == 0 {
		return nil
	}
	keep := make(map[string]struct{}, len(kept))
	for _, id := range kept {
		keep[id] = struct{}{}
	}
	missing := make([]string, 0, len(known))
	for _, id := range known {
		if _, ok := keep[id]; ok {
			continue
		}
		missing = append(missing, id)
	}
	return missing
}

// mergeDocMetadata replaces stored keys with the freshly aggregated values,
// removes any key in managedKeys absent from metadata (so re-parsing clears
// stale extractor-owned values), and retains all other stored keys (such as
// user-maintained fields).
// It splits combined string-list values before writing the complete map back.
// A read failure aborts the merge to avoid losing keys during replacement.
func mergeDocMetadata(ctx context.Context, svc docStateSvc, docID string, metadata map[string]any, managedKeys []string) error {
	existing, err := svc.GetDocumentMetadataByID(ctx, docID)
	if err != nil {
		return err
	}
	merged := make(map[string]any, len(existing)+len(metadata))
	for key, value := range existing {
		merged[key] = value
	}
	removed := false
	for _, key := range managedKeys {
		if _, ok := merged[key]; ok {
			delete(merged, key)
			removed = true
		}
	}
	for key, value := range metadata {
		merged[key] = value
	}
	if len(metadata) == 0 && !removed {
		return nil
	}
	merged = common.SplitCombinedMetadataValues(merged)
	return svc.SetDocumentMetadata(ctx, docID, merged)
}

// applyBuiltInMetadata writes the configured built-in metadata fields into the
// document's metadata, overwriting existing values. The fields are:
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
	return mergeDocMetadata(ctx, svc, docID, builtIn, nil)
}
