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

package dao

import (
	"testing"
	"time"

	"ragflow/internal/entity"
)

func TestSyncResumeCursor(t *testing.T) {
	committed := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	windowStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	incremental := "0"
	full := "1"

	t.Run("failed run resumes from committed progress", func(t *testing.T) {
		task := &entity.SyncLogs{
			PollRangeEnd:   entity.NewFlexibleTime(&committed),
			PollRangeStart: entity.NewFlexibleTime(&windowStart),
			FromBeginning:  &incremental,
		}
		got := syncResumeCursor(task)
		if got == nil || !got.Equal(committed) {
			t.Fatalf("cursor = %v, want %s", got, committed)
		}
	})

	t.Run("failed run without a batch keeps its window", func(t *testing.T) {
		task := &entity.SyncLogs{
			PollRangeStart: entity.NewFlexibleTime(&windowStart),
			FromBeginning:  &incremental,
		}
		got := syncResumeCursor(task)
		if got == nil || !got.Equal(windowStart) {
			t.Fatalf("cursor = %v, want %s", got, windowStart)
		}
	})

	t.Run("full reindex without progress starts over", func(t *testing.T) {
		task := &entity.SyncLogs{FromBeginning: &full}
		if got := syncResumeCursor(task); got != nil {
			t.Fatalf("cursor = %v, want nil", got)
		}
		if !failedFullReindexWithoutProgress(task) {
			t.Fatal("expected a full reindex with no progress to start over")
		}
	})
}
