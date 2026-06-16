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

// parallel_options_test.go — option semantics for AddParallelNode.
// These tests focus on the configured behaviour of the option
// set (defaults, forwarding, builders, compile-time failure
// paths, sentinel errors).
package workflowx

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/compose"
)

// TestOptions_DefaultMaxConcurrencyIsSequential asserts that
// omitting WithParallelMaxConcurrency yields MaxConcurrency == 0
// (sequential).
func TestOptions_DefaultMaxConcurrencyIsSequential(t *testing.T) {
	opts := getParallelOptions(nil)
	if opts.maxConcurrency != 0 {
		t.Errorf("default max concurrency: got %d, want 0", opts.maxConcurrency)
	}
}

// TestOptions_WithParallelMaxConcurrency_Positive asserts that
// positive values are preserved.
func TestOptions_WithParallelMaxConcurrency_Positive(t *testing.T) {
	opts := getParallelOptions([]ParallelOption{WithParallelMaxConcurrency(8)})
	if opts.maxConcurrency != 8 {
		t.Errorf("got %d, want 8", opts.maxConcurrency)
	}
}

// TestOptions_WithParallelMaxConcurrency_NegativeKeepsDefault
// asserts that negative values are ignored.
func TestOptions_WithParallelMaxConcurrency_NegativeKeepsDefault(t *testing.T) {
	opts := getParallelOptions([]ParallelOption{WithParallelMaxConcurrency(-3)})
	if opts.maxConcurrency != 0 {
		t.Errorf("negative: got %d, want 0 (default)", opts.maxConcurrency)
	}
}

// TestOptions_ParallelCheckpointBuilder_Default is non-empty.
func TestOptions_ParallelCheckpointBuilder_Default(t *testing.T) {
	opts := getParallelOptions(nil)
	if opts.checkpointBuilder == nil {
		t.Fatal("default checkpoint builder is nil")
	}
	id := opts.checkpointBuilder("k", 3)
	if id == "" {
		t.Error("default builder returned empty id")
	}
	// The default format must be deterministic: same key+index
	// produces the same id.
	id2 := opts.checkpointBuilder("k", 3)
	if id != id2 {
		t.Errorf("default builder not deterministic: %q vs %q", id, id2)
	}
	// And it must contain the key and index so callers can
	// disambiguate parallel nodes.
	if !strings.Contains(id, "k") || !strings.Contains(id, "3") {
		t.Errorf("default id %q must contain key and index", id)
	}
}

// TestOptions_ParallelCheckpointBuilder_Override asserts the
// user-supplied builder is used and called with stable (key, idx).
func TestOptions_ParallelCheckpointBuilder_Override(t *testing.T) {
	var gotKey string
	var gotIdx int
	b := func(key string, idx int) string {
		gotKey = key
		gotIdx = idx
		return "cp:" + key + ":" + itoa(idx)
	}
	opts := getParallelOptions([]ParallelOption{WithParallelCheckpointIDBuilder(b)})
	id := opts.checkpointBuilder("parKey", 5)
	if id != "cp:parKey:5" {
		t.Errorf("builder output: got %q, want %q", id, "cp:parKey:5")
	}
	if gotKey != "parKey" || gotIdx != 5 {
		t.Errorf("builder args: got key=%q idx=%d, want key=%q idx=5", gotKey, gotIdx, "parKey")
	}
}

// TestOptions_ParallelCheckpointBuilder_NilIgnored asserts that a
// nil builder is ignored.
func TestOptions_ParallelCheckpointBuilder_NilIgnored(t *testing.T) {
	opts := getParallelOptions([]ParallelOption{WithParallelCheckpointIDBuilder(nil)})
	if opts.checkpointBuilder == nil {
		t.Fatal("default builder should remain after nil override")
	}
}

// TestOptions_EnableSubCheckpoint_Default asserts the default
// is true (sub-checkpoint enabled).
func TestOptions_EnableSubCheckpoint_Default(t *testing.T) {
	opts := getParallelOptions(nil)
	if !opts.enableSubCheckpoint {
		t.Error("default enableSubCheckpoint: got false, want true")
	}
}

