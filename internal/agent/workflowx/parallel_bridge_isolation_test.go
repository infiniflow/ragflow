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

package workflowx

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/compose"
)

func allRootInterruptIDs(t *testing.T, err error) []string {
	t.Helper()
	info, ok := extractInterruptInfoDeep(err)
	if !ok {
		t.Fatalf("ExtractInterruptInfo: got %v", err)
	}
	var ids []string
	for _, ic := range info.InterruptContexts {
		if ic.IsRootCause {
			ids = append(ids, ic.ID)
		}
	}
	if len(ids) == 0 {
		for _, ic := range info.InterruptContexts {
			ids = append(ids, ic.ID)
		}
	}
	if len(ids) == 0 {
		t.Fatal("InterruptContexts is empty")
	}
	return ids
}

func TestParallel_FreshRunIsolatedFromPriorInterruptedRun(t *testing.T) {
	for _, concurrency := range []int{0, 2} {
		name := "sequential"
		if concurrency > 1 {
			name = "concurrent"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			store := newInMemoryStore()

			var step1Calls sync.Map // int -> *atomic.Int32
			countStep1 := func(in int) int32 {
				v, _ := step1Calls.LoadOrStore(in, &atomic.Int32{})
				return v.(*atomic.Int32).Add(1)
			}
			getStep1Calls := func(in int) int32 {
				v, ok := step1Calls.Load(in)
				if !ok {
					return 0
				}
				return v.(*atomic.Int32).Load()
			}

			var run2ObservedInterrupt atomic.Bool
			var activeRun atomic.Int32

			sub := compose.NewWorkflow[int, int]()
			step1 := sub.AddLambdaNode("step1", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
				countStep1(in)
				return in * 100, nil
			}))
			step2 := sub.AddLambdaNode("step2", compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
				was, hasState, saved := compose.GetInterruptState[int](ctx)
				if activeRun.Load() == 2 && was {
					run2ObservedInterrupt.Store(true)
				}
				if was && hasState {
					return saved + 1, nil
				}
				if in == 100 {
					return 0, compose.StatefulInterrupt(ctx, "pause-item-0", in)
				}
				return in + 1, nil
			}))
			step1.AddInput(compose.START)
			step2.AddInput("step1")
			sub.End().AddInput("step2")

			outer := compose.NewWorkflow[[]int, []int]()
			parNode, err := AddParallelNode(ctx, outer, "par", sub, WithParallelMaxConcurrency(concurrency))
			if err != nil {
				t.Fatalf("AddParallelNode: %v", err)
			}
			parNode.AddInput(compose.START)
			outer.End().AddInput("par")

			compiled, err := outer.Compile(ctx, compose.WithCheckPointStore(store))
			if err != nil {
				t.Fatalf("compile: %v", err)
			}

			// Run 1 interrupts inside item 0 (input 1 -> step1 100 -> interrupt).
			activeRun.Store(1)
			_, err1 := compiled.Invoke(ctx, []int{1, 2}, compose.WithCheckPointID("run-1"))
			if _, ok := compose.ExtractInterruptInfo(err1); !ok {
				t.Fatalf("run 1: expected interrupt, got %v", err1)
			}
			if got := getStep1Calls(1); got != 1 {
				t.Fatalf("run 1 step1(1) calls = %d, want 1", got)
			}

			// Run 2 is an independent fresh run with a distinct outer checkpoint ID.
			activeRun.Store(2)
			out2, err2 := compiled.Invoke(ctx, []int{5, 6}, compose.WithCheckPointID("run-2"))
			if err2 != nil {
				t.Fatalf("run 2: unexpected error: %v", err2)
			}
			if run2ObservedInterrupt.Load() {
				t.Error("run 2 observed stale interrupt state from run 1")
			}
			if got := getStep1Calls(5); got != 1 {
				t.Errorf("run 2 step1(5) calls = %d, want 1 (step1 skipped due to stale sub-checkpoint)", got)
			}
			want2 := []int{501, 601}
			if len(out2) != len(want2) || out2[0] != want2[0] || out2[1] != want2[1] {
				t.Errorf("run 2 output = %v, want %v", out2, want2)
			}

			// Run 1 must still resume cleanly from its own outer checkpoint without re-running step1(1).
			activeRun.Store(3)
			resumeCtx := compose.Resume(ctx, firstRootInterruptID(t, err1))
			out1, errResume := compiled.Invoke(resumeCtx, nil, compose.WithCheckPointID("run-1"))
			if errResume != nil {
				t.Fatalf("run 1 resume: %v", errResume)
			}
			want1 := []int{101, 201}
			if len(out1) != len(want1) || out1[0] != want1[0] || out1[1] != want1[1] {
				t.Errorf("run 1 resumed output = %v, want %v", out1, want1)
			}
			if got := getStep1Calls(1); got != 1 {
				t.Errorf("run 1 step1(1) calls after resume = %d, want 1", got)
			}
		})
	}
}

