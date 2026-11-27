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

//go:build cgo

package chunker

import (
	"context"
	"sync"
	"testing"

	agentruntime "ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/ingestion/component/schema"
)

type cropDetailRecorder struct {
	mu      sync.Mutex
	details []agentruntime.ProgressDetail
}

func (r *cropDetailRecorder) context(t *testing.T) context.Context {
	t.Helper()
	ctx := agentruntime.WithProgressDetailCallback(t.Context(), func(_ string, d agentruntime.ProgressDetail) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.details = append(r.details, d)
	})
	return agentruntime.BindComponentDetail(ctx, "Chunker:pdf")
}

func (r *cropDetailRecorder) snapshot() []agentruntime.ProgressDetail {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]agentruntime.ProgressDetail, len(r.details))
	copy(out, r.details)
	return out
}

func positionedCropChunk(t *testing.T, text string, page int) schema.ChunkDoc {
	t.Helper()
	return schema.ChunkDoc{
		Text:      text,
		Positions: jsonPositions(t, []float64{1, float64(page*100 - 100), float64(page * 100), float64(page*100 - 100), float64(page * 100)}),
	}
}

func TestCropImageChunksReportsImageCropDetail(t *testing.T) {
	rec := &cropDetailRecorder{}
	ctx := rec.context(t)

	chunks := []schema.ChunkDoc{
		positionedCropChunk(t, "content 1", 1),
		positionedCropChunk(t, "content 2", 2),
		positionedCropChunk(t, "content 3", 3),
	}

	_ = cropImageChunks(ctx, mockCropEngine{}, chunks)

	details := rec.snapshot()
	if len(details) == 0 {
		t.Fatal("expected image:crop detail reports, got none")
	}
	currents := make(map[uint32]bool)
	for _, d := range details {
		if d.Unit != "image:crop" {
			t.Errorf("detail unit = %q, want image:crop", d.Unit)
		}
		if d.Total != 3 {
			t.Errorf("detail total = %d, want 3", d.Total)
		}
		if d.Current > 3 {
			t.Errorf("detail current = %d, exceeds total 3", d.Current)
		}
		currents[d.Current] = true
	}
	for want := uint32(1); want <= 3; want++ {
		if !currents[want] {
			t.Errorf("missing detail current %d (got %v)", want, currents)
		}
	}
}

// Adversarial: total must be the number of crop candidates, not the number of
// chunks. A fixture where half the chunks have no positions distinguishes the
// correct implementation from one that reports len(chunks).
func TestCropImageChunksDetailTotalIsCandidateCount(t *testing.T) {
	rec := &cropDetailRecorder{}
	ctx := rec.context(t)

	chunks := []schema.ChunkDoc{
		positionedCropChunk(t, "with positions 1", 1),
		{Text: "no positions"},
		positionedCropChunk(t, "with positions 2", 2),
		{Text: "also no positions"},
	}

	_ = cropImageChunks(ctx, mockCropEngine{}, chunks)

	details := rec.snapshot()
	if len(details) == 0 {
		t.Fatal("expected image:crop detail reports, got none")
	}
	for _, d := range details {
		if d.Total != 2 {
			t.Errorf("detail total = %d, want 2 candidates (not 4 chunks)", d.Total)
		}
	}
}

func TestCropImageChunksZeroCandidatesNoDetail(t *testing.T) {
	rec := &cropDetailRecorder{}
	ctx := rec.context(t)

	chunks := []schema.ChunkDoc{{Text: "no positions"}}
	_ = cropImageChunks(ctx, mockCropEngine{}, chunks)

	if details := rec.snapshot(); len(details) != 0 {
		t.Errorf("expected no detail reports for zero candidates, got %v", details)
	}
}

func TestCropImageChunksCancelledNoDetail(t *testing.T) {
	rec := &cropDetailRecorder{}
	ctx, cancel := context.WithCancel(rec.context(t))
	cancel()

	chunks := []schema.ChunkDoc{
		positionedCropChunk(t, "content 1", 1),
		positionedCropChunk(t, "content 2", 2),
	}
	_ = cropImageChunks(ctx, mockCropEngine{}, chunks)

	if details := rec.snapshot(); len(details) != 0 {
		t.Errorf("cancelled context must not report detail, got %v", details)
	}
}

// BenchmarkCropDetailReport guards the per-crop-task counter report added to
// the worker defer — it must stay O(1) with no allocations on the emit path.
func BenchmarkCropDetailReport(b *testing.B) {
	ctx := agentruntime.WithProgressDetailCallback(context.Background(), func(_ string, _ agentruntime.ProgressDetail) {})
	ctx = agentruntime.BindComponentDetail(ctx, "Chunker:pdf")
	counter := progress.NewCounter(ctx, "image:crop", uint32(b.N), progress.Window{Span: 1})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Report(uint32(i+1), uint32(b.N))
	}
}