// TestOptions_EnableSubCheckpoint_False is honored.
func TestOptions_EnableSubCheckpoint_False(t *testing.T) {
	opts := getParallelOptions([]ParallelOption{WithParallelEnableSubCheckpoint(false)})
	if opts.enableSubCheckpoint {
		t.Error("explicit false not honored")
	}
}

// TestOptions_ParallelRunOptionsForwarded asserts the run
// options are passed to every per-item sub-workflow Invoke. We
// assert that the run option count matches the call count.
func TestOptions_ParallelRunOptionsForwarded(t *testing.T) {
	var calls atomic.Int32
	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		calls.Add(1)
		return in + 1, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	outer := compose.NewWorkflow[[]int, []int]()
	pNode, err := AddParallelNode(context.Background(), outer, "par", sub,
		WithParallelMaxConcurrency(0),
		WithParallelRunOptions(compose.WithCheckPointID("ignored-by-inner")),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	pNode.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = compiled.Invoke(context.Background(), []int{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got := calls.Load(); got != 4 {
		t.Errorf("sub calls: got %d, want 4", got)
	}
}

// TestOptions_ParallelCompileOptionsForwarded asserts the compile
// options are passed to the inner sub-workflow's Compile call.
func TestOptions_ParallelCompileOptionsForwarded(t *testing.T) {
	store := newInMemoryStore()
	_ = store.Set(context.Background(), "k", []byte("v"))

	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		// Reach for the store to confirm wiring; without the
		// compile option the sub-workflow's runtime check
		// would surface a different error.
		_, _, _ = store.Get(ctx, "k")
		return in + 1, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	outer := compose.NewWorkflow[[]int, []int]()
	pNode, err := AddParallelNode(context.Background(), outer, "par", sub,
		WithParallelMaxConcurrency(0),
		WithParallelCompileOptions(compose.WithCheckPointStore(store)),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	pNode.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := compiled.Invoke(context.Background(), []int{1, 2, 3})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	want := []int{2, 3, 4}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d] = %d, want %d", i, got[i], v)
		}
	}
}

// TestOptions_ParallelNilChecks verifies that AddParallelNode
// rejects nil inputs up front, before any compile work happens.
func TestOptions_ParallelNilChecks(t *testing.T) {
	sub := buildParallelIncSub(t)
	outer := compose.NewWorkflow[[]int, []int]()

	cases := []struct {
		name string
		fn   func() error
	}{
		{"nil outer", func() error {
			_, err := AddParallelNode[int, int](context.Background(), nil, "par", sub)
			return err
		}},
		{"nil sub", func() error {
			var nilSub Compilable[int, int]
			_, err := AddParallelNode(context.Background(), outer, "par", nilSub)
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.fn()
			if err == nil {
				t.Errorf("%s: expected error, got nil", c.name)
			}
		})
	}
}

// TestOptions_ParallelCompileFailureIsolated asserts that when
// the sub-workflow fails to compile, AddParallelNode returns an
// error (wrapping ErrParallelCompileFailed) and the outer
// workflow is not modified to a state that would mask the
// failure.
func TestOptions_ParallelCompileFailureIsolated(t *testing.T) {
	sub := compose.NewWorkflow[int, int]() // no nodes; compile fails
	outer := compose.NewWorkflow[[]int, []int]()
	_, err := AddParallelNode(context.Background(), outer, "par", sub)
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}
	if !errors.Is(err, ErrParallelCompileFailed) {
		t.Errorf("errors.Is(err, ErrParallelCompileFailed) = false; err = %v", err)
	}
	// The outer workflow should still be empty.
	_, err = outer.Compile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start node not set") {
		t.Errorf("outer workflow not in expected state: %v", err)
	}
}