func TestParallel_ForceNewRunDoesNotReuseSubCheckpoints(t *testing.T) {
	ctx := t.Context()
	store := newInMemoryStore()

	var step1Calls atomic.Int32
	var interrupts atomic.Int32

	sub := compose.NewWorkflow[int, int]()
	step1 := sub.AddLambdaNode("step1", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		step1Calls.Add(1)
		return in * 100, nil
	}))
	step2 := sub.AddLambdaNode("step2", compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		was, hasState, saved := compose.GetInterruptState[int](ctx)
		if was && hasState {
			return saved + 1, nil
		}
		if in == 100 {
			interrupts.Add(1)
			return 0, compose.StatefulInterrupt(ctx, "force-new", in)
		}
		return in + 1, nil
	}))
	step1.AddInput(compose.START)
	step2.AddInput("step1")
	sub.End().AddInput("step2")

	outer := compose.NewWorkflow[[]int, []int]()
	parNode, err := AddParallelNode(ctx, outer, "par", sub)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	parNode.AddInput(compose.START)
	outer.End().AddInput("par")

	compiled, err := outer.Compile(ctx, compose.WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	const cpID = "force-new-cp"
	_, err = compiled.Invoke(ctx, []int{1, 2}, compose.WithCheckPointID(cpID))
	if _, ok := compose.ExtractInterruptInfo(err); !ok {
		t.Fatalf("first run: expected interrupt, got %v", err)
	}

	// Force a brand-new run with the same checkpoint ID and same input: item 0 must interrupt again.
	_, err = compiled.Invoke(ctx, []int{1, 2}, compose.WithCheckPointID(cpID), compose.WithForceNewRun())
	if _, ok := compose.ExtractInterruptInfo(err); !ok {
		t.Fatalf("forced new run (same input): expected interrupt, got %v", err)
	}
	if got := interrupts.Load(); got != 2 {
		t.Errorf("interrupt count = %d, want 2", got)
	}
	if got := step1Calls.Load(); got != 4 {
		t.Errorf("step1 calls = %d, want 4 (2 per fresh run)", got)
	}

	// Force a brand-new run with different inputs: must compute fresh outputs rather than reusing run 1's item 0.
	out, err := compiled.Invoke(ctx, []int{7, 8}, compose.WithCheckPointID(cpID), compose.WithForceNewRun())
	if err != nil {
		t.Fatalf("forced new run (new input): unexpected error: %v", err)
	}
	want := []int{701, 801}
	if len(out) != len(want) || out[0] != want[0] || out[1] != want[1] {
		t.Errorf("forced new run output = %v, want %v", out, want)
	}
}

