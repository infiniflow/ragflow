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

package task

import (
	"context"
	"testing"
)

func TestChunkIndexWriter_EmptyChunks(t *testing.T) {
	called := false
	w := newChunkIndexWriter(
		func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			called = true
			if len(chunks) != 0 {
				t.Errorf("expected empty chunks, got %d", len(chunks))
			}
			return nil, nil
		},
		"test-base",
		"kb-1",
		10,
	)
	if err := w.Write(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("insertFunc was not called for empty chunks")
	}
}

func TestChunkIndexWriter_SingleBatch(t *testing.T) {
	var batchSizes []int
	w := newChunkIndexWriter(
		func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			batchSizes = append(batchSizes, len(chunks))
			if baseName != "test-base" {
				t.Errorf("baseName = %q, want test-base", baseName)
			}
			if datasetID != "kb-1" {
				t.Errorf("datasetID = %q, want kb-1", datasetID)
			}
			return nil, nil
		},
		"test-base",
		"kb-1",
		10,
	)
	chunks := make([]map[string]any, 5)
	if err := w.Write(context.Background(), chunks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batchSizes) != 1 {
		t.Fatalf("expected 1 batch, got %d: %v", len(batchSizes), batchSizes)
	}
	if batchSizes[0] != 5 {
		t.Fatalf("batch size = %d, want 5", batchSizes[0])
	}
}

func TestChunkIndexWriter_MultipleBatches(t *testing.T) {
	var batchSizes []int
	w := newChunkIndexWriter(
		func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			batchSizes = append(batchSizes, len(chunks))
			return nil, nil
		},
		"base",
		"kb-1",
		3,
	)
	chunks := make([]map[string]any, 7)
	if err := w.Write(context.Background(), chunks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(batchSizes) != 3 {
		t.Fatalf("expected 3 batches for 7 chunks with bulkSize=3, got %d: %v", len(batchSizes), batchSizes)
	}
	if batchSizes[0] != 3 || batchSizes[1] != 3 || batchSizes[2] != 1 {
		t.Fatalf("batch sizes = %v, want [3,3,1]", batchSizes)
	}
}

func TestChunkIndexWriter_BulkSizeZero(t *testing.T) {
	var lastBatchSize int
	w := newChunkIndexWriter(
		func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			lastBatchSize = len(chunks)
			return nil, nil
		},
		"base",
		"kb-1",
		0, // bulkSize=0 → should use len(chunks)
	)
	chunks := make([]map[string]any, 20)
	if err := w.Write(context.Background(), chunks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lastBatchSize != 20 {
		t.Fatalf("batch size = %d, want 20 (bulkSize=0 should degrade to len(chunks))", lastBatchSize)
	}
}
