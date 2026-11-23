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

package chunkversion

import (
	"context"

	"gorm.io/gorm"
)

// AppendParseBaseline records nothing: the community edition keeps no chunk
// history.
func AppendParseBaseline(context.Context, *gorm.DB, string, int64, SnapshotMeta, []map[string]interface{}) error {
	return nil
}

// AppendEmptyParseBaseline records nothing: the community edition keeps no chunk
// history, so there is no baseline to mark a document that parsed into no chunks.
func AppendEmptyParseBaseline(context.Context, *gorm.DB, string, int64, string) error {
	return nil
}

// AppendAddBaseline records nothing: the community edition keeps no chunk
// history, so a manually added chunk has no timeline to start.
func AppendAddBaseline(context.Context, *gorm.DB, string, string, string, int64, SnapshotMeta, map[string]interface{}, string) {
}

// HasEmptyParseMarker has no history to consult: the community edition keeps no
// parse marker, so a restored empty document reads as unparsed.
func HasEmptyParseMarker(context.Context, *gorm.DB, string) (bool, error) {
	return false, nil
}

// OpenChunkIDs has no history to consult.
func OpenChunkIDs(context.Context, *gorm.DB, string) ([]string, error) {
	return nil, nil
}

// AppendTombstones records nothing: without a history there is no timeline to
// keep straight.
func AppendTombstones(context.Context, *gorm.DB, string, string, int64, []string, string) {}

// AppendDocumentTombstones records nothing: the community edition keeps no
// history to replay.
func AppendDocumentTombstones(context.Context, *gorm.DB, string, string, int64, string) {}

// StatesAt has no history to replay.
func StatesAt(context.Context, *gorm.DB, string, int64) ([]ChunkState, error) {
	return nil, nil
}

// HasAnyHistory keeps no history: nothing reaches a document, so a replay has
// nothing to clear a document from.
func HasAnyHistory(context.Context, *gorm.DB, string) (bool, error) {
	return false, nil
}

// PurgeDocumentHistoryEE purges nothing: the community edition keeps no chunk
// history, so there is nothing to drop with a removed document.
func PurgeDocumentHistoryEE(_ context.Context, _ *gorm.DB, _, _ string) error { return nil }