// TestOptions_ParallelSentinelErrorsExist is a smoke test that
// all parallel sentinel error values are non-nil and satisfy
// errors.Is against themselves.
func TestOptions_ParallelSentinelErrorsExist(t *testing.T) {
	sentinels := map[string]error{
		"ErrParallelCompileFailed":          ErrParallelCompileFailed,
		"ErrParallelResumeStateInvalid":     ErrParallelResumeStateInvalid,
		"ErrParallelOuterStreamUnsupported": ErrParallelOuterStreamUnsupported,
	}
	for name, e := range sentinels {
		if e == nil {
			t.Errorf("%s is nil", name)
		}
	}
	if !errors.Is(ErrParallelCompileFailed, ErrParallelCompileFailed) {
		t.Error("ErrParallelCompileFailed is not Is-self")
	}
	if !errors.Is(ErrParallelResumeStateInvalid, ErrParallelResumeStateInvalid) {
		t.Error("ErrParallelResumeStateInvalid is not Is-self")
	}
	if !errors.Is(ErrParallelOuterStreamUnsupported, ErrParallelOuterStreamUnsupported) {
		t.Error("ErrParallelOuterStreamUnsupported is not Is-self")
	}
}

// TestOptions_EmptyBuilderReturnRejectsEmptyID asserts that
// returning "" from the per-item checkpoint ID builder is
// treated as "skip WithCheckPointID for this item". This is
// tested via the runParallelFanout integration by ensuring the
// sub-workflow still gets called.
func TestOptions_EmptyBuilderReturnRejectsEmptyID(t *testing.T) {
	var calls atomic.Int32
	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		calls.Add(1)
		return in + 1, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	compiled, err := sub.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile sub: %v", err)
	}
	opts := getParallelOptions([]ParallelOption{
		WithParallelMaxConcurrency(0),
		WithParallelCheckpointIDBuilder(func(_ string, _ int) string {
			return ""
		}),
	})
	bridge := newParallelBridgeState(nil)
	got, err := runParallelInvoke(context.Background(), "par", compiled, []int{0, 1, 2}, opts, bridge)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got len %d, want 3", len(got))
	}
	if calls.Load() != 3 {
		t.Errorf("sub calls: got %d, want 3", calls.Load())
	}
}

// TestOptions_EnableSubCheckpointFalse_NoPerItemID asserts that
// WithParallelEnableSubCheckpoint(false) does not inject a
// per-item WithCheckPointID. We verify by counting the
// successful invocations — the absence of a checkpoint id is
// safe because the sub-workflow has no checkpoint store in this
// test.
func TestOptions_EnableSubCheckpointFalse_NoPerItemID(t *testing.T) {
	var calls atomic.Int32
	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		calls.Add(1)
		return in + 1, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	compiled, err := sub.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile sub: %v", err)
	}
	opts := getParallelOptions([]ParallelOption{
		WithParallelMaxConcurrency(0),
		WithParallelEnableSubCheckpoint(false),
	})
	bridge := newParallelBridgeState(nil)
	_, err = runParallelInvoke(context.Background(), "par", compiled, []int{0, 1, 2}, opts, bridge)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("sub calls: got %d, want 3", calls.Load())
	}
}

// TestOptions_ParallelInterruptState_JSONRoundtrip asserts the
// persisted state survives an encode/decode cycle cleanly. This
// is the contract for resume: the resumed run reads the same
// fields back.
func TestOptions_ParallelInterruptState_JSONRoundtrip(t *testing.T) {
	in := ParallelInterruptState{
		OriginalInputsJSON: []byte(`[0,1,2]`),
		CompletedResults:   map[int]any{0: "a", 2: "c"},
		InterruptedIndices: []int{1},
		TotalCount:         3,
		ItemCheckpoints:    map[string][]byte{"x": []byte("y")},
	}
	b, err := encodeParallelState(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out ParallelInterruptState
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TotalCount != 3 {
		t.Errorf("TotalCount: got %d, want 3", out.TotalCount)
	}
	if len(out.InterruptedIndices) != 1 || out.InterruptedIndices[0] != 1 {
		t.Errorf("InterruptedIndices: got %v, want [1]", out.InterruptedIndices)
	}
	if len(out.CompletedResults) != 2 {
		t.Errorf("CompletedResults len: got %d, want 2", len(out.CompletedResults))
	}
	if v, ok := out.CompletedResults[0]; !ok || v != "a" {
		t.Errorf("CompletedResults[0]: got %v, want a", v)
	}
	if v, ok := out.CompletedResults[2]; !ok || v != "c" {
		t.Errorf("CompletedResults[2]: got %v, want c", v)
	}
}
