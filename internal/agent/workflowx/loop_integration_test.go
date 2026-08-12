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

// loop_integration_test.go — full eino integration tests for the
// loop extension. These tests use real compose.Runnable +
// compose.Workflow to exercise the loop's invoke and stream
// paths end-to-end.
package workflowx

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// counterSub is a non-interrupting sub-workflow whose every call
// increments a counter. Used for max-iter and per-iteration tests.
func counterSub(t *testing.T, counter *atomic.Int64) *compose.Workflow[int, int] {
	t.Helper()
	wf := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		counter.Add(1)
		return in + 1, nil
	})
	node := wf.AddLambdaNode("inc", lambda)
	node.AddInput(compose.START)
	wf.End().AddInput("inc")
	return wf
}

func readAllInts(t *testing.T, sr *schema.StreamReader[int]) ([]int, error) {
	t.Helper()
	defer sr.Close()
	var out []int
	for {
		v, err := sr.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		out = append(out, v)
	}
}

func drainStreamUntilError(t *testing.T, sr *schema.StreamReader[int]) ([]int, error) {
	t.Helper()
	defer sr.Close()
	var out []int
	for {
		v, err := sr.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		out = append(out, v)
	}
}

// TestIntegration_OuterVsInnerCallback_Counts asserts the P1
// "Outer callbacks versus inner callbacks" requirement: the
// sub-workflow sees one execution per iteration.
func TestIntegration_OuterVsInnerCallback_Counts(t *testing.T) {
	var subCalls atomic.Int64
	subStore := newInMemoryStore()
	sub := counterSub(t, &subCalls)

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
		WithLoopCompileOptions(compose.WithCheckPointStore(subStore)),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := compiled.Invoke(context.Background(), 0); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got := subCalls.Load(); got != 3 {
		t.Errorf("sub invocations: got %d, want 3", got)
	}
}

// TestIntegration_ExplicitMaxIterationsStops_OnInvokePath asserts that a
// sustained non-converging loop returns normally when it reaches an explicit
// cap. This uses a non-interrupting sub-workflow so the loop actually reaches
// the cap.
func TestIntegration_ExplicitMaxIterationsStops_OnInvokePath(t *testing.T) {
	var subCalls atomic.Int64
	subStore := newInMemoryStore()
	sub := counterSub(t, &subCalls)

	shouldQuit := func(_ context.Context, _, _, _ int) (bool, error) {
		return false, nil // never quits
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(3),
		WithLoopCompileOptions(compose.WithCheckPointStore(subStore)),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	out, err := compiled.Invoke(context.Background(), 0)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out != 3 {
		t.Errorf("output: got %d, want 3", out)
	}
	if got := subCalls.Load(); got != 3 {
		t.Errorf("sub invocations: got %d, want 3", got)
	}
}

// TestIntegration_LoopRunsConcurrentlyWithResumeData checks the
// loop completes the do-while contract end-to-end through a real
// eino workflow with a checkpoint store. Unlike the unit tests
// in loop_test.go, this exercises the full compile/invoke path
// and confirms the loop survives eino's runner.

// TestIntegration_EnableSubCheckpoint_HappyPath asserts that
// streamingIncSub builds a sub-workflow whose Stream emits two chunks
// per iteration: {in, in+1}. The second chunk is the value the loop
// machinery uses as `next` (loop.go derives next from the last value
// emitted in the iteration).
func streamingIncSub(t *testing.T) *compose.Workflow[int, int] {
	t.Helper()
	wf := compose.NewWorkflow[int, int]()
	lambda, err := compose.AnyLambda[int, int, struct{}](
		nil,
		func(_ context.Context, in int, _ ...struct{}) (*schema.StreamReader[int], error) {
			return schema.StreamReaderFromArray([]int{in, in + 1}), nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("AnyLambda: %v", err)
	}
	node := wf.AddLambdaNode("stream", lambda)
	node.AddInput(compose.START)
	wf.End().AddInput("stream")
	return wf
}

// TestIntegration_StreamFinalOnly_HappyPath exercises the
// LoopStreamFinalOnly mode end-to-end. The test asserts the documented
// buffer-and-emit-last contract.
//
// Iterations: in=0 -> [0,1] (next=1), in=1 -> [1,2] (next=2), in=2 ->
// [2,3] (next=3, quit). Caller must observe ONLY the final iteration's
// chunks: [2, 3].
func TestIntegration_StreamFinalOnly_HappyPath(t *testing.T) {
	sub := streamingIncSub(t)
	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopStream(LoopStreamFinalOnly),
		WithLoopMaxIterations(10),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	sr, err := compiled.Stream(context.Background(), 0)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	got, err := readAllInts(t, sr)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	want := []int{2, 3}
	if len(got) != len(want) {
		t.Fatalf("chunks: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunks[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

// TestIntegration_StreamEveryIteration_HappyPath exercises the
// LoopStreamEveryIteration mode end-to-end. The test asserts the
// documented forward-every-iteration contract.
//
// Iterations: in=0 -> [0,1], in=1 -> [1,2], in=2 -> [2,3] (quit).
// Caller must observe every iteration's chunks concatenated in order:
// [0, 1, 1, 2, 2, 3].
func TestIntegration_StreamEveryIteration_HappyPath(t *testing.T) {
	sub := streamingIncSub(t)
	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopStream(LoopStreamEveryIteration),
		WithLoopMaxIterations(10),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	sr, err := compiled.Stream(context.Background(), 0)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	got, err := readAllInts(t, sr)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	want := []int{0, 1, 1, 2, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("chunks: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunks[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

// TestIntegration_Stream_EmptyIterationFails covers the empty-stream
// error branch in runLoopStream: a sub-workflow that yields zero
// chunks for an iteration leaves the loop with no value to feed into
// shouldQuit or into the next iteration's input, so the loop must
// fail with the documented "produced empty stream" error. Without
// this test the branch (loop.go: "iteration N produced empty stream")
// is unreachable from the existing test surface.
func TestIntegration_Stream_EmptyIterationFails(t *testing.T) {
	sub := compose.NewWorkflow[int, int]()
	lambda, err := compose.AnyLambda[int, int, struct{}](
		nil,
		func(_ context.Context, _ int, _ ...struct{}) (*schema.StreamReader[int], error) {
			return schema.StreamReaderFromArray([]int{}), nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("AnyLambda: %v", err)
	}
	node := sub.AddLambdaNode("empty", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("empty")

	shouldQuit := func(_ context.Context, _, _, _ int) (bool, error) {
		t.Fatal("shouldQuit must not be called when iteration stream is empty")
		return false, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopStream(LoopStreamFinalOnly),
		WithLoopMaxIterations(3),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	sr, err := compiled.Stream(context.Background(), 0)
	if err == nil {
		_, err = readAllInts(t, sr)
	}
	if err == nil {
		t.Fatal("expected empty-stream error, got nil")
	}
	if msg := err.Error(); !contains(msg, "produced empty stream") {
		t.Fatalf("error %q must mention 'produced empty stream'", msg)
	}
}

// contains is a tiny strings.Contains shim kept in this file to
// avoid pulling the strings import into the test package solely for
// one assertion (loop_test.go already imports it; loop_integration_
// test.go does not).
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
