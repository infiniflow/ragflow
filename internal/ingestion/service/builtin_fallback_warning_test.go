//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package service

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// captureWarns swaps in an observer-backed logger so tests can assert on the
// builtin-fallback warning; the previous logger is restored on cleanup.
func captureWarns(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	old := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = old })
	return logs
}

func builtinFallbackWarnings(logs *observer.ObservedLogs) []string {
	var msgs []string
	for _, e := range logs.FilterMessageSnippet("falling back to the built-in").All() {
		msgs = append(msgs, e.Message)
	}
	return msgs
}

// TestDefaultRunDocumentTask_WarnsWhenDatasetPipelineNotSelected pins the
// observable signal for the silent fallback: a document without a pipeline
// selection runs the built-in template even while its dataset is bound to a
// pipeline (binding updates the dataset row only; documents are updated by
// the explicit apply-parser-config step). The run itself still fails here
// (no storage engine in the unit environment) — the assertion is the warning.
func TestDefaultRunDocumentTask_WarnsWhenDatasetPipelineNotSelected(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, kbID, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithTenantID("tenant-1"),
		testutil.WithKBID("kb-1"),
		testutil.WithDocID("doc-1"),
		testutil.WithTaskID("task-1"),
	)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", kbID).Update("pipeline_id", "flow-1").Error; err != nil {
		t.Fatalf("bind pipeline on dataset: %v", err)
	}

	logs := captureWarns(t)
	ingestor := newUnitIngestor("test", 1)
	err := ingestor.defaultRunDocumentTask(context.Background(), &entity.IngestionTask{
		ID:         taskID,
		DocumentID: docID,
		DatasetID:  kbID,
		Status:     common.RUNNING,
	})
	if err == nil {
		t.Fatal("expected downstream error (no storage engine)")
	}

	warns := builtinFallbackWarnings(logs)
	if len(warns) != 1 {
		t.Fatalf("got %d builtin-fallback warnings, want 1; logs: %v", len(warns), logs.All())
	}
	for _, want := range []string{docID, kbID, "flow-1", "naive", "parser settings", "apply-parser-config"} {
		if !strings.Contains(warns[0], want) {
			t.Fatalf("warning %q does not mention %q", warns[0], want)
		}
	}
}

// TestDefaultRunDocumentTask_NoWarnWhenDocSelectsPipeline covers the
// document-selected pipeline path: the fallback warning must stay silent
// even though the dataset is also pipeline-bound.
func TestDefaultRunDocumentTask_NoWarnWhenDocSelectsPipeline(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, kbID, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
		testutil.WithKBID("kb-1"),
		testutil.WithDocID("doc-1"),
		testutil.WithTaskID("task-1"),
	)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", kbID).Update("pipeline_id", "flow-1").Error; err != nil {
		t.Fatalf("bind pipeline on dataset: %v", err)
	}

	logs := captureWarns(t)
	ingestor := newUnitIngestor("test", 1)
	err := ingestor.defaultRunDocumentTask(context.Background(), &entity.IngestionTask{
		ID:         taskID,
		DocumentID: docID,
		DatasetID:  kbID,
		Status:     common.RUNNING,
	})
	if err == nil {
		t.Fatal("expected downstream error (no canvas in test environment)")
	}

	if warns := builtinFallbackWarnings(logs); len(warns) != 0 {
		t.Fatalf("got %d builtin-fallback warnings, want 0: %v", len(warns), warns)
	}
}

// TestDefaultRunDocumentTask_NoWarnWhenDatasetHasNoPipeline covers the plain
// built-in dataset: falling back there is the normal path and must stay quiet.
func TestDefaultRunDocumentTask_NoWarnWhenDatasetHasNoPipeline(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, kbID, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithTenantID("tenant-1"),
		testutil.WithKBID("kb-1"),
		testutil.WithDocID("doc-1"),
		testutil.WithTaskID("task-1"),
	)

	logs := captureWarns(t)
	ingestor := newUnitIngestor("test", 1)
	err := ingestor.defaultRunDocumentTask(context.Background(), &entity.IngestionTask{
		ID:         taskID,
		DocumentID: docID,
		DatasetID:  kbID,
		Status:     common.RUNNING,
	})
	if err == nil {
		t.Fatal("expected downstream error (no storage engine)")
	}

	if warns := builtinFallbackWarnings(logs); len(warns) != 0 {
		t.Fatalf("got %d builtin-fallback warnings, want 0: %v", len(warns), warns)
	}
}
