//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may not the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

// r3_interrupt_test.go — R3 spike for PROGRESS_LOG_RESUME_PLAN.md.
//
// R3 (P0 gate for §8 step 3) asks whether eino v0.9.12's
// WithInterruptAfterNodes + full-graph ResumeWithData behaves correctly
// under DAG / Loop / Parallel topology. The ingestion canvas CAN contain
// Loop and Parallel nodes (via AddLoopNode / AddParallelNode), so step 3's
// "interrupt-after-every-non-terminal-node then resume" strategy must not
// re-execute already-completed nodes and must re-enter loops/parallels
// cleanly.
//
// These tests build real compose.Workflow graphs with a real
// CheckPointStore and assert the post-resume node-execution counts. They
// double as a permanent regression guard: if a future eino bump breaks
// interrupt-after + resume, these fail instead of silently re-running
// completed ingestion components (the "re-parse a file" hazard).
//
// NOTE: eino only wraps an after-node interrupt into the *interruptError
// shape that ExtractInterruptInfo recognizes when the graph is STATEFUL
// (WithGenLocalState). The ingestion canvas is stateful via CanvasState,
// so the spike mirrors that: every outer workflow is built with
// WithGenLocalState + WithGraphName, exactly as canvas.BuildWorkflow does.
package workflowx

import (
	"context"
	"strconv"
	"testing"

	"github.com/cloudwego/eino/compose"
)

// r3State is a minimal local state so the outer graph is stateful and
// after-node interrupts surface as ExtractInterruptInfo-recognizable
// errors (the production canvas path is also stateful via CanvasState).
// It must be registered with eino's serializer, exactly like CanvasState
// (runtime/state.go), or the checkpoint marshal fails with "unknown type".
type r3State struct{ A int }

func init() {
	compose.RegisterSerializableType[r3State]("workflowx.r3State")
}

// r3NewWorkflow builds a stateful outer workflow mirroring the canvas
// compile path.
func r3NewWorkflow[I, O any]() *compose.Workflow[I, O] {
	return compose.NewWorkflow[I, O](
		compose.WithGenLocalState(func(context.Context) *r3State { return &r3State{} }),
	)
}

// r3FirstInterruptID extracts the single interrupt id from an after-node
// interrupt error. WithInterruptAfterNodes produces exactly one
// InterruptContext whose ID is the paused node's address.
func r3FirstInterruptID(t *testing.T, err error) string {
	t.Helper()
	info, ok := compose.ExtractInterruptInfo(err)
	if !ok {
		if info2, ok2 := compose.IsInterruptRerunError(err); ok2 {
			t.Fatalf("ExtractInterruptInfo=nil but IsInterruptRerunError ok (info=%v); err=%v [%T]", info2, err, err)
		}
		t.Fatalf("ExtractInterruptInfo: %v [%T]", err, err)
	}
	if len(info.InterruptContexts) == 0 {
		t.Fatal("InterruptContexts empty")
	}
	return info.InterruptContexts[0].ID
}

// r3cp is a tiny deterministic checkpoint id builder for loop/parallel
// sub-checkpoints (mirrors the pattern in loop_integration_test.go).
func r3cp(prefix string) func(string, int) string {
	return func(_ string, iter int) string {
		return prefix + ":" + strconv.Itoa(iter)
	}
}

