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

package nav

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestDocScore(t *testing.T) {
	// sum=2, hits=1 -> 2/sqrt(2) ≈ 1.414
	got := docScore(2, 1)
	want := 2 / math.Sqrt(2)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("docScore(2,1) = %v, want %v", got, want)
	}
	// More hits at the same total score less because of the sqrt damping.
	if docScore(2, 4) >= docScore(2, 1) {
		t.Errorf("more hits should damp the docScore")
	}
}

func TestAggregateChunks(t *testing.T) {
	chunks := []map[string]any{
		{"doc_id": "d1", "similarity": 0.9},
		{"doc_id": "d1", "similarity": 0.3},
		{"doc_id": "d2", "score": 0.5},
		{"doc_id": "   ", "similarity": 0.8}, // empty doc_id ignored
		{"similarity": 0.7},                  // no doc_id ignored
	}
	agg := aggregateChunks(chunks)
	if len(agg) != 2 {
		t.Fatalf("aggregated %d docs, want 2", len(agg))
	}
	d1 := agg["d1"]
	if d1.Total != 1.2 || d1.Best != 0.9 || d1.Hits != 2 {
		t.Errorf("d1 = %+v, want total 1.2 best 0.9 hits 2", d1)
	}
	d2 := agg["d2"]
	if d2.Total != 0.5 || d2.Best != 0.5 || d2.Hits != 1 {
		t.Errorf("d2 = %+v, want total 0.5 best 0.5 hits 1", d2)
	}
}

func TestChunkAggRouterRoute(t *testing.T) {
	ctx := context.Background()
	rtr := &ChunkAggRouter{
		Retrieve: func(_ context.Context, _, _ string, _ string, _ []string, topN int, vecWeight float64) ([]map[string]any, error) {
			if vecWeight != chunkAggVecWeight {
				t.Errorf("vecWeight = %v, want %v", vecWeight, chunkAggVecWeight)
			}
			if topN != chunkAggPool {
				t.Errorf("topN = %d, want pool %d", topN, chunkAggPool)
			}
			return []map[string]any{
				{"doc_id": "d1", "similarity": 0.9},
				{"doc_id": "d1", "similarity": 0.7},
				{"doc_id": "d2", "similarity": 0.4},
			}, nil
		},
		Summarize: func(_ context.Context, _, _ string, ids []string) map[string]string {
			out := map[string]string{}
			for _, id := range ids {
				out[id] = "summary of " + id
			}
			return out
		},
	}
	routed, err := rtr.Route(ctx, "t", "kb", "question", nil, 0)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	// d1 outranks d2 (total 1.6 hits 2 -> 1.6/sqrt3 vs d2 0.4/sqrt2), focus cap 3.
	if len(routed) != 2 {
		t.Fatalf("routed %d docs, want 2", len(routed))
	}
	if routed[0][0] != "d1" || routed[1][0] != "d2" {
		t.Errorf("order = %v, want [d1 d2]", routed)
	}
	if routed[0][1] != "summary of d1" {
		t.Errorf("summary[0] = %q, want summary of d1", routed[0][1])
	}
}

func TestChunkAggRouterEmpty(t *testing.T) {
	ctx := context.Background()
	// Nil retriever -> (nil, nil): treated as no compiled tree.
	rtr := &ChunkAggRouter{}
	routed, err := rtr.Route(ctx, "t", "kb", "q", nil, 0)
	if err != nil || routed != nil {
		t.Errorf("nil retriever: routed=%v err=%v, want (nil, nil)", routed, err)
	}
}

func TestChunkAggRouterNoRoute(t *testing.T) {
	ctx := context.Background()
	rtr := &ChunkAggRouter{
		Retrieve: func(_ context.Context, _, _ string, _ string, _ []string, _ int, _ float64) ([]map[string]any, error) {
			return nil, nil
		},
	}
	routed, err := rtr.Route(ctx, "t", "kb", "q", nil, 5)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if routed == nil {
		t.Fatal("expected empty non-nil slice when nothing routes (structure exists)")
	}
	if len(routed) != 0 {
		t.Errorf("routed len = %d, want 0", len(routed))
	}
}

// TestChunkAggRouterPropagatesRetrievalError pins the router contract: a
// retrieval FAILURE is an error, not "no document routed". Python's
// _search_layers_nav_chunk_agg returns ok=False/SERVER_ERROR there, reserving
// ok=True/total=0 for an empty aggregation.
func TestChunkAggRouterPropagatesRetrievalError(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("ES down")
	rtr := &ChunkAggRouter{
		Retrieve: func(_ context.Context, _, _ string, _ string, _ []string, _ int, _ float64) ([]map[string]any, error) {
			return nil, wantErr
		},
	}
	routed, err := rtr.Route(ctx, "t", "kb", "q", nil, 5)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if routed != nil {
		t.Errorf("routed = %v, want nil alongside the error", routed)
	}
}
