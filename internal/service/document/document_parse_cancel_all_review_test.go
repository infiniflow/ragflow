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

package document

import (
	"fmt"
	"reflect"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// TestChunkDocumentIDs is the unit that prevents the GetLatestByDocumentIDs
// IN-clause from exceeding the driver's prepared-statement parameter limit on
// very large queues. It must round-trip every id and preserve order.
func TestChunkDocumentIDs(t *testing.T) {
	cases := []struct {
		name string
		ids  []string
		size int
		want int
	}{
		{"empty", nil, 1000, 0},
		{"single", []string{"a"}, 1000, 1},
		{"exact multiple", []string{"a", "b", "c", "d"}, 2, 2},
		{"non multiple", []string{"a", "b", "c"}, 2, 2}, // [a,b],[c]
		{"size larger than len", []string{"a", "b"}, 10, 1},
		{"size zero treated as one", []string{"a", "b", "c"}, 0, 3},
		{"size negative treated as one", []string{"a", "b"}, -1, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := chunkDocumentIDs(c.ids, c.size)
			if len(got) != c.want {
				t.Fatalf("chunk count = %d, want %d (ids=%v size=%d)", len(got), c.want, c.ids, c.size)
			}
			var flat []string
			for _, b := range got {
				flat = append(flat, b...)
			}
			if !reflect.DeepEqual(flat, c.ids) {
				t.Fatalf("chunks flattened %v != original %v", flat, c.ids)
			}
		})
	}
}

// TestStopParseDocumentsAll_DedupeDuplicateFileMappings covers the case where a
// document is linked to more than one file (multiple file2document rows), which
// makes the list query return the document id more than once. Without
// deduplication each such document would be cancelled and counted multiple
// times, inflating success_count.
func TestStopParseDocumentsAll_DedupeDuplicateFileMappings(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	// doc-1 is linked to TWO files -> two file2document rows -> duplicate id.
	insertTestDocWithFile(t, "doc-1", "file-1", "kb-1")
	insertTestFile(t, "file-2", "", "file-2.txt", nil)
	insertTestFile2Document(t, "f2d-2", "file-2", "doc-1")
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.SCHEDULED)

	svc := testDocumentService(t)
	ctx := t.Context()
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", nil)
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}
	if sc := stopAllSuccessCount(t, result); sc != 1 {
		t.Fatalf("expected success_count=1 after dedup, got %d", sc)
	}
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if err != nil || task == nil {
		t.Fatalf("load doc-1 task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("doc-1 task status = %s, want STOPPED", task.Status)
	}
}

// TestStopParseDocumentsAll_MissingTaskRecordedAsFailure covers the review
// finding: a document selected by the status filter but having no ingestion
// task to stop must be reported as a failure, not a misleading success. Here the
// document has no task, so its ingestion_status is UNSTART and is selected when
// UNSTART is requested.
func TestStopParseDocumentsAll_MissingTaskRecordedAsFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithFile(t, "doc-1", "file-1", "kb-1") // no ingestion task

	svc := testDocumentService(t)
	ctx := t.Context()
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", []string{common.UNSTART})
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}
	if sc := stopAllSuccessCount(t, result); sc != 0 {
		t.Fatalf("expected success_count=0 for missing task, got %d", sc)
	}
	errs, ok := result["errors"].([]string)
	if !ok || len(errs) != 1 {
		t.Fatalf("expected exactly 1 error for missing task, got %v", result["errors"])
	}
}

// TestStopParseDocumentsAll_ManyDocumentsAllCancelled exercises the bounded
// batching of the task lookup. The batch size is lowered so the multi-chunk
// path (and the map merge) is actually exercised, guarding against the
// MySQL prepared-statement parameter-limit failure on large queues.
func TestStopParseDocumentsAll_ManyDocumentsAllCancelled(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	const n = 60
	for i := 0; i < n; i++ {
		docID := fmt.Sprintf("doc-%d", i)
		fileID := fmt.Sprintf("file-%d", i)
		insertTestDocWithFile(t, docID, fileID, "kb-1")
		insertTestIngestionTaskWithStatus(t, fmt.Sprintf("task-%d", i), "user-1", docID, "kb-1", common.SCHEDULED)
	}

	orig := cancelAllTaskBatchSize
	cancelAllTaskBatchSize = 10
	defer func() { cancelAllTaskBatchSize = orig }()

	svc := testDocumentService(t)
	ctx := t.Context()
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", nil)
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}
	if sc := stopAllSuccessCount(t, result); sc != n {
		t.Fatalf("expected success_count=%d, got %d", n, sc)
	}
}
