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

package datasetversion

import (
	"context"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// WithStateSink has nothing to defer: the community edition keeps no
// per-document states, so the context travels unchanged.
func WithStateSink(ctx context.Context) context.Context {
	return ctx
}

// WithStateWritesSuppressed has nothing to suppress for the same reason.
func WithStateWritesSuppressed(ctx context.Context) context.Context {
	return ctx
}

// FlushStates writes nothing: the community edition keeps no document states.
//
// EnqueueStates has no counterpart here: every caller of it lives behind the ee
// build tag, together with the document states it parks.
func FlushStates(_ context.Context, _ *gorm.DB, _ string, _ int64) error { return nil }

// PendingStates always reports none: no flow parks a state in this edition.
func PendingStates(_ context.Context, _ string) int { return 0 }

// Snapshot returns nothing: the community edition keeps no dataset history, so
// no configuration snapshot is ever stored.
func Snapshot(_ *entity.Knowledgebase) entity.JSONMap {
	return nil
}

// RecordContentVersion records no version: the community edition keeps no
// dataset history.
func RecordContentVersion(_ context.Context, _ *gorm.DB, _, _, _ string) (int64, error) {
	return 0, nil
}

// RecordContentVersionChanges records no version: the community edition keeps no
// dataset history, so there is nothing a write could fail on.
func RecordContentVersionChanges(_ context.Context, _ *gorm.DB, _, _ string, _ []entity.DatasetVersionChange) (int64, error) {
	return 0, nil
}

// RecordContentVersionChangesOwnTx has no history to record, and therefore
// nowhere to keep a change set: nothing a write could fail on.
func RecordContentVersionChangesOwnTx(_ context.Context, _, _ string, _ []entity.DatasetVersionChange) (int64, error) {
	return 0, nil
}

// RecordContentVersionChangesAtVersionOwnTx has no history to record either: the
// community edition stores no version, so a pinned number has nowhere to go.
func RecordContentVersionChangesAtVersionOwnTx(_ context.Context, _, _ string, _ int64, _ []entity.DatasetVersionChange) (int64, error) {
	return 0, nil
}

// RecordVersionWithChangesAtVersion records no version either: a pinned number
// has nowhere to go without a history.
func RecordVersionWithChangesAtVersion(_ context.Context, _ *gorm.DB, _, _, _ string, _ int64, _ int64, _ []entity.DatasetVersionChange) (int64, error) {
	return 0, nil
}

// ReserveVersion reserves nothing: the community edition keeps no dataset
// history and no chunk history, so no writer needs a number to anchor a row. 0
// is what the callers read as "no history to write".
func ReserveVersion(_ context.Context, _ *gorm.DB, _ string) (int64, error) {
	return 0, nil
}

// WithActor names the user a version write would be attributed to. The community
// edition stores no versions, so the name has nowhere to go; the function exists
// so a caller can name its writer without a build tag of its own.
func WithActor(ctx context.Context, _ string) context.Context {
	return ctx
}