// TestR3_DAG_InterruptAfterResumesWithoutRerun is the baseline DAG
// assertion: with WithInterruptAfterNodes([B]) on the chain
// A->B->C (B non-terminal), the first Invoke pauses after B; the resume
// Invoke must continue to C WITHOUT re-running A or B.
func TestR3_DAG_InterruptAfterResumesWithoutRerun(t *testing.T) {
	store := newInMemoryStore()
	var aCount, bCount, cCount int

	wf := r3NewWorkflow[int, int]()
	a := wf.AddLambdaNode("A", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		aCount++
		return in + 1, nil
	}))
	b := wf.AddLambdaNode("B", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		bCount++
		return in * 2, nil
	}))
	c := wf.AddLambdaNode("C", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		cCount++
		return in + 10, nil
	}))
	a.AddInput(compose.START)
	b.AddInput("A")
	c.AddInput("B")
	wf.End().AddInput("C")

	compiled, err := wf.Compile(context.Background(),
		compose.WithGraphName("root"),
		compose.WithCheckPointStore(store),
		compose.WithInterruptAfterNodes([]string{"B"}),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "r3-dag"
	_, invokeErr := compiled.Invoke(context.Background(), 1, compose.WithCheckPointID(cpID))
	if invokeErr == nil {
		t.Fatal("expected interrupt after B, got nil")
	}

	resumeCtx := compose.ResumeWithData(context.Background(), r3FirstInterruptID(t, invokeErr), nil)
	out, err := compiled.Invoke(resumeCtx, 1, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	// A(1->2), B(2->4), C(4->14)
	if out != 14 {
		t.Fatalf("output: got %d, want 14", out)
	}
	if aCount != 1 || bCount != 1 || cCount != 1 {
		t.Fatalf("rerun on resume: A=%d B=%d C=%d; want all 1 (completed nodes must not re-execute)", aCount, bCount, cCount)
	}
}

// TestR3_DAG_CrashRecovery_RecompiledGraph simulates a process crash
// between the interrupt and the resume: a brand-new compiled runnable
// (process 2) loads the same CheckPointID and resumes. This proves the
// recovery path step 3 relies on (a fresh Pipeline.Run picking up the
// orphaned checkpoint) actually re-enters at C and does not restart A/B.
func TestR3_DAG_CrashRecovery_RecompiledGraph(t *testing.T) {
	store := newInMemoryStore()
	var aCount, bCount, cCount int

	build := func() *compose.Workflow[int, int] {
		wf := r3NewWorkflow[int, int]()
		a := wf.AddLambdaNode("A", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
			aCount++
			return in + 1, nil
		}))
		b := wf.AddLambdaNode("B", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
			bCount++
			return in * 2, nil
		}))
		c := wf.AddLambdaNode("C", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
			cCount++
			return in + 10, nil
		}))
		a.AddInput(compose.START)
		b.AddInput("A")
		c.AddInput("B")
		wf.End().AddInput("C")
		return wf
	}

	// Process 1: compile, run, pause after B.
	compiled1, err := build().Compile(context.Background(),
		compose.WithGraphName("root"),
		compose.WithCheckPointStore(store),
		compose.WithInterruptAfterNodes([]string{"B"}),
	)
	if err != nil {
		t.Fatalf("compile1: %v", err)
	}
	cpID := "r3-dag-crash"
	_, err = compiled1.Invoke(context.Background(), 1, compose.WithCheckPointID(cpID))
	if err == nil {
		t.Fatal("expected interrupt, got nil")
	}
	interruptID := r3FirstInterruptID(t, err)
	// Process 1 "crashes" — compiled1 is discarded.

	// Process 2: fresh compile (same store, same cpID), resume.
	compiled2, err := build().Compile(context.Background(),
		compose.WithGraphName("root"),
		compose.WithCheckPointStore(store),
		compose.WithInterruptAfterNodes([]string{"B"}),
	)
	if err != nil {
		t.Fatalf("compile2: %v", err)
	}
	resumeCtx := compose.ResumeWithData(context.Background(), interruptID, nil)
	out, err := compiled2.Invoke(resumeCtx, 1, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if out != 14 {
		t.Fatalf("output: got %d, want 14", out)
	}
	if aCount != 1 || bCount != 1 || cCount != 1 {
		t.Fatalf("cross-process rerun: A=%d B=%d C=%d; want all 1", aCount, bCount, cCount)
	}
}

