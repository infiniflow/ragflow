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
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// dao.DB is a process-wide handle that test helpers swap in and restore, while a
// worker goroutine is free to still be processing a task. Both the worker's
// terminal bookkeeping and settleMessage's panic handler read it, so a nil
// handle has to degrade to a logged error: a panic raised inside a deferred
// function is unrecoverable, so it escapes the handler that exists to keep the
// worker alive and kills the process with it. Regression: CI run 35325530403,
// where that turned a torn-down test database into a crashed test binary.
func TestMarkFailedToleratesNilDatabase(t *testing.T) {
	defer testutil.ReplaceDBForTest(t, nil)()

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	if terminal := ingestor.markFailed(context.Background(), "task-1"); terminal {
		t.Fatal("markFailed reported a durable FAILED write without a database")
	}
}

// TestRunTaskToleratesNilDatabase covers the other half of the same path: the
// terminal pipeline-log/reset bookkeeping that runs after the pipeline itself.
func TestRunTaskToleratesNilDatabase(t *testing.T) {
	defer testutil.ReplaceDBForTest(t, nil)()

	runID := "run-1"
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(context.Context, *entity.IngestionTask) error { return nil }
	ingestor.runTask(context.Background(), &entity.IngestionTask{
		ID: "task-1", DocumentID: "doc-1", DatasetID: "kb-1", PipelineLogID: &runID,
	})
}
