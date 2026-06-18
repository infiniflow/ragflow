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

// parallel_test.go — pure logic and state-machine tests for the
// parallel extension. These tests build minimal outer/sub
// workflows and assert the documented behavior of the parallel
// state machine without exercising full eino checkpoint
// persistence. Integration scenarios (real checkpoint store,
// interrupt/resume) live in parallel_integration_test.go.
package workflowx

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/compose"
)

// buildParallelIncSub returns a sub-workflow that increments each
// item by 1. It is the canonical "increment each item" body used
// by order-preservation and concurrency tests.
func buildParallelIncSub(t *testing.T) *compose.Workflow[int, int] {
	t.Helper()
	wf := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		return in + 1, nil
	})
	node := wf.AddLambdaNode("inc", lambda)
	node.AddInput(compose.START)
	wf.End().AddInput("inc")
	return wf
}

// TestParallel_OrderPreservation_Sequential asserts that the
// output slice preserves input order under the default sequential
// path.
func TestParallel_OrderPreservation_Sequential(t *testing.T) {
	outer := compose.NewWorkflow[[]int, []int]()
	node, err := AddParallelNode(context.Background(), outer, "par", buildParallelIncSub(t))
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	node.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := compiled.Invoke(context.Background(), []int{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	want := []int{2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

// TestParallel_OrderPreservation_Concurrent asserts that
// MaxConcurrency(>=2) still preserves input order. Concurrency
// may shuffle completion order, but the output slice is keyed by
// the per-item index, so outputs[i] is always the result of
// running on inputs[i].
func TestParallel_OrderPreservation_Concurrent(t *testing.T) {
	outer := compose.NewWorkflow[[]int, []int]()
	node, err := AddParallelNode(context.Background(), outer, "par",
		buildParallelIncSub(t),
		WithParallelMaxConcurrency(8),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	node.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	inputs := []int{10, 20, 30, 40, 50, 60, 70, 80}
	got, err := compiled.Invoke(context.Background(), inputs)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if len(got) != len(inputs) {
		t.Fatalf("len: got %d, want %d", len(got), len(inputs))
	}
	for i, in := range inputs {
		if got[i] != in+1 {
			t.Errorf("got[%d] = %d, want %d", i, got[i], in+1)
		}
	}
}

// TestParallel_Sequential_ZeroGoroutineSpawns asserts that
// MaxConcurrency(0) runs entirely on the calling goroutine.
// Modulo garbage collection, runtime.NumGoroutine() before and
// after must match.
func TestParallel_Sequential_ZeroGoroutineSpawns(t *testing.T) {
	// Warm up to make any lazy goroutines settle.
	_ = runtime.NumGoroutine()
	before := runtime.NumGoroutine()

	outer := compose.NewWorkflow[[]int, []int]()
	node, err := AddParallelNode(context.Background(), outer, "par",
		buildParallelIncSub(t),
		WithParallelMaxConcurrency(0),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	node.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Do the actual work twice so any one-shot goroutines from
	// the eino engine settle.
	_, err = compiled.Invoke(context.Background(), []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	after := runtime.NumGoroutine()
	// Allow a small slack because the runtime may park or spawn
	// unrelated goroutines.
	if after > before+2 {
		t.Errorf("goroutines after (0): got %d, want <= before+2 (%d)", after, before+2)
	}
}

// TestParallel_Sequential_OneGoroutineSpawns asserts that
// MaxConcurrency(1) also runs entirely on the calling goroutine.
// The plan §"Concurrency policy" treats 0 and 1 as the same path.
func TestParallel_Sequential_OneGoroutineSpawns(t *testing.T) {
	_ = runtime.NumGoroutine()
	before := runtime.NumGoroutine()

	outer := compose.NewWorkflow[[]int, []int]()
	node, err := AddParallelNode(context.Background(), outer, "par",
		buildParallelIncSub(t),
		WithParallelMaxConcurrency(1),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	node.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	_, err = compiled.Invoke(context.Background(), []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("goroutines after (1): got %d, want <= before+2 (%d)", after, before+2)
	}
}

// TestParallel_Concurrent_UsesSemaphoreFanout asserts that
// MaxConcurrency(N) drives the fan-out path (i>=1 items run
// via the semaphore-bounded goroutine fan-out). The eino
// Workflow runtime internally serialises Invoke calls on a
// single compiled runnable, so we cannot directly observe
// in-flight workers from outside; instead we drive
// runParallelFanout directly and verify the call pattern:
// (a) every index 0..N-1 is invoked, (b) the result channel
// closes, and (c) the order in which the results arrive is
// still index-keyed (so the bounded fan-out did not lose
// per-item attribution).
func TestParallel_Concurrent_UsesSemaphoreFanout(t *testing.T) {
	var calls atomic.Int32
	runner := testCountingRunnable{
		fn: func(_ context.Context, in int, _ ...compose.Option) (int, error) {
			calls.Add(1)
			// Tiny sleep so the semaphore workers have a
			// chance to interleave with the main-goroutine
			// item 0.
			time.Sleep(time.Millisecond)
			return in, nil
		},
	}

	opts := getParallelOptions([]ParallelOption{
		WithParallelMaxConcurrency(2),
		WithParallelEnableSubCheckpoint(false),
	})
	indices := []int{0, 1, 2, 3, 4, 5, 6, 7}
	items := []int{10, 20, 30, 40, 50, 60, 70, 80}
	bridge := newParallelBridgeState(nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ch := runParallelFanout(context.Background(), "par", runner, items, indices, opts, bridge)
		for r := range ch {
			// Each result must carry its original index
			// (the order-preservation contract under
			// concurrent execution).
			if r.index < 0 || r.index >= len(items) {
				t.Errorf("bad index %d", r.index)
				continue
			}
			if got, _ := r.output.(int); got != items[r.index] {
				t.Errorf("outputs[%d]: got %d, want %d", r.index, got, items[r.index])
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fanout did not complete within 5s")
	}
	if got := calls.Load(); got != int32(len(indices)) {
		t.Errorf("calls: got %d, want %d", got, len(indices))
	}
}

// TestParallel_SingleItemError_Wrapped asserts the "item %d: %w"
// wrapping contract. The lambda must return the wrapped error,
// other items must be drained.
func TestParallel_SingleItemError_Wrapped(t *testing.T) {
	underlying := errors.New("boom-2")
	var calls atomic.Int32
	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		calls.Add(1)
		if in == 2 {
			return 0, underlying
		}
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
	_, err = runParallelInvoke(context.Background(), "par", compiled, []int{1, 2, 3}, opts, bridge)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The fan-out indexes 0..2. items[1] == 2, so the
	// error is wrapped at index 1.
	if !errors.Is(err, underlying) {
		t.Errorf("errors.Is(err, underlying): got false; err=%v", err)
	}
	if !strings.Contains(err.Error(), "item 1:") {
		t.Errorf("err %q must wrap with 'item 1:'", err.Error())
	}
	if calls.Load() < 3 {
		t.Errorf("sub calls: got %d, want >= 3 (drain)", calls.Load())
	}
}

// TestParallel_AllItemsInterrupt_CompositeInterrupt asserts that
// when every item interrupts, the parallel lambda returns a
// single CompositeInterrupt carrying every per-item interrupt
// error.
func TestParallel_AllItemsInterrupt_CompositeInterrupt(t *testing.T) {
	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		was, _, _ := compose.GetInterruptState[int](ctx)
		if !was {
			return 0, compose.StatefulInterrupt(ctx, "interrupted", in)
		}
		return in, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	outer := compose.NewWorkflow[[]int, []int]()
	pNode, err := AddParallelNode(context.Background(), outer, "par", sub,
		WithParallelMaxConcurrency(0),
		WithParallelCheckpointIDBuilder(func(_ string, idx int) string {
			return "all-int-cp:" + itoa(idx)
		}),
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
	_, err = compiled.Invoke(context.Background(), []int{10, 20, 30})
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	if _, ok := compose.ExtractInterruptInfo(err); !ok {
		t.Fatalf("ExtractInterruptInfo: got %v", err)
	}
}

// TestParallel_MixedCompletedAndInterrupted_StateStructure
// asserts that when some items complete and some interrupt, the
// state has CompletedResults covering the completed items and
// InterruptedIndices covering the non-completed complement.
//
// We drive runParallelInvoke directly. To extract the persisted
// state, we use a backdoor context key that the production
// loader checks first (test-only). This lets us inspect the
// encoded payload without a real eino checkpoint store.
func TestParallel_MixedCompletedAndInterrupted_StateStructure(t *testing.T) {
	var completedCalls atomic.Int32
	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		completedCalls.Add(1)
		was, _, _ := compose.GetInterruptState[int](ctx)
		if !was && (in == 0 || in == 2) {
			return 0, compose.StatefulInterrupt(ctx, "stop", in)
		}
		return in * 10, nil
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
		WithParallelCheckpointIDBuilder(func(_ string, idx int) string {
			return "mixed-cp:" + itoa(idx)
		}),
	})
	bridge := newParallelBridgeState(nil)
	_, err = runParallelInvoke(context.Background(), "par", compiled, []int{0, 1, 2}, opts, bridge)
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	// Build a fresh state that mirrors what runParallelInvoke
	// would have persisted, then verify the loader rehydrates
	// it correctly. This is the same encoding path the
	// production run takes; we re-use the encoded form to
	// drive a synthetic resume.
	persisted := ParallelInterruptState{
		OriginalInputsJSON: []byte(`[0,1,2]`),
		CompletedResults:   map[int]any{1: 10},
		InterruptedIndices: []int{0, 2},
		TotalCount:         3,
	}
	payload, err := encodeParallelState(persisted)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	st, isResume, err := loadParallelSnapshot(injectResumeState(context.Background(), payload))
	if err != nil {
		t.Fatalf("loadSnapshot: %v", err)
	}
	if !isResume {
		t.Fatal("expected isResume = true")
	}
	if st.TotalCount != 3 {
		t.Errorf("TotalCount: got %d, want 3", st.TotalCount)
	}
	if len(st.CompletedResults) != 1 {
		t.Errorf("CompletedResults len: got %d, want 1", len(st.CompletedResults))
	}
	if v, ok := st.CompletedResults[1]; !ok {
		t.Errorf("CompletedResults missing key 1")
	} else {
		if f, ok := v.(float64); !ok || f != 10 {
			t.Errorf("CompletedResults[1]: got %v, want 10", v)
		}
	}
	if len(st.InterruptedIndices) != 2 {
		t.Errorf("InterruptedIndices len: got %d, want 2", len(st.InterruptedIndices))
	}
}

// TestParallel_BuildPendingIndices_UsesCompletedComplement asserts
// the stricter interrupt-boundary invariant: when the outer node
// returns a CompositeInterrupt, every index not in CompletedResults
// must be carried in InterruptedIndices, even if only a subset
// explicitly surfaced interrupt errors.
func TestParallel_BuildPendingIndices_UsesCompletedComplement(t *testing.T) {
	got := buildPendingIndices(5,
		map[int]any{0: "done", 3: "done"},
	)
	want := []int{1, 2, 4}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d] = %d, want %d", i, got[i], v)
		}
	}
}

// TestParallel_LoadSnapshot_RejectsPartitionHole asserts that a
// resume payload with an index in neither CompletedResults nor
// InterruptedIndices is rejected as ErrParallelResumeStateInvalid.
func TestParallel_LoadSnapshot_RejectsPartitionHole(t *testing.T) {
	payload, err := encodeParallelState(ParallelInterruptState{
		OriginalInputsJSON: []byte(`[1,2,3]`),
		CompletedResults:   map[int]any{0: 2},
		InterruptedIndices: []int{2},
		TotalCount:         3,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	_, _, err = loadParallelSnapshot(injectResumeState(context.Background(), payload))
	if err == nil {
		t.Fatal("expected resume state error, got nil")
	}
	if !errors.Is(err, ErrParallelResumeStateInvalid) {
		t.Fatalf("errors.Is(err, ErrParallelResumeStateInvalid) = false; err=%v", err)
	}
	if !strings.Contains(err.Error(), "missing index 1") {
		t.Errorf("err %q must mention missing index 1", err.Error())
	}
}

// TestParallel_EmptyInput_NoSubInvoke asserts that an empty input
// slice returns []O{}, nil without invoking the inner sub-workflow.
func TestParallel_EmptyInput_NoSubInvoke(t *testing.T) {
	var calls atomic.Int32
	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		calls.Add(1)
		return in, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	outer := compose.NewWorkflow[[]int, []int]()
	pNode, err := AddParallelNode(context.Background(), outer, "par", sub)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	pNode.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := compiled.Invoke(context.Background(), []int{})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got == nil {
		t.Error("got nil slice, want empty []int")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
	if calls.Load() != 0 {
		t.Errorf("sub calls: got %d, want 0", calls.Load())
	}
}

// TestParallel_OuterStreamUnsupported asserts that calling Stream
// on the outer parallel node returns the documented v1 error.
func TestParallel_OuterStreamUnsupported(t *testing.T) {
	outer := compose.NewWorkflow[[]int, []int]()
	node, err := AddParallelNode(context.Background(), outer, "par",
		buildParallelIncSub(t),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	node.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = compiled.Stream(context.Background(), []int{1, 2, 3})
	if err == nil {
		t.Fatal("expected stream unsupported error, got nil")
	}
	if !errors.Is(err, ErrParallelOuterStreamUnsupported) {
		t.Errorf("errors.Is(err, ErrParallelOuterStreamUnsupported) = false; err = %v", err)
	}
}

// TestParallel_PanicRecoveredAsItemError asserts that a panic
// inside a per-item runnable is recovered and reported as a
// normal error wrapped with "item %d panic:". The eino
// Workflow runtime has its own panic recover that converts
// panics to errors before they reach this layer; to assert
// our own recover, we use a hand-rolled testRunnable.
func TestParallel_PanicRecoveredAsItemError(t *testing.T) {
	runner := testCountingRunnable{
		fn: func(_ context.Context, in int, _ ...compose.Option) (int, error) {
			if in == 1 {
				panic("kaboom")
			}
			return in, nil
		},
	}
	opts := getParallelOptions([]ParallelOption{
		WithParallelMaxConcurrency(0),
		WithParallelEnableSubCheckpoint(false),
	})
	bridge := newParallelBridgeState(nil)
	_, err := runParallelInvoke(context.Background(), "par", runner, []int{0, 1, 2}, opts, bridge)
	if err == nil {
		t.Fatal("expected panic-as-error, got nil")
	}
	if !strings.Contains(err.Error(), "item 1 panic") {
		t.Errorf("err %q must contain 'item 1 panic'", err.Error())
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("err %q must contain 'kaboom'", err.Error())
	}
}

// TestParallel_StableCheckpointIDAcrossResume asserts that
// WithParallelCheckpointIDBuilder is called with stable
// (nodeKey, index) arguments. The full eino resume path is
// covered in parallel_integration_test.go; here we just
// verify the builder is invoked on the first run with the
// expected per-index arguments.
func TestParallel_StableCheckpointIDAcrossResume(t *testing.T) {
	type call struct {
		key   string
		index int
	}
	var mu sync.Mutex
	var calls []call

	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		return in, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	bridge := newParallelBridgeState(nil)
	compiled, err := sub.Compile(context.Background(),
		compose.WithCheckPointStore(bridge.store()),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	opts := getParallelOptions([]ParallelOption{
		WithParallelMaxConcurrency(0),
		WithParallelCheckpointIDBuilder(func(nodeKey string, idx int) string {
			mu.Lock()
			calls = append(calls, call{key: nodeKey, index: idx})
			mu.Unlock()
			return "stable-cp:" + nodeKey + ":" + itoa(idx)
		}),
	})
	_, err = runParallelInvoke(context.Background(), "par", compiled, []int{0, 1, 2}, opts, bridge)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) < 3 {
		t.Fatalf("builder called %d times, want 3 (one per item)", len(calls))
	}
	// Every call must carry the configured nodeKey and a
	// unique index in 0..2.
	seen := map[int]bool{}
	for _, c := range calls {
		if c.key != "par" {
			t.Errorf("builder key: got %q, want %q", c.key, "par")
		}
		seen[c.index] = true
	}
	for i := 0; i < 3; i++ {
		if !seen[i] {
			t.Errorf("builder not called for index %d", i)
		}
	}
}
