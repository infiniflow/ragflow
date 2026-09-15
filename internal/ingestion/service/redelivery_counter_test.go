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

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

// The two tests below guard counter idempotency across at-least-once redelivery.
// runTask applies the pipeline's chunk/token counts to the document and its
// knowledgebase (runDocumentTask -> docState.apply -> ApplyDocCounts) before it
// marks the task complete, so a redelivered task re-runs the pipeline. Because
// ApplyDocCounts sets the document's own counts and rolls only the delta into the
// knowledge base aggregate, re-applying the same run contributes zero: the same
// task, processed twice, applies its counters once. The chunk store is likewise
// idempotent (upsert by deterministic id).

const (
	rcChunks int64 = 5
	rcTokens int64 = 100
)

// applyResult stands in for defaultRunDocumentTask: a successful pipeline run
// that applies its result to the doc + KB counters, as docState.apply does after
// Execute returns.
func applyResult(ingestor *Ingestor, docID, kbID string) func(context.Context, *entity.IngestionTask) error {
	return func(ctx context.Context, _ *entity.IngestionTask) error {
		ingestor.docState.apply(ctx, &taskpkg.PipelineResult{
			DocID:            docID,
			KbID:             kbID,
			ChunkCount:       int(rcChunks),
			TokenConsumption: int(rcTokens),
			Duration:         1,
		})
		return nil
	}
}

// assertCountersAppliedOnce fails when the doc/KB counters reflect more than a
// single application of the pipeline result - i.e. a redelivery re-counted.
func assertCountersAppliedOnce(t *testing.T, db *gorm.DB, kbID, docID string) {
	t.Helper()
	var kb entity.Knowledgebase
	if err := db.First(&kb, "id = ?", kbID).Error; err != nil {
		t.Fatalf("load kb: %v", err)
	}
	var doc entity.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("load doc: %v", err)
	}
	if kb.ChunkNum != rcChunks || kb.TokenNum != rcTokens {
		t.Errorf("kb counters = (chunk %d, token %d), want (%d, %d) - redelivery double-counted", kb.ChunkNum, kb.TokenNum, rcChunks, rcTokens)
	}
	if doc.ChunkNum != rcChunks || doc.TokenNum != rcTokens {
		t.Errorf("doc counters = (chunk %d, token %d), want (%d, %d) - redelivery double-counted", doc.ChunkNum, doc.TokenNum, rcChunks, rcTokens)
	}
}

func rcMsg(taskID, docID, kbID string) *entity.IngestionTask {
	return &entity.IngestionTask{ID: taskID, DocumentID: docID, DatasetID: kbID, Status: common.RUNNING}
}

// TestRunTask_RedeliveryOfCompletedTaskCountsOnce: the first delivery fully
// completes (task -> COMPLETED), then the broker redelivers the same message
// because its Ack was lost. runTask has no already-completed guard, so it
// re-runs the pipeline; the counter application must stay idempotent.
func TestRunTask_RedeliveryOfCompletedTaskCountsOnce(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, kbID, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = applyResult(ingestor, docID, kbID)

	// First delivery: parse succeeds, counters applied once, task -> COMPLETED.
	if terminal := ingestor.runTask(context.Background(), rcMsg(taskID, docID, kbID)); !terminal {
		t.Fatalf("expected the first delivery to complete (terminal=true)")
	}
	// Redelivery (Ack lost): the same message is processed again. Must NOT re-count.
	ingestor.runTask(context.Background(), rcMsg(taskID, docID, kbID))

	assertCountersAppliedOnce(t, db, kbID, docID)
}

// TestRunTask_RedeliveryAfterIncompleteRunCountsOnce: the crash/nack window. A
// prior run applied the counters but died before MarkCompleted, so the task is
// left RUNNING and the broker redelivers it; the redelivery re-runs and
// completes, and the counters must not be applied twice.
func TestRunTask_RedeliveryAfterIncompleteRunCountsOnce(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, kbID, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = applyResult(ingestor, docID, kbID)

	// Prior run: counters applied, but the task never completed (crash before
	// MarkCompleted) - the task row is left RUNNING, so the broker redelivers.
	applyResult(ingestor, docID, kbID)(context.Background(), nil)

	// Redelivery of the still-RUNNING task: re-runs and completes.
	if terminal := ingestor.runTask(context.Background(), rcMsg(taskID, docID, kbID)); !terminal {
		t.Fatalf("expected the redelivery run to complete (terminal=true)")
	}

	assertCountersAppliedOnce(t, db, kbID, docID)
}