// TestR3_LoopInDAG_InterruptAfterPreLoopNode covers the ingestion-relevant
// topology: A -> loop(B->C) -> D, with WithInterruptAfterNodes([A]) (A is
// non-terminal, immediately before the loop). The first run pauses after A;
// resume must NOT re-run A, must enter and complete the loop, then run D.
//
// Loop math: in=1 -> A=2 -> loop iter1: B=20,C=21 (next=21<31) -> iter2:
// B=210,C=211 (next=211>=31 quit, out=211) -> D=311.
func TestR3_LoopInDAG_InterruptAfterPreLoopNode(t *testing.T) {
	store := newInMemoryStore()
	var aCount, bCount, cCount, dCount int

	sub := compose.NewWorkflow[int, int]()
	b := sub.AddLambdaNode("B", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		bCount++
		return in * 10, nil
	}))
	c := sub.AddLambdaNode("C", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		cCount++
		return in + 1, nil
	}))
	b.AddInput(compose.START)
	c.AddInput("B")
	sub.End().AddInput("C")

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 31, nil
	}

	outer := r3NewWorkflow[int, int]()
	aNode := outer.AddLambdaNode("A", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		aCount++
		return in + 1, nil
	}))
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
		WithLoopCheckpointIDBuilder(r3cp("r3-loop-pre")),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	aNode.AddInput(compose.START)
	loopNode.AddInput("A")
	dNode := outer.AddLambdaNode("D", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		dCount++
		return in + 100, nil
	}))
	dNode.AddInput("loop")
	outer.End().AddInput("D")

	compiled, err := outer.Compile(context.Background(),
		compose.WithGraphName("root"),
		compose.WithCheckPointStore(store),
		compose.WithInterruptAfterNodes([]string{"A"}),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "r3-loop-pre"
	_, invokeErr := compiled.Invoke(context.Background(), 1, compose.WithCheckPointID(cpID))
	if invokeErr == nil {
		t.Fatal("expected interrupt after A, got nil")
	}
	resumeCtx := compose.ResumeWithData(context.Background(), r3FirstInterruptID(t, invokeErr), nil)
	out, err := compiled.Invoke(resumeCtx, 1, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	if out != 311 {
		t.Fatalf("output: got %d, want 311", out)
	}
	if aCount != 1 {
		t.Fatalf("A ran %d times; want 1 (must not re-run on resume)", aCount)
	}
	if bCount != 2 || cCount != 2 {
		t.Fatalf("loop ran B=%d C=%d; want 2 each", bCount, cCount)
	}
	if dCount != 1 {
		t.Fatalf("D ran %d times; want 1", dCount)
	}
}

// TestR3_LoopInDAG_InterruptAfterLoopNode is the strictest R3 case: the
// after-node interrupt lands ON the loop node itself. The loop fully
// executes in run 1 (B,C twice), then the graph pauses after the loop.
// Resume must run D and must NOT re-execute the loop (B,C stay at 2).
// This is exactly step 3's risk: an interrupt on a composite node whose
// subtree already has checkpoint state.
func TestR3_LoopInDAG_InterruptAfterLoopNode(t *testing.T) {
	store := newInMemoryStore()
	var aCount, bCount, cCount, dCount int

	sub := compose.NewWorkflow[int, int]()
	b := sub.AddLambdaNode("B", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		bCount++
		return in * 10, nil
	}))
	c := sub.AddLambdaNode("C", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		cCount++
		return in + 1, nil
	}))
	b.AddInput(compose.START)
	c.AddInput("B")
	sub.End().AddInput("C")

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 31, nil
	}

	outer := r3NewWorkflow[int, int]()
	aNode := outer.AddLambdaNode("A", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		aCount++
		return in + 1, nil
	}))
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
		WithLoopCheckpointIDBuilder(r3cp("r3-loop-node")),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	aNode.AddInput(compose.START)
	loopNode.AddInput("A")
	dNode := outer.AddLambdaNode("D", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		dCount++
		return in + 100, nil
	}))
	dNode.AddInput("loop")
	outer.End().AddInput("D")

	compiled, err := outer.Compile(context.Background(),
		compose.WithGraphName("root"),
		compose.WithCheckPointStore(store),
		compose.WithInterruptAfterNodes([]string{"loop"}),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "r3-loop-node"
	_, invokeErr := compiled.Invoke(context.Background(), 1, compose.WithCheckPointID(cpID))
	if invokeErr == nil {
		t.Fatal("expected interrupt after loop, got nil")
	}
	resumeCtx := compose.ResumeWithData(context.Background(), r3FirstInterruptID(t, invokeErr), nil)
	out, err := compiled.Invoke(resumeCtx, 1, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	if out != 311 {
		t.Fatalf("output: got %d, want 311", out)
	}
	if aCount != 1 || bCount != 2 || cCount != 2 {
		t.Fatalf("run1 partial rerun: A=%d B=%d C=%d; want 1/2/2", aCount, bCount, cCount)
	}
	if dCount != 1 {
		t.Fatalf("D ran %d times; want 1", dCount)
	}
}

// TestR3_ParallelInDAG_InterruptAfterParallelNode covers the Parallel branch
// of R3: A -> parallel(B) -> D (outer type []int -> []int), with
// WithInterruptAfterNodes([parallel]). Run 1: A + parallel run, pause after
// parallel. Resume must run D and must NOT re-run A or the parallel fan-out
// (B stays at len(input)).
func TestR3_ParallelInDAG_InterruptAfterParallelNode(t *testing.T) {
	store := newInMemoryStore()
	var aCount, bCount, dCount int

	sub := compose.NewWorkflow[int, int]()
	b := sub.AddLambdaNode("B", compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		bCount++
		return in + 10, nil
	}))
	b.AddInput(compose.START)
	sub.End().AddInput("B")

	outer := r3NewWorkflow[[]int, []int]()
	aNode := outer.AddLambdaNode("A", compose.InvokableLambda(func(_ context.Context, in []int) ([]int, error) {
		aCount++
		out := make([]int, len(in))
		for i, v := range in {
			out[i] = v + 1
		}
		return out, nil
	}))
	parNode, err := AddParallelNode(context.Background(), outer, "parallel", sub)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	aNode.AddInput(compose.START)
	parNode.AddInput("A")
	dNode := outer.AddLambdaNode("D", compose.InvokableLambda(func(_ context.Context, in []int) ([]int, error) {
		dCount++
		sum := 0
		for _, v := range in {
			sum += v
		}
		return []int{sum}, nil
	}))
	dNode.AddInput("parallel")
	outer.End().AddInput("D")

	compiled, err := outer.Compile(context.Background(),
		compose.WithGraphName("root"),
		compose.WithCheckPointStore(store),
		compose.WithInterruptAfterNodes([]string{"parallel"}),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "r3-parallel"
	input := []int{1, 2, 3}
	_, invokeErr := compiled.Invoke(context.Background(), input, compose.WithCheckPointID(cpID))
	if invokeErr == nil {
		t.Fatal("expected interrupt after parallel, got nil")
	}
	resumeCtx := compose.ResumeWithData(context.Background(), r3FirstInterruptID(t, invokeErr), nil)
	out, err := compiled.Invoke(resumeCtx, input, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}

	// A: [2,3,4]; parallel: [12,13,14]; D: sum=39 -> [39]
	if len(out) != 1 || out[0] != 39 {
		t.Fatalf("output: got %v, want [39]", out)
	}
	if aCount != 1 {
		t.Fatalf("A ran %d times; want 1", aCount)
	}
	if bCount != 3 {
		t.Fatalf("parallel fan-out ran B %d times; want 3 (one per item, no re-run on resume)", bCount)
	}
	if dCount != 1 {
		t.Fatalf("D ran %d times; want 1", dCount)
	}
}
