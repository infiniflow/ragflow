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

package globals

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

// withChunkCap attaches a CanvasState carrying debug_chunk_cap to ctx.
func withChunkCap(t *testing.T, cap int) context.Context {
	t.Helper()
	st := &runtime.CanvasState{Globals: map[string]any{DebugChunkCapKey: cap}}
	return runtime.WithState(context.Background(), st)
}

// TestDebugChunkCap_DefaultZero asserts the cap reads 0 when no state is
// attached (headless unit tests) or when the key is absent — 0 means "no cap".
func TestDebugChunkCap_DefaultZero(t *testing.T) {
	if got := DebugChunkCap(context.Background()); got != 0 {
		t.Errorf("DebugChunkCap(nil ctx) = %d, want 0", got)
	}
	st := &runtime.CanvasState{Globals: map[string]any{}}
	if got := DebugChunkCap(runtime.WithState(context.Background(), st)); got != 0 {
		t.Errorf("DebugChunkCap(empty globals) = %d, want 0", got)
	}
}

// TestDebugChunkCap_ReadsFromGlobals asserts the cap is read from
// CanvasState.Globals, in all numeric shapes a run input may carry (int from a
// Go-side default, float64 from a JSON-decoded override).
func TestDebugChunkCap_ReadsFromGlobals(t *testing.T) {
	cases := []struct {
		name string
		cap  any
		want int
	}{
		{"int default", 3, 3},
		{"int64", int64(5), 5},
		{"float64 from json", float64(7), 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := runtime.WithState(context.Background(), &runtime.CanvasState{
				Globals: map[string]any{DebugChunkCapKey: c.cap},
			})
			if got := DebugChunkCap(ctx); got != c.want {
				t.Errorf("DebugChunkCap = %d, want %d", got, c.want)
			}
		})
	}
}

// TestSeedIngestionGlobals_PicksUpDebugChunkCap asserts that adding
// DebugChunkCapKey to GlobalMetadataKeys makes SeedIngestionGlobals copy the
// run-input value into CanvasState.Globals — the link the executor relies on
// to deliver the cap to the chunker decorator. Production inputs never set
// this key, so persist runs are unaffected.
func TestSeedIngestionGlobals_PicksUpDebugChunkCap(t *testing.T) {
	st := &runtime.CanvasState{Globals: map[string]any{}}
	ctx := runtime.WithState(context.Background(), st)
	SeedIngestionGlobals(ctx, map[string]any{
		"name":           "doc.pdf",
		DebugChunkCapKey: 3,
	})
	if got, ok := st.GetGlobal(DebugChunkCapKey); !ok || got != 3 {
		t.Errorf("after SeedIngestionGlobals, globals[%q] = %v (ok=%v), want 3", DebugChunkCapKey, got, ok)
	}
}
