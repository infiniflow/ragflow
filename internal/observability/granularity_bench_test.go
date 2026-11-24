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

package observability

import (
	"context"
	"testing"

	"ragflow/internal/entity"
)

// BenchmarkBuildProgressIngestLog guards the allocation profile of the
// high-frequency progress path (one call per chunk/batch). It builds but does
// not emit; any future regression that adds allocations/DB I/O here surfaces
// as a higher ns/op, B/op or allocs/op.
func BenchmarkBuildProgressIngestLog(b *testing.B) {
	task := &entity.IngestionTask{ID: "t", DocumentID: "d", DatasetID: "kb", UserID: "u", Status: "RUNNING"}
	doc := &entity.Document{Name: ptrStr("r"), Suffix: ".pdf", Type: "pdf"}
	run := &entity.PipelineOperationLog{ID: "run", TenantID: "tenant"}
	ctx := WithIngestorName(context.Background(), "ingestor-1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildProgressIngestLog(ctx, task, doc, run, "Tokenizer", "chunk", 257, 3000)
	}
}