func TestParallel_ConcurrentFreshRunsIsolateSubCheckpoints(t *testing.T) {
	ctx := t.Context()
	store := newInMemoryStore()

	runAItem0Interrupted := make(chan struct{})
	releaseRunAItem1 := make(chan struct{})
	var runAItem0Once sync.Once

	var bridgesMu sync.Mutex
	bridgesByRun := map[string]*parallelBridgeState{}
	type runTagKey struct{}

	sub := compose.NewWorkflow[int, int]()
	step1 := sub.AddLambdaNode("step1", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		if in == 2 {
			<-releaseRunAItem1
		}
		return in * 100, nil
	}))
	step2 := sub.AddLambdaNode("step2", compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		was, hasState, saved := compose.GetInterruptState[int](ctx)
		if was && hasState {
			return saved + 1, nil
		}
		if in == 100 || in == 600 {
			if in == 100 {
				defer runAItem0Once.Do(func() { close(runAItem0Interrupted) })
			}
			return 0, compose.StatefulInterrupt(ctx, fmt.Sprintf("pause-%d", in), in)
		}
		return in + 1, nil
	}))
	step1.AddInput(compose.START)
	step2.AddInput("step1")
	sub.End().AddInput("step2")

	outer := compose.NewWorkflow[[]int, []int]()
	parNode, err := AddParallelNode(ctx, outer, "par", sub,
		WithParallelMaxConcurrency(2),
		WithParallelContextBuilder(func(itemCtx context.Context, _ any, _ int) (context.Context, error) {
			if tag, ok := itemCtx.Value(runTagKey{}).(string); ok {
				if br, ok := itemCtx.Value(parallelBridgeStoreKey{}).(*parallelBridgeState); ok {
					bridgesMu.Lock()
					bridgesByRun[tag] = br
					bridgesMu.Unlock()
				}
			}
			return itemCtx, nil
		}),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	parNode.AddInput(compose.START)
	outer.End().AddInput("par")

	compiled, err := outer.Compile(ctx, compose.WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	var errA error
	doneA := make(chan struct{})
	go func() {
		defer close(doneA)
		ctxA := context.WithValue(ctx, runTagKey{}, "A")
		_, errA = compiled.Invoke(ctxA, []int{1, 2}, compose.WithCheckPointID("concurrent-A"))
	}()

	// Wait until Run A has interrupted item 0 (persisting workflowx-parallel:par:0) while Run A is still in flight.
	<-runAItem0Interrupted

	// Run B runs concurrently while Run A is still open: item 0 (5) should complete, item 1 (6) should interrupt.
	ctxB := context.WithValue(ctx, runTagKey{}, "B")
	_, errB := compiled.Invoke(ctxB, []int{5, 6}, compose.WithCheckPointID("concurrent-B"))
	close(releaseRunAItem1)
	<-doneA

	if _, ok := compose.ExtractInterruptInfo(errA); !ok {
		t.Fatalf("run A: expected interrupt, got %v", errA)
	}
	if _, ok := compose.ExtractInterruptInfo(errB); !ok {
		t.Fatalf("run B: expected interrupt, got %v", errB)
	}

	bridgesMu.Lock()
	bridgeA := bridgesByRun["A"]
	bridgeB := bridgesByRun["B"]
	bridgesMu.Unlock()
	if bridgeA == nil || bridgeB == nil {
		t.Fatalf("expected non-nil bridge states, got A=%v B=%v", bridgeA, bridgeB)
	}
	if bridgeA == bridgeB {
		t.Error("concurrent fresh runs shared the same parallelBridgeState instance")
	}
	snapA := bridgeA.snapshot()
	snapB := bridgeB.snapshot()
	if _, hasItem1 := snapA["workflowx-parallel:par:1"]; hasItem1 {
		t.Error("run A bridge leaked run B's item 1 checkpoint")
	}
	if _, hasItem0 := snapB["workflowx-parallel:par:0"]; hasItem0 {
		t.Error("run B bridge leaked run A's item 0 checkpoint")
	}

	outA, err := compiled.Invoke(compose.Resume(ctx, firstRootInterruptID(t, errA)), nil, compose.WithCheckPointID("concurrent-A"))
	if err != nil {
		t.Fatalf("resume A: %v", err)
	}
	if len(outA) != 2 || outA[0] != 101 || outA[1] != 201 {
		t.Errorf("run A resumed output = %v, want [101 201]", outA)
	}

	outB, err := compiled.Invoke(compose.Resume(ctx, firstRootInterruptID(t, errB)), nil, compose.WithCheckPointID("concurrent-B"))
	if err != nil {
		t.Fatalf("resume B: %v", err)
	}
	if len(outB) != 2 || outB[0] != 501 || outB[1] != 601 {
		t.Errorf("run B resumed output = %v, want [501 601]", outB)
	}
}

func TestParallel_MultiInterruptResumeDropsCompletedItemCheckpoints(t *testing.T) {
	ctx := t.Context()
	store := newInMemoryStore()

	var gateACalls [2]atomic.Int32
	var gateBCalls [2]atomic.Int32
	var mu sync.Mutex
	bridgesByPhase := map[string]*parallelBridgeState{}
	type phaseKey struct{}

	sub := compose.NewWorkflow[int, int]()
	gateA := sub.AddLambdaNode("gateA", compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		was, hasState, saved := compose.GetInterruptState[int](ctx)
		if was && hasState {
			return saved + 10, nil
		}
		gateACalls[in].Add(1)
		return 0, compose.StatefulInterrupt(ctx, "gateA", in)
	}))
	gateB := sub.AddLambdaNode("gateB", compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		was, hasState, saved := compose.GetInterruptState[int](ctx)
		if was && hasState {
			return saved + 100, nil
		}
		idx := in - 10
		if idx == 1 {
			gateBCalls[idx].Add(1)
			return 0, compose.StatefulInterrupt(ctx, "gateB", in)
		}
		return in + 100, nil
	}))
	gateA.AddInput(compose.START)
	gateB.AddInput("gateA")
	sub.End().AddInput("gateB")

	outer := compose.NewWorkflow[[]int, []int]()
	parNode, err := AddParallelNode(ctx, outer, "par", sub,
		WithParallelContextBuilder(func(itemCtx context.Context, _ any, _ int) (context.Context, error) {
			if phase, ok := itemCtx.Value(phaseKey{}).(string); ok {
				if br, ok := itemCtx.Value(parallelBridgeStoreKey{}).(*parallelBridgeState); ok {
					mu.Lock()
					bridgesByPhase[phase] = br
					mu.Unlock()
				}
			}
			return itemCtx, nil
		}),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	parNode.AddInput(compose.START)
	outer.End().AddInput("par")

	compiled, err := outer.Compile(ctx, compose.WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	const cpID = "multi-int-cp"
	// Run 1: both items 0 and 1 interrupt at gateA.
	_, err1 := compiled.Invoke(context.WithValue(ctx, phaseKey{}, "run1"), []int{0, 1}, compose.WithCheckPointID(cpID))
	if _, ok := compose.ExtractInterruptInfo(err1); !ok {
		t.Fatalf("run 1: expected interrupt, got %v", err1)
	}
	snap1 := bridgesByPhase["run1"].snapshot()
	if _, ok := snap1["workflowx-parallel:par:0"]; !ok {
		t.Fatal("run 1 bridge missing sub-checkpoint for item 0")
	}
	if _, ok := snap1["workflowx-parallel:par:1"]; !ok {
		t.Fatal("run 1 bridge missing sub-checkpoint for item 1")
	}

	// Resume 1: item 0 finishes; item 1 advances past gateA and interrupts at gateB.
	resume1 := compose.Resume(context.WithValue(ctx, phaseKey{}, "resume1"), allRootInterruptIDs(t, err1)...)
	_, err2 := compiled.Invoke(resume1, nil, compose.WithCheckPointID(cpID))
	if _, ok := compose.ExtractInterruptInfo(err2); !ok {
		t.Fatalf("resume 1: expected second interrupt from item 1 gateB, got %v", err2)
	}
	snap2 := bridgesByPhase["resume1"].snapshot()
	if _, stale := snap2["workflowx-parallel:par:0"]; stale {
		t.Error("resume 1 bridge retained stale sub-checkpoint workflowx-parallel:par:0 for completed item 0")
	}
	if _, ok := snap2["workflowx-parallel:par:1"]; !ok {
		t.Error("resume 1 bridge missing sub-checkpoint workflowx-parallel:par:1 for item 1")
	}

	// Resume 2: item 1 completes past gateB without re-running gateA for either item.
	resume2 := compose.Resume(context.WithValue(ctx, phaseKey{}, "resume2"), allRootInterruptIDs(t, err2)...)
	out, err := compiled.Invoke(resume2, nil, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume 2: %v", err)
	}
	if len(out) != 2 || out[0] != 110 || out[1] != 111 {
		t.Errorf("final output = %v, want [110 111]", out)
	}
	if gateACalls[0].Load() != 1 || gateACalls[1].Load() != 1 {
		t.Errorf("gateA initial calls = [%d %d], want [1 1]", gateACalls[0].Load(), gateACalls[1].Load())
	}
	if gateBCalls[1].Load() != 1 {
		t.Errorf("gateB initial calls for item 1 = %d, want 1", gateBCalls[1].Load())
	}
	if snap3 := bridgesByPhase["resume2"].snapshot(); len(snap3) != 0 {
		t.Errorf("resume 2 bridge still holds %d completed sub-checkpoints, want 0", len(snap3))
	}
}
