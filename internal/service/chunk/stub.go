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

//go:build !ee

package chunk

import (
	"context"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service/chunkversion"
)

// recordChunkVersionEE keeps no history: the community edition edits a chunk in
// place.
func (s *ChunkService) recordChunkVersionEE(context.Context, string, string, string, int64, map[string]interface{}, map[string]interface{}, chunkversion.SnapshotMeta) {
}

// chunkHistoryVersion takes no version: without a chunk history there is nothing
// to anchor, and 0 tells the callers to skip the rows.
func chunkHistoryVersion(context.Context, string, []entity.DatasetVersionChange) (int64, error) {
	return 0, nil
}

// chunkEditMovesDataset cannot tell the two apart without a snapshot to compare,
// but it does not have to: chunkHistoryVersion records nothing here, so the
// caller's branch is a no-op either way.
func chunkEditMovesDataset(_, _ map[string]interface{}) bool { return true }

// chunkSwitchMoves cannot tell either, and for the same reason: the toggle
// records no version in this edition, so nothing downstream depends on it.
func chunkSwitchMoves(_ map[string]interface{}, _ int) bool { return false }

// beginChunkEditEE has no history counter to compare an expected version
// against, so a request that carries one is rejected instead of silently
// ignored: the caller would believe its edit was conflict-checked.
func (s *ChunkService) beginChunkEditEE(
	_ context.Context, _, _, _ string, expected *int, _, _ map[string]interface{}, _ chunkversion.SnapshotMeta,
) (func(error) error, int, error) {
	if expected == nil {
		return nil, 0, nil
	}
	return nil, 0, updateChunkError{
		code:    common.CodeArgumentError,
		message: "`expected_version` requires the enterprise edition, which keeps a chunk edit history",
	}
}

// chunkEmbeddingForEditEE returns no vector: the community edition does not
// re-embed edited chunks, so the edit carries the text alone.
func (s *ChunkService) chunkEmbeddingForEditEE(context.Context, *entity.Knowledgebase, *entity.Document, string) (*ChunkEmbedding, error) {
	return nil, nil
}

// annotateChunkVersionsEE leaves the list untouched: without an edit history
// every chunk is still at the version the parser wrote, i.e. v0.
func (s *ChunkService) annotateChunkVersionsEE(context.Context, []map[string]interface{}) {}
